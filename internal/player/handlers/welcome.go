package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	"github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Welcome, eonet.PacketAction_Request, handleWelcomeRequest)
	player.Register(eonet.PacketFamily_Welcome, eonet.PacketAction_Agree, handleWelcomeAgree)
	player.Register(eonet.PacketFamily_Welcome, eonet.PacketAction_Msg, handleWelcomeMsg)
}

// handleWelcomeRequest handles character selection (choosing which character to play).
func handleWelcomeRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.WelcomeRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize welcome request", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateLoggedIn {
		return nil
	}

	// Load character and verify ownership
	var (
		charID, accountID, level, mapID, x, y, direction int
		hp, maxHP, tp, maxTP, exp                        int
		str, intl, wis, agi, con, cha                    int
		statPoints, skillPoints                          int
		adminLevel, gender, race, hairStyle, hairColor   int
		goldBank, classID                                int
		name, home, guildTag                             string
		eqBoots, eqAccessory, eqGloves, eqBelt           int
		eqArmor, eqNecklace, eqHat, eqShield, eqWeapon   int
		eqRing1, eqRing2, eqArmlet1, eqArmlet2           int
		eqBracer1, eqBracer2                             int
	)
	_ = home

	err := p.DB.QueryRow(ctx,
		`SELECT c.id, c.account_id, c.name, COALESCE(c.home, ''), c.level, c.map, c.x, c.y, c.direction,
		        c.hp, c.hp, c.tp, c.tp, c.experience,
		        c.strength, c.intelligence, c.wisdom, c.agility, c.constitution, c.charisma,
		        c.stat_points, c.skill_points,
		        c.admin_level, c.gender, c.race, c.hair_style, c.hair_color,
		        c.gold_bank, c.class, COALESCE(g.tag, ''),
		        c.boots, c.accessory, c.gloves, c.belt,
		        c.armor, c.necklace, c.hat, c.shield, c.weapon,
		        c.ring, c.ring2, c.armlet, c.armlet2,
		        c.bracer, c.bracer2
		 FROM characters c
		 LEFT JOIN guilds g ON c.guild_id = g.id
		 WHERE c.id = ?`, pkt.CharacterId,
	).Scan(&charID, &accountID, &name, &home, &level, &mapID, &x, &y, &direction,
		&hp, &maxHP, &tp, &maxTP, &exp,
		&str, &intl, &wis, &agi, &con, &cha,
		&statPoints, &skillPoints,
		&adminLevel, &gender, &race, &hairStyle, &hairColor,
		&goldBank, &classID, &guildTag,
		&eqBoots, &eqAccessory, &eqGloves, &eqBelt,
		&eqArmor, &eqNecklace, &eqHat, &eqShield, &eqWeapon,
		&eqRing1, &eqRing2, &eqArmlet1, &eqArmlet2,
		&eqBracer1, &eqBracer2)
	if errors.Is(err, sql.ErrNoRows) {
		slog.Warn("character not found", "character_id", pkt.CharacterId)
		p.Close()
		return nil
	}
	if err != nil {
		slog.Error("error loading character", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if accountID != p.AccountID {
		slog.Warn("attempt to select another account's character", "id", p.ID)
		p.Close()
		return nil
	}

	if mapID == 0 {
		mapID = p.Cfg.NewCharacter.SpawnMap
		x = p.Cfg.NewCharacter.SpawnX
		y = p.Cfg.NewCharacter.SpawnY
	}

	p.CharacterID = &charID
	p.MapID = mapID
	p.CharName = name
	p.CharX = x
	p.CharY = y
	p.CharDirection = direction
	p.CharGender = gender
	p.CharHairStyle = hairStyle
	p.CharHairColor = hairColor
	p.CharSkin = race
	p.CharAdmin = adminLevel
	p.CharLevel = level
	p.CharHP = hp
	p.CharMaxHP = maxHP
	p.CharTP = tp
	p.CharMaxTP = maxTP
	p.CharSP = 0 // Initialize to 0, will be set by CalculateStats
	p.CharExp = exp
	p.Stats = player.CharacterStats{
		Str: str, Intl: intl, Wis: wis, Agi: agi, Con: con, Cha: cha,
	}
	p.StatPoints = statPoints
	p.SkillPoints = skillPoints
	p.GoldBank = goldBank
	p.ClassID = classID
	p.GuildTag = guildTag
	p.Equipment = player.Equipment{
		Boots: eqBoots, Accessory: eqAccessory, Gloves: eqGloves, Belt: eqBelt,
		Armor: eqArmor, Necklace: eqNecklace, Hat: eqHat, Shield: eqShield, Weapon: eqWeapon,
		Ring: [2]int{eqRing1, eqRing2}, Armlet: [2]int{eqArmlet1, eqArmlet2},
		Bracer: [2]int{eqBracer1, eqBracer2},
	}
	p.State = player.StateEnteringGame

	// Load inventory from DB
	if err := loadInventory(ctx, p); err != nil {
		slog.Warn("failed to load inventory", "id", p.ID, "err", err)
	}

	// Load spells from DB
	if err := loadSpells(ctx, p); err != nil {
		slog.Warn("failed to load spells", "id", p.ID, "err", err)
	}

	if err := loadQuestProgress(ctx, p); err != nil {
		slog.Warn("failed to load quest progress", "id", p.ID, "err", err)
	}

	// Calculate derived stats from base + equipment + class
	p.CalculateStats()

	sessionID := p.GenerateSessionID()

	return p.Bus.SendPacket(&server.WelcomeReplyServerPacket{
		WelcomeCode: server.WelcomeCode_SelectCharacter,
		WelcomeCodeData: &server.WelcomeReplyWelcomeCodeDataSelectCharacter{
			SessionId:     sessionID,
			CharacterId:   charID,
			MapId:         mapID,
			MapRid:        pubdata.MapRid(mapID),
			MapFileSize:   pubdata.MapFileSize(mapID),
			EifRid:        pubdata.EifRid(),
			EifLength:     pubdata.EifLength(),
			EnfRid:        pubdata.EnfRid(),
			EnfLength:     pubdata.EnfLength(),
			EsfRid:        pubdata.EsfRid(),
			EsfLength:     pubdata.EsfLength(),
			EcfRid:        pubdata.EcfRid(),
			EcfLength:     pubdata.EcfLength(),
			Name:          name,
			Title:         "",
			GuildName:     "",
			GuildRankName: "",
			ClassId:       p.ClassID,
			GuildTag:      padGuildTag(guildTag),
			Admin:         protocol.AdminLevel(adminLevel),
			Level:         level,
			Experience:    exp,
			Usage:         0,
			Stats: server.CharacterStatsWelcome{
				Hp:          hp,
				MaxHp:       p.CharMaxHP,
				Tp:          tp,
				MaxTp:       p.CharMaxTP,
				MaxSp:       p.CharMaxSP,
				StatPoints:  p.StatPoints,
				SkillPoints: p.SkillPoints,
				Karma:       0,
				Secondary: server.CharacterSecondaryStats{
					MinDamage: p.MinDamage,
					MaxDamage: p.MaxDamage,
					Accuracy:  p.Accuracy,
					Evade:     p.Evade,
					Armor:     p.Armor,
				},
				Base: server.CharacterBaseStatsWelcome{
					Str:  str,
					Intl: intl,
					Wis:  wis,
					Agi:  agi,
					Con:  con,
					Cha:  cha,
				},
			},
			Equipment: server.EquipmentWelcome{
				Boots:     p.Equipment.Boots,
				Gloves:    p.Equipment.Gloves,
				Accessory: p.Equipment.Accessory,
				Armor:     p.Equipment.Armor,
				Belt:      p.Equipment.Belt,
				Necklace:  p.Equipment.Necklace,
				Hat:       p.Equipment.Hat,
				Shield:    p.Equipment.Shield,
				Weapon:    p.Equipment.Weapon,
				Ring:      []int{p.Equipment.Ring[0], p.Equipment.Ring[1]},
				Armlet:    []int{p.Equipment.Armlet[0], p.Equipment.Armlet[1]},
				Bracer:    []int{p.Equipment.Bracer[0], p.Equipment.Bracer[1]},
			},
		},
	})
}

// handleWelcomeAgree handles file requests from the client during character select.
func handleWelcomeAgree(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.WelcomeAgreeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize welcome agree", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateEnteringGame {
		return nil
	}

	switch pkt.FileType {
	case client.File_Emf:
		// Send map file
		mapFile := fmt.Sprintf("data/maps/%05d.emf", p.MapID)
		content, err := os.ReadFile(mapFile)
		if err != nil {
			slog.Error("failed to read map file", "path", mapFile, "err", err)
			return nil
		}
		return p.Bus.SendPacket(&server.InitInitServerPacket{
			ReplyCode: server.InitReply_FileEmf,
			ReplyCodeData: &server.InitInitReplyCodeDataFileEmf{
				MapFile: server.MapFile{Content: content},
			},
		})

	case client.File_Eif:
		return sendPubFile(p, "data/pub/dat001.eif", 1, server.InitReply_FileEif)
	case client.File_Enf:
		return sendPubFile(p, "data/pub/dtn001.enf", 1, server.InitReply_FileEnf)
	case client.File_Esf:
		return sendPubFile(p, "data/pub/dsl001.esf", 1, server.InitReply_FileEsf)
	case client.File_Ecf:
		return sendPubFile(p, "data/pub/dat001.ecf", 1, server.InitReply_FileEcf)
	}

	return nil
}

func loadQuestProgress(ctx context.Context, p *player.Player) error {
	if p.CharacterID == nil {
		return nil
	}
	rows, err := p.DB.Query(ctx, `SELECT quest_id, state, npc_kills, completions FROM character_quest_progress WHERE character_id = ?`, *p.CharacterID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type questPayload struct {
		StateName string         `json:"state_name"`
		NpcKills  map[string]int `json:"npc_kills"`
	}

	p.QuestProgress = player.NewQuestProgress()
	for rows.Next() {
		var questID, stateCode, completions int
		var raw string
		if err := rows.Scan(&questID, &stateCode, &raw, &completions); err != nil {
			continue
		}
		payload := questPayload{StateName: "Begin", NpcKills: map[string]int{}}
		_ = json.Unmarshal([]byte(raw), &payload)
		if stateCode == 1 || completions > 0 || payload.StateName == "done" {
			p.QuestProgress.CompletedQuests[questID] = true
			continue
		}
		if payload.StateName == "" {
			payload.StateName = "Begin"
		}
		npcKills := map[int]int{}
		for k, v := range payload.NpcKills {
			if id, err := strconv.Atoi(k); err == nil {
				npcKills[id] = v
			}
		}
		p.QuestProgress.ActiveQuests[questID] = &player.QuestState{QuestID: questID, StateName: payload.StateName, NpcKills: npcKills}
	}
	return nil
}

func sendPubFile(p *player.Player, path string, fileID int, replyCode server.InitReply) error {
	content, err := os.ReadFile(path)
	if err != nil {
		slog.Error("failed to read pub file", "path", path, "err", err)
		return nil
	}

	pubFile := server.PubFile{FileId: fileID, Content: content}

	var replyData server.InitInitReplyCodeData
	switch replyCode {
	case server.InitReply_FileEif:
		replyData = &server.InitInitReplyCodeDataFileEif{PubFile: pubFile}
	case server.InitReply_FileEnf:
		replyData = &server.InitInitReplyCodeDataFileEnf{PubFile: pubFile}
	case server.InitReply_FileEsf:
		replyData = &server.InitInitReplyCodeDataFileEsf{PubFile: pubFile}
	case server.InitReply_FileEcf:
		replyData = &server.InitInitReplyCodeDataFileEcf{PubFile: pubFile}
	default:
		return nil
	}

	return p.Bus.SendPacket(&server.InitInitServerPacket{
		ReplyCode:     replyCode,
		ReplyCodeData: replyData,
	})
}

// handleWelcomeMsg handles the enter-game confirmation.
func handleWelcomeMsg(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.WelcomeMsgClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize welcome msg", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateEnteringGame {
		return nil
	}

	p.State = player.StateInGame

	// Enter the map via world (must happen before getting NearbyInfo so we're included)
	if p.World != nil && p.MapID > 0 {
		p.World.EnterMap(p.MapID, &gamemap.MapCharacter{
			PlayerID:  p.ID,
			Name:      p.CharName,
			MapID:     p.MapID,
			X:         p.CharX,
			Y:         p.CharY,
			Direction: p.CharDirection,
			Gender:    p.CharGender,
			HairStyle: p.CharHairStyle,
			HairColor: p.CharHairColor,
			Skin:      p.CharSkin,
			Admin:     p.CharAdmin,
			Level:     p.CharLevel,
			HP:        p.CharHP,
			MaxHP:     p.CharMaxHP,
			TP:        p.CharTP,
			MaxTP:     p.CharMaxTP,
			SP:        p.CharSP,
			MaxSP:     p.CharMaxSP,
			Evade:     p.Evade,
			Armor:     p.Armor,
			Equipment: gamemap.EquipmentData{
				Boots:  pubdata.ItemGraphicID(p.Equipment.Boots),
				Armor:  pubdata.ItemGraphicID(p.Equipment.Armor),
				Hat:    pubdata.ItemGraphicID(p.Equipment.Hat),
				Shield: pubdata.ItemGraphicID(p.Equipment.Shield),
				Weapon: pubdata.ItemGraphicID(p.Equipment.Weapon),
			},
			Bus: p.Bus,
		})
		p.World.BindPlayerSession(p.ID, p)
	}

	// Get nearby info (includes ourselves + other players + NPCs + items)
	var nearby server.NearbyInfo
	if p.World != nil {
		if raw := p.World.GetNearbyInfo(p.MapID); raw != nil {
			if ni, ok := raw.(*server.NearbyInfo); ok {
				nearby = *ni
			}
		}
	}

	slog.Info("player entered game", "id", p.ID, "character", p.CharName,
		"map", p.MapID, "x", p.CharX, "y", p.CharY)

	// Build inventory list
	var items []eonet.Item
	for itemID, qty := range p.Inventory {
		if qty > 0 {
			items = append(items, eonet.Item{Id: itemID, Amount: qty})
		}
	}

	// Build spell list
	var spells []eonet.Spell
	for _, sp := range p.Spells {
		spells = append(spells, eonet.Spell{Id: sp.ID, Level: sp.Level})
	}

	return p.Bus.SendPacket(&server.WelcomeReplyServerPacket{
		WelcomeCode: server.WelcomeCode_EnterGame,
		WelcomeCodeData: &server.WelcomeReplyWelcomeCodeDataEnterGame{
			News:   []string{"Welcome to goeoserv!", "", "", "", "", "", "", "", ""},
			Weight: eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
			Items:  items,
			Spells: spells,
			Nearby: nearby,
		},
	})
}

func loadInventory(ctx context.Context, p *player.Player) error {
	if p.CharacterID == nil {
		return nil
	}
	rows, err := p.DB.Query(ctx,
		"SELECT item_id, quantity FROM character_inventory WHERE character_id = ?", *p.CharacterID)
	if err != nil {
		return err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "err", err)
		}
	}()
	for rows.Next() {
		var itemID, qty int
		if err := rows.Scan(&itemID, &qty); err != nil {
			return err
		}
		p.Inventory[itemID] = qty
	}
	return rows.Err()
}

func loadSpells(ctx context.Context, p *player.Player) error {
	if p.CharacterID == nil {
		return nil
	}
	rows, err := p.DB.Query(ctx,
		"SELECT spell_id, level FROM character_spells WHERE character_id = ?", *p.CharacterID)
	if err != nil {
		return err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "err", err)
		}
	}()
	p.Spells = nil
	for rows.Next() {
		var spellID, level int
		if err := rows.Scan(&spellID, &level); err != nil {
			return err
		}
		p.Spells = append(p.Spells, player.SpellState{ID: spellID, Level: level})
	}
	return rows.Err()
}

// padGuildTag ensures the guild tag is exactly 3 characters (required by the protocol).
func padGuildTag(tag string) string {
	for len(tag) < 3 {
		tag += " "
	}
	return tag[:3]
}

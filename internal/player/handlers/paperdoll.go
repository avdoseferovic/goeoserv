package handlers

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	"github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
	eopub "github.com/ethanmoffat/eolib-go/v3/protocol/pub"
)

func init() {
	player.Register(eonet.PacketFamily_Paperdoll, eonet.PacketAction_Request, handlePaperdollRequest)
	player.Register(eonet.PacketFamily_Paperdoll, eonet.PacketAction_Add, handlePaperdollAdd)
	player.Register(eonet.PacketFamily_Paperdoll, eonet.PacketAction_Remove, handlePaperdollRemove)
}

func handlePaperdollRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.PaperdollRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize paperdoll request", "id", p.ID, "err", err)
		return nil
	}

	var (
		name, home, partner, title, guild, guildRank string
		classID, gender, admin                       int
		boots, armor, hat, shield, weapon            int
		bracer, bracer2, gloves, belt, necklace      int
		ring1, ring2, armlet1, armlet2, accessory    int
	)

	// Look up character by name (resolved from player ID via world)
	err := p.DB.QueryRow(ctx,
		`SELECT c.name, COALESCE(c.home, ''), COALESCE(c.partner, ''), COALESCE(c.title, ''),
		        COALESCE(g.name, ''), COALESCE(c.guild_rank_string, ''),
		        c.class, c.gender, c.admin_level,
		        c.boots, c.armor, c.hat, c.shield, c.weapon,
		        c.bracer, c.bracer2, c.gloves, c.belt, c.necklace,
		        c.ring, c.ring2, c.armlet, c.armlet2, c.accessory
		 FROM characters c
		 LEFT JOIN guilds g ON c.guild_id = g.id
		 WHERE c.name = ?`,
		func() string {
			if p.World != nil {
				return p.World.GetPlayerName(pkt.PlayerId)
			}
			return ""
		}(),
	).Scan(&name, &home, &partner, &title, &guild, &guildRank,
		&classID, &gender, &admin,
		&boots, &armor, &hat, &shield, &weapon,
		&bracer, &bracer2, &gloves, &belt, &necklace,
		&ring1, &ring2, &armlet1, &armlet2, &accessory)

	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		slog.Debug("paperdoll query error", "id", p.ID, "target", pkt.PlayerId, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.PaperdollReplyServerPacket{
		Details: server.CharacterDetails{
			Name:      name,
			Home:      home,
			Partner:   partner,
			Title:     title,
			Guild:     guild,
			GuildRank: guildRank,
			PlayerId:  pkt.PlayerId,
			ClassId:   classID,
			Gender:    protocol.Gender(gender),
			Admin:     protocol.AdminLevel(admin),
		},
		Equipment: server.EquipmentPaperdoll{
			Boots:     boots,
			Armor:     armor,
			Hat:       hat,
			Shield:    shield,
			Weapon:    weapon,
			Gloves:    gloves,
			Belt:      belt,
			Necklace:  necklace,
			Accessory: accessory,
			Ring:      []int{ring1, ring2},
			Armlet:    []int{armlet1, armlet2},
			Bracer:    []int{bracer, bracer2},
		},
		Icon: server.CharacterIcon(admin),
	})
}

// handlePaperdollAdd equips an item from inventory.
func handlePaperdollAdd(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.PaperdollAddClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize paperdoll add", "id", p.ID, "err", err)
		return nil
	}

	if p.Inventory[pkt.ItemId] <= 0 {
		return nil
	}

	item := pubdata.GetItem(pkt.ItemId)
	if item == nil || !player.IsEquipable(item.Type) {
		return nil
	}

	// Validate requirements
	if item.LevelRequirement > p.CharLevel ||
		item.StrRequirement > p.Stats.Str ||
		item.IntRequirement > p.Stats.Intl ||
		item.WisRequirement > p.Stats.Wis ||
		item.AgiRequirement > p.Stats.Agi ||
		item.ConRequirement > p.Stats.Con ||
		item.ChaRequirement > p.Stats.Cha {
		return p.Bus.SendPacket(&server.PaperdollPingServerPacket{ClassId: p.ClassID})
	}

	// Class requirement
	if item.ClassRequirement > 0 && item.ClassRequirement != p.ClassID {
		return p.Bus.SendPacket(&server.PaperdollPingServerPacket{ClassId: p.ClassID})
	}

	// Gender requirement for armor (Spec2 = gender: 0=female, 1=male)
	if item.Type == eopub.Item_Armor && item.Spec2 != p.CharGender {
		return nil
	}

	// Equip the item, getting back any previously equipped item
	oldItemID := p.Equipment.Equip(item.Type, pkt.ItemId, pkt.SubLoc)

	p.Inventory[pkt.ItemId]--
	if p.Inventory[pkt.ItemId] <= 0 {
		delete(p.Inventory, pkt.ItemId)
	}
	if oldItemID > 0 {
		p.Inventory[oldItemID]++
	}

	p.CalculateStats()

	eqChange := equipmentChange(p)

	// Update map character equipment
	if p.World != nil {
		p.World.UpdateMapEquipment(p.MapID, p.ID,
			pubdata.ItemGraphicID(p.Equipment.Boots),
			pubdata.ItemGraphicID(p.Equipment.Armor),
			pubdata.ItemGraphicID(p.Equipment.Hat),
			pubdata.ItemGraphicID(p.Equipment.Shield),
			pubdata.ItemGraphicID(p.Equipment.Weapon))
		p.World.BroadcastMap(p.MapID, p.ID, &server.AvatarAgreeServerPacket{
			Change: eqChange,
		})
	}

	return p.Bus.SendPacket(&server.PaperdollAgreeServerPacket{
		Change:          eqChange,
		ItemId:          pkt.ItemId,
		RemainingAmount: p.Inventory[pkt.ItemId],
		SubLoc:          pkt.SubLoc,
		Stats:           equipStats(p),
	})
}

// handlePaperdollRemove unequips an item.
func handlePaperdollRemove(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.PaperdollRemoveClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize paperdoll remove", "id", p.ID, "err", err)
		return nil
	}

	item := pubdata.GetItem(pkt.ItemId)
	if item == nil {
		return nil
	}

	removedID := p.Equipment.Unequip(item.Type, pkt.SubLoc)
	if removedID == 0 {
		return nil
	}

	p.Inventory[removedID]++
	p.CalculateStats()

	eqChange := equipmentChange(p)

	if p.World != nil {
		p.World.UpdateMapEquipment(p.MapID, p.ID,
			pubdata.ItemGraphicID(p.Equipment.Boots),
			pubdata.ItemGraphicID(p.Equipment.Armor),
			pubdata.ItemGraphicID(p.Equipment.Hat),
			pubdata.ItemGraphicID(p.Equipment.Shield),
			pubdata.ItemGraphicID(p.Equipment.Weapon))
		p.World.BroadcastMap(p.MapID, p.ID, &server.AvatarAgreeServerPacket{
			Change: eqChange,
		})
	}

	return p.Bus.SendPacket(&server.PaperdollRemoveServerPacket{
		Change: eqChange,
		ItemId: pkt.ItemId,
		SubLoc: pkt.SubLoc,
		Stats:  equipStats(p),
	})
}

func equipmentChange(p *player.Player) server.AvatarChange {
	return server.AvatarChange{
		PlayerId:   p.ID,
		ChangeType: server.AvatarChange_Equipment,
		Sound:      true,
		ChangeTypeData: &server.ChangeTypeDataEquipment{
			Equipment: server.EquipmentChange{
				Boots:  pubdata.ItemGraphicID(p.Equipment.Boots),
				Armor:  pubdata.ItemGraphicID(p.Equipment.Armor),
				Hat:    pubdata.ItemGraphicID(p.Equipment.Hat),
				Weapon: pubdata.ItemGraphicID(p.Equipment.Weapon),
				Shield: pubdata.ItemGraphicID(p.Equipment.Shield),
			},
		},
	}
}

func equipStats(p *player.Player) server.CharacterStatsEquipmentChange {
	return server.CharacterStatsEquipmentChange{
		MaxHp: p.CharMaxHP,
		MaxTp: p.CharMaxTP,
		BaseStats: server.CharacterBaseStats{
			Str:  p.Stats.Str,
			Intl: p.Stats.Intl,
			Wis:  p.Stats.Wis,
			Agi:  p.Stats.Agi,
			Con:  p.Stats.Con,
			Cha:  p.Stats.Cha,
		},
		SecondaryStats: server.CharacterSecondaryStats{
			MinDamage: p.MinDamage,
			MaxDamage: p.MaxDamage,
			Accuracy:  p.Accuracy,
			Evade:     p.Evade,
			Armor:     p.Armor,
		},
	}
}

package player

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand/v2"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/db"
	"github.com/avdo/goeoserv/internal/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

// ClientState tracks the player's connection state.
type ClientState int

const (
	StateUninitialized ClientState = iota
	StateInitialized
	StateAccepted
	StateLoggedIn
	StateEnteringGame
	StateInGame
)

// WorldInterface allows the player to interact with the world without import cycles.
type WorldInterface interface {
	EnterMap(mapID int, charInfo any)
	LeaveMap(mapID, playerID int)
	Walk(mapID, playerID int, direction int, coords [2]int)
	Face(mapID, playerID int, direction int)
	BroadcastMap(mapID, excludePlayerID int, pkt any)
	BroadcastGlobal(excludePlayerID int, pkt any)
	BroadcastToAdmins(excludePlayerID int, minAdmin int, pkt any)
	SendToPlayer(playerID int, pkt any)
	FindPlayerByName(name string) (playerID int, found bool)
	GetNearbyInfo(mapID int) any
	DamageNpc(mapID, npcIndex, playerID, damage int) (actualDmg int, killed bool, hpPct int)
	GetNpcAt(mapID, x, y int) int // returns npc index or -1
	DropItem(mapID, itemID, amount, x, y, droppedBy int) int
	PickupItem(mapID, uid int) (itemID, amount int, ok bool)
	GetPlayerBus(playerID int) any
	OnlinePlayerCount() int
	IsLoggedIn(accountID int) bool
	AddLoggedInAccount(accountID int)
	GetOnlinePlayers() any
	WarpPlayer(playerID, fromMapID, toMapID, toX, toY int) any
	GetPendingWarp(mapID, playerID int) (toMapID, toX, toY int, ok bool)
	SetPendingWarp(mapID, playerID, toMapID, toX, toY int)
	GetPlayerName(playerID int) string
	GetPlayerPosition(playerID int) any
	UpdateMapEquipment(mapID, playerID, boots, armor, hat, shield, weapon int)
	BroadcastToGuild(excludePlayerID int, guildTag string, pkt any)
	BroadcastToParty(playerID int, pkt any)
	GetNpcEnfID(mapID, npcIndex int) int
	GetChestItems(mapID, x, y int) any
	AddChestItem(mapID, x, y, itemID, amount int) any
	TakeChestItem(mapID, x, y, itemID int) (amount int, items any)
}

type Player struct {
	ID    int
	IP    string
	State ClientState
	Bus   *protocol.PacketBus
	Cfg   *config.Config
	DB    *db.Database
	World WorldInterface
	conn  *protocol.Conn

	// Account state
	AccountID     int
	LoginAttempts int
	SessionID     *int

	// Character state
	CharacterID   *int
	MapID         int
	CharName      string
	CharX, CharY  int
	CharDirection int
	CharGender    int
	CharHairStyle int
	CharHairColor int
	CharSkin      int
	CharAdmin     int
	CharLevel     int
	CharHP        int
	CharMaxHP     int
	CharTP        int
	CharMaxTP     int
	CharMaxSP     int
	CharExp       int

	// Inventory: itemID -> amount
	Inventory     map[int]int
	GoldBank      int
	Stats         CharacterStats
	StatPoints    int
	SkillPoints   int
	Spells        []SpellState
	QuestProgress *QuestProgressTracker
	Equipment     Equipment
	ClassID       int
	GuildTag      string

	// Derived stats (recalculated from base + equipment + class)
	Weight    int
	MaxWeight int
	MinDamage int
	MaxDamage int
	Accuracy  int
	Evade     int
	Armor     int

	// Pending warp destination (set when warp request is sent, consumed on warp accept)
	PendingWarp *PendingWarp

	// Trade state
	TradePartnerID int         // 0 = not trading
	TradeItems     map[int]int // itemID -> amount offered
	TradeAgreed    bool
}

// PendingWarp tracks a warp destination the player is being sent to.
type PendingWarp struct {
	MapID int
	X, Y  int
}

func New(id int, conn *protocol.Conn, ip string, cfg *config.Config, database *db.Database) *Player {
	return &Player{
		ID:            id,
		IP:            ip,
		State:         StateUninitialized,
		Bus:           protocol.NewPacketBus(conn),
		Cfg:           cfg,
		DB:            database,
		conn:          conn,
		Inventory:     make(map[int]int),
		QuestProgress: NewQuestProgress(),
	}
}

// Run is the main player loop, reading and dispatching packets.
func (p *Player) Run(ctx context.Context) {
	// Create a connection-scoped context that cancels when the player disconnects.
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() {
		if err := p.conn.Close(); err != nil {
			slog.Debug("failed to close connection", "id", p.ID, "err", err)
		}
	}()
	for {
		select {
		case <-connCtx.Done():
			return
		default:
		}

		action, family, reader, err := p.Bus.Recv()
		if err != nil {
			slog.Debug("player disconnected", "id", p.ID, "err", err)
			return
		}

		// After init, all packets have a sequence byte before the payload
		if p.State != StateUninitialized {
			if family != eonet.PacketFamily_Init {
				// Apply pending sequence reset on pong (before validation)
				if family == eonet.PacketFamily_Connection && action == eonet.PacketAction_Ping {
					if p.Bus.HasPendingSequence {
						p.Bus.Sequencer.SetStart(p.Bus.PendingSequenceStart)
						p.Bus.HasPendingSequence = false
					}
				}

				clientSeq := reader.GetChar()
				expectedSeq := p.Bus.Sequencer.NextSequence()

				if p.Cfg.Server.EnforceSequence && clientSeq != expectedSeq {
					slog.Warn("invalid sequence",
						"id", p.ID, "got", clientSeq, "expected", expectedSeq)
					return
				}
			} else {
				// Init packets still consume a sequence slot
				p.Bus.Sequencer.NextSequence()
			}
		}

		slog.Debug("packet received", "id", p.ID, "family", int(family), "action", int(action), "state", p.State)

		if err := p.handlePacket(connCtx, action, family, reader); err != nil {
			slog.Error("error handling packet",
				"id", p.ID,
				"family", family,
				"action", action,
				"err", err,
			)
		}
	}
}

func (p *Player) Close() {
	_ = p.conn.Close()
}

// Die handles player death — warp to spawn/rescue point.
func (p *Player) Die() {
	if p.World == nil {
		return
	}

	p.CharHP = p.CharMaxHP // revive at full HP

	// Determine spawn location
	spawnMap := p.Cfg.Rescue.Map
	spawnX := p.Cfg.Rescue.X
	spawnY := p.Cfg.Rescue.Y

	if spawnMap <= 0 {
		spawnMap = p.Cfg.NewCharacter.SpawnMap
		spawnX = p.Cfg.NewCharacter.SpawnX
		spawnY = p.Cfg.NewCharacter.SpawnY
	}

	// Warp to spawn
	p.World.WarpPlayer(p.ID, p.MapID, spawnMap, spawnX, spawnY)
	p.MapID = spawnMap
	p.CharX = spawnX
	p.CharY = spawnY
}

// SaveCharacter persists the current character state to the database.
// It saves position, stats, and inventory in a single transaction.
// Uses context.Background() since this may be called during disconnect/save loops.
func (p *Player) SaveCharacter() error {
	if p.CharacterID == nil {
		return nil
	}
	charID := *p.CharacterID
	ctx := context.Background() // intentional: saves run outside request scope

	tx, err := p.DB.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			slog.Warn("failed to rollback transaction", "err", err)
		}
	}()
	_, err = tx.ExecContext(ctx,
		`UPDATE characters SET
			map = ?, x = ?, y = ?, direction = ?,
			level = ?, experience = ?, hp = ?, tp = ?,
			strength = ?, intelligence = ?, wisdom = ?,
			agility = ?, constitution = ?, charisma = ?,
			stat_points = ?, skill_points = ?,
			admin_level = ?, gold_bank = ?,
			boots = ?, accessory = ?, gloves = ?, belt = ?,
			armor = ?, necklace = ?, hat = ?, shield = ?, weapon = ?,
			ring = ?, ring2 = ?, armlet = ?, armlet2 = ?,
			bracer = ?, bracer2 = ?
		WHERE id = ?`,
		p.MapID, p.CharX, p.CharY, p.CharDirection,
		p.CharLevel, p.CharExp, p.CharHP, p.CharTP,
		p.Stats.Str, p.Stats.Intl, p.Stats.Wis,
		p.Stats.Agi, p.Stats.Con, p.Stats.Cha,
		p.StatPoints, p.SkillPoints,
		p.CharAdmin, p.GoldBank,
		p.Equipment.Boots, p.Equipment.Accessory, p.Equipment.Gloves, p.Equipment.Belt,
		p.Equipment.Armor, p.Equipment.Necklace, p.Equipment.Hat, p.Equipment.Shield, p.Equipment.Weapon,
		p.Equipment.Ring[0], p.Equipment.Ring[1], p.Equipment.Armlet[0], p.Equipment.Armlet[1],
		p.Equipment.Bracer[0], p.Equipment.Bracer[1],
		charID,
	)
	if err != nil {
		return fmt.Errorf("update character: %w", err)
	}

	// Save inventory: delete all and re-insert
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_inventory WHERE character_id = ?", charID); err != nil {
		return fmt.Errorf("clear inventory: %w", err)
	}
	for itemID, qty := range p.Inventory {
		if qty <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO character_inventory (character_id, item_id, quantity) VALUES (?, ?, ?)",
			charID, itemID, qty,
		); err != nil {
			return fmt.Errorf("insert inventory item %d: %w", itemID, err)
		}
	}

	// Save spells: delete all and re-insert
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_spells WHERE character_id = ?", charID); err != nil {
		return fmt.Errorf("clear spells: %w", err)
	}
	for _, sp := range p.Spells {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO character_spells (character_id, spell_id, level) VALUES (?, ?, ?)",
			charID, sp.ID, sp.Level,
		); err != nil {
			return fmt.Errorf("insert spell %d: %w", sp.ID, err)
		}
	}

	return tx.Commit()
}

func (p *Player) handlePacket(ctx context.Context, action eonet.PacketAction, family eonet.PacketFamily, reader *protocol.EoReader) error {
	handler := GetHandler(family, action)
	if handler == nil {
		slog.Warn("unhandled packet", "family", family, "action", action)
		return nil
	}
	return handler(ctx, p, reader)
}

// GenerateSessionID creates and stores a random session ID.
// Must fit in an EO short (max 64008).
func (p *Player) GenerateSessionID() int {
	id := rand.IntN(64000) + 1
	p.SessionID = &id
	return id
}

// TakeSessionID returns and clears the stored session ID.
func (p *Player) TakeSessionID() (int, bool) {
	if p.SessionID == nil {
		return 0, false
	}
	id := *p.SessionID
	p.SessionID = nil
	return id, true
}

// CharacterStats holds the base stats for a character.
type CharacterStats struct {
	Str, Intl, Wis, Agi, Con, Cha int
	MaxHP, MaxTP                  int
}

// SpellState tracks a learned spell and its level.
type SpellState struct {
	ID    int
	Level int
}

// QuestProgressTracker tracks all quest progress for a player.
type QuestProgressTracker struct {
	ActiveQuests    map[int]*QuestState // questID -> state
	CompletedQuests map[int]bool
}

// QuestState tracks progress for a single quest.
type QuestState struct {
	QuestID   int
	StateName string
}

func NewQuestProgress() *QuestProgressTracker {
	return &QuestProgressTracker{
		ActiveQuests:    make(map[int]*QuestState),
		CompletedQuests: make(map[int]bool),
	}
}

func (p *QuestProgressTracker) GetQuestState(questID int) string {
	if qs, ok := p.ActiveQuests[questID]; ok {
		return qs.StateName
	}
	return "Begin"
}

func (p *QuestProgressTracker) SetQuestState(questID int, stateName string) {
	if _, ok := p.ActiveQuests[questID]; !ok {
		p.ActiveQuests[questID] = &QuestState{QuestID: questID, StateName: stateName}
	} else {
		p.ActiveQuests[questID].StateName = stateName
	}
}

func (p *QuestProgressTracker) CompleteQuest(questID int) {
	delete(p.ActiveQuests, questID)
	p.CompletedQuests[questID] = true
}

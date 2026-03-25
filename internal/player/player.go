package player

import (
	"context"
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
	SendToPlayer(playerID int, pkt any)
	FindPlayerByName(name string) (playerID int, found bool)
	GetNearbyInfo(mapID int) any
	DamageNpc(mapID, npcIndex, playerID, damage int) (actualDmg int, killed bool, hpPct int)
	GetNpcAt(mapID, x, y int) int // returns npc index or -1
	DropItem(mapID, itemID, amount, x, y, droppedBy int) int
	PickupItem(mapID, uid int) (itemID, amount int, ok bool)
	GetPlayerBus(playerID int) any
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
	CharExp       int

	// Inventory: itemID -> amount
	Inventory     map[int]int
	GoldBank      int
	Stats         CharacterStats
	StatPoints    int
	SkillPoints   int
	Spells        []SpellState
	QuestProgress *QuestProgressTracker
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
	defer p.conn.Close() //nolint:errcheck

	for {
		select {
		case <-ctx.Done():
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

		if err := p.handlePacket(action, family, reader); err != nil {
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

// SaveCharacter persists the current character state to the database.
// It saves position, stats, and inventory in a single transaction.
func (p *Player) SaveCharacter() error {
	if p.CharacterID == nil {
		return nil
	}
	charID := *p.CharacterID
	ctx := context.Background()

	tx, err := p.DB.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(ctx,
		`UPDATE characters SET
			map = ?, x = ?, y = ?, direction = ?,
			level = ?, experience = ?, hp = ?, tp = ?,
			strength = ?, intelligence = ?, wisdom = ?,
			agility = ?, constitution = ?, charisma = ?,
			stat_points = ?, skill_points = ?,
			admin_level = ?, gold_bank = ?
		WHERE id = ?`,
		p.MapID, p.CharX, p.CharY, p.CharDirection,
		p.CharLevel, p.CharExp, p.CharHP, p.CharTP,
		p.Stats.Str, p.Stats.Intl, p.Stats.Wis,
		p.Stats.Agi, p.Stats.Con, p.Stats.Cha,
		p.StatPoints, p.SkillPoints,
		p.CharAdmin, p.GoldBank,
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

func (p *Player) handlePacket(action eonet.PacketAction, family eonet.PacketFamily, reader *protocol.EoReader) error {
	handler := GetHandler(family, action)
	if handler == nil {
		slog.Warn("unhandled packet", "family", family, "action", action)
		return nil
	}
	return handler(p, reader)
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

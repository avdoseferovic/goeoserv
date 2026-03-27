package player

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/db"
	"github.com/avdo/goeoserv/internal/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	eonetstructs "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
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
	BindPlayerSession(playerID int, session *Player)
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
	GetNpcHpPercentage(mapID, npcIndex int) int
	GetNpcAt(mapID, x, y int) int // returns npc index or -1
	DropItem(mapID, itemID, amount, x, y, droppedBy int) int
	PickupItem(mapID, uid, playerID int) (itemID, amount int, ok bool)
	GetPlayerBus(playerID int) any
	GetPlayerSession(playerID int) *Player
	GetPlayerAt(mapID, x, y int) int
	IsAttackTileBlocked(mapID, x, y int) bool
	IsPkMap(mapID int) bool
	CanPlayerAttackPlayer(mapID, attackerID, targetID int) bool
	HandlePlayerDefeat(mapID, attackerID, targetID, direction int) bool
	UpdatePlayerVitals(mapID, playerID, hp, tp int)
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
	OpenDoor(mapID, playerID, x, y int) bool
	SetMutedUntil(playerID int, until time.Time)
	ClearMuted(playerID int)
	GetMutedUntil(playerID int) (until time.Time, ok bool)
	IsMuted(playerID int) bool
	StartCaptcha(playerID int, reward int) bool
	RefreshCaptcha(playerID int) bool
	VerifyCaptcha(playerID int, value string) (reward int, solved bool)
	HasCaptcha(playerID int) bool
	GetChestItems(mapID, x, y int) any
	AddChestItem(mapID, x, y, itemID, amount int) any
	TakeChestItem(mapID, x, y, itemID int) (amount int, items any)
	StartEvacuate(mapID, ticks int)
	TryStartJukebox(mapID, trackID int) bool
}

type Player struct {
	// Mu protects all mutable character state (inventory, stats, trade, quest).
	// Held during handler execution and SaveCharacter to prevent concurrent access
	// between the handler goroutine and the save/ping loops.
	Mu sync.Mutex

	ID      int
	IP      string
	State   ClientState
	Bus     *protocol.PacketBus
	Cfg     *config.Config
	DB      *db.Database
	World   WorldInterface
	conn    *protocol.Conn
	Version eonetstructs.Version

	// Account state
	AccountID            int
	LoginAttempts        int
	SessionID            *int
	AccountSessionToken  string
	EmailPin             string
	RecoveryAccountName  string
	RecoveryPinExpiresAt time.Time

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
	BankLevel     int // locker upgrade level
	Stats         CharacterStats
	StatPoints    int
	SkillPoints   int
	Spells        []SpellState
	PendingSpell  *SpellCastState
	LastSpellCast int
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

// maxPacketsPerSecond limits the rate of incoming packets per connection
// to prevent packet flooding DoS attacks.
const maxPacketsPerSecond = 100

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

	// Hangup timer: disconnect clients that don't initialize within HangupDelay
	hangupDelay := p.Cfg.Server.HangupDelay
	if hangupDelay <= 0 {
		hangupDelay = 10 // default 10 seconds
	}
	hangupTimer := time.AfterFunc(time.Duration(hangupDelay)*time.Second, func() {
		if p.State == StateUninitialized {
			slog.Info("hangup timeout (no init)", "id", p.ID)
			_ = p.conn.Close()
		}
	})
	defer hangupTimer.Stop()

	// Packet rate limiter: simple token bucket
	var pktCount int
	pktWindowStart := time.Now()

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

		// Rate limit: disconnect clients that exceed maxPacketsPerSecond
		pktCount++
		if elapsed := time.Since(pktWindowStart); elapsed >= time.Second {
			pktCount = 1
			pktWindowStart = time.Now()
		} else if pktCount > maxPacketsPerSecond {
			slog.Warn("packet rate limit exceeded, disconnecting", "id", p.ID)
			return
		}

		// After init, all packets have a sequence byte before the payload
		if p.State != StateUninitialized {
			if err := consumePacketSequence(p.Bus, family, action, reader, p.Cfg.Server.EnforceSequence); err != nil {
				slog.Warn("invalid sequence", "id", p.ID, "err", err)
				return
			}
		}

		slog.Debug("packet received", "id", p.ID, "family", int(family), "action", int(action), "state", p.State)

		// Lock player state during handler execution to prevent races
		// with the save loop and other concurrent accessors.
		p.Mu.Lock()
		err = p.handlePacket(connCtx, action, family, reader)
		p.Mu.Unlock()
		if err != nil {
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

// Die handles player death by restoring HP and requesting a rescue warp.
func (p *Player) Die() {
	if p.World == nil {
		return
	}

	if p.CharMaxHP > 0 {
		p.CharHP = p.CharMaxHP
	}

	spawnMap := p.Cfg.Rescue.Map
	spawnX := p.Cfg.Rescue.X
	spawnY := p.Cfg.Rescue.Y

	if spawnMap <= 0 {
		spawnMap = p.Cfg.NewCharacter.SpawnMap
		spawnX = p.Cfg.NewCharacter.SpawnX
		spawnY = p.Cfg.NewCharacter.SpawnY
	}

	p.PendingWarp = &PendingWarp{MapID: spawnMap, X: spawnX, Y: spawnY}
	p.World.UpdatePlayerVitals(p.MapID, p.ID, p.CharHP, p.CharTP)
	_ = p.Bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: p.CharHP, Tp: p.CharTP})
	_ = p.Bus.SendPacket(&server.WarpRequestServerPacket{
		WarpType:     server.Warp_Local,
		MapId:        spawnMap,
		WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
	})
}

// SaveCharacter persists the current character state to the database.
// It saves position, stats, and inventory in a single transaction.
// Uses context.Background() since this may be called during disconnect/save loops.
func (p *Player) SaveCharacter() error {
	p.Mu.Lock()
	defer p.Mu.Unlock()

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
			class = ?, race = ?,
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
		p.ClassID, p.CharSkin,
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

	if _, err := tx.ExecContext(ctx, "DELETE FROM character_quest_progress WHERE character_id = ?", charID); err != nil {
		return fmt.Errorf("clear quest progress: %w", err)
	}
	for questID, qs := range p.QuestProgress.ActiveQuests {
		npcKills := map[string]int{}
		for npcID, count := range qs.NpcKills {
			npcKills[fmt.Sprintf("%d", npcID)] = count
		}
		payload, err := json.Marshal(map[string]any{"state_name": qs.StateName, "npc_kills": npcKills})
		if err != nil {
			return fmt.Errorf("marshal active quest %d: %w", questID, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO character_quest_progress (character_id, quest_id, state, npc_kills, player_kills, completions) VALUES (?, ?, ?, ?, ?, ?)",
			charID, questID, 0, string(payload), 0, 0,
		); err != nil {
			return fmt.Errorf("insert active quest %d: %w", questID, err)
		}
	}
	for questID := range p.QuestProgress.CompletedQuests {
		payload, err := json.Marshal(map[string]any{"state_name": "done", "npc_kills": map[string]int{}})
		if err != nil {
			return fmt.Errorf("marshal completed quest %d: %w", questID, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO character_quest_progress (character_id, quest_id, state, npc_kills, player_kills, done_at, completions) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)",
			charID, questID, 1, string(payload), 0, 1,
		); err != nil {
			return fmt.Errorf("insert completed quest %d: %w", questID, err)
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

// GenerateSessionID creates and stores a cryptographically random session ID.
// Must fit in an EO short (max 64008).
func (p *Player) GenerateSessionID() int {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	id := int(binary.LittleEndian.Uint32(buf[:]))%64000 + 1
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

// TakeAndValidateSessionID returns true when the stored session id exists and
// matches the expected value. The session is cleared in both cases to preserve
// current single-use behavior.
func (p *Player) TakeAndValidateSessionID(expected int) bool {
	id, ok := p.TakeSessionID()
	if !ok {
		return false
	}
	return id == expected
}

// ValidateSessionID reports whether the current session id matches the expected
// value without clearing it.
func (p *Player) ValidateSessionID(expected int) bool {
	if p.SessionID == nil {
		return false
	}
	return *p.SessionID == expected
}

// ClearSessionID clears any stored handler session id.
func (p *Player) ClearSessionID() {
	p.SessionID = nil
}

func (p *Player) ClearRecoveryState() {
	p.EmailPin = ""
	p.RecoveryAccountName = ""
	p.RecoveryPinExpiresAt = time.Time{}
}

func (p *Player) StartRecovery(accountName, pin string, now time.Time) {
	p.ClearRecoveryState()
	p.RecoveryAccountName = accountName
	p.EmailPin = pin
	p.RecoveryPinExpiresAt = now.Add(p.Cfg.Account.DelayDuration())
}

func (p *Player) HasActiveRecoveryPIN(now time.Time) bool {
	if p.EmailPin == "" || p.RecoveryAccountName == "" {
		return false
	}

	if p.RecoveryPinExpiresAt.IsZero() || !now.Before(p.RecoveryPinExpiresAt) {
		p.ClearRecoveryState()
		return false
	}

	return true
}

func (p *Player) IsDeep() bool {
	return p.Version.Minor > 0
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

// SpellCastState tracks a requested spell cast until the target packet arrives.
type SpellCastState struct {
	ID        int
	Timestamp int
	StartedAt time.Time
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
	NpcKills  map[int]int
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
		p.ActiveQuests[questID] = &QuestState{QuestID: questID, StateName: stateName, NpcKills: map[int]int{}}
	} else {
		p.ActiveQuests[questID].StateName = stateName
		if p.ActiveQuests[questID].NpcKills == nil {
			p.ActiveQuests[questID].NpcKills = map[int]int{}
		}
	}
}

func (p *QuestProgressTracker) CompleteQuest(questID int) {
	delete(p.ActiveQuests, questID)
	p.CompletedQuests[questID] = true
}

func (p *QuestProgressTracker) RecordNpcKill(npcID int) {
	for _, qs := range p.ActiveQuests {
		if qs.NpcKills == nil {
			qs.NpcKills = map[int]int{}
		}
		qs.NpcKills[npcID]++
	}
}

package world

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/db"
	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/protocol"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	"github.com/ethanmoffat/eolib-go/v3/data"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

// playerEntry tracks which map a player is on and their bus for O(1) lookups.
type playerEntry struct {
	mapID int
	name  string
	bus   *protocol.PacketBus
}

// World manages all game maps, online players, and the game tick loop.
type World struct {
	mapMu sync.RWMutex
	maps  map[int]*gamemap.GameMap

	accountMu      sync.RWMutex
	loggedAccounts map[int]bool // accountID -> logged in

	// Global player index for O(1) lookups by ID or name.
	playerMu    sync.RWMutex
	playerIndex map[int]*playerEntry // playerID -> entry
	nameIndex   map[string]int       // lowercase name -> playerID

	cfg *config.Config
	db  *db.Database
}

func New(cfg *config.Config, database *db.Database) *World {
	return &World{
		maps:           make(map[int]*gamemap.GameMap),
		loggedAccounts: make(map[int]bool),
		playerIndex:    make(map[int]*playerEntry),
		nameIndex:      make(map[string]int),
		cfg:            cfg,
		db:             database,
	}
}

// RegisterPlayer adds a player to the global index. Call when entering a map.
func (w *World) RegisterPlayer(playerID, mapID int, name string, bus *protocol.PacketBus) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	w.playerIndex[playerID] = &playerEntry{mapID: mapID, name: name, bus: bus}
	if name != "" {
		w.nameIndex[strings.ToLower(name)] = playerID
	}
}

// UnregisterPlayer removes a player from the global index.
func (w *World) UnregisterPlayer(playerID int) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	if e, ok := w.playerIndex[playerID]; ok {
		if e.name != "" {
			delete(w.nameIndex, strings.ToLower(e.name))
		}
		delete(w.playerIndex, playerID)
	}
}

// UpdatePlayerMap updates the map ID in the player index (e.g., after warp).
func (w *World) UpdatePlayerMap(playerID, mapID int) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	if e, ok := w.playerIndex[playerID]; ok {
		e.mapID = mapID
	}
}

// getMap returns a map by ID using mapMu.
func (w *World) getMap(mapID int) *gamemap.GameMap {
	w.mapMu.RLock()
	m := w.maps[mapID]
	w.mapMu.RUnlock()
	return m
}

// LoadMaps loads all EMF files from the data/maps directory.
func (w *World) LoadMaps() error {
	mapDir := "data/maps"
	entries, err := os.ReadDir(mapDir)
	if err != nil {
		return fmt.Errorf("reading map directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".emf") {
			continue
		}

		mapPath := filepath.Join(mapDir, entry.Name())
		mapData, err := os.ReadFile(mapPath)
		if err != nil {
			slog.Warn("failed to read map file", "path", mapPath, "err", err)
			continue
		}

		reader := data.NewEoReader(mapData)
		var emf eomap.Emf
		if err := emf.Deserialize(reader); err != nil {
			slog.Warn("failed to deserialize map", "path", mapPath, "err", err)
			continue
		}

		// Extract map ID from filename (e.g., "00001.emf" -> 1)
		baseName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		mapID, err := strconv.Atoi(baseName)
		if err != nil {
			slog.Warn("invalid map filename", "name", entry.Name())
			continue
		}

		gm := gamemap.New(mapID, &emf, w.cfg)
		gm.SpawnNPCs(w.cfg.NPCs.InstantSpawn)
		w.maps[mapID] = gm
		count++
	}

	slog.Info("maps loaded", "count", count)
	return nil
}

// InitNpcStats sets NPC HP/stats from ENF data for all loaded maps.
func (w *World) InitNpcStats() {
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	for _, m := range w.maps {
		m.InitNpcStats(func(npcID int) int {
			rec := pubdata.GetNpc(npcID)
			if rec != nil {
				return rec.Hp
			}
			return 1
		})
	}
}

// GetMap returns the map with the given ID, or nil if not found.
func (w *World) GetMap(mapID int) *gamemap.GameMap {
	return w.getMap(mapID)
}

// IsLoggedIn checks if an account is currently logged in.
func (w *World) IsLoggedIn(accountID int) bool {
	w.accountMu.RLock()
	defer w.accountMu.RUnlock()
	return w.loggedAccounts[accountID]
}

// AddLoggedInAccount marks an account as logged in.
func (w *World) AddLoggedInAccount(accountID int) {
	w.accountMu.Lock()
	defer w.accountMu.Unlock()
	w.loggedAccounts[accountID] = true
}

// RemoveLoggedInAccount marks an account as logged out.
func (w *World) RemoveLoggedInAccount(accountID int) {
	w.accountMu.Lock()
	defer w.accountMu.Unlock()
	delete(w.loggedAccounts, accountID)
}

// EnterMap adds a player's character to a map and broadcasts their appearance.
func (w *World) EnterMap(mapID int, charInfo any) {
	m := w.getMap(mapID)
	if m == nil {
		slog.Warn("map not found for enter", "map_id", mapID)
		return
	}
	if mc, ok := charInfo.(*gamemap.MapCharacter); ok {
		m.Enter(mc)
		// Register in global player index
		w.RegisterPlayer(mc.PlayerID, mapID, mc.Name, mc.Bus)
	}
}

// LeaveMap removes a player's character from a map.
func (w *World) LeaveMap(mapID, playerID int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.Leave(playerID)
	w.UnregisterPlayer(playerID)
}

// Walk handles a player walking on a map.
func (w *World) Walk(mapID, playerID int, direction int, coords [2]int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.Walk(playerID, direction, coords)
}

// Face handles a player changing direction on a map.
func (w *World) Face(mapID, playerID int, direction int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.Face(playerID, direction)
}

// Broadcast sends a packet to all players on a map except the sender.
func (w *World) Broadcast(mapID, excludePlayerID int, pkt eonet.Packet) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.Broadcast(excludePlayerID, pkt)
}

// RunTickLoop starts the world tick loop. Call in a goroutine.
func (w *World) RunTickLoop(ctx context.Context) {
	tickRate := time.Duration(w.cfg.World.TickRate) * time.Millisecond
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick()
		}
	}
}

func (w *World) tick() {
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	for _, m := range w.maps {
		m.Tick()
	}
	// Advance wedding state machines
	delayTicks := w.cfg.Marriage.CeremonyStartDelaySeconds * 8
	if delayTicks <= 0 {
		delayTicks = 160 // default 20 seconds
	}
	TickWeddings(delayTicks)
}

// BroadcastMap sends a packet to all players on a map except excludeID.
func (w *World) BroadcastMap(mapID, excludePlayerID int, pkt any) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	if p, ok := pkt.(eonet.Packet); ok {
		m.Broadcast(excludePlayerID, p)
	}
}

// BroadcastGlobal sends a packet to all players on all maps except excludeID.
func (w *World) BroadcastGlobal(excludePlayerID int, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok {
		return
	}
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	for _, m := range w.maps {
		m.Broadcast(excludePlayerID, p)
	}
}

// SendToPlayer sends a packet to a specific player using O(1) index lookup.
func (w *World) SendToPlayer(playerID int, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok {
		return
	}
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e != nil {
		_ = e.bus.SendPacket(p)
	}
}

// FindPlayerByName finds a player using O(1) name index lookup.
func (w *World) FindPlayerByName(name string) (int, bool) {
	w.playerMu.RLock()
	id, ok := w.nameIndex[strings.ToLower(name)]
	w.playerMu.RUnlock()
	return id, ok
}

// DamageNpc applies damage to an NPC on a map.
func (w *World) DamageNpc(mapID, npcIndex, playerID, damage int) (int, bool, int) {
	m := w.getMap(mapID)
	if m == nil {
		return 0, false, 0
	}
	return m.DamageNpc(npcIndex, playerID, damage)
}

// GetNpcAt returns the NPC index at the given coordinates on a map.
func (w *World) GetNpcAt(mapID, x, y int) int {
	m := w.getMap(mapID)
	if m == nil {
		return -1
	}
	return m.IsNpcAt(x, y)
}

// DropItem drops an item on a map. Returns the ground item UID.
func (w *World) DropItem(mapID, itemID, amount, x, y, droppedBy int) int {
	m := w.getMap(mapID)
	if m == nil {
		return 0
	}
	return m.DropItem(itemID, amount, x, y, droppedBy)
}

// PickupItem picks up a ground item. Returns (itemID, amount, ok).
func (w *World) PickupItem(mapID, uid, playerID int) (int, int, bool) {
	m := w.getMap(mapID)
	if m == nil {
		return 0, 0, false
	}
	item := m.PickupItem(uid, playerID)
	if item == nil {
		return 0, 0, false
	}
	return item.ItemID, item.Amount, true
}

// GetNearbyInfo returns the NearbyInfo for a given map.
func (w *World) GetNearbyInfo(mapID int) any {
	m := w.getMap(mapID)
	if m == nil {
		return nil
	}
	info := m.GetNearbyInfo()
	return &info
}

// SendTo sends a packet to a specific player using O(1) index lookup.
func (w *World) SendTo(playerID int, pkt eonet.Packet) {
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e != nil {
		_ = e.bus.SendPacket(pkt)
	}
}

// GetPlayerBus retrieves a player's PacketBus using O(1) index lookup.
func (w *World) GetPlayerBus(playerID int) any {
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e != nil {
		return e.bus
	}
	return nil
}

// GetPlayerPosition finds a player across all maps and returns their position.
func (w *World) GetPlayerPosition(playerID int) any {
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e == nil {
		return nil
	}
	m := w.getMap(e.mapID)
	if m == nil {
		return nil
	}
	return m.GetPlayerPosition(playerID)
}

// OnlinePlayerCount returns the total number of players across all maps.
func (w *World) OnlinePlayerCount() int {
	w.playerMu.RLock()
	defer w.playerMu.RUnlock()
	return len(w.playerIndex)
}

// GetOnlinePlayers returns info for all online players across all maps.
func (w *World) GetOnlinePlayers() any {
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	var result []gamemap.OnlinePlayerInfo
	for _, m := range w.maps {
		result = append(result, m.GetOnlinePlayers()...)
	}
	return result
}

// BroadcastToAdmins sends a packet to all players with admin level >= minAdmin.
func (w *World) BroadcastToAdmins(excludePlayerID int, minAdmin int, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok {
		return
	}
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	for _, m := range w.maps {
		m.BroadcastToAdmins(excludePlayerID, minAdmin, p)
	}
}

// WarpPlayer moves a player from one map to another. Returns NearbyInfo for the new map.
func (w *World) WarpPlayer(playerID, fromMapID, toMapID, toX, toY int) any {
	w.mapMu.RLock()
	fromMap := w.maps[fromMapID]
	toMap := w.maps[toMapID]
	w.mapMu.RUnlock()

	if fromMap == nil || toMap == nil {
		return nil
	}

	ch := fromMap.RemoveAndReturn(playerID)
	if ch == nil {
		return nil
	}

	ch.X = toX
	ch.Y = toY
	ch.MapID = toMapID
	toMap.Enter(ch)

	// Update player index with new map
	w.UpdatePlayerMap(playerID, toMapID)

	info := toMap.GetNearbyInfo()
	return &info
}

// BroadcastToGuild sends a packet to all online players in a guild (by tag).
func (w *World) BroadcastToGuild(excludePlayerID int, guildTag string, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok || guildTag == "" {
		return
	}
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	for _, m := range w.maps {
		m.BroadcastToGuild(excludePlayerID, guildTag, p)
	}
}

// BroadcastToParty sends a packet to all party members of the player's party.
func (w *World) BroadcastToParty(playerID int, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok {
		return
	}
	party := GetParty(playerID)
	if party != nil {
		party.BroadcastToParty(p)
	}
}

// GetChestItems returns items in a chest at given coords on a map.
func (w *World) GetChestItems(mapID, x, y int) any {
	m := w.getMap(mapID)
	if m == nil {
		return nil
	}
	return m.GetChestItems(x, y)
}

// AddChestItem adds an item to a chest. Returns updated item list.
func (w *World) AddChestItem(mapID, x, y, itemID, amount int) any {
	m := w.getMap(mapID)
	if m == nil {
		return nil
	}
	return m.AddChestItem(x, y, itemID, amount)
}

// TakeChestItem takes an item from a chest.
func (w *World) TakeChestItem(mapID, x, y, itemID int) (int, any) {
	m := w.getMap(mapID)
	if m == nil {
		return 0, nil
	}
	amt, items := m.TakeChestItem(x, y, itemID)
	return amt, items
}

// GetNpcEnfID returns the ENF record ID for an NPC at a given index on a map.
func (w *World) GetNpcEnfID(mapID, npcIndex int) int {
	m := w.getMap(mapID)
	if m == nil {
		return 0
	}
	npc := m.GetNpc(npcIndex)
	if npc == nil {
		return 0
	}
	return npc.ID
}

// GetPlayerName returns a player's character name using O(1) index lookup.
func (w *World) GetPlayerName(playerID int) string {
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e != nil {
		return e.name
	}
	return ""
}

// GetPendingWarp returns the pending warp destination for a player.
func (w *World) GetPendingWarp(mapID, playerID int) (int, int, int, bool) {
	m := w.getMap(mapID)
	if m == nil {
		return 0, 0, 0, false
	}
	warp := m.GetPendingWarp(playerID)
	if warp == nil {
		return 0, 0, 0, false
	}
	return warp.MapID, warp.X, warp.Y, true
}

// SetPendingWarp sets a pending warp on a player's map character.
func (w *World) SetPendingWarp(mapID, playerID, toMapID, toX, toY int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.SetPendingWarp(playerID, &gamemap.WarpDest{MapID: toMapID, X: toX, Y: toY})
}

// UpdateMapEquipment updates the visible equipment on a player's map character.
func (w *World) UpdateMapEquipment(mapID, playerID, boots, armor, hat, shield, weapon int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.UpdateEquipment(playerID, boots, armor, hat, shield, weapon)
}

// StartEvacuate begins a map evacuation countdown.
func (w *World) StartEvacuate(mapID, ticks int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.StartEvacuate(ticks)
}

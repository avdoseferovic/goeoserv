package gamemap

import (
	"log/slog"
	"sync"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/protocol"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// MapCharacter represents a player's character on a map.
type MapCharacter struct {
	PlayerID    int
	Name        string
	MapID       int
	X, Y        int
	Direction   int
	ClassID     int
	GuildTag    string
	Level       int
	Gender      int
	HairStyle   int
	HairColor   int
	Skin        int
	Admin       int
	HP, MaxHP   int
	TP, MaxTP   int
	Equipment   EquipmentData
	Bus         *protocol.PacketBus
	SitState    int // 0 = standing, 1 = chair, 2 = floor
	PendingWarp *WarpDest
}

// WarpDest stores a pending warp destination.
type WarpDest struct {
	MapID int
	X, Y  int
}

// EquipmentData stores visible equipment as graphic IDs (for rendering).
type EquipmentData struct {
	Boots, Armor, Hat, Shield, Weapon int
}

// GameMap represents a single game map with its players, tiles, warps, etc.
type GameMap struct {
	mu          sync.RWMutex
	ID          int
	emf         *eomap.Emf
	cfg         *config.Config
	players     map[int]*MapCharacter
	npcs        []*NpcState
	groundItems []*GroundItem
	chests      map[[2]int]*Chest
	tiles       map[[2]int]eomap.MapTileSpec
	warps       map[[2]int]eomap.MapWarp
	tickCount   int
}

// Chest holds items at a specific map tile.
type Chest struct {
	Items []ChestItem
}

type ChestItem struct {
	ItemID int
	Amount int
}

func New(id int, emf *eomap.Emf, cfg *config.Config) *GameMap {
	m := &GameMap{
		ID:      id,
		emf:     emf,
		cfg:     cfg,
		players: make(map[int]*MapCharacter),
		chests:  make(map[[2]int]*Chest),
		tiles:   make(map[[2]int]eomap.MapTileSpec),
		warps:   make(map[[2]int]eomap.MapWarp),
	}

	for _, row := range emf.TileSpecRows {
		for _, tile := range row.Tiles {
			m.tiles[[2]int{tile.X, row.Y}] = tile.TileSpec
		}
	}

	// Initialize chests at chest tile locations
	for coords, spec := range m.tiles {
		if spec == eomap.MapTileSpec_Chest {
			m.chests[coords] = &Chest{}
		}
	}

	for _, row := range emf.WarpRows {
		for _, tile := range row.Tiles {
			m.warps[[2]int{tile.X, row.Y}] = tile.Warp
		}
	}

	return m
}

func (m *GameMap) Width() int  { return m.emf.Width }
func (m *GameMap) Height() int { return m.emf.Height }

// Enter adds a character to the map and broadcasts appearance.
func (m *GameMap) Enter(ch *MapCharacter) {
	m.mu.Lock()
	m.players[ch.PlayerID] = ch
	m.mu.Unlock()

	m.Broadcast(ch.PlayerID, &server.PlayersAgreeServerPacket{
		Nearby: server.NearbyInfo{
			Characters: []server.CharacterMapInfo{m.buildCharMapInfo(ch)},
		},
	})
}

// Leave removes a character and broadcasts removal.
func (m *GameMap) Leave(playerID int) {
	m.mu.Lock()
	_, exists := m.players[playerID]
	delete(m.players, playerID)
	m.mu.Unlock()

	if exists {
		m.Broadcast(playerID, &server.AvatarRemoveServerPacket{
			PlayerId: playerID,
		})
	}
}

// Walk processes a player walk.
func (m *GameMap) Walk(playerID int, direction int, coords [2]int) {
	m.mu.Lock()
	ch, ok := m.players[playerID]
	if !ok {
		m.mu.Unlock()
		return
	}

	targetX, targetY := coords[0], coords[1]

	if targetX < 0 || targetY < 0 || targetX > m.emf.Width || targetY > m.emf.Height {
		m.mu.Unlock()
		return
	}

	if m.isBlocked(targetX, targetY, playerID) {
		m.mu.Unlock()
		return
	}

	ch.X = targetX
	ch.Y = targetY
	ch.Direction = direction
	m.mu.Unlock()

	m.Broadcast(playerID, &server.WalkPlayerServerPacket{
		PlayerId:  playerID,
		Direction: eoproto.Direction(direction),
		Coords:    eoproto.Coords{X: targetX, Y: targetY},
	})

	// Check for warps
	m.mu.RLock()
	warp, hasWarp := m.warps[[2]int{targetX, targetY}]
	m.mu.RUnlock()

	if hasWarp && warp.DestinationMap > 0 {
		ch.PendingWarp = &WarpDest{
			MapID: warp.DestinationMap,
			X:     warp.DestinationCoords.X,
			Y:     warp.DestinationCoords.Y,
		}
		_ = ch.Bus.SendPacket(&server.WarpRequestServerPacket{
			WarpType:     server.Warp_Local,
			MapId:        warp.DestinationMap,
			WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
		})
	}
}

// Face processes a direction change.
func (m *GameMap) Face(playerID int, direction int) {
	m.mu.Lock()
	ch, ok := m.players[playerID]
	if !ok {
		m.mu.Unlock()
		return
	}
	ch.Direction = direction
	m.mu.Unlock()

	m.Broadcast(playerID, &server.FacePlayerServerPacket{
		PlayerId:  playerID,
		Direction: eoproto.Direction(direction),
	})
}

// Broadcast sends a packet to all players on this map except excludeID.
// Collects player bus refs under lock, then sends outside the lock
// to avoid blocking map operations during network I/O.
func (m *GameMap) Broadcast(excludeID int, pkt eonet.Packet) {
	m.mu.RLock()
	buses := make([]*protocol.PacketBus, 0, len(m.players))
	for pid, ch := range m.players {
		if pid != excludeID {
			buses = append(buses, ch.Bus)
		}
	}
	m.mu.RUnlock()

	for _, bus := range buses {
		if err := bus.SendPacket(pkt); err != nil {
			slog.Debug("broadcast send error", "err", err)
		}
	}
}

// GetPlayerBus returns the PacketBus for a player on this map.
func (m *GameMap) GetPlayerBus(playerID int) *protocol.PacketBus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ch, ok := m.players[playerID]; ok {
		return ch.Bus
	}
	return nil
}

// GetNearbyInfo builds the NearbyInfo for all players on the map.
func (m *GameMap) GetNearbyInfo() server.NearbyInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chars := make([]server.CharacterMapInfo, 0, len(m.players))
	for _, ch := range m.players {
		chars = append(chars, m.buildCharMapInfo(ch))
	}
	return server.NearbyInfo{
		Characters: chars,
		Npcs:       m.getNpcMapInfosLocked(),
		Items:      m.getGroundItemInfosLocked(),
	}
}

// Tick processes one game tick.
func (m *GameMap) Tick() {
	m.mu.Lock()
	m.tickCount++
	tc := m.tickCount
	m.mu.Unlock()

	m.TickNPCs(m.cfg.NPCs.ActRate)

	// HP/TP recovery
	if m.cfg.World.RecoverRate > 0 && tc%m.cfg.World.RecoverRate == 0 {
		m.tickRecovery()
	}

	// Timed spikes
	if m.cfg.World.SpikeRate > 0 && tc%m.cfg.World.SpikeRate == 0 {
		m.tickSpikes()
	}

	// HP/TP drain maps
	if m.cfg.World.DrainRate > 0 && tc%m.cfg.World.DrainRate == 0 {
		m.tickDrain()
	}

	// Door auto-close
	if m.cfg.Map.DoorCloseRate > 0 && tc%m.cfg.Map.DoorCloseRate == 0 {
		m.tickDoorClose()
	}
}

func (m *GameMap) tickRecovery() {
	type recoveryUpdate struct {
		bus    *protocol.PacketBus
		hp, tp int
	}

	m.mu.Lock()
	updates := make([]recoveryUpdate, 0, len(m.players))
	for _, ch := range m.players {
		changed := false
		if ch.HP < ch.MaxHP {
			ch.HP += ch.MaxHP / 20 // recover 5% of max HP
			if ch.HP > ch.MaxHP {
				ch.HP = ch.MaxHP
			}
			changed = true
		}
		if ch.TP < ch.MaxTP {
			ch.TP += ch.MaxTP / 20
			if ch.TP > ch.MaxTP {
				ch.TP = ch.MaxTP
			}
			changed = true
		}
		if changed {
			updates = append(updates, recoveryUpdate{bus: ch.Bus, hp: ch.HP, tp: ch.TP})
		}
	}
	m.mu.Unlock()

	for _, u := range updates {
		_ = u.bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: u.hp, Tp: u.tp})
	}
}

// tickSpikes applies damage to players standing on spike tiles.
func (m *GameMap) tickSpikes() {
	type spikeUpdate struct {
		bus    *protocol.PacketBus
		hp, tp int
	}

	m.mu.Lock()
	if len(m.players) == 0 {
		m.mu.Unlock()
		return
	}

	spikeDmgPct := m.cfg.World.SpikeDamage
	if spikeDmgPct <= 0 {
		spikeDmgPct = 0.1 // default 10%
	}

	var updates []spikeUpdate
	for _, ch := range m.players {
		spec, ok := m.tiles[[2]int{ch.X, ch.Y}]
		if !ok {
			continue
		}
		if spec != eomap.MapTileSpec_TimedSpikes && spec != eomap.MapTileSpec_Spikes {
			continue
		}
		damage := int(float64(ch.MaxHP) * spikeDmgPct)
		if damage < 1 {
			damage = 1
		}
		if damage >= ch.HP {
			damage = ch.HP - 1 // spikes don't kill
		}
		if damage <= 0 {
			continue
		}
		ch.HP -= damage
		updates = append(updates, spikeUpdate{bus: ch.Bus, hp: ch.HP, tp: ch.TP})
	}
	m.mu.Unlock()

	for _, u := range updates {
		_ = u.bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: u.hp, Tp: u.tp})
	}
}

// tickDrain applies HP or TP drain on drain-effect maps.
func (m *GameMap) tickDrain() {
	type drainUpdate struct {
		bus    *protocol.PacketBus
		hp, tp int
	}

	m.mu.Lock()
	if len(m.players) == 0 {
		m.mu.Unlock()
		return
	}

	timedEffect := m.emf.TimedEffect
	var updates []drainUpdate

	if timedEffect == eomap.MapTimedEffect_HpDrain {
		dmgPct := m.cfg.World.DrainHPDamage
		if dmgPct <= 0 {
			m.mu.Unlock()
			return
		}
		for _, ch := range m.players {
			damage := int(float64(ch.MaxHP) * dmgPct)
			if damage < 1 {
				damage = 1
			}
			if damage >= ch.HP {
				damage = ch.HP - 1
			}
			if damage <= 0 {
				continue
			}
			ch.HP -= damage
			updates = append(updates, drainUpdate{bus: ch.Bus, hp: ch.HP, tp: ch.TP})
		}
	}

	if timedEffect == eomap.MapTimedEffect_TpDrain {
		dmgPct := m.cfg.World.DrainTPDamage
		if dmgPct <= 0 {
			m.mu.Unlock()
			return
		}
		for _, ch := range m.players {
			if ch.TP <= 0 {
				continue
			}
			damage := int(float64(ch.MaxTP) * dmgPct)
			if damage < 1 {
				damage = 1
			}
			if damage >= ch.TP {
				damage = ch.TP - 1
			}
			if damage <= 0 {
				continue
			}
			ch.TP -= damage
			updates = append(updates, drainUpdate{bus: ch.Bus, hp: ch.HP, tp: ch.TP})
		}
	}
	m.mu.Unlock()

	for _, u := range updates {
		_ = u.bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: u.hp, Tp: u.tp})
	}
}

// tickDoorClose auto-closes any doors (placeholder — door state tracking not yet implemented).
func (m *GameMap) tickDoorClose() {
	// Door state tracking would require storing which doors are currently open
	// and broadcasting DoorCloseServerPacket when they expire.
	// For now this is a no-op until door state is tracked.
}

// FindPlayerByName finds a player by character name on this map.
func (m *GameMap) FindPlayerByName(name string) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ch := range m.players {
		if ch.Name == name {
			return ch.PlayerID, true
		}
	}
	return 0, false
}

// PlayerCount returns the number of players on this map.
func (m *GameMap) PlayerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.players)
}

func (m *GameMap) getGroundItemInfosLocked() []server.ItemMapInfo {
	infos := make([]server.ItemMapInfo, 0, len(m.groundItems))
	for _, item := range m.groundItems {
		infos = append(infos, server.ItemMapInfo{
			Uid:    item.UID,
			Id:     item.ItemID,
			Coords: eoproto.Coords{X: item.X, Y: item.Y},
			Amount: item.Amount,
		})
	}
	return infos
}

func (m *GameMap) getNpcMapInfosLocked() []server.NpcMapInfo {
	infos := make([]server.NpcMapInfo, 0, len(m.npcs))
	for _, npc := range m.npcs {
		if npc.Alive {
			infos = append(infos, server.NpcMapInfo{
				Index:     npc.Index,
				Id:        npc.ID,
				Coords:    eoproto.Coords{X: npc.X, Y: npc.Y},
				Direction: eoproto.Direction(npc.Direction),
			})
		}
	}
	return infos
}

func (m *GameMap) broadcastNpcPositions() {
	m.mu.RLock()
	if len(m.players) == 0 {
		m.mu.RUnlock()
		return
	}

	positions := make([]server.NpcUpdatePosition, 0, len(m.npcs))
	for _, npc := range m.npcs {
		if npc.Alive {
			positions = append(positions, server.NpcUpdatePosition{
				NpcIndex:  npc.Index,
				Coords:    eoproto.Coords{X: npc.X, Y: npc.Y},
				Direction: eoproto.Direction(npc.Direction),
			})
		}
	}

	if len(positions) == 0 {
		m.mu.RUnlock()
		return
	}

	buses := make([]*protocol.PacketBus, 0, len(m.players))
	for _, ch := range m.players {
		buses = append(buses, ch.Bus)
	}
	m.mu.RUnlock()

	pkt := &server.NpcPlayerServerPacket{Positions: positions}
	for _, bus := range buses {
		_ = bus.SendPacket(pkt)
	}
}

func padGuildTag(tag string) string {
	for len(tag) < 3 {
		tag += " "
	}
	return tag[:3]
}

func (m *GameMap) isBlocked(x, y, excludePlayerID int) bool {
	spec, ok := m.tiles[[2]int{x, y}]
	if ok {
		switch spec {
		case eomap.MapTileSpec_Wall, eomap.MapTileSpec_Edge:
			return true
		}
	}

	for _, ch := range m.players {
		if ch.PlayerID != excludePlayerID && ch.X == x && ch.Y == y {
			return true
		}
	}
	return false
}

// OnlinePlayerInfo holds basic info about an online player.
type OnlinePlayerInfo struct {
	Name     string
	Title    string
	Level    int
	Gender   int
	Admin    int
	ClassID  int
	GuildTag string
	PlayerID int
}

// GetOnlinePlayers returns info for all players on this map.
func (m *GameMap) GetOnlinePlayers() []OnlinePlayerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []OnlinePlayerInfo
	for _, ch := range m.players {
		result = append(result, OnlinePlayerInfo{
			Name:     ch.Name,
			Level:    ch.Level,
			Gender:   ch.Gender,
			Admin:    ch.Admin,
			ClassID:  ch.ClassID,
			GuildTag: ch.GuildTag,
			PlayerID: ch.PlayerID,
		})
	}
	return result
}

// BroadcastToAdmins sends a packet to players with admin level >= minAdmin.
func (m *GameMap) BroadcastToAdmins(excludeID int, minAdmin int, pkt eonet.Packet) {
	m.mu.RLock()
	buses := make([]*protocol.PacketBus, 0, len(m.players))
	for pid, ch := range m.players {
		if pid != excludeID && ch.Admin >= minAdmin {
			buses = append(buses, ch.Bus)
		}
	}
	m.mu.RUnlock()

	for _, bus := range buses {
		_ = bus.SendPacket(pkt)
	}
}

// PlayerPosition holds a player's map and coordinates.
type PlayerPosition struct {
	MapID, X, Y int
}

// GetPlayerPosition returns the position of a player on this map, or nil.
func (m *GameMap) GetPlayerPosition(playerID int) *PlayerPosition {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ch, ok := m.players[playerID]; ok {
		return &PlayerPosition{MapID: ch.MapID, X: ch.X, Y: ch.Y}
	}
	return nil
}

// GetPlayerName returns the character name of a player on this map, or "".
func (m *GameMap) GetPlayerName(playerID int) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ch, ok := m.players[playerID]; ok {
		return ch.Name
	}
	return ""
}

// GetPendingWarp returns and clears the pending warp for a player.
func (m *GameMap) GetPendingWarp(playerID int) *WarpDest {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.players[playerID]
	if !ok || ch.PendingWarp == nil {
		return nil
	}
	warp := ch.PendingWarp
	ch.PendingWarp = nil
	return warp
}

// SetPendingWarp sets a pending warp destination on a player's map character.
func (m *GameMap) SetPendingWarp(playerID int, dest *WarpDest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.players[playerID]; ok {
		ch.PendingWarp = dest
	}
}

// UpdateEquipment updates the visible equipment on a player's map character.
func (m *GameMap) UpdateEquipment(playerID, boots, armor, hat, shield, weapon int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.players[playerID]; ok {
		ch.Equipment = EquipmentData{
			Boots: boots, Armor: armor, Hat: hat, Shield: shield, Weapon: weapon,
		}
	}
}

// GetChestItems returns the items in a chest at the given coordinates.
func (m *GameMap) GetChestItems(x, y int) []ChestItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	chest := m.chests[[2]int{x, y}]
	if chest == nil {
		return nil
	}
	result := make([]ChestItem, len(chest.Items))
	copy(result, chest.Items)
	return result
}

// AddChestItem adds an item to a chest. Returns the updated item list.
func (m *GameMap) AddChestItem(x, y, itemID, amount int) []ChestItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	chest := m.chests[[2]int{x, y}]
	if chest == nil {
		return nil
	}
	// Check if item already in chest
	for i := range chest.Items {
		if chest.Items[i].ItemID == itemID {
			chest.Items[i].Amount += amount
			result := make([]ChestItem, len(chest.Items))
			copy(result, chest.Items)
			return result
		}
	}
	if len(chest.Items) >= m.cfg.Chest.Slots {
		return nil // chest full
	}
	chest.Items = append(chest.Items, ChestItem{ItemID: itemID, Amount: amount})
	result := make([]ChestItem, len(chest.Items))
	copy(result, chest.Items)
	return result
}

// TakeChestItem removes an item from a chest. Returns (amount taken, updated items).
func (m *GameMap) TakeChestItem(x, y, itemID int) (int, []ChestItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	chest := m.chests[[2]int{x, y}]
	if chest == nil {
		return 0, nil
	}
	for i := range chest.Items {
		if chest.Items[i].ItemID == itemID {
			amount := chest.Items[i].Amount
			chest.Items = append(chest.Items[:i], chest.Items[i+1:]...)
			result := make([]ChestItem, len(chest.Items))
			copy(result, chest.Items)
			return amount, result
		}
	}
	return 0, nil
}

// BroadcastToGuild sends a packet to players in the specified guild.
func (m *GameMap) BroadcastToGuild(excludeID int, guildTag string, pkt eonet.Packet) {
	m.mu.RLock()
	buses := make([]*protocol.PacketBus, 0, len(m.players))
	for pid, ch := range m.players {
		if pid != excludeID && ch.GuildTag == guildTag {
			buses = append(buses, ch.Bus)
		}
	}
	m.mu.RUnlock()

	for _, bus := range buses {
		_ = bus.SendPacket(pkt)
	}
}

// RemoveAndReturn removes a player from the map and returns their MapCharacter.
func (m *GameMap) RemoveAndReturn(playerID int) *MapCharacter {
	m.mu.Lock()
	ch, ok := m.players[playerID]
	if ok {
		delete(m.players, playerID)
	}
	m.mu.Unlock()

	if ok {
		m.Broadcast(playerID, &server.AvatarRemoveServerPacket{
			PlayerId: playerID,
		})
		return ch
	}
	return nil
}

func (m *GameMap) buildCharMapInfo(ch *MapCharacter) server.CharacterMapInfo {
	return server.CharacterMapInfo{
		Name:      ch.Name,
		PlayerId:  ch.PlayerID,
		MapId:     m.ID,
		Coords:    server.BigCoords{X: ch.X, Y: ch.Y},
		Direction: eoproto.Direction(ch.Direction),
		ClassId:   ch.ClassID,
		GuildTag:  padGuildTag(ch.GuildTag),
		Level:     ch.Level,
		Gender:    eoproto.Gender(ch.Gender),
		HairStyle: ch.HairStyle,
		HairColor: ch.HairColor,
		Skin:      ch.Skin,
		Hp:        ch.HP,
		MaxHp:     ch.MaxHP,
		Tp:        ch.TP,
		MaxTp:     ch.MaxTP,
		Equipment: server.EquipmentMapInfo{
			Boots:  ch.Equipment.Boots,
			Armor:  ch.Equipment.Armor,
			Hat:    ch.Equipment.Hat,
			Shield: ch.Equipment.Shield,
			Weapon: ch.Equipment.Weapon,
		},
		SitState: server.SitState(ch.SitState),
	}
}

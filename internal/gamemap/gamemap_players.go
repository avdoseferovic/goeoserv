package gamemap

import (
	"github.com/avdo/goeoserv/internal/protocol"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// GetPlayerBus returns the PacketBus for a player on this map.
func (m *GameMap) GetPlayerBus(playerID int) *protocol.PacketBus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ch, ok := m.players[playerID]; ok {
		return ch.Bus
	}
	return nil
}

// GetPlayerAt returns the player ID at the given coordinates, or 0.
func (m *GameMap) GetPlayerAt(x, y int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ch := range m.players {
		if ch.X == x && ch.Y == y {
			return ch.PlayerID
		}
	}
	return 0
}

// UpdatePlayerVitals updates the tracked HP/TP for a player on the map.
func (m *GameMap) UpdatePlayerVitals(playerID, hp, tp int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.players[playerID]; ok {
		ch.HP = hp
		ch.TP = tp
	}
}

// UpdatePlayerCombatStats updates the tracked combat stats for a player on the map.
func (m *GameMap) UpdatePlayerCombatStats(playerID, armor, evade int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.players[playerID]; ok {
		ch.Armor = armor
		ch.Evade = evade
	}
}

// UpdatePlayerCombatSnapshot updates the tracked vitals and combat stats for a player on the map.
func (m *GameMap) UpdatePlayerCombatSnapshot(playerID, hp, maxHP, tp, maxTP, armor, evade int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.players[playerID]; ok {
		ch.HP = hp
		ch.MaxHP = maxHP
		ch.TP = tp
		ch.MaxTP = maxTP
		ch.Armor = armor
		ch.Evade = evade
	}
}

// UpdatePlayerSitState updates the sitting state for a player on the map.
func (m *GameMap) UpdatePlayerSitState(playerID, sitState int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.players[playerID]; ok {
		ch.SitState = sitState
	}
}

// GetPlayerVitalsSnapshot returns the current tracked vitals for players on the map.
func (m *GameMap) GetPlayerVitalsSnapshot() []PlayerVitalsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshots := make([]PlayerVitalsSnapshot, 0, len(m.players))
	for _, ch := range m.players {
		snapshots = append(snapshots, PlayerVitalsSnapshot{
			PlayerID: ch.PlayerID,
			HP:       ch.HP,
			TP:       ch.TP,
		})
	}

	return snapshots
}

// PlayerIDs returns the player IDs currently on the map.
func (m *GameMap) PlayerIDs() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	playerIDs := make([]int, 0, len(m.players))
	for playerID := range m.players {
		playerIDs = append(playerIDs, playerID)
	}
	return playerIDs
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

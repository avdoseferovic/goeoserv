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
	PlayerID  int
	Name      string
	MapID     int
	X, Y      int
	Direction int
	ClassID   int
	GuildTag  string
	Level     int
	Gender    int
	HairStyle int
	HairColor int
	Skin      int
	Admin     int
	HP, MaxHP int
	TP, MaxTP int
	Equipment EquipmentData
	Bus       *protocol.PacketBus
	SitState  int // 0 = standing, 1 = chair, 2 = floor
}

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
	tiles       map[[2]int]eomap.MapTileSpec
	warps       map[[2]int]eomap.MapWarp
	tickCount   int
}

func New(id int, emf *eomap.Emf, cfg *config.Config) *GameMap {
	m := &GameMap{
		ID:      id,
		emf:     emf,
		cfg:     cfg,
		players: make(map[int]*MapCharacter),
		tiles:   make(map[[2]int]eomap.MapTileSpec),
		warps:   make(map[[2]int]eomap.MapWarp),
	}

	for _, row := range emf.TileSpecRows {
		for _, tile := range row.Tiles {
			m.tiles[[2]int{tile.X, row.Y}] = tile.TileSpec
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
func (m *GameMap) Broadcast(excludeID int, pkt eonet.Packet) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for pid, ch := range m.players {
		if pid == excludeID {
			continue
		}
		if err := ch.Bus.SendPacket(pkt); err != nil {
			slog.Debug("broadcast send error", "player_id", pid, "err", err)
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

	var chars []server.CharacterMapInfo
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

	// Broadcast NPC positions periodically
	if tc%m.cfg.NPCs.Speed0 == 0 {
		m.broadcastNpcPositions()
	}

	// HP/TP recovery
	if m.cfg.World.RecoverRate > 0 && tc%m.cfg.World.RecoverRate == 0 {
		m.tickRecovery()
	}
}

func (m *GameMap) tickRecovery() {
	m.mu.RLock()
	defer m.mu.RUnlock()

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
			_ = ch.Bus.SendPacket(&server.RecoverPlayerServerPacket{
				Hp: ch.HP,
				Tp: ch.TP,
			})
		}
	}
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
	var infos []server.ItemMapInfo
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
	var infos []server.NpcMapInfo
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
	defer m.mu.RUnlock()

	if len(m.players) == 0 {
		return
	}

	var positions []server.NpcUpdatePosition
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
		return
	}

	pkt := &server.NpcPlayerServerPacket{Positions: positions}
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(pkt)
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

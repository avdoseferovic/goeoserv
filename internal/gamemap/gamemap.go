package gamemap

import (
	"sync"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/protocol"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// MapCharacter represents a player's character on a map.
type MapCharacter struct {
	PlayerID      int
	Name          string
	MapID         int
	X, Y          int
	Direction     int
	ClassID       int
	GuildTag      string
	Level         int
	Gender        int
	HairStyle     int
	HairColor     int
	Skin          int
	Admin         int
	HP, MaxHP     int
	TP, MaxTP     int
	SP, MaxSP     int
	Evade         int
	Armor         int
	Equipment     EquipmentData
	Bus           *protocol.PacketBus
	SitState      int // 0 = standing, 1 = chair, 2 = floor
	PendingWarp   *WarpDest
	WarpSuckTicks int // countdown to warp suck check
	GhostTicks    int
}

// PlayerVitalsSnapshot captures a player's tracked map vitals.
type PlayerVitalsSnapshot struct {
	PlayerID int
	HP       int
	TP       int
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
	arenaTiles  map[[2]int]struct{}
	warps       map[[2]int]eomap.MapWarp
	openDoors   map[[2]int]int
	hasJukebox  bool
	tickCount   int

	// Quake effect state (for maps with quake timed effect)
	quakeRate     int // randomized interval between quakes (0 = not set)
	quakeStrength int // randomized intensity
	quakeTicks    int // counter toward next quake

	// Evacuate state
	EvacuateTicks int // >0 means evacuation in progress; countdown in ticks

	// Jukebox state
	jukeboxTrackID int
	jukeboxTicks   int
}

func New(id int, emf *eomap.Emf, cfg *config.Config) *GameMap {
	m := &GameMap{
		ID:         id,
		emf:        emf,
		cfg:        cfg,
		players:    make(map[int]*MapCharacter),
		chests:     make(map[[2]int]*Chest),
		tiles:      make(map[[2]int]eomap.MapTileSpec),
		arenaTiles: make(map[[2]int]struct{}),
		warps:      make(map[[2]int]eomap.MapWarp),
		openDoors:  make(map[[2]int]int),
	}

	for _, row := range emf.TileSpecRows {
		for _, tile := range row.Tiles {
			coords := [2]int{tile.X, row.Y}
			m.tiles[coords] = tile.TileSpec
			if tile.TileSpec == eomap.MapTileSpec_Arena {
				m.arenaTiles[coords] = struct{}{}
			}
			if tile.TileSpec == eomap.MapTileSpec_Jukebox {
				m.hasJukebox = true
			}
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
	if m.cfg.World.GhostRate > 0 {
		ch.GhostTicks = m.cfg.World.GhostRate
	}
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

// IsPkMap reports whether this map allows PK combat.
func (m *GameMap) IsPkMap() bool {
	return m.emf != nil && m.emf.Type == eomap.Map_Pk
}

// HasArena reports whether this map contains arena tiles.
func (m *GameMap) HasArena() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.arenaTiles) > 0
}

// IsAttackTileBlocked reports whether a projectile/melee line should stop on this tile.
func (m *GameMap) IsAttackTileBlocked(x, y int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if x < 0 || y < 0 || x > m.emf.Width || y > m.emf.Height {
		return true
	}

	if spec, ok := m.tiles[[2]int{x, y}]; ok {
		switch spec {
		case eomap.MapTileSpec_Wall, eomap.MapTileSpec_Edge:
			return true
		}
	}

	if warp, ok := m.warps[[2]int{x, y}]; ok && warp.Door > 0 {
		_, isOpen := m.openDoors[[2]int{x, y}]
		return !isOpen
	}

	return false
}

package gamemap

import (
	"math/rand/v2"

	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// NpcState represents a live NPC instance on a map.
type NpcState struct {
	Index      int // unique index on this map
	ID         int // ENF record ID
	X, Y       int
	Direction  int
	SpawnX     int
	SpawnY     int
	SpawnType  int
	SpawnTime  int // ticks until respawn
	HP, MaxHP  int
	SpawnTicks int // countdown to respawn when dead
	ActTicks   int // countdown to next action
	Alive      bool
	Opponents  map[int]int // playerID -> damage dealt (O(1) lookup)
}

// SpawnNPCs creates NPC instances from the EMF map data.
func (m *GameMap) SpawnNPCs(instantSpawn bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	index := 0
	for _, npcDef := range m.emf.Npcs {
		for i := 0; i < npcDef.Amount; i++ {
			spawnX, spawnY := npcDef.Coords.X, npcDef.Coords.Y
			if instantSpawn {
				spawnX, spawnY = m.findFreeSpawnTile(spawnX, spawnY)
			}

			npc := &NpcState{
				Index:     index,
				ID:        npcDef.Id,
				X:         spawnX,
				Y:         spawnY,
				SpawnX:    npcDef.Coords.X,
				SpawnY:    npcDef.Coords.Y,
				Direction: rand.IntN(4),
				SpawnType: npcDef.SpawnType,
				SpawnTime: npcDef.SpawnTime,
				Alive:     instantSpawn,
				HP:        0, // Will be set when ENF data is loaded
				MaxHP:     0,
			}

			if !instantSpawn {
				npc.SpawnTicks = npc.SpawnTime * 8 // convert seconds to ticks (125ms each)
			}

			m.npcs = append(m.npcs, npc)
			index++
		}
	}
}

// findFreeSpawnTile finds a walkable unoccupied tile near (x, y).
// Searches in expanding rings up to 5 tiles away. Returns the original coords if nothing is free.
// Must be called while holding m.mu.
func (m *GameMap) findFreeSpawnTile(x, y int) (int, int) {
	if !m.isTileOccupiedLocked(x, y) {
		return x, y
	}
	for radius := 1; radius <= 5; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				if dx == 0 && dy == 0 {
					continue
				}
				nx, ny := x+dx, y+dy
				if nx < 0 || ny < 0 || nx > m.emf.Width || ny > m.emf.Height {
					continue
				}
				if m.isTileWalkableNpcLocked(nx, ny) && !m.isTileOccupiedLocked(nx, ny) {
					return nx, ny
				}
			}
		}
	}
	return x, y
}

// isTileOccupiedLocked is isTileOccupied without locking (caller must hold mu).
func (m *GameMap) isTileOccupiedLocked(x, y int) bool {
	for _, ch := range m.players {
		if ch.X == x && ch.Y == y {
			return true
		}
	}
	for _, other := range m.npcs {
		if other.Alive && other.X == x && other.Y == y {
			return true
		}
	}
	return false
}

// isTileWalkableNpcLocked is isTileWalkableNpc without locking (caller must hold mu).
func (m *GameMap) isTileWalkableNpcLocked(x, y int) bool {
	if _, hasWarp := m.warps[[2]int{x, y}]; hasWarp {
		return false
	}
	if spec, ok := m.tiles[[2]int{x, y}]; ok {
		switch spec {
		case eomap.MapTileSpec_Wall,
			eomap.MapTileSpec_Edge,
			eomap.MapTileSpec_NpcBoundary,
			eomap.MapTileSpec_ChairDown,
			eomap.MapTileSpec_ChairLeft,
			eomap.MapTileSpec_ChairRight,
			eomap.MapTileSpec_ChairUp,
			eomap.MapTileSpec_ChairDownRight,
			eomap.MapTileSpec_ChairUpLeft,
			eomap.MapTileSpec_ChairAll,
			eomap.MapTileSpec_Chest,
			eomap.MapTileSpec_BankVault,
			eomap.MapTileSpec_Board1,
			eomap.MapTileSpec_Board2,
			eomap.MapTileSpec_Board3,
			eomap.MapTileSpec_Board4,
			eomap.MapTileSpec_Board5,
			eomap.MapTileSpec_Board6,
			eomap.MapTileSpec_Board7,
			eomap.MapTileSpec_Board8,
			eomap.MapTileSpec_Jukebox:
			return false
		}
	}
	return true
}

// InitNpcStats sets HP for all NPCs using a lookup function.
func (m *GameMap) InitNpcStats(getHP func(npcID int) int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, npc := range m.npcs {
		hp := getHP(npc.ID)
		if hp <= 0 {
			hp = 1
		}
		npc.MaxHP = hp
		if npc.Alive {
			npc.HP = hp
		}
	}
}

// SetNpcHP sets the HP for NPCs based on ENF data lookup.
func (m *GameMap) SetNpcHP(npcID, hp int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, npc := range m.npcs {
		if npc.ID == npcID {
			npc.MaxHP = hp
			if npc.Alive {
				npc.HP = hp
			}
		}
	}
}

// GetNpc returns the NPC at the given index.
func (m *GameMap) GetNpc(index int) *NpcState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if index >= 0 && index < len(m.npcs) {
		return m.npcs[index]
	}
	return nil
}

// GetNpcMapInfos returns NpcMapInfo for all alive NPCs.
func (m *GameMap) GetNpcMapInfos() []server.NpcMapInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

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

// TickNPCs processes NPC logic for one tick: respawning, movement, actions.
func (m *GameMap) TickNPCs(actRate int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.players) == 0 && m.cfg.NPCs.FreezeOnEmptyMap {
		return
	}

	for _, npc := range m.npcs {
		if !npc.Alive {
			// Respawn countdown
			npc.SpawnTicks--
			if npc.SpawnTicks <= 0 {
				sx, sy := m.findFreeSpawnTile(npc.SpawnX, npc.SpawnY)
				npc.Alive = true
				npc.HP = npc.MaxHP
				npc.X = sx
				npc.Y = sy
				npc.Direction = rand.IntN(4)
				npc.Opponents = nil

				// Broadcast NPC appear
				m.broadcastNpcAppear(npc)
			}
			continue
		}

		npc.ActTicks++

		// Random movement
		if npc.ActTicks >= actRate && len(npc.Opponents) == 0 {
			npc.ActTicks = 0
			if rand.IntN(4) == 0 { // 25% chance to move
				if moved := m.npcRandomWalk(npc); moved {
					m.broadcastNpcWalk(npc)
				}
			}
		}
	}
}

func (m *GameMap) npcRandomWalk(npc *NpcState) bool {
	dir := rand.IntN(4)
	newX, newY := npc.X, npc.Y
	switch dir {
	case 0: // Down
		newY++
	case 1: // Left
		newX--
	case 2: // Up
		newY--
	case 3: // Right
		newX++
	}

	if newX < 0 || newY < 0 || newX > m.emf.Width || newY > m.emf.Height {
		return false
	}

	if !m.isTileWalkableNpc(newX, newY) || m.isTileOccupied(newX, newY) {
		return false
	}

	npc.X = newX
	npc.Y = newY
	npc.Direction = dir
	return true
}

// isTileWalkableNpc checks if an NPC can walk on a tile (walls, chairs, warps, etc. block).
func (m *GameMap) isTileWalkableNpc(x, y int) bool {
	// Use pre-built warps map for O(1) lookup instead of scanning warp rows
	if _, hasWarp := m.warps[[2]int{x, y}]; hasWarp {
		return false
	}

	if spec, ok := m.tiles[[2]int{x, y}]; ok {
		switch spec {
		case eomap.MapTileSpec_Wall,
			eomap.MapTileSpec_Edge,
			eomap.MapTileSpec_NpcBoundary,
			eomap.MapTileSpec_ChairDown,
			eomap.MapTileSpec_ChairLeft,
			eomap.MapTileSpec_ChairRight,
			eomap.MapTileSpec_ChairUp,
			eomap.MapTileSpec_ChairDownRight,
			eomap.MapTileSpec_ChairUpLeft,
			eomap.MapTileSpec_ChairAll,
			eomap.MapTileSpec_Chest,
			eomap.MapTileSpec_BankVault,
			eomap.MapTileSpec_Board1,
			eomap.MapTileSpec_Board2,
			eomap.MapTileSpec_Board3,
			eomap.MapTileSpec_Board4,
			eomap.MapTileSpec_Board5,
			eomap.MapTileSpec_Board6,
			eomap.MapTileSpec_Board7,
			eomap.MapTileSpec_Board8,
			eomap.MapTileSpec_Jukebox:
			return false
		}
	}
	return true
}

// isTileOccupied checks if a tile is occupied by a player or another alive NPC.
func (m *GameMap) isTileOccupied(x, y int) bool {
	for _, ch := range m.players {
		if ch.X == x && ch.Y == y {
			return true
		}
	}
	for _, other := range m.npcs {
		if other.Alive && other.X == x && other.Y == y {
			return true
		}
	}
	return false
}

func (m *GameMap) broadcastNpcWalk(npc *NpcState) {
	pkt := &server.NpcPlayerServerPacket{
		Positions: []server.NpcUpdatePosition{{
			NpcIndex:  npc.Index,
			Coords:    eoproto.Coords{X: npc.X, Y: npc.Y},
			Direction: eoproto.Direction(npc.Direction),
		}},
	}
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(pkt)
	}
}

func (m *GameMap) broadcastNpcAppear(npc *NpcState) {
	// Send via NPC_PLAYER with position update
	pkt := &server.NpcPlayerServerPacket{
		Positions: []server.NpcUpdatePosition{{
			NpcIndex:  npc.Index,
			Coords:    eoproto.Coords{X: npc.X, Y: npc.Y},
			Direction: eoproto.Direction(npc.Direction),
		}},
	}
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(pkt)
	}
}

// DamageNpc applies damage to an NPC. Returns (actualDamage, killed, hpPercentage).
func (m *GameMap) DamageNpc(npcIndex, playerID, damage int) (int, bool, int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if npcIndex < 0 || npcIndex >= len(m.npcs) {
		return 0, false, 0
	}

	npc := m.npcs[npcIndex]
	if !npc.Alive {
		return 0, false, 0
	}

	// Track opponent with O(1) map lookup
	if npc.Opponents == nil {
		npc.Opponents = make(map[int]int)
	}
	npc.Opponents[playerID] += damage

	npc.HP -= damage
	if npc.HP <= 0 {
		npc.HP = 0
		npc.Alive = false
		npc.SpawnTicks = npc.SpawnTime * 8
		npc.Opponents = nil
		return damage, true, 0
	}

	hpPct := int(float64(npc.HP) / float64(npc.MaxHP) * 100)
	return damage, false, hpPct
}

// IsNpcAt checks if an alive NPC is at the given coordinates.
// Returns the NPC index or -1.
func (m *GameMap) IsNpcAt(x, y int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, npc := range m.npcs {
		if npc.Alive && npc.X == x && npc.Y == y {
			return npc.Index
		}
	}
	return -1
}

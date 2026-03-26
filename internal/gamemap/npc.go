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
	TalkTicks  int // countdown to next talk attempt
	Alive      bool
	Opponents  map[int]*NpcOpponent // playerID -> opponent state
}

// NpcOpponent tracks per-player combat state for an NPC.
type NpcOpponent struct {
	DamageDealt int
	BoredTicks  int // ticks since last hit; removed when >= config.NPCs.BoredTimer
}

// SpawnNPCs creates NPC instances from the EMF map data.
func (m *GameMap) SpawnNPCs(instantSpawn bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	index := 0
	for _, npcDef := range m.emf.Npcs {
		for range npcDef.Amount {
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
		npc.TalkTicks++

		// Determine act rate based on NPC spawn type (speed tier)
		npcActRate := m.npcSpeedForType(npc.SpawnType)
		if npcActRate <= 0 {
			continue // frozen NPC (type 7)
		}

		if npc.ActTicks < npcActRate {
			continue
		}
		npc.ActTicks = 0

		// Expire bored opponents
		boredTimer := m.cfg.NPCs.BoredTimer
		if boredTimer > 0 {
			for pid, opp := range npc.Opponents {
				opp.BoredTicks += actRate
				if opp.BoredTicks >= boredTimer {
					delete(npc.Opponents, pid)
				}
			}
		}

		if len(npc.Opponents) > 0 {
			// Chase closest opponent within chase distance
			m.npcChase(npc)
		} else {
			// Random idle movement (25% chance)
			if rand.IntN(4) == 0 {
				if moved := m.npcRandomWalk(npc); moved {
					m.broadcastNpcWalk(npc)
				}
			}
		}
	}
}

// npcChase moves an NPC toward its closest opponent within chase distance.
func (m *GameMap) npcChase(npc *NpcState) {
	chaseDist := m.cfg.NPCs.ChaseDistance
	if chaseDist <= 0 {
		return
	}

	// Find closest opponent
	bestDist := chaseDist + 1
	bestX, bestY := 0, 0
	for pid := range npc.Opponents {
		ch, ok := m.players[pid]
		if !ok {
			continue
		}
		dx := npc.X - ch.X
		dy := npc.Y - ch.Y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		dist := max(dx, dy)
		if dist < bestDist {
			bestDist = dist
			bestX = ch.X
			bestY = ch.Y
		}
	}

	if bestDist > chaseDist {
		return // no opponent in range
	}

	// Move one tile toward target
	dir := -1
	if bestY > npc.Y {
		dir = 0 // Down
	} else if bestX < npc.X {
		dir = 1 // Left
	} else if bestY < npc.Y {
		dir = 2 // Up
	} else if bestX > npc.X {
		dir = 3 // Right
	}
	if dir < 0 {
		return // already adjacent
	}

	newX, newY := npc.X, npc.Y
	switch dir {
	case 0:
		newY++
	case 1:
		newX--
	case 2:
		newY--
	case 3:
		newX++
	}

	if newX < 0 || newY < 0 || newX > m.emf.Width || newY > m.emf.Height {
		return
	}
	if !m.isTileWalkableNpc(newX, newY) || m.isTileOccupied(newX, newY) {
		return
	}

	npc.X = newX
	npc.Y = newY
	npc.Direction = dir
	m.broadcastNpcWalk(npc)
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

	// Track opponent with O(1) map lookup; reset bored timer on hit
	if npc.Opponents == nil {
		npc.Opponents = make(map[int]*NpcOpponent)
	}
	if opp, ok := npc.Opponents[playerID]; ok {
		opp.DamageDealt += damage
		opp.BoredTicks = 0 // reset bored on hit
	} else {
		npc.Opponents[playerID] = &NpcOpponent{DamageDealt: damage}
	}

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

// npcSpeedForType returns the act rate threshold for a given NPC spawn type.
func (m *GameMap) npcSpeedForType(spawnType int) int {
	switch spawnType {
	case 0:
		return m.cfg.NPCs.Speed0
	case 1:
		return m.cfg.NPCs.Speed1
	case 2:
		return m.cfg.NPCs.Speed2
	case 3:
		return m.cfg.NPCs.Speed3
	case 4:
		return m.cfg.NPCs.Speed4
	case 5:
		return m.cfg.NPCs.Speed5
	case 6:
		return m.cfg.NPCs.Speed6
	case 7:
		return 0 // frozen/static
	default:
		return m.cfg.NPCs.Speed0
	}
}

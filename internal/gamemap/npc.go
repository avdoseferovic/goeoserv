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
	Alive      bool
	HP, MaxHP  int
	SpawnTicks int // countdown to respawn when dead
	ActTicks   int // countdown to next action
	Opponents  []NpcOpponent
}

// NpcOpponent tracks damage dealt by a player.
type NpcOpponent struct {
	PlayerID    int
	DamageDealt int
}

// SpawnNPCs creates NPC instances from the EMF map data.
func (m *GameMap) SpawnNPCs(instantSpawn bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	index := 0
	for _, npcDef := range m.emf.Npcs {
		for i := 0; i < npcDef.Amount; i++ {
			npc := &NpcState{
				Index:     index,
				ID:        npcDef.Id,
				X:         npcDef.Coords.X,
				Y:         npcDef.Coords.Y,
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
				npc.Alive = true
				npc.HP = npc.MaxHP
				npc.X = npc.SpawnX
				npc.Y = npc.SpawnY
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

	// Check tile blocking
	if spec, ok := m.tiles[[2]int{newX, newY}]; ok {
		switch spec {
		case eomap.MapTileSpec_Wall, eomap.MapTileSpec_Edge, eomap.MapTileSpec_NpcBoundary:
			return false
		}
	}

	npc.X = newX
	npc.Y = newY
	npc.Direction = dir
	return true
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

	// Track opponent
	found := false
	for i := range npc.Opponents {
		if npc.Opponents[i].PlayerID == playerID {
			npc.Opponents[i].DamageDealt += damage
			found = true
			break
		}
	}
	if !found {
		npc.Opponents = append(npc.Opponents, NpcOpponent{
			PlayerID:    playerID,
			DamageDealt: damage,
		})
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

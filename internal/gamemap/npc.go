package gamemap

import (
	"math/rand/v2"

	pubdata "github.com/avdo/goeoserv/internal/pub"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
	eopub "github.com/ethanmoffat/eolib-go/v3/protocol/pub"
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

		if target, _ := m.npcClosestOpponentLocked(npc); target == nil {
			m.npcAcquireAggroLocked(npc)
		}

		if target, _ := m.npcClosestOpponentLocked(npc); target != nil {
			// Attack if adjacent, otherwise chase closest opponent within chase distance.
			m.npcActAgainstOpponent(npc)
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

func (m *GameMap) npcAcquireAggroLocked(npc *NpcState) {
	if npc == nil {
		return
	}

	npcRec := pubdata.GetNpc(npc.ID)
	if npcRec == nil || npcRec.Type != eopub.Npc_Aggressive {
		return
	}

	chaseDist := m.cfg.NPCs.ChaseDistance
	if chaseDist <= 0 {
		return
	}

	for _, ch := range m.players {
		if ch.HP <= 0 || npcDistance(npc.X, npc.Y, ch.X, ch.Y) > chaseDist {
			continue
		}

		if npc.Opponents == nil {
			npc.Opponents = make(map[int]*NpcOpponent)
		}
		npc.Opponents[ch.PlayerID] = &NpcOpponent{}
	}
}

func (m *GameMap) npcClosestOpponentLocked(npc *NpcState) (*MapCharacter, int) {
	chaseDist := m.cfg.NPCs.ChaseDistance
	if npc == nil || chaseDist <= 0 {
		return nil, 0
	}

	bestDist := chaseDist + 1
	var bestTarget *MapCharacter
	for pid := range npc.Opponents {
		ch, ok := m.players[pid]
		if !ok || ch.HP <= 0 {
			continue
		}

		dist := npcDistance(npc.X, npc.Y, ch.X, ch.Y)
		if dist >= bestDist {
			continue
		}

		bestDist = dist
		bestTarget = ch
	}

	if bestTarget == nil || bestDist > chaseDist {
		return nil, 0
	}

	return bestTarget, bestDist
}

func npcIsOrthogonallyAdjacent(fromX, fromY, toX, toY int) bool {
	dx := fromX - toX
	if dx < 0 {
		dx = -dx
	}

	dy := fromY - toY
	if dy < 0 {
		dy = -dy
	}

	return dx+dy == 1
}

func npcDistance(fromX, fromY, toX, toY int) int {
	dx := fromX - toX
	if dx < 0 {
		dx = -dx
	}
	dy := fromY - toY
	if dy < 0 {
		dy = -dy
	}
	return max(dx, dy)
}

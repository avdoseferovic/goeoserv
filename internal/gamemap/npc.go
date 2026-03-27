package gamemap

import (
	"math"
	"math/rand/v2"

	pubdata "github.com/avdo/goeoserv/internal/pub"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
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

func (m *GameMap) npcActAgainstOpponent(npc *NpcState) {
	target, bestDist := m.npcClosestOpponentLocked(npc)
	if target == nil {
		return
	}

	dir := npcDirectionToward(npc.X, npc.Y, target.X, target.Y)
	if dir >= 0 {
		npc.Direction = dir
	}

	if bestDist <= 1 && npcIsOrthogonallyAdjacent(npc.X, npc.Y, target.X, target.Y) {
		attack, ok := m.npcAttackLocked(npc, target)
		if ok {
			m.broadcastNpcAttack(attack)
		}
		return
	}

	m.npcChase(npc)
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

func (m *GameMap) npcAttackLocked(npc *NpcState, target *MapCharacter) (server.NpcUpdateAttack, bool) {
	if npc == nil || target == nil || !npc.Alive || target.HP <= 0 {
		return server.NpcUpdateAttack{}, false
	}
	if !npcIsOrthogonallyAdjacent(npc.X, npc.Y, target.X, target.Y) {
		return server.NpcUpdateAttack{}, false
	}

	npcAccuracy := 0
	if npcRec := pubdata.GetNpc(npc.ID); npcRec != nil {
		npcAccuracy = npcRec.Accuracy
	}

	damage := 0
	if npcCombatHitRoll(npcAccuracy, target.Evade) {
		damage = reduceNpcDamageByArmor(npcDamageAmount(npc.ID), target.Armor)
		if damage > target.HP {
			damage = target.HP
		}

		target.HP -= damage
		if target.HP < 0 {
			target.HP = 0
		}
	}

	killed := server.PlayerKilledState_Alive
	if target.HP == 0 {
		killed = server.PlayerKilledState_Killed
	}

	return server.NpcUpdateAttack{
		NpcIndex:     npc.Index,
		Killed:       killed,
		Direction:    eoproto.Direction(npc.Direction),
		PlayerId:     target.PlayerID,
		Damage:       damage,
		HpPercentage: playerHpPercentage(target.HP, target.MaxHP),
	}, true
}

// npcChase moves an NPC toward its closest opponent within chase distance.
func (m *GameMap) npcChase(npc *NpcState) {
	target, _ := m.npcClosestOpponentLocked(npc)
	if target == nil {
		return
	}
	newX, newY, dir, ok := m.npcNextChaseStepLocked(npc, target)
	if !ok {
		return
	}

	npc.X = newX
	npc.Y = newY
	npc.Direction = dir
	m.broadcastNpcWalk(npc)
}

func (m *GameMap) npcNextChaseStepLocked(npc *NpcState, target *MapCharacter) (int, int, int, bool) {
	if npc == nil || target == nil || !npc.Alive || target.HP <= 0 {
		return 0, 0, 0, false
	}
	if npcIsOrthogonallyAdjacent(npc.X, npc.Y, target.X, target.Y) {
		return 0, 0, 0, false
	}

	chaseDist := m.cfg.NPCs.ChaseDistance
	if chaseDist <= 0 {
		return 0, 0, 0, false
	}

	minX := max(0, min(npc.X, target.X)-chaseDist)
	maxX := min(m.emf.Width, max(npc.X, target.X)+chaseDist)
	minY := max(0, min(npc.Y, target.Y)-chaseDist)
	maxY := min(m.emf.Height, max(npc.Y, target.Y)+chaseDist)

	type chaseNode struct {
		x, y           int
		firstX, firstY int
		firstDir       int
	}

	queue := []chaseNode{{x: npc.X, y: npc.Y, firstDir: -1}}
	visited := map[[2]int]struct{}{{npc.X, npc.Y}: {}}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		for _, dir := range npcChaseDirectionOrder(node.x, node.y, target.X, target.Y) {
			nextX, nextY := npcStep(node.x, node.y, dir)
			if nextX < minX || nextX > maxX || nextY < minY || nextY > maxY {
				continue
			}

			coords := [2]int{nextX, nextY}
			if _, seen := visited[coords]; seen {
				continue
			}
			if !m.isTileWalkableNpcLocked(nextX, nextY) || m.isTileOccupiedLocked(nextX, nextY) {
				continue
			}

			firstX, firstY, firstDir := node.firstX, node.firstY, node.firstDir
			if firstDir < 0 {
				firstX, firstY, firstDir = nextX, nextY, dir
			}

			if npcIsOrthogonallyAdjacent(nextX, nextY, target.X, target.Y) {
				return firstX, firstY, firstDir, true
			}

			visited[coords] = struct{}{}
			queue = append(queue, chaseNode{x: nextX, y: nextY, firstX: firstX, firstY: firstY, firstDir: firstDir})
		}
	}

	return 0, 0, 0, false
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
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(newNpcPlayerPacket(ch, []server.NpcUpdatePosition{{
			NpcIndex:  npc.Index,
			Coords:    eoproto.Coords{X: npc.X, Y: npc.Y},
			Direction: eoproto.Direction(npc.Direction),
		}}, nil, nil))
	}
}

func (m *GameMap) broadcastNpcAppear(npc *NpcState) {
	// Send via NPC_PLAYER with position update
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(newNpcPlayerPacket(ch, []server.NpcUpdatePosition{{
			NpcIndex:  npc.Index,
			Coords:    eoproto.Coords{X: npc.X, Y: npc.Y},
			Direction: eoproto.Direction(npc.Direction),
		}}, nil, nil))
	}
}

func (m *GameMap) broadcastNpcAttack(attack server.NpcUpdateAttack) {
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(newNpcPlayerPacket(ch, nil, []server.NpcUpdateAttack{attack}, nil))
	}
}

func newNpcPlayerPacket(
	player *MapCharacter,
	positions []server.NpcUpdatePosition,
	attacks []server.NpcUpdateAttack,
	chats []server.NpcUpdateChat,
) *server.NpcPlayerServerPacket {
	pkt := &server.NpcPlayerServerPacket{
		Positions: positions,
		Attacks:   attacks,
		Chats:     chats,
	}

	if !npcAttackTargetsPlayer(attacks, player.PlayerID) {
		return pkt
	}

	hp, tp := player.HP, player.TP
	pkt.Hp = &hp
	pkt.Tp = &tp
	return pkt
}

func npcAttackTargetsPlayer(attacks []server.NpcUpdateAttack, playerID int) bool {
	for _, attack := range attacks {
		if attack.PlayerId == playerID {
			return true
		}
	}

	return false
}

func npcDirectionToward(fromX, fromY, toX, toY int) int {
	if toY > fromY {
		return 0
	}
	if toX < fromX {
		return 1
	}
	if toY < fromY {
		return 2
	}
	if toX > fromX {
		return 3
	}
	return -1
}

func npcChaseDirectionOrder(fromX, fromY, toX, toY int) [4]int {
	switch npcDirectionToward(fromX, fromY, toX, toY) {
	case 0:
		return [4]int{0, 1, 3, 2}
	case 1:
		return [4]int{1, 2, 0, 3}
	case 2:
		return [4]int{2, 1, 3, 0}
	case 3:
		return [4]int{3, 0, 2, 1}
	default:
		return [4]int{0, 1, 2, 3}
	}
}

func npcStep(x, y, dir int) (int, int) {
	switch dir {
	case 0:
		return x, y + 1
	case 1:
		return x - 1, y
	case 2:
		return x, y - 1
	case 3:
		return x + 1, y
	default:
		return x, y
	}
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

func npcDamageAmount(npcID int) int {
	npcRec := pubdata.GetNpc(npcID)
	if npcRec == nil {
		return 1
	}

	damage := npcRec.MinDamage
	if npcRec.MaxDamage > npcRec.MinDamage {
		damage += rand.IntN(npcRec.MaxDamage - npcRec.MinDamage + 1)
	}
	if damage < 1 {
		return 1
	}
	return damage
}

func npcCombatHitRoll(accuracy, evade int) bool {
	return rand.Float64() <= npcCombatHitRate(accuracy, evade)
}

func npcCombatHitRate(accuracy, evade int) float64 {
	hitRate := 0.5
	if accuracy+evade > 0 {
		if evade > 0 {
			hitRate = float64(accuracy) / float64(evade*2)
		} else {
			hitRate = 0.8
		}
	}
	return max(0.5, min(0.8, hitRate))
}

func reduceNpcDamageByArmor(damage, armor int) int {
	if damage < 1 {
		return 1
	}
	if armor > 0 && damage < armor*2 {
		reduced := float64(damage) * math.Pow(float64(damage)/float64(armor*2), 2)
		damage = int(reduced)
	}
	if damage < 1 {
		return 1
	}
	return damage
}

func playerHpPercentage(currentHP, maxHP int) int {
	if maxHP <= 0 {
		return 0
	}

	hpPct := currentHP * 100 / maxHP
	if hpPct < 0 {
		return 0
	}
	if hpPct > 100 {
		return 100
	}
	return hpPct
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
	if damage <= 0 {
		return 0, false, m.npcHpPercentageLocked(npc)
	}

	actualDamage := damage
	if actualDamage > npc.HP {
		actualDamage = npc.HP
	}

	// Track opponent with O(1) map lookup; reset bored timer on hit
	if npc.Opponents == nil {
		npc.Opponents = make(map[int]*NpcOpponent)
	}
	if opp, ok := npc.Opponents[playerID]; ok {
		opp.DamageDealt += actualDamage
		opp.BoredTicks = 0 // reset bored on hit
	} else {
		npc.Opponents[playerID] = &NpcOpponent{DamageDealt: actualDamage}
	}

	npc.HP -= actualDamage
	if npc.HP <= 0 {
		npc.HP = 0
		npc.Alive = false
		npc.SpawnTicks = npc.SpawnTime * 8
		npc.Opponents = nil
		return actualDamage, true, 0
	}

	return actualDamage, false, m.npcHpPercentageLocked(npc)
}

func (m *GameMap) GetNpcHpPercentage(npcIndex int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if npcIndex < 0 || npcIndex >= len(m.npcs) {
		return 0
	}

	npc := m.npcs[npcIndex]
	if !npc.Alive {
		return 0
	}

	return m.npcHpPercentageLocked(npc)
}

func (m *GameMap) npcHpPercentageLocked(npc *NpcState) int {
	if npc == nil || npc.MaxHP <= 0 {
		return 0
	}

	hpPct := int(float64(npc.HP) / float64(npc.MaxHP) * 100)
	if hpPct < 0 {
		return 0
	}
	if hpPct > 100 {
		return 100
	}
	return hpPct
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

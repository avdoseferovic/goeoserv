package gamemap

import (
	"math/rand/v2"
)

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

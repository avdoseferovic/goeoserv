package gamemap

import (
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
)

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

// isTileWalkableNpc checks if an NPC can walk on a tile (walls, chairs, warps, etc. block).
func (m *GameMap) isTileWalkableNpc(x, y int) bool {
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

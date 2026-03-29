package gamemap

import (
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// OpenDoor opens a door warp tile if the player is adjacent and it is not already open.
func (m *GameMap) OpenDoor(playerID, x, y int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, ok := m.players[playerID]
	if !ok {
		return false
	}
	warp, ok := m.warps[[2]int{x, y}]
	if !ok || warp.Door <= 0 {
		return false
	}
	if abs(ch.X-x) > 1 || abs(ch.Y-y) > 1 {
		return false
	}
	if _, alreadyOpen := m.openDoors[[2]int{x, y}]; alreadyOpen {
		return false
	}
	m.openDoors[[2]int{x, y}] = 0
	return true
}

// StartEvacuate begins an evacuation countdown on this map.
func (m *GameMap) StartEvacuate(ticks int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EvacuateTicks = ticks
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (m *GameMap) getGroundItemInfosLocked() []server.ItemMapInfo {
	infos := make([]server.ItemMapInfo, 0, len(m.groundItems))
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

func (m *GameMap) isBlocked(x, y, excludePlayerID int) bool {
	spec, ok := m.tiles[[2]int{x, y}]
	if ok {
		switch spec {
		case eomap.MapTileSpec_Wall, eomap.MapTileSpec_Edge:
			return true
		}
	}

	if warp, ok := m.warps[[2]int{x, y}]; ok && warp.Door > 0 {
		if _, isOpen := m.openDoors[[2]int{x, y}]; !isOpen {
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

func padGuildTag(tag string) string {
	for len(tag) < 3 {
		tag += " "
	}
	return tag[:3]
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

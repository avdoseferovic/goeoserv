package world

import (
	"github.com/avdo/goeoserv/internal/gamemap"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func (w *World) DamageNpc(mapID, npcIndex, playerID, damage int) (int, bool, int) {
	m := w.getMap(mapID)
	if m == nil {
		return 0, false, 0
	}
	return m.DamageNpc(npcIndex, playerID, damage)
}

func (w *World) GetNpcHpPercentage(mapID, npcIndex int) int {
	m := w.getMap(mapID)
	if m == nil {
		return 0
	}
	return m.GetNpcHpPercentage(npcIndex)
}

func (w *World) GetNpcAt(mapID, x, y int) int {
	m := w.getMap(mapID)
	if m == nil {
		return -1
	}
	return m.IsNpcAt(x, y)
}

func (w *World) GetNpcEnfID(mapID, npcIndex int) int {
	m := w.getMap(mapID)
	if m == nil {
		return 0
	}
	npc := m.GetNpc(npcIndex)
	if npc == nil {
		return 0
	}
	return npc.ID
}

func (w *World) DropItem(mapID, itemID, amount, x, y, droppedBy int) int {
	m := w.getMap(mapID)
	if m == nil {
		return 0
	}
	return m.DropItem(itemID, amount, x, y, droppedBy)
}

func (w *World) PickupItem(mapID, uid, playerID int) (int, int, bool) {
	m := w.getMap(mapID)
	if m == nil {
		return 0, 0, false
	}
	item := m.PickupItem(uid, playerID)
	if item == nil {
		return 0, 0, false
	}
	return item.ItemID, item.Amount, true
}

func (w *World) tickAutoPickup(m *gamemap.GameMap) {
	for _, playerID := range m.AutoPickupPlayerIDs() {
		session := w.GetPlayerSession(playerID)
		if session == nil {
			continue
		}

		item := m.PickupAutoItem(playerID)
		if item == nil {
			continue
		}

		session.Mu.Lock()
		session.AddItem(item.ItemID, item.Amount)
		session.CalculateStats()
		currentAmount := session.Inventory[item.ItemID]
		currentWeight := session.Weight
		maxWeight := session.MaxWeight
		bus := session.Bus
		session.Mu.Unlock()

		_ = bus.SendPacket(&server.ItemGetServerPacket{
			TakenItemIndex: item.UID,
			TakenItem:      eonet.ThreeItem{Id: item.ItemID, Amount: currentAmount},
			Weight:         eonet.Weight{Current: currentWeight, Max: maxWeight},
		})
	}
}

func (w *World) GetNearbyInfo(mapID int) any {
	m := w.getMap(mapID)
	if m == nil {
		return nil
	}
	info := m.GetNearbyInfo()
	return &info
}

func (w *World) GetPlayerPosition(playerID int) any {
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e == nil {
		return nil
	}
	m := w.getMap(e.mapID)
	if m == nil {
		return nil
	}
	return m.GetPlayerPosition(playerID)
}

func (w *World) GetPlayerAt(mapID, x, y int) int {
	m := w.getMap(mapID)
	if m == nil {
		return 0
	}
	return m.GetPlayerAt(x, y)
}

func (w *World) IsAttackTileBlocked(mapID, x, y int) bool {
	m := w.getMap(mapID)
	if m == nil {
		return true
	}
	return m.IsAttackTileBlocked(x, y)
}

func (w *World) IsPkMap(mapID int) bool {
	m := w.getMap(mapID)
	if m == nil {
		return false
	}
	return m.IsPkMap()
}

func (w *World) UpdatePlayerVitals(mapID, playerID, hp, tp int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.UpdatePlayerVitals(playerID, hp, tp)
}

func (w *World) UpdatePlayerCombatStats(mapID, playerID, armor, evade int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.UpdatePlayerCombatStats(playerID, armor, evade)
}

func (w *World) UpdatePlayerCombatSnapshot(mapID, playerID, hp, maxHP, tp, maxTP, armor, evade int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.UpdatePlayerCombatSnapshot(playerID, hp, maxHP, tp, maxTP, armor, evade)
}

func (w *World) UpdatePlayerSitState(mapID, playerID, sitState int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.UpdatePlayerSitState(playerID, sitState)
}

func (w *World) GetOnlinePlayers() any {
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	var result []gamemap.OnlinePlayerInfo
	for _, m := range w.maps {
		result = append(result, m.GetOnlinePlayers()...)
	}
	return result
}

func (w *World) WarpPlayer(playerID, fromMapID, toMapID, toX, toY int) any {
	w.mapMu.RLock()
	fromMap := w.maps[fromMapID]
	toMap := w.maps[toMapID]
	w.mapMu.RUnlock()

	if fromMap == nil || toMap == nil {
		return nil
	}

	sameMapWarp := fromMapID == toMapID
	ch := fromMap.RemoveAndReturn(playerID)
	if ch == nil {
		return nil
	}
	if !sameMapWarp {
		w.leaveArena(fromMapID, playerID)
	}

	ch.X = toX
	ch.Y = toY
	ch.MapID = toMapID
	toMap.Enter(ch)
	if !sameMapWarp {
		w.syncArenaParticipation(toMapID, playerID, ch.Name)
	}

	w.UpdatePlayerMap(playerID, toMapID)

	info := toMap.GetNearbyInfo()
	return &info
}

func (w *World) GetChestItems(mapID, x, y int) any {
	m := w.getMap(mapID)
	if m == nil {
		return nil
	}
	return m.GetChestItems(x, y)
}

func (w *World) AddChestItem(mapID, x, y, itemID, amount int) any {
	m := w.getMap(mapID)
	if m == nil {
		return nil
	}
	return m.AddChestItem(x, y, itemID, amount)
}

func (w *World) TakeChestItem(mapID, x, y, itemID int) (int, any) {
	m := w.getMap(mapID)
	if m == nil {
		return 0, nil
	}
	amt, items := m.TakeChestItem(x, y, itemID)
	return amt, items
}

func (w *World) OpenDoor(mapID, playerID, x, y int) bool {
	m := w.getMap(mapID)
	if m == nil {
		return false
	}
	return m.OpenDoor(playerID, x, y)
}

func (w *World) GetPendingWarp(mapID, playerID int) (int, int, int, bool) {
	m := w.getMap(mapID)
	if m == nil {
		return 0, 0, 0, false
	}
	warp := m.GetPendingWarp(playerID)
	if warp == nil {
		return 0, 0, 0, false
	}
	return warp.MapID, warp.X, warp.Y, true
}

func (w *World) SetPendingWarp(mapID, playerID, toMapID, toX, toY int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.SetPendingWarp(playerID, &gamemap.WarpDest{MapID: toMapID, X: toX, Y: toY})
}

func (w *World) UpdateMapEquipment(mapID, playerID, boots, armor, hat, shield, weapon int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.UpdateEquipment(playerID, boots, armor, hat, shield, weapon)
}

func (w *World) StartEvacuate(mapID, ticks int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.StartEvacuate(ticks)
}

func (w *World) TryStartJukebox(mapID, trackID int) bool {
	m := w.getMap(mapID)
	if m == nil {
		return false
	}
	return m.TryStartJukebox(trackID)
}

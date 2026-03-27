package gamemap

import (
	"sync/atomic"

	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

var nextItemUID atomic.Int64

func init() {
	nextItemUID.Store(1)
}

// GroundItem represents an item on the map floor.
type GroundItem struct {
	UID            int
	ItemID         int
	Amount         int
	X, Y           int
	DroppedBy      int // playerID who dropped it, 0 for NPC/chest drops
	ProtectedTicks int // ticks remaining where only DroppedBy can pick up
}

func (m *GameMap) pickupGroundItemLocked(playerID int, matches func(*GroundItem) bool) *GroundItem {
	for i, item := range m.groundItems {
		if !matches(item) {
			continue
		}
		if item.ProtectedTicks > 0 && item.DroppedBy != playerID {
			continue
		}

		m.groundItems = append(m.groundItems[:i], m.groundItems[i+1:]...)

		pkt := &server.ItemRemoveServerPacket{ItemIndex: item.UID}
		for _, ch := range m.players {
			_ = ch.Bus.SendPacket(pkt)
		}

		return item
	}

	return nil
}

func (m *GameMap) trimGroundItemsLocked(maxItems int) []int {
	if maxItems <= 0 || len(m.groundItems) <= maxItems {
		return nil
	}

	overflowCount := len(m.groundItems) - maxItems
	removedUIDs := make([]int, 0, overflowCount)
	for i := 0; i < overflowCount; i++ {
		if m.groundItems[i] == nil {
			continue
		}
		removedUIDs = append(removedUIDs, m.groundItems[i].UID)
	}

	copy(m.groundItems, m.groundItems[overflowCount:])
	for i := len(m.groundItems) - overflowCount; i < len(m.groundItems); i++ {
		m.groundItems[i] = nil
	}
	m.groundItems = m.groundItems[:len(m.groundItems)-overflowCount]

	return removedUIDs
}

// DropItem adds an item to the map floor. Returns the UID.
func (m *GameMap) DropItem(itemID, amount, x, y, droppedBy int) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	uid := int(nextItemUID.Add(1))

	// Calculate drop protection ticks (8 ticks per second)
	protectedTicks := 0
	if droppedBy > 0 && m.cfg.World.DropProtectPlayer > 0 {
		protectedTicks = m.cfg.World.DropProtectPlayer * 8
	} else if droppedBy == 0 && m.cfg.World.DropProtectNPC > 0 {
		protectedTicks = m.cfg.World.DropProtectNPC * 8
	}

	item := &GroundItem{
		UID:            uid,
		ItemID:         itemID,
		Amount:         amount,
		X:              x,
		Y:              y,
		DroppedBy:      droppedBy,
		ProtectedTicks: protectedTicks,
	}

	m.groundItems = append(m.groundItems, item)
	removedUIDs := m.trimGroundItemsLocked(m.cfg.Map.MaxItems)

	// Broadcast item appear to nearby players (exclude dropper — they get ItemDropServerPacket)
	pkt := &server.ItemAddServerPacket{
		ItemId:     itemID,
		ItemIndex:  uid,
		ItemAmount: amount,
		Coords:     eoproto.Coords{X: x, Y: y},
	}
	for _, ch := range m.players {
		for _, removedUID := range removedUIDs {
			_ = ch.Bus.SendPacket(&server.ItemRemoveServerPacket{ItemIndex: removedUID})
		}
		if ch.PlayerID == droppedBy {
			continue
		}
		_ = ch.Bus.SendPacket(pkt)
	}

	return uid
}

func (m *GameMap) tickCleanup() {
	m.mu.Lock()
	removedUIDs := m.trimGroundItemsLocked(m.cfg.Map.MaxItems)
	if len(removedUIDs) == 0 {
		m.mu.Unlock()
		return
	}

	players := make([]*MapCharacter, 0, len(m.players))
	for _, ch := range m.players {
		players = append(players, ch)
	}
	m.mu.Unlock()

	for _, ch := range players {
		for _, removedUID := range removedUIDs {
			_ = ch.Bus.SendPacket(&server.ItemRemoveServerPacket{ItemIndex: removedUID})
		}
	}
}

// PickupItem removes a ground item by UID and returns it. Returns nil if not found
// or if the item is still protected and playerID is not the owner.
func (m *GameMap) PickupItem(uid, playerID int) *GroundItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.pickupGroundItemLocked(playerID, func(item *GroundItem) bool {
		return item.UID == uid
	})
}

// PickupAutoItem removes the first pickup-eligible item on the player's tile.
func (m *GameMap) PickupAutoItem(playerID int) *GroundItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, ok := m.players[playerID]
	if !ok {
		return nil
	}

	return m.pickupGroundItemLocked(playerID, func(item *GroundItem) bool {
		return item.X == ch.X && item.Y == ch.Y
	})
}

// AutoPickupPlayerIDs returns players currently standing on at least one ground item.
func (m *GameMap) AutoPickupPlayerIDs() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.players) == 0 || len(m.groundItems) == 0 {
		return nil
	}

	playerIDs := make([]int, 0, len(m.players))
	for playerID, ch := range m.players {
		for _, item := range m.groundItems {
			if item.X != ch.X || item.Y != ch.Y {
				continue
			}
			playerIDs = append(playerIDs, playerID)
			break
		}
	}

	return playerIDs
}

// GetGroundItemInfos returns ItemMapInfo for all ground items.
func (m *GameMap) GetGroundItemInfos() []server.ItemMapInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

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

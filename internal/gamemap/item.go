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

	// Enforce max items on map — copy to avoid backing array leak
	if len(m.groundItems) >= m.cfg.Map.MaxItems {
		copy(m.groundItems, m.groundItems[1:])
		m.groundItems[len(m.groundItems)-1] = nil
		m.groundItems = m.groundItems[:len(m.groundItems)-1]
	}

	m.groundItems = append(m.groundItems, item)

	// Broadcast item appear to nearby players (exclude dropper — they get ItemDropServerPacket)
	pkt := &server.ItemAddServerPacket{
		ItemId:     itemID,
		ItemIndex:  uid,
		ItemAmount: amount,
		Coords:     eoproto.Coords{X: x, Y: y},
	}
	for _, ch := range m.players {
		if ch.PlayerID == droppedBy {
			continue
		}
		_ = ch.Bus.SendPacket(pkt)
	}

	return uid
}

// PickupItem removes a ground item by UID and returns it. Returns nil if not found
// or if the item is still protected and playerID is not the owner.
func (m *GameMap) PickupItem(uid, playerID int) *GroundItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, item := range m.groundItems {
		if item.UID == uid {
			// Drop protection: only owner can pick up while protected
			if item.ProtectedTicks > 0 && item.DroppedBy != playerID {
				return nil
			}
			m.groundItems = append(m.groundItems[:i], m.groundItems[i+1:]...)

			// Broadcast item removal
			pkt := &server.ItemRemoveServerPacket{ItemIndex: uid}
			for _, ch := range m.players {
				_ = ch.Bus.SendPacket(pkt)
			}

			return item
		}
	}
	return nil
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

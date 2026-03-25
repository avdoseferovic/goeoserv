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
	UID       int
	ItemID    int
	Amount    int
	X, Y      int
	DroppedBy int // playerID who dropped it, 0 for NPC/chest drops
}

// DropItem adds an item to the map floor. Returns the UID.
func (m *GameMap) DropItem(itemID, amount, x, y, droppedBy int) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	uid := int(nextItemUID.Add(1))
	item := &GroundItem{
		UID:       uid,
		ItemID:    itemID,
		Amount:    amount,
		X:         x,
		Y:         y,
		DroppedBy: droppedBy,
	}

	// Enforce max items on map
	if len(m.groundItems) >= m.cfg.Map.MaxItems {
		// Remove oldest item
		m.groundItems = m.groundItems[1:]
	}

	m.groundItems = append(m.groundItems, item)

	// Broadcast item appear to all players
	pkt := &server.ItemAddServerPacket{
		ItemId:     itemID,
		ItemIndex:  uid,
		ItemAmount: amount,
		Coords:     eoproto.Coords{X: x, Y: y},
	}
	for _, ch := range m.players {
		_ = ch.Bus.SendPacket(pkt)
	}

	return uid
}

// PickupItem removes a ground item by UID and returns it. Returns nil if not found.
func (m *GameMap) PickupItem(uid int) *GroundItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, item := range m.groundItems {
		if item.UID == uid {
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

	var infos []server.ItemMapInfo
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

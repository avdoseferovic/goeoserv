package handlers

import (
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Item, eonet.PacketAction_Get, handleItemGet)
	player.Register(eonet.PacketFamily_Item, eonet.PacketAction_Drop, handleItemDrop)
	player.Register(eonet.PacketFamily_Item, eonet.PacketAction_Junk, handleItemJunk)
}

// handleItemGet picks up a ground item.
func handleItemGet(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.ItemGetClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize item get", "id", p.ID, "err", err)
		return nil
	}

	itemID, amount, ok := p.World.PickupItem(p.MapID, pkt.ItemIndex)
	if !ok {
		return nil
	}

	p.Inventory[itemID] += amount

	return p.Bus.SendPacket(&server.ItemGetServerPacket{
		TakenItemIndex: pkt.ItemIndex,
		TakenItem:      eonet.ThreeItem{Id: itemID, Amount: p.Inventory[itemID]},
		Weight:         eonet.Weight{Current: 0, Max: 70},
	})
}

// handleItemDrop drops an item from inventory onto the map.
func handleItemDrop(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.ItemDropClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize item drop", "id", p.ID, "err", err)
		return nil
	}

	itemID := pkt.Item.Id
	amount := pkt.Item.Amount

	if p.Inventory[itemID] < amount {
		return nil
	}

	p.Inventory[itemID] -= amount
	if p.Inventory[itemID] <= 0 {
		delete(p.Inventory, itemID)
	}

	dropX, dropY := p.CharX, p.CharY
	if pkt.Coords.X != 255 && pkt.Coords.Y != 255 {
		dropX = pkt.Coords.X
		dropY = pkt.Coords.Y
	}

	uid := p.World.DropItem(p.MapID, itemID, amount, dropX, dropY, p.ID)

	remaining := p.Inventory[itemID]
	return p.Bus.SendPacket(&server.ItemDropServerPacket{
		DroppedItem:     eonet.ThreeItem{Id: itemID, Amount: amount},
		RemainingAmount: remaining,
		ItemIndex:       uid,
		Coords:          eoproto.Coords{X: dropX, Y: dropY},
		Weight:          eonet.Weight{Current: 0, Max: 70},
	})
}

// handleItemJunk destroys items from inventory.
func handleItemJunk(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.ItemJunkClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize item junk", "id", p.ID, "err", err)
		return nil
	}

	itemID := pkt.Item.Id
	amount := pkt.Item.Amount

	if p.Inventory[itemID] < amount {
		return nil
	}

	p.Inventory[itemID] -= amount
	if p.Inventory[itemID] <= 0 {
		delete(p.Inventory, itemID)
	}

	remaining := p.Inventory[itemID]
	return p.Bus.SendPacket(&server.ItemJunkServerPacket{
		JunkedItem:      eonet.ThreeItem{Id: itemID, Amount: amount},
		RemainingAmount: remaining,
		Weight:          eonet.Weight{Current: 0, Max: 70},
	})
}

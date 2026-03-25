package handlers

import (
	"context"

	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Chest, eonet.PacketAction_Open, handleChestOpen)
	player.Register(eonet.PacketFamily_Chest, eonet.PacketAction_Add, handleChestAdd)
	player.Register(eonet.PacketFamily_Chest, eonet.PacketAction_Take, handleChestTake)
}

func handleChestOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	var pkt client.ChestOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	raw := p.World.GetChestItems(p.MapID, pkt.Coords.X, pkt.Coords.Y)
	return p.Bus.SendPacket(&server.ChestOpenServerPacket{
		Coords: pkt.Coords,
		Items:  chestItemsToEo(raw),
	})
}

func handleChestAdd(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	var pkt client.ChestAddClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.RemoveItem(pkt.AddItem.Id, pkt.AddItem.Amount) {
		return nil
	}
	raw := p.World.AddChestItem(p.MapID, pkt.Coords.X, pkt.Coords.Y, pkt.AddItem.Id, pkt.AddItem.Amount)
	if raw == nil {
		p.AddItem(pkt.AddItem.Id, pkt.AddItem.Amount)
		return nil
	}
	return p.Bus.SendPacket(&server.ChestReplyServerPacket{
		AddedItemId:     pkt.AddItem.Id,
		RemainingAmount: p.Inventory[pkt.AddItem.Id],
		Weight:          eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
		Items:           chestItemsToEo(raw),
	})
}

func handleChestTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	var pkt client.ChestTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	amount, raw := p.World.TakeChestItem(p.MapID, pkt.Coords.X, pkt.Coords.Y, pkt.TakeItemId)
	if amount == 0 {
		return nil
	}
	p.AddItem(pkt.TakeItemId, amount)
	return p.Bus.SendPacket(&server.ChestGetServerPacket{
		TakenItem: eonet.ThreeItem{Id: pkt.TakeItemId, Amount: p.Inventory[pkt.TakeItemId]},
		Weight:    eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
		Items:     chestItemsToEo(raw),
	})
}

func chestItemsToEo(raw any) []eonet.ThreeItem {
	items, _ := raw.([]gamemap.ChestItem)
	eoItems := make([]eonet.ThreeItem, 0, len(items))
	for _, ci := range items {
		eoItems = append(eoItems, eonet.ThreeItem{Id: ci.ItemID, Amount: ci.Amount})
	}
	return eoItems
}

package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Shop, eonet.PacketAction_Open, handleShopOpen)
	player.Register(eonet.PacketFamily_Shop, eonet.PacketAction_Buy, handleShopBuy)
	player.Register(eonet.PacketFamily_Shop, eonet.PacketAction_Sell, handleShopSell)
}

func handleShopOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.ShopOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize shop open", "id", p.ID, "err", err)
		return nil
	}

	sessionID := p.GenerateSessionID()

	// Shop data files not yet loaded; return basic response with session
	return p.Bus.SendPacket(&server.ShopOpenServerPacket{
		SessionId: sessionID,
		ShopName:  "Shop",
	})
}

func handleShopBuy(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.ShopBuyClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize shop buy", "id", p.ID, "err", err)
		return nil
	}

	// Validate session
	if _, ok := p.TakeSessionID(); !ok {
		return nil
	}

	// Calculate cost (1 gold per item as placeholder since shop data files aren't loaded)
	cost := pkt.BuyItem.Amount
	gold := p.Inventory[1]
	if gold < cost {
		return nil
	}

	// Deduct gold and add item
	p.Inventory[1] -= cost
	if p.Inventory[1] <= 0 {
		delete(p.Inventory, 1)
	}
	p.Inventory[pkt.BuyItem.Id] += pkt.BuyItem.Amount

	return p.Bus.SendPacket(&server.ShopBuyServerPacket{
		GoldAmount: p.Inventory[1], // item 1 = gold by convention
		BoughtItem: eonet.Item{Id: pkt.BuyItem.Id, Amount: p.Inventory[pkt.BuyItem.Id]},
		Weight:     eonet.Weight{Current: 0, Max: 70},
	})
}

func handleShopSell(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.ShopSellClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize shop sell", "id", p.ID, "err", err)
		return nil
	}

	if p.Inventory[pkt.SellItem.Id] < pkt.SellItem.Amount {
		return nil
	}

	p.Inventory[pkt.SellItem.Id] -= pkt.SellItem.Amount
	if p.Inventory[pkt.SellItem.Id] <= 0 {
		delete(p.Inventory, pkt.SellItem.Id)
	}

	// Add gold based on sell price (1 gold per item as placeholder)
	sellPrice := pkt.SellItem.Amount
	p.Inventory[1] += sellPrice

	return p.Bus.SendPacket(&server.ShopSellServerPacket{
		SoldItem:   server.ShopSoldItem{Id: pkt.SellItem.Id, Amount: p.Inventory[pkt.SellItem.Id]},
		GoldAmount: p.Inventory[1],
		Weight:     eonet.Weight{Current: 0, Max: 70},
	})
}

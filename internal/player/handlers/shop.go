package handlers

import (
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

func handleShopOpen(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.ShopOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize shop open", "id", p.ID, "err", err)
		return nil
	}

	sessionID := p.GenerateSessionID()

	// TODO: Load actual shop data from ShopFile based on NPC ID
	return p.Bus.SendPacket(&server.ShopOpenServerPacket{
		SessionId: sessionID,
		ShopName:  "Shop",
	})
}

func handleShopBuy(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.ShopBuyClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize shop buy", "id", p.ID, "err", err)
		return nil
	}

	// TODO: Validate session, check gold, look up item price from shop data
	// For now: add item to inventory
	p.Inventory[pkt.BuyItem.Id] += pkt.BuyItem.Amount

	return p.Bus.SendPacket(&server.ShopBuyServerPacket{
		GoldAmount: p.Inventory[1], // item 1 = gold by convention
		BoughtItem: eonet.Item{Id: pkt.BuyItem.Id, Amount: p.Inventory[pkt.BuyItem.Id]},
		Weight:     eonet.Weight{Current: 0, Max: 70},
	})
}

func handleShopSell(p *player.Player, reader *player.EoReader) error {
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

	// TODO: Add gold based on item sell price

	return p.Bus.SendPacket(&server.ShopSellServerPacket{
		SoldItem:   server.ShopSoldItem{Id: pkt.SellItem.Id, Amount: p.Inventory[pkt.SellItem.Id]},
		GoldAmount: p.Inventory[1],
		Weight:     eonet.Weight{Current: 0, Max: 70},
	})
}

package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/content"
	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Shop, eonet.PacketAction_Open, handleShopOpen)
	player.Register(eonet.PacketFamily_Shop, eonet.PacketAction_Create, handleShopCreate)
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
	npcID := 0
	if p.World != nil {
		npcID = p.World.GetNpcEnfID(p.MapID, pkt.NpcIndex)
	}

	shop, ok := content.GetShop(npcID)
	if !ok {
		return p.Bus.SendPacket(&server.ShopOpenServerPacket{SessionId: sessionID, ShopName: "Shop"})
	}

	tradeItems := make([]server.ShopTradeItem, 0, len(shop.Buy))
	for _, offer := range shop.Buy {
		sellPrice := findSellPrice(shop, offer.ItemID)
		tradeItems = append(tradeItems, server.ShopTradeItem{
			ItemId:       offer.ItemID,
			BuyPrice:     offer.Cost,
			SellPrice:    sellPrice,
			MaxBuyAmount: 100,
		})
	}

	craftItems := make([]server.ShopCraftItem, 0, len(shop.Craft))
	for _, craft := range shop.Craft {
		ingredients := []eonet.CharItem{}
		for _, ing := range craft.Ingredients {
			ingredients = append(ingredients, eonet.CharItem{Id: ing.ItemID, Amount: ing.Amount})
		}
		for len(ingredients) < 4 {
			ingredients = append(ingredients, eonet.CharItem{})
		}
		craftItems = append(craftItems, server.ShopCraftItem{ItemId: craft.ItemID, Ingredients: ingredients[:4]})
	}

	return p.Bus.SendPacket(&server.ShopOpenServerPacket{
		SessionId:  sessionID,
		ShopName:   shop.Name,
		TradeItems: tradeItems,
		CraftItems: craftItems,
	})
}

func handleShopCreate(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.ShopCreateClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize shop create", "id", p.ID, "err", err)
		return nil
	}

	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}

	craft, ok := findCraftOffer(pkt.CraftItemId)
	if !ok {
		return nil
	}
	if craft.Requirements.MinLevel > 0 && p.CharLevel < craft.Requirements.MinLevel {
		return nil
	}
	if craft.Requirements.ClassID > 0 && p.ClassID != craft.Requirements.ClassID {
		return nil
	}
	if craft.Cost > 0 && !p.RemoveItem(1, craft.Cost) {
		return nil
	}
	for _, ing := range craft.Ingredients {
		if p.Inventory[ing.ItemID] < ing.Amount {
			return nil
		}
	}
	for _, ing := range craft.Ingredients {
		p.RemoveItem(ing.ItemID, ing.Amount)
	}
	p.AddItem(craft.ItemID, 1)
	p.CalculateStats()

	ingredients := []eonet.Item{}
	for _, ing := range craft.Ingredients {
		ingredients = append(ingredients, eonet.Item{Id: ing.ItemID, Amount: p.Inventory[ing.ItemID]})
	}
	for len(ingredients) < 4 {
		ingredients = append(ingredients, eonet.Item{})
	}

	return p.Bus.SendPacket(&server.ShopCreateServerPacket{
		CraftItemId: craft.ItemID,
		Weight:      eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
		Ingredients: ingredients[:4],
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

	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}

	price := findBuyPrice(pkt.BuyItem.Id)
	if price <= 0 {
		price = 1
	}
	cost := price * pkt.BuyItem.Amount
	if !p.RemoveItem(1, cost) {
		return nil
	}
	p.AddItem(pkt.BuyItem.Id, pkt.BuyItem.Amount)
	p.CalculateStats()

	return p.Bus.SendPacket(&server.ShopBuyServerPacket{
		GoldAmount: p.Inventory[1],
		BoughtItem: eonet.Item{Id: pkt.BuyItem.Id, Amount: p.Inventory[pkt.BuyItem.Id]},
		Weight:     eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
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

	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	if !p.RemoveItem(pkt.SellItem.Id, pkt.SellItem.Amount) {
		return nil
	}

	sellPrice := findSellPriceByItem(pkt.SellItem.Id)
	if sellPrice <= 0 {
		sellPrice = 1
	}
	p.AddItem(1, sellPrice*pkt.SellItem.Amount)
	p.CalculateStats()

	return p.Bus.SendPacket(&server.ShopSellServerPacket{
		SoldItem:   server.ShopSoldItem{Id: pkt.SellItem.Id, Amount: p.Inventory[pkt.SellItem.Id]},
		GoldAmount: p.Inventory[1],
		Weight:     eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
	})
}

func findBuyPrice(itemID int) int {
	for _, shop := range content.Current().Shops {
		for _, offer := range shop.Buy {
			if offer.ItemID == itemID {
				return offer.Cost
			}
		}
	}
	return 0
}

func findSellPrice(shop content.Shop, itemID int) int {
	for _, offer := range shop.Sell {
		if offer.ItemID == itemID {
			return offer.Cost
		}
	}
	return 0
}

func findSellPriceByItem(itemID int) int {
	for _, shop := range content.Current().Shops {
		for _, offer := range shop.Sell {
			if offer.ItemID == itemID {
				return offer.Cost
			}
		}
	}
	return 0
}

func findCraftOffer(itemID int) (content.CraftOffer, bool) {
	for _, shop := range content.Current().Shops {
		for _, offer := range shop.Craft {
			if offer.ItemID == itemID {
				return offer, true
			}
		}
	}
	return content.CraftOffer{}, false
}

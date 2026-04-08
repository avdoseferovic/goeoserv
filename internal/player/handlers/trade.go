package handlers

import (
	"context"
	"log/slog"

	"github.com/avdoseferovic/geoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Request, handleTradeRequest)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Accept, handleTradeAccept)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Add, handleTradeAdd)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Remove, handleTradeRemove)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Agree, handleTradeAgree)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Close, handleTradeClose)
}

// handleTradeRequest — player requests to trade with another player.
func handleTradeRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.TradeRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize trade request", "id", p.ID, "err", err)
		return nil
	}

	// Can't trade if already in a trade
	if p.TradePartnerID != 0 {
		return nil
	}

	partnerName := p.World.GetPlayerName(pkt.PlayerId)
	if partnerName == "" {
		return nil
	}

	// Send trade request to the target player
	p.World.SendToPlayer(pkt.PlayerId, &server.TradeRequestServerPacket{
		PartnerPlayerId:   p.ID,
		PartnerPlayerName: p.CharName,
	})

	// Store that we want to trade with this player
	p.TradePartnerID = pkt.PlayerId

	return nil
}

// handleTradeAccept — player accepts a trade request.
func handleTradeAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.TradeAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize trade accept", "id", p.ID, "err", err)
		return nil
	}

	partnerName := p.World.GetPlayerName(pkt.PlayerId)
	if partnerName == "" {
		return nil
	}

	// Set up trade session for both players
	p.TradePartnerID = pkt.PlayerId
	p.TradeItems = make(map[int]int)
	p.TradeAgreed = false

	// Open trade windows for both players
	_ = p.Bus.SendPacket(&server.TradeOpenServerPacket{
		PartnerPlayerId:   pkt.PlayerId,
		PartnerPlayerName: partnerName,
		YourPlayerId:      p.ID,
		YourPlayerName:    p.CharName,
	})

	p.World.SendToPlayer(pkt.PlayerId, &server.TradeOpenServerPacket{
		PartnerPlayerId:   p.ID,
		PartnerPlayerName: p.CharName,
		YourPlayerId:      pkt.PlayerId,
		YourPlayerName:    partnerName,
	})

	return nil
}

// handleTradeAdd — player adds an item to the trade.
func handleTradeAdd(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil || p.TradePartnerID == 0 {
		return nil
	}

	var pkt client.TradeAddClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize trade add", "id", p.ID, "err", err)
		return nil
	}

	// Enforce trade amount limit
	amount := pkt.AddItem.Amount
	if maxTrade := p.Cfg.Limits.MaxTrade; maxTrade > 0 && amount > maxTrade {
		amount = maxTrade
	}

	// Check player has enough of the item
	if p.Inventory[pkt.AddItem.Id] < amount {
		return nil
	}
	for _, protectedID := range p.Cfg.Items.ProtectedItems {
		if protectedID == pkt.AddItem.Id {
			return nil
		}
	}

	if p.TradeItems == nil {
		p.TradeItems = make(map[int]int)
	}
	p.TradeItems[pkt.AddItem.Id] += amount

	// Reset both players' agreement when items change
	p.TradeAgreed = false

	// Send updated trade data to both players
	sendTradeUpdate(p)

	return nil
}

// handleTradeRemove — player removes an item from the trade.
func handleTradeRemove(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil || p.TradePartnerID == 0 {
		return nil
	}

	var pkt client.TradeRemoveClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize trade remove", "id", p.ID, "err", err)
		return nil
	}

	delete(p.TradeItems, pkt.ItemId)
	p.TradeAgreed = false

	sendTradeUpdate(p)

	return nil
}

// handleTradeAgree — player agrees to the trade (or unagrees).
func handleTradeAgree(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil || p.TradePartnerID == 0 {
		return nil
	}

	var pkt client.TradeAgreeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize trade agree", "id", p.ID, "err", err)
		return nil
	}

	p.TradeAgreed = pkt.Agree
	_ = p.Bus.SendPacket(&server.TradeSpecServerPacket{Agree: pkt.Agree})

	partner := p.World.GetPlayerSession(p.TradePartnerID)
	if partner == nil {
		return nil
	}

	_ = partner.Bus.SendPacket(&server.TradeAgreeServerPacket{
		PartnerPlayerId: p.ID,
		Agree:           pkt.Agree,
	})

	if !pkt.Agree || !partner.TradeAgreed {
		return nil
	}

	finalizeTrade(p, partner)
	return nil
}

func finalizeTrade(p, partner *player.Player) {
	for itemID, amount := range p.TradeItems {
		if amount <= 0 || p.Inventory[itemID] < amount {
			p.TradeAgreed = false
			partner.TradeAgreed = false
			sendTradeUpdate(p)
			return
		}
	}
	for itemID, amount := range partner.TradeItems {
		if amount <= 0 || partner.Inventory[itemID] < amount {
			p.TradeAgreed = false
			partner.TradeAgreed = false
			sendTradeUpdate(p)
			return
		}
	}

	myItems := buildTradeItems(p.ID, p.TradeItems)
	partnerItems := buildTradeItems(partner.ID, partner.TradeItems)

	for itemID, amount := range p.TradeItems {
		p.RemoveItem(itemID, amount)
		partner.AddItem(itemID, amount)
	}
	for itemID, amount := range partner.TradeItems {
		partner.RemoveItem(itemID, amount)
		p.AddItem(itemID, amount)
	}

	p.CalculateStats()
	partner.CalculateStats()

	_ = p.Bus.SendPacket(&server.TradeUseServerPacket{
		TradeData: []server.TradeItemData{myItems, partnerItems},
	})
	_ = partner.Bus.SendPacket(&server.TradeUseServerPacket{
		TradeData: []server.TradeItemData{partnerItems, myItems},
	})

	p.TradePartnerID = 0
	p.TradeItems = nil
	p.TradeAgreed = false
	partner.TradePartnerID = 0
	partner.TradeItems = nil
	partner.TradeAgreed = false
}

// handleTradeClose — player closes the trade window.
func handleTradeClose(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil || p.TradePartnerID == 0 {
		return nil
	}

	partnerID := p.TradePartnerID

	// Close trade for this player
	p.TradePartnerID = 0
	p.TradeItems = nil
	p.TradeAgreed = false

	// Notify partner
	p.World.SendToPlayer(partnerID, &server.TradeCloseServerPacket{
		PartnerPlayerId: p.ID,
	})

	return nil
}

// sendTradeUpdate sends current trade item data to both players.
func sendTradeUpdate(p *player.Player) {
	partner := p.World.GetPlayerSession(p.TradePartnerID)
	if partner == nil {
		return
	}

	myItems := buildTradeItems(p.ID, p.TradeItems)
	partnerItems := buildTradeItems(partner.ID, partner.TradeItems)

	_ = p.Bus.SendPacket(&server.TradeReplyServerPacket{
		TradeData: []server.TradeItemData{myItems, partnerItems},
	})
	_ = partner.Bus.SendPacket(&server.TradeReplyServerPacket{
		TradeData: []server.TradeItemData{partnerItems, myItems},
	})
}

func buildTradeItems(playerID int, items map[int]int) server.TradeItemData {
	var eoItems []eonet.Item
	for itemID, amount := range items {
		if amount > 0 {
			eoItems = append(eoItems, eonet.Item{Id: itemID, Amount: amount})
		}
	}
	return server.TradeItemData{
		PlayerId: playerID,
		Items:    eoItems,
	}
}

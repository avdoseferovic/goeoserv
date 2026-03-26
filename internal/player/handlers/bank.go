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
	player.Register(eonet.PacketFamily_Bank, eonet.PacketAction_Open, handleBankOpen)
	player.Register(eonet.PacketFamily_Bank, eonet.PacketAction_Add, handleBankDeposit)
	player.Register(eonet.PacketFamily_Bank, eonet.PacketAction_Take, handleBankWithdraw)
}

func handleBankOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.BankOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize bank open", "id", p.ID, "err", err)
		return nil
	}

	sessionID := p.GenerateSessionID()

	// gold_bank is loaded from DB during character select (welcome.go)
	return p.Bus.SendPacket(&server.BankOpenServerPacket{
		GoldBank:       p.GoldBank,
		SessionId:      sessionID,
		LockerUpgrades: 0,
	})
}

func handleBankDeposit(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.BankAddClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize bank deposit", "id", p.ID, "err", err)
		return nil
	}

	// Validate session to ensure bank was opened
	if _, ok := p.TakeSessionID(); !ok {
		return nil
	}

	// Cap deposit to not exceed max bank gold
	amount := pkt.Amount
	if amount <= 0 {
		return nil
	}
	if maxGold := p.Cfg.Limits.MaxBankGold; maxGold > 0 {
		amount = min(amount, maxGold-p.GoldBank)
	}
	if amount <= 0 || !p.RemoveItem(1, amount) {
		return nil
	}
	p.GoldBank += amount

	return p.Bus.SendPacket(&server.BankReplyServerPacket{
		GoldInventory: p.Inventory[1],
		GoldBank:      p.GoldBank,
	})
}

func handleBankWithdraw(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.BankTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize bank withdraw", "id", p.ID, "err", err)
		return nil
	}

	// Validate session to ensure bank was opened
	if _, ok := p.TakeSessionID(); !ok {
		return nil
	}

	if pkt.Amount > p.GoldBank || pkt.Amount <= 0 {
		return nil
	}

	p.GoldBank -= pkt.Amount
	p.AddItem(1, pkt.Amount)

	return p.Bus.SendPacket(&server.BankReplyServerPacket{
		GoldInventory: p.Inventory[1],
		GoldBank:      p.GoldBank,
	})
}

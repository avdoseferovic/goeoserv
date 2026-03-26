package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	"github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Open, handleLockerOpen)
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Add, handleLockerAdd)
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Take, handleLockerTake)
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Buy, handleLockerBuy)
}

func handleLockerOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.LockerOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	items, err := queryLockerItems(ctx, p)
	if err != nil {
		slog.Error("locker open query failed", "id", p.ID, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.LockerOpenServerPacket{
		LockerCoords: protocol.Coords{X: pkt.LockerCoords.X, Y: pkt.LockerCoords.Y},
		LockerItems:  items,
	})
}

func handleLockerAdd(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.LockerAddClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	itemID := pkt.DepositItem.Id
	amount := pkt.DepositItem.Amount
	if itemID <= 0 || amount <= 0 {
		return nil
	}

	// Check locker capacity: base_size + (bank_level * size_step)
	capacity := p.Cfg.Bank.BaseSize + (p.BankLevel * p.Cfg.Bank.SizeStep)
	currentItems, _ := queryLockerItems(ctx, p)
	if len(currentItems) >= capacity {
		return nil
	}

	// Enforce max item amount per stack in bank
	if maxAmt := p.Cfg.Bank.MaxItemAmount; maxAmt > 0 {
		for _, li := range currentItems {
			if li.Id == itemID && li.Amount+amount > maxAmt {
				return nil
			}
		}
	}

	if !p.RemoveItem(itemID, amount) {
		return nil
	}

	err := p.DB.Execute(ctx,
		`INSERT INTO character_bank (character_id, item_id, quantity) VALUES (?, ?, ?)
		ON CONFLICT(character_id, item_id) DO UPDATE SET quantity = quantity + ?`,
		*p.CharacterID, itemID, amount, amount)
	if err != nil {
		slog.Error("locker add db failed", "id", p.ID, "err", err)
		p.AddItem(itemID, amount)
		return nil
	}

	items, err := queryLockerItems(ctx, p)
	if err != nil {
		slog.Error("locker add re-query failed", "id", p.ID, "err", err)
		return nil
	}

	remaining := p.Inventory[itemID]
	return p.Bus.SendPacket(&server.LockerReplyServerPacket{
		DepositedItem: eonet.Item{Id: itemID, Amount: remaining},
		Weight:        eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
		LockerItems:   items,
	})
}

func handleLockerTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.LockerTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	itemID := pkt.TakeItemId
	if itemID <= 0 {
		return nil
	}

	var bankQty int
	err := p.DB.QueryRow(ctx,
		"SELECT quantity FROM character_bank WHERE character_id = ? AND item_id = ?",
		*p.CharacterID, itemID).Scan(&bankQty)
	if err != nil || bankQty <= 0 {
		return nil
	}

	err = p.DB.Execute(ctx,
		"DELETE FROM character_bank WHERE character_id = ? AND item_id = ?",
		*p.CharacterID, itemID)
	if err != nil {
		slog.Error("locker take db failed", "id", p.ID, "err", err)
		return nil
	}

	p.AddItem(itemID, bankQty)

	items, err := queryLockerItems(ctx, p)
	if err != nil {
		slog.Error("locker take re-query failed", "id", p.ID, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.LockerGetServerPacket{
		TakenItem:   eonet.ThreeItem{Id: itemID, Amount: p.Inventory[itemID]},
		Weight:      eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
		LockerItems: items,
	})
}

func handleLockerBuy(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}

	// Check max upgrades
	if p.BankLevel >= p.Cfg.Bank.MaxUpgrades {
		return nil
	}

	// Calculate cost: base + (level * step)
	cost := p.Cfg.Bank.UpgradeBaseCost + (p.BankLevel * p.Cfg.Bank.UpgradeCostStep)
	if !p.RemoveItem(1, cost) {
		return nil
	}

	p.BankLevel++

	// Persist to DB
	if p.CharacterID != nil {
		_ = p.DB.Execute(ctx,
			`UPDATE characters SET bank_level = ? WHERE id = ?`, p.BankLevel, *p.CharacterID)
	}

	return p.Bus.SendPacket(&server.LockerBuyServerPacket{
		GoldAmount:     p.Inventory[1],
		LockerUpgrades: p.BankLevel,
	})
}

func queryLockerItems(ctx context.Context, p *player.Player) ([]eonet.ThreeItem, error) {
	rows, err := p.DB.Query(ctx,
		"SELECT item_id, quantity FROM character_bank WHERE character_id = ?", *p.CharacterID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "err", err)
		}
	}()

	var items []eonet.ThreeItem
	for rows.Next() {
		var item eonet.ThreeItem
		if err := rows.Scan(&item.Id, &item.Amount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

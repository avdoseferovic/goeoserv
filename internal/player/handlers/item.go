package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/formula"
	"github.com/avdo/goeoserv/internal/player"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
	eopub "github.com/ethanmoffat/eolib-go/v3/protocol/pub"
)

func init() {
	player.Register(eonet.PacketFamily_Item, eonet.PacketAction_Get, handleItemGet)
	player.Register(eonet.PacketFamily_Item, eonet.PacketAction_Drop, handleItemDrop)
	player.Register(eonet.PacketFamily_Item, eonet.PacketAction_Junk, handleItemJunk)
	player.Register(eonet.PacketFamily_Item, eonet.PacketAction_Use, handleItemUse)
}

// handleItemGet picks up a ground item.
func handleItemGet(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.ItemGetClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize item get", "id", p.ID, "err", err)
		return nil
	}

	itemID, amount, ok := p.World.PickupItem(p.MapID, pkt.ItemIndex, p.ID)
	if !ok {
		return nil
	}

	p.AddItem(itemID, amount)

	return p.Bus.SendPacket(&server.ItemGetServerPacket{
		TakenItemIndex: pkt.ItemIndex,
		TakenItem:      eonet.ThreeItem{Id: itemID, Amount: p.Inventory[itemID]},
		Weight:         eonet.Weight{Current: 0, Max: 70},
	})
}

// handleItemDrop drops an item from inventory onto the map.
func handleItemDrop(ctx context.Context, p *player.Player, reader *player.EoReader) error {
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
	if isProtectedItem(p, itemID) {
		return nil
	}

	if !p.RemoveItem(itemID, amount) {
		return nil
	}

	dropX, dropY := p.CharX, p.CharY
	if pkt.Coords.X != 255 || pkt.Coords.Y != 255 {
		// ByteCoords are offset by 1 (raw byte encoding)
		dropX = pkt.Coords.X - 1
		dropY = pkt.Coords.Y - 1
	}

	// Enforce drop distance
	if dist := p.Cfg.World.DropDistance; dist > 0 && p.DistanceTo(dropX, dropY) > dist {
		p.AddItem(itemID, amount) // rollback
		return nil
	}

	uid := p.World.DropItem(p.MapID, itemID, amount, dropX, dropY, p.ID)

	p.CalculateStats()

	remaining := p.Inventory[itemID]
	return p.Bus.SendPacket(&server.ItemDropServerPacket{
		DroppedItem:     eonet.ThreeItem{Id: itemID, Amount: amount},
		RemainingAmount: remaining,
		ItemIndex:       uid,
		Coords:          eoproto.Coords{X: dropX, Y: dropY},
		Weight:          eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
	})
}

// handleItemJunk destroys items from inventory.
func handleItemJunk(ctx context.Context, p *player.Player, reader *player.EoReader) error {
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
	if isProtectedItem(p, itemID) {
		return nil
	}

	if !p.RemoveItem(itemID, amount) {
		return nil
	}

	remaining := p.Inventory[itemID]
	return p.Bus.SendPacket(&server.ItemJunkServerPacket{
		JunkedItem:      eonet.ThreeItem{Id: itemID, Amount: amount},
		RemainingAmount: remaining,
		Weight:          eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
	})
}

// handleItemUse uses a consumable item from inventory.
func handleItemUse(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.ItemUseClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize item use", "id", p.ID, "err", err)
		return nil
	}

	if p.Inventory[pkt.ItemId] <= 0 {
		return nil
	}

	item := pubdata.GetItem(pkt.ItemId)
	if item == nil {
		return nil
	}

	reply := &server.ItemReplyServerPacket{
		ItemType: item.Type,
	}

	switch item.Type {
	case eopub.Item_Heal:
		hpGain := p.GainHP(item.Hp)
		tpGain := p.GainTP(item.Tp)
		if hpGain == 0 && tpGain == 0 {
			return nil
		}
		reply.ItemTypeData = &server.ItemReplyItemTypeDataHeal{
			HpGain: hpGain,
			Hp:     p.CharHP,
			Tp:     p.CharTP,
		}

	case eopub.Item_HairDye:
		p.CharHairColor = item.Spec1
		reply.ItemTypeData = &server.ItemReplyItemTypeDataHairDye{
			HairColor: item.Spec1,
		}
		if p.World != nil {
			p.World.BroadcastMap(p.MapID, p.ID, &server.AvatarAgreeServerPacket{
				Change: server.AvatarChange{
					PlayerId:   p.ID,
					ChangeType: server.AvatarChange_HairColor,
					Sound:      false,
					ChangeTypeData: &server.ChangeTypeDataHairColor{
						HairColor: item.Spec1,
					},
				},
			})
		}

	case eopub.Item_ExpReward:
		expGain := item.Spec1
		p.CharExp += expGain
		leveledUp := false
		newLevel := formula.LevelForExp(p.CharExp)
		if newLevel > p.CharLevel {
			p.CharLevel = newLevel
			p.StatPoints += p.Cfg.World.StatPointsPerLvl
			p.SkillPoints += p.Cfg.World.SkillPointsPerLvl
			p.CalculateStats()
			leveledUp = true
		}
		levelUp := 0
		if leveledUp {
			levelUp = p.CharLevel
		}
		reply.ItemTypeData = &server.ItemReplyItemTypeDataExpReward{
			Experience:  p.CharExp,
			LevelUp:     levelUp,
			StatPoints:  p.StatPoints,
			SkillPoints: p.SkillPoints,
			MaxHp:       p.CharMaxHP,
			MaxTp:       p.CharMaxTP,
			MaxSp:       p.CharMaxSP,
		}

	case eopub.Item_Teleport:
		// Scroll - warp to destination
		if p.World != nil && item.Spec1 > 0 {
			p.World.WarpPlayer(p.ID, p.MapID, item.Spec1, item.Spec2, item.Spec3)
			p.MapID = item.Spec1
			p.CharX = item.Spec2
			p.CharY = item.Spec3
		}

	default:
		return nil
	}

	// Consume the item (unless it's in infinite use list)
	infinite := false
	for _, id := range p.Cfg.Items.InfiniteUseItems {
		if id == pkt.ItemId {
			infinite = true
			break
		}
	}
	if !infinite {
		p.RemoveItem(pkt.ItemId, 1)
	}

	p.CalculateStats()

	reply.UsedItem = eonet.Item{Id: pkt.ItemId, Amount: p.Inventory[pkt.ItemId]}
	reply.Weight = eonet.Weight{Current: p.Weight, Max: p.MaxWeight}

	return p.Bus.SendPacket(reply)
}

func isProtectedItem(p *player.Player, itemID int) bool {
	for _, protectedID := range p.Cfg.Items.ProtectedItems {
		if protectedID == itemID {
			return true
		}
	}
	return false
}

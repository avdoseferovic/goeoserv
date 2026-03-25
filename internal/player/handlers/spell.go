package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_Request, handleSpellRequest)
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_TargetSelf, handleSpellTargetSelf)
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_TargetOther, handleSpellTargetOther)
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_TargetGroup, handleSpellTargetGroup)
}

// handleSpellRequest begins spell casting (starts the cast bar).
func handleSpellRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.SpellRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell request", "id", p.ID, "err", err)
		return nil
	}

	// Check player has this spell
	hasSpell := false
	for _, s := range p.Spells {
		if s.ID == pkt.SpellId {
			hasSpell = true
			break
		}
	}
	if !hasSpell {
		return nil
	}

	// Check TP cost (spell level * 5)
	spellLevel := max(p.GetSpellLevel(pkt.SpellId), 1)
	tpCost := spellLevel * 5
	if p.CharTP < tpCost {
		return nil
	}

	// Send cast begin confirmation
	return p.Bus.SendPacket(&server.SpellRequestServerPacket{
		PlayerId: p.ID,
		SpellId:  pkt.SpellId,
	})
}

// handleSpellTargetSelf handles self-targeted spells (heals).
func handleSpellTargetSelf(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellTargetSelfClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell target self", "id", p.ID, "err", err)
		return nil
	}

	// Look up spell level and consume TP
	spellLevel := max(p.GetSpellLevel(pkt.SpellId), 1)
	tpCost := spellLevel * 5
	if p.CharTP < tpCost {
		return nil
	}
	p.CharTP -= tpCost

	// Heal based on spell level (10 HP per level)
	healAmount := spellLevel * 10
	p.CharHP += healAmount
	if p.CharHP > p.CharMaxHP {
		p.CharHP = p.CharMaxHP
	}

	hpPct := 100
	if p.CharMaxHP > 0 {
		hpPct = p.CharHP * 100 / p.CharMaxHP
	}

	hp := p.CharHP
	tp := p.CharTP
	p.World.BroadcastMap(p.MapID, -1, &server.SpellTargetSelfServerPacket{
		PlayerId:     p.ID,
		SpellId:      pkt.SpellId,
		SpellHealHp:  healAmount,
		HpPercentage: hpPct,
		Hp:           &hp,
		Tp:           &tp,
	})

	return nil
}

// handleSpellTargetOther handles targeted spells (damage or heal on another player/NPC).
func handleSpellTargetOther(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellTargetOtherClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell target other", "id", p.ID, "err", err)
		return nil
	}

	// Look up spell level and consume TP
	spellLevel := max(p.GetSpellLevel(pkt.SpellId), 1)
	tpCost := spellLevel * 5
	if p.CharTP < tpCost {
		return nil
	}
	p.CharTP -= tpCost

	// Apply spell effect (heal 10 HP per level on target)
	spellHealHp := spellLevel * 10

	// Broadcast spell animation
	p.World.BroadcastMap(p.MapID, -1, &server.SpellTargetOtherServerPacket{
		VictimId:        pkt.VictimId,
		CasterId:        p.ID,
		CasterDirection: eoproto.Direction(p.CharDirection),
		SpellId:         pkt.SpellId,
		SpellHealHp:     spellHealHp,
		HpPercentage:    100,
	})

	return nil
}

// handleSpellTargetGroup handles group heal spells.
func handleSpellTargetGroup(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellTargetGroupClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell target group", "id", p.ID, "err", err)
		return nil
	}

	// Look up spell level and consume TP
	spellLevel := max(p.GetSpellLevel(pkt.SpellId), 1)
	tpCost := spellLevel * 5
	if p.CharTP < tpCost {
		return nil
	}
	p.CharTP -= tpCost

	// Group heal: 8 HP per level
	spellHealHp := spellLevel * 8

	p.World.BroadcastMap(p.MapID, -1, &server.SpellTargetGroupServerPacket{
		SpellId:     pkt.SpellId,
		CasterId:    p.ID,
		CasterTp:    p.CharTP,
		SpellHealHp: spellHealHp,
	})

	return nil
}

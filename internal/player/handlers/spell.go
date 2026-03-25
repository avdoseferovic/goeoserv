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
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_Request, handleSpellRequest)
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_TargetSelf, handleSpellTargetSelf)
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_TargetOther, handleSpellTargetOther)
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_TargetGroup, handleSpellTargetGroup)
}

// handleSpellRequest begins spell casting (starts the cast bar).
func handleSpellRequest(p *player.Player, reader *player.EoReader) error {
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

	// TODO: Check TP cost, cooldown

	// Send cast begin confirmation
	return p.Bus.SendPacket(&server.SpellRequestServerPacket{
		PlayerId: p.ID,
		SpellId:  pkt.SpellId,
	})
}

// handleSpellTargetSelf handles self-targeted spells (heals).
func handleSpellTargetSelf(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellTargetSelfClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell target self", "id", p.ID, "err", err)
		return nil
	}

	// TODO: Look up spell effect from ESF, consume TP
	// Simplified: heal 20 HP
	healAmount := 20

	// Broadcast spell animation + heal to map
	hp := healAmount // simplified
	tp := 0
	p.World.BroadcastMap(p.MapID, -1, &server.SpellTargetSelfServerPacket{
		PlayerId:     p.ID,
		SpellId:      pkt.SpellId,
		SpellHealHp:  healAmount,
		HpPercentage: 100,
		Hp:           &hp,
		Tp:           &tp,
	})

	return nil
}

// handleSpellTargetOther handles targeted spells (damage or heal on another player/NPC).
func handleSpellTargetOther(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellTargetOtherClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell target other", "id", p.ID, "err", err)
		return nil
	}

	// TODO: Determine if target is NPC or player, apply damage/heal, consume TP

	// Broadcast spell animation
	p.World.BroadcastMap(p.MapID, -1, &server.SpellTargetOtherServerPacket{
		VictimId:        pkt.VictimId,
		CasterId:        p.ID,
		CasterDirection: eoproto.Direction(p.CharDirection),
		SpellId:         pkt.SpellId,
		SpellHealHp:     0,
		HpPercentage:    100,
	})

	return nil
}

// handleSpellTargetGroup handles group heal spells.
func handleSpellTargetGroup(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellTargetGroupClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell target group", "id", p.ID, "err", err)
		return nil
	}

	// TODO: Apply group heal to party members, consume TP

	p.World.BroadcastMap(p.MapID, -1, &server.SpellTargetGroupServerPacket{
		SpellId:     pkt.SpellId,
		CasterId:    p.ID,
		CasterTp:    0,
		SpellHealHp: 0,
	})

	return nil
}

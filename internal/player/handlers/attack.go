package handlers

import (
	"log/slog"
	"math/rand/v2"

	"github.com/avdo/goeoserv/internal/formula"
	"github.com/avdo/goeoserv/internal/player"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Attack, eonet.PacketAction_Use, handleAttackUse)
}

func handleAttackUse(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.AttackUseClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize attack", "id", p.ID, "err", err)
		return nil
	}

	p.CharDirection = int(pkt.Direction)

	// Broadcast attack animation to other players
	p.World.BroadcastMap(p.MapID, p.ID, &server.AttackPlayerServerPacket{
		PlayerId:  p.ID,
		Direction: pkt.Direction,
	})

	// Check if there's an NPC in front of the player
	targetX, targetY := p.CharX, p.CharY
	switch pkt.Direction {
	case eoproto.Direction_Down:
		targetY++
	case eoproto.Direction_Left:
		targetX--
	case eoproto.Direction_Up:
		targetY--
	case eoproto.Direction_Right:
		targetX++
	}

	npcIndex := p.World.GetNpcAt(p.MapID, targetX, targetY)
	if npcIndex < 0 {
		return nil
	}

	// Calculate damage (simplified formula)
	damage := formula.BaseDamage(1, 1, 5) + rand.IntN(3)

	actualDmg, killed, hpPct := p.World.DamageNpc(p.MapID, npcIndex, p.ID, damage)
	if actualDmg == 0 {
		return nil
	}

	if killed {
		// NPC killed — send kill notification
		exp := 10 // simplified exp gain
		p.World.BroadcastMap(p.MapID, -1, &server.NpcAcceptServerPacket{
			NpcKilledData: server.NpcKilledData{
				KillerId:        p.ID,
				KillerDirection: pkt.Direction,
				NpcIndex:        npcIndex,
				Damage:          actualDmg,
			},
			Experience: &exp,
		})

		slog.Info("npc killed", "player", p.ID, "npc_index", npcIndex, "exp", exp)
	} else {
		// NPC damaged — send damage notification
		return p.Bus.SendPacket(&server.NpcReplyServerPacket{
			PlayerId:        p.ID,
			PlayerDirection: pkt.Direction,
			NpcIndex:        npcIndex,
			Damage:          actualDmg,
			HpPercentage:    hpPct,
		})
	}

	return nil
}

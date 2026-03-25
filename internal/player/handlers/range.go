package handlers

import (
	"context"
	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	// NpcRange_Request — client periodically asks about nearby NPCs
	player.Register(eonet.PacketFamily_NpcRange, eonet.PacketAction_Request, handleNpcRangeRequest)
	// PlayerRange_Request — client asks about nearby players
	player.Register(eonet.PacketFamily_PlayerRange, eonet.PacketAction_Request, handlePlayerRangeRequest)
	// Range_Request — client asks about nearby everything
	player.Register(eonet.PacketFamily_Range, eonet.PacketAction_Request, handleRangeRequest)
}

func handleNpcRangeRequest(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	raw := p.World.GetNearbyInfo(p.MapID)
	if ni, ok := raw.(*server.NearbyInfo); ok && ni != nil {
		return p.Bus.SendPacket(&server.RangeReplyServerPacket{
			Nearby: server.NearbyInfo{
				Npcs: ni.Npcs,
			},
		})
	}
	return nil
}

func handlePlayerRangeRequest(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	raw := p.World.GetNearbyInfo(p.MapID)
	if ni, ok := raw.(*server.NearbyInfo); ok && ni != nil {
		return p.Bus.SendPacket(&server.RangeReplyServerPacket{
			Nearby: server.NearbyInfo{
				Characters: ni.Characters,
			},
		})
	}
	return nil
}

func handleRangeRequest(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	raw := p.World.GetNearbyInfo(p.MapID)
	if ni, ok := raw.(*server.NearbyInfo); ok && ni != nil {
		return p.Bus.SendPacket(&server.RangeReplyServerPacket{
			Nearby: *ni,
		})
	}
	return nil
}

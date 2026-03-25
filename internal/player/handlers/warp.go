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
	player.Register(eonet.PacketFamily_Warp, eonet.PacketAction_Accept, handleWarpAccept)
	player.Register(eonet.PacketFamily_Warp, eonet.PacketAction_Take, handleWarpNoop)
}

func handleWarpAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.WarpAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize warp accept", "id", p.ID, "err", err)
		return nil
	}

	if p.World == nil {
		return nil
	}

	var toMapID, toX, toY int
	var ok bool
	if p.PendingWarp != nil {
		toMapID = p.PendingWarp.MapID
		toX = p.PendingWarp.X
		toY = p.PendingWarp.Y
		p.PendingWarp = nil
		ok = true
	} else {
		toMapID, toX, toY, ok = p.World.GetPendingWarp(p.MapID, p.ID)
	}
	if !ok {
		return nil
	}

	raw := p.World.WarpPlayer(p.ID, p.MapID, toMapID, toX, toY)
	p.MapID = toMapID
	p.CharX = toX
	p.CharY = toY

	var nearby server.NearbyInfo
	if ni, ok := raw.(*server.NearbyInfo); ok && ni != nil {
		nearby = *ni
	}

	return p.Bus.SendPacket(&server.WarpAgreeServerPacket{
		WarpType: server.Warp_Local,
		WarpTypeData: &server.WarpAgreeWarpTypeDataMapSwitch{
			MapId: toMapID,
		},
		Nearby: nearby,
	})
}

func handleWarpNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }

package handlers

import (
	"context"
	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Refresh, eonet.PacketAction_Request, handleRefreshRequest)
}

func handleRefreshRequest(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	nearbyRaw := p.World.GetNearbyInfo(p.MapID)
	if nearbyRaw == nil {
		return nil
	}

	nearby, ok := nearbyRaw.(*server.NearbyInfo)
	if !ok {
		return nil
	}

	return p.Bus.SendPacket(&server.RefreshReplyServerPacket{
		Nearby: *nearby,
	})
}

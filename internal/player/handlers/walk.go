package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
)

func init() {
	player.Register(eonet.PacketFamily_Walk, eonet.PacketAction_Player, handleWalkPlayer)
}

func handleWalkPlayer(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		slog.Debug("walk rejected: not in game", "id", p.ID, "state", p.State, "hasWorld", p.World != nil)
		return nil
	}

	var pkt client.WalkPlayerClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize walk", "id", p.ID, "err", err)
		return nil
	}

	slog.Debug("walk", "id", p.ID, "map", p.MapID, "dir", pkt.WalkAction.Direction,
		"x", pkt.WalkAction.Coords.X, "y", pkt.WalkAction.Coords.Y)

	coords := [2]int{pkt.WalkAction.Coords.X, pkt.WalkAction.Coords.Y}
	p.World.Walk(p.MapID, p.ID, int(pkt.WalkAction.Direction), coords)

	p.CharX = pkt.WalkAction.Coords.X
	p.CharY = pkt.WalkAction.Coords.Y
	p.CharDirection = int(pkt.WalkAction.Direction)

	return nil
}

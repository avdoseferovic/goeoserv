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
	player.Register(eonet.PacketFamily_Door, eonet.PacketAction_Open, handleDoorOpen)
}

func handleDoorOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.DoorOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize door open", "id", p.ID, "err", err)
		return nil
	}

	if !p.World.OpenDoor(p.MapID, p.ID, pkt.Coords.X, pkt.Coords.Y) {
		return nil
	}

	// Broadcast door open to all players on map
	p.World.BroadcastMap(p.MapID, -1, &server.DoorOpenServerPacket{Coords: pkt.Coords})

	return nil
}

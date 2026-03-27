package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
)

func init() {
	player.Register(eonet.PacketFamily_Connection, eonet.PacketAction_Accept, handleConnectionAccept)
	player.Register(eonet.PacketFamily_Connection, eonet.PacketAction_Ping, handleConnectionPing)
}

func handleConnectionAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.ConnectionAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize connection accept", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if pkt.PlayerId != p.ID {
		slog.Warn("invalid player id in connection accept",
			"id", p.ID, "got", pkt.PlayerId)
		p.Close()
		return nil
	}

	if pkt.ClientEncryptionMultiple != p.Bus.ClientEncryptionMultiple ||
		pkt.ServerEncryptionMultiple != p.Bus.ServerEncryptionMultiple {
		slog.Warn("invalid encryption multiples in connection accept", "id", p.ID)
		p.Close()
		return nil
	}

	p.State = player.StateAccepted
	slog.Debug("connection accepted by client", "id", p.ID)
	return nil
}

func handleConnectionPing(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	return nil
}

package handlers

import (
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
)

func init() {
	player.Register(eonet.PacketFamily_Face, eonet.PacketAction_Player, handleFacePlayer)
}

func handleFacePlayer(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.FacePlayerClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize face", "id", p.ID, "err", err)
		return nil
	}

	p.World.Face(p.MapID, p.ID, int(pkt.Direction))
	p.CharDirection = int(pkt.Direction)

	return nil
}

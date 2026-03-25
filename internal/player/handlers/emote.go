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
	player.Register(eonet.PacketFamily_Emote, eonet.PacketAction_Report, handleEmoteReport)
}

func handleEmoteReport(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.EmoteReportClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize emote", "id", p.ID, "err", err)
		return nil
	}

	p.World.BroadcastMap(p.MapID, p.ID, &server.EmotePlayerServerPacket{
		PlayerId: p.ID,
		Emote:    eoproto.Emote(pkt.Emote),
	})
	return nil
}

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
	player.Register(eonet.PacketFamily_Sit, eonet.PacketAction_Request, handleSitRequest)
	player.Register(eonet.PacketFamily_Chair, eonet.PacketAction_Request, handleChairRequest)
}

func handleSitRequest(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SitRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize sit request", "id", p.ID, "err", err)
		return nil
	}

	// Toggle sit/stand
	p.World.BroadcastMap(p.MapID, p.ID, &server.SitPlayerServerPacket{
		PlayerId:  p.ID,
		Coords:    eoproto.Coords{X: p.CharX, Y: p.CharY},
		Direction: eoproto.Direction(p.CharDirection),
	})

	return nil
}

func handleChairRequest(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.ChairRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize chair request", "id", p.ID, "err", err)
		return nil
	}
	_ = pkt

	p.World.BroadcastMap(p.MapID, p.ID, &server.SitPlayerServerPacket{
		PlayerId:  p.ID,
		Coords:    eoproto.Coords{X: p.CharX, Y: p.CharY},
		Direction: eoproto.Direction(p.CharDirection),
	})

	return nil
}

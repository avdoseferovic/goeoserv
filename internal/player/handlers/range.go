package handlers

import (
	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

func init() {
	// NpcRange_Request — client periodically asks about nearby NPCs
	player.Register(eonet.PacketFamily_NpcRange, eonet.PacketAction_Request, handleNpcRangeRequest)
	// PlayerRange_Request — client asks about nearby players
	player.Register(eonet.PacketFamily_PlayerRange, eonet.PacketAction_Request, handlePlayerRangeRequest)
	// Range_Request — client asks about nearby everything
	player.Register(eonet.PacketFamily_Range, eonet.PacketAction_Request, handleRangeRequest)
}

func handleNpcRangeRequest(_ *player.Player, _ *player.EoReader) error {
	// TODO: Send nearby NPC data
	return nil
}

func handlePlayerRangeRequest(_ *player.Player, _ *player.EoReader) error {
	// TODO: Send nearby player data
	return nil
}

func handleRangeRequest(_ *player.Player, _ *player.EoReader) error {
	// TODO: Send nearby everything
	return nil
}

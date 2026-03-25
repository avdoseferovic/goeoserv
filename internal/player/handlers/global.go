package handlers

import (
	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

func init() {
	// Global_Open and Global_Close are no-ops (client toggles global chat)
	player.Register(eonet.PacketFamily_Global, eonet.PacketAction_Open, handleGlobalNoop)
	player.Register(eonet.PacketFamily_Global, eonet.PacketAction_Close, handleGlobalNoop)
}

func handleGlobalNoop(_ *player.Player, _ *player.EoReader) error {
	return nil
}

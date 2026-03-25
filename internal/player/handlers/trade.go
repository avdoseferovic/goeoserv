package handlers

import (
	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

func init() {
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Request, handleTradeRequest)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Accept, handleTradeAccept)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Add, handleTradeAdd)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Remove, handleTradeRemove)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Agree, handleTradeAgree)
	player.Register(eonet.PacketFamily_Trade, eonet.PacketAction_Close, handleTradeClose)
}

// TODO: Implement full trading system with trade sessions between two players.
// For now, register as no-ops to prevent "unhandled packet" warnings.

func handleTradeRequest(_ *player.Player, _ *player.EoReader) error { return nil }
func handleTradeAccept(_ *player.Player, _ *player.EoReader) error  { return nil }
func handleTradeAdd(_ *player.Player, _ *player.EoReader) error     { return nil }
func handleTradeRemove(_ *player.Player, _ *player.EoReader) error  { return nil }
func handleTradeAgree(_ *player.Player, _ *player.EoReader) error   { return nil }
func handleTradeClose(_ *player.Player, _ *player.EoReader) error   { return nil }

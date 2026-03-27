package player

import (
	"github.com/avdo/goeoserv/internal/protocol"
	"github.com/ethanmoffat/eolib-go/v3/data"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

func consumePacketSequence(
	bus *protocol.PacketBus,
	family eonet.PacketFamily,
	action eonet.PacketAction,
	reader *data.EoReader,
	enforceSequence bool,
) error {
	if family == eonet.PacketFamily_Init {
		return bus.ConsumeSequence(family, action, 0, enforceSequence)
	}

	clientSequence := reader.GetChar()
	return bus.ConsumeSequence(family, action, clientSequence, enforceSequence)
}

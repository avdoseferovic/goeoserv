package player

import (
	"fmt"

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
		bus.Sequencer.NextSequence()
		return nil
	}

	clientSequence := reader.GetChar()
	expectedSequence := bus.Sequencer.NextSequence()
	if enforceSequence && clientSequence != expectedSequence {
		return fmt.Errorf("invalid sequence: got=%d expected=%d", clientSequence, expectedSequence)
	}

	if family != eonet.PacketFamily_Connection || action != eonet.PacketAction_Ping || !bus.HasPendingSequence {
		return nil
	}

	bus.Sequencer.Reset(bus.PendingSequenceStart)
	bus.PendingSequenceStart = 0
	bus.HasPendingSequence = false
	return nil
}

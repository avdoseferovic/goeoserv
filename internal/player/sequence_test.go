package player

import (
	"testing"

	"github.com/avdo/goeoserv/internal/protocol"
	"github.com/ethanmoffat/eolib-go/v3/data"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

func TestConsumePacketSequence_PongUsesOldSequenceThenResetsForNextPacket(t *testing.T) {
	t.Parallel()

	bus := protocol.NewPacketBus(nil)
	bus.Sequencer.Reset(52)
	for range 9 {
		bus.Sequencer.NextSequence()
	}
	bus.PendingSequenceStart = 54
	bus.HasPendingSequence = true

	pongReader := newSequenceReader(t, 61, "k")
	if err := consumePacketSequence(bus, eonet.PacketFamily_Connection, eonet.PacketAction_Ping, pongReader, true); err != nil {
		t.Fatalf("consumePacketSequence(pong) error = %v, want nil", err)
	}

	if bus.HasPendingSequence {
		t.Fatal("pong should clear pending sequence reset")
	}

	if got := bus.Sequencer.Start(); got != 54 {
		t.Fatalf("sequencer start after pong = %d, want 54", got)
	}

	nextReader := newSequenceReader(t, 54, "")
	if err := consumePacketSequence(bus, eonet.PacketFamily_Welcome, eonet.PacketAction_Request, nextReader, true); err != nil {
		t.Fatalf("consumePacketSequence(next packet) error = %v, want nil", err)
	}

	if got := bus.Sequencer.NextSequence(); got != 55 {
		t.Fatalf("next expected sequence after reset flow = %d, want 55", got)
	}
}

func TestConsumePacketSequence_PongDoesNotValidateAgainstPendingResetStart(t *testing.T) {
	t.Parallel()

	bus := protocol.NewPacketBus(nil)
	bus.Sequencer.Reset(52)
	for range 9 {
		bus.Sequencer.NextSequence()
	}
	bus.PendingSequenceStart = 54
	bus.HasPendingSequence = true

	pongReader := newSequenceReader(t, 54, "k")
	err := consumePacketSequence(bus, eonet.PacketFamily_Connection, eonet.PacketAction_Ping, pongReader, true)
	if err == nil {
		t.Fatal("consumePacketSequence(pong) error = nil, want invalid old-window sequence error")
	}

	if got := bus.Sequencer.Start(); got != 52 {
		t.Fatalf("sequencer start after rejected pong = %d, want 52", got)
	}

	if !bus.HasPendingSequence {
		t.Fatal("rejected pong should not clear pending sequence reset")
	}
}

func newSequenceReader(t *testing.T, sequence int, payload string) *data.EoReader {
	t.Helper()

	writer := data.NewEoWriter()
	if err := writer.AddChar(sequence); err != nil {
		t.Fatalf("AddChar(%d) error = %v", sequence, err)
	}
	if payload != "" {
		if err := writer.AddString(payload); err != nil {
			t.Fatalf("AddString(%q) error = %v", payload, err)
		}
	}

	return data.NewEoReader(writer.Array())
}

package player

import (
	"sync"
	"testing"
	"time"

	"github.com/avdo/goeoserv/internal/protocol"
	"github.com/ethanmoffat/eolib-go/v3/data"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

func TestConsumePacketSequence_PongUsesPendingStartWithoutResettingCounter(t *testing.T) {
	t.Parallel()

	bus := protocol.NewPacketBus(nil)
	bus.Sequencer.Reset(187)
	for range 9 {
		bus.Sequencer.NextSequence()
	}
	if got := bus.StartPing(time.Now(), time.Second, 238); got != protocol.PingStarted {
		t.Fatalf("StartPing() = %v, want %v", got, protocol.PingStarted)
	}

	pongReader := newSequenceReader(t, 247, "k")
	if err := consumePacketSequence(bus, eonet.PacketFamily_Connection, eonet.PacketAction_Ping, pongReader, true); err != nil {
		t.Fatalf("consumePacketSequence(pong) error = %v, want nil", err)
	}

	if bus.HasPendingSequence() {
		t.Fatal("pong should clear pending sequence reset")
	}

	if got := bus.Sequencer.Start(); got != 238 {
		t.Fatalf("sequencer start after pong = %d, want 238", got)
	}

	nextReader := newSequenceReader(t, 238, "")
	if err := consumePacketSequence(bus, eonet.PacketFamily_Welcome, eonet.PacketAction_Request, nextReader, true); err != nil {
		t.Fatalf("consumePacketSequence(next packet) error = %v, want nil", err)
	}

	if got := bus.Sequencer.NextSequence(); got != 239 {
		t.Fatalf("next expected sequence after pong flow = %d, want 239", got)
	}
}

func TestConsumePacketSequence_PongRejectsOldWindowSequence(t *testing.T) {
	t.Parallel()

	bus := protocol.NewPacketBus(nil)
	bus.Sequencer.Reset(187)
	for range 9 {
		bus.Sequencer.NextSequence()
	}
	if got := bus.StartPing(time.Now(), time.Second, 238); got != protocol.PingStarted {
		t.Fatalf("StartPing() = %v, want %v", got, protocol.PingStarted)
	}

	pongReader := newSequenceReader(t, 196, "k")
	err := consumePacketSequence(bus, eonet.PacketFamily_Connection, eonet.PacketAction_Ping, pongReader, true)
	if err == nil {
		t.Fatal("consumePacketSequence(pong) error = nil, want invalid old-window sequence error")
	}

	if got := bus.Sequencer.Start(); got != 187 {
		t.Fatalf("sequencer start after rejected pong = %d, want 187", got)
	}

	if !bus.HasPendingSequence() {
		t.Fatal("rejected pong should not clear pending sequence reset")
	}
}

func TestConsumePacketSequence_ValidPongAllowsNextPingImmediately(t *testing.T) {
	t.Parallel()

	bus := protocol.NewPacketBus(nil)
	bus.Sequencer.Reset(187)
	for range 9 {
		bus.Sequencer.NextSequence()
	}
	if got := bus.StartPing(time.Now(), time.Second, 238); got != protocol.PingStarted {
		t.Fatalf("StartPing() = %v, want %v", got, protocol.PingStarted)
	}

	pongReader := newSequenceReader(t, 247, "k")
	if err := consumePacketSequence(bus, eonet.PacketFamily_Connection, eonet.PacketAction_Ping, pongReader, true); err != nil {
		t.Fatalf("consumePacketSequence(pong) error = %v, want nil", err)
	}

	if got := bus.StartPing(time.Now(), time.Second, 111); got != protocol.PingStarted {
		t.Fatalf("StartPing() after valid pong = %v, want %v", got, protocol.PingStarted)
	}

	if !bus.HasPendingSequence() {
		t.Fatal("next ping should install a fresh pending sequence reset")
	}
}

func TestConsumePacketSequence_PingStateAccessIsRaceSafe(t *testing.T) {
	t.Parallel()

	bus := protocol.NewPacketBus(nil)
	bus.Sequencer.Reset(100)

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range iterations {
			bus.CompletePong()
			_ = bus.StartPing(time.Now(), time.Second, 120)
		}
	}()

	go func() {
		defer wg.Done()
		for range iterations {
			reader := mustSequenceReader(120)
			_ = consumePacketSequence(bus, eonet.PacketFamily_Connection, eonet.PacketAction_Ping, reader, false)
		}
	}()

	wg.Wait()
	bus.CompletePong()
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

func mustSequenceReader(sequence int) *data.EoReader {
	writer := data.NewEoWriter()
	if err := writer.AddChar(sequence); err != nil {
		panic(err)
	}

	return data.NewEoReader(writer.Array())
}

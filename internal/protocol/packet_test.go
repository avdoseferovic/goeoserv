package protocol

import (
	"testing"
	"time"
)

func TestStartPing_WaitsForFullTimeoutWindow(t *testing.T) {
	t.Parallel()

	bus := NewPacketBus(nil)
	now := time.Unix(100, 0)

	if got := bus.StartPing(now, time.Second, 123); got != PingStarted {
		t.Fatalf("first StartPing() = %v, want %v", got, PingStarted)
	}

	if got := bus.StartPing(now.Add(900*time.Millisecond), time.Second, 234); got != PingAwaitingPong {
		t.Fatalf("second StartPing() before timeout = %v, want %v", got, PingAwaitingPong)
	}

	if !bus.needPong {
		t.Fatal("needPong = false, want true while waiting for pong")
	}

	if got := bus.pendingSequenceStart; got != 123 {
		t.Fatalf("pendingSequenceStart = %d, want original value 123", got)
	}
}

func TestStartPing_TimesOutAfterFullTimeoutWindow(t *testing.T) {
	t.Parallel()

	bus := NewPacketBus(nil)
	now := time.Unix(100, 0)

	if got := bus.StartPing(now, time.Second, 123); got != PingStarted {
		t.Fatalf("first StartPing() = %v, want %v", got, PingStarted)
	}

	if got := bus.StartPing(now.Add(time.Second), time.Second, 234); got != PingTimedOut {
		t.Fatalf("second StartPing() at timeout = %v, want %v", got, PingTimedOut)
	}
}

func TestCompletePong_ClearsPingDeadline(t *testing.T) {
	t.Parallel()

	bus := NewPacketBus(nil)
	now := time.Unix(100, 0)

	if got := bus.StartPing(now, time.Second, 123); got != PingStarted {
		t.Fatalf("first StartPing() = %v, want %v", got, PingStarted)
	}

	bus.CompletePong()

	if bus.needPong {
		t.Fatal("needPong = true, want false after pong")
	}

	if !bus.lastPingAt.IsZero() {
		t.Fatalf("lastPingAt = %v, want zero time after pong", bus.lastPingAt)
	}

	if got := bus.StartPing(now.Add(500*time.Millisecond), time.Second, 234); got != PingStarted {
		t.Fatalf("StartPing() after CompletePong = %v, want %v", got, PingStarted)
	}

	if got := bus.pendingSequenceStart; got != 234 {
		t.Fatalf("pendingSequenceStart = %d, want 234 after re-arm", got)
	}
}

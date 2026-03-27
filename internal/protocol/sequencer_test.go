package protocol

import (
	"testing"

	"github.com/ethanmoffat/eolib-go/v3/data"
)

func TestSequencer_NextSequence(t *testing.T) {
	t.Parallel()
	s := NewSequencer()

	// Counter cycles 0-9, added to start
	for i := range 10 {
		got := s.NextSequence()
		if got != i {
			t.Errorf("iteration %d: NextSequence() = %d, want %d", i, got, i)
		}
	}

	// Should wrap back to start (0) after 10 calls
	got := s.NextSequence()
	if got != 0 {
		t.Errorf("after wrap: NextSequence() = %d, want 0", got)
	}
}

func TestSequencer_SetStart(t *testing.T) {
	t.Parallel()
	s := NewSequencer()

	// Advance counter to 3
	for range 3 {
		s.NextSequence()
	}

	// SetStart doesn't reset counter
	s.SetStart(100)

	// Counter is at 3, start is 100 → next should be 103
	got := s.NextSequence()
	if got != 103 {
		t.Errorf("NextSequence() after SetStart(100) = %d, want 103", got)
	}
}

func TestSequencer_Start(t *testing.T) {
	t.Parallel()
	s := NewSequencer()
	if s.Start() != 0 {
		t.Errorf("initial Start() = %d, want 0", s.Start())
	}
	s.SetStart(42)
	if s.Start() != 42 {
		t.Errorf("Start() after SetStart(42) = %d, want 42", s.Start())
	}
}

func TestSequencer_Reset(t *testing.T) {
	t.Parallel()
	s := NewSequencer()

	for range 3 {
		s.NextSequence()
	}

	s.Reset(100)

	if got := s.Start(); got != 100 {
		t.Fatalf("Start() after Reset(100) = %d, want 100", got)
	}

	if got := s.NextSequence(); got != 100 {
		t.Fatalf("NextSequence() after Reset(100) = %d, want 100", got)
	}
}

func TestGenerateInitSequenceBytes(t *testing.T) {
	t.Parallel()
	for range 1000 {
		seq1, seq2, value := GenerateInitSequenceBytes()

		// Verify encoding: value = seq1*7 + seq2 - 13
		decoded := seq1*7 + seq2 - 13
		if decoded != value {
			t.Fatalf("init encoding broken: seq1=%d, seq2=%d, value=%d, decoded=%d",
				seq1, seq2, value, decoded)
		}

		// Value must fit in EO char range
		if value < 0 || value >= data.CHAR_MAX-9 {
			t.Fatalf("value %d out of range [0, %d)", value, data.CHAR_MAX-9)
		}

		// seq1 and seq2 should be non-negative
		if seq1 < 0 || seq2 < 0 {
			t.Fatalf("negative sequence bytes: seq1=%d, seq2=%d", seq1, seq2)
		}
	}
}

func TestGeneratePingSequenceBytes(t *testing.T) {
	t.Parallel()
	for range 1000 {
		seq1, seq2, value := GeneratePingSequenceBytes()

		// Verify encoding: value = seq1 - seq2
		decoded := seq1 - seq2
		if decoded != value {
			t.Fatalf("ping encoding broken: seq1=%d, seq2=%d, value=%d, decoded=%d",
				seq1, seq2, value, decoded)
		}

		if value < 0 || value >= data.CHAR_MAX-9 {
			t.Fatalf("value %d out of range", value)
		}

		if seq1 < 0 || seq2 < 0 {
			t.Fatalf("negative sequence bytes: seq1=%d, seq2=%d", seq1, seq2)
		}
	}
}

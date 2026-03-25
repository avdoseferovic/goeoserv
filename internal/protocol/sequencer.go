package protocol

import (
	"math/rand/v2"

	"github.com/ethanmoffat/eolib-go/v3/data"
	"github.com/ethanmoffat/eolib-go/v3/utils"
)

// Sequencer tracks packet sequence numbers for the EO protocol.
// Mirrors eolib-go's PacketSequencer behavior.
type Sequencer struct {
	start   int
	counter int
}

func NewSequencer() *Sequencer {
	return &Sequencer{start: 0, counter: 0}
}

// NextSequence returns the next expected sequence value.
// Counter cycles from 0 to 9.
func (s *Sequencer) NextSequence() int {
	result := s.start + s.counter
	s.counter = (s.counter + 1) % 10
	return result
}

// SetStart sets the sequence start value.
// Note: does NOT reset the counter (matches eolib behavior).
func (s *Sequencer) SetStart(start int) {
	s.start = start
}

// Start returns the current sequence start value.
func (s *Sequencer) Start() int {
	return s.start
}

// GenerateInitSequenceBytes generates the seq1/seq2 pair for the init handshake.
// Returns (seq1, seq2, sequenceStart).
// Init encoding: value = seq1*7 + seq2 - 13
func GenerateInitSequenceBytes() (int, int, int) {
	value := rand.IntN(data.CHAR_MAX - 9) // 0 to 243 so value+9 fits in a char
	seq1Max := (value + 13) / 7
	seq1Min := utils.Max(0, (value-(data.CHAR_MAX-1)+13+6)/7)

	diff := seq1Max - seq1Min
	if diff <= 0 {
		diff = 1
	}
	seq1 := rand.IntN(diff) + seq1Min
	seq2 := value - seq1*7 + 13

	return seq1, seq2, value
}

// GeneratePingSequenceBytes generates the seq1/seq2 pair for ping packets.
// Returns (seq1, seq2, sequenceStart).
// Ping encoding: value = seq1 - seq2
func GeneratePingSequenceBytes() (int, int, int) {
	value := rand.IntN(data.CHAR_MAX - 9) // 0 to 243
	seq1 := value + rand.IntN(data.CHAR_MAX-1)
	seq2 := seq1 - value

	return seq1, seq2, value
}

package protocol

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ethanmoffat/eolib-go/v3/data"
	"github.com/ethanmoffat/eolib-go/v3/encrypt"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

// packetBufPool reduces GC pressure by reusing packet send buffers.
var packetBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 256)
		return &b
	},
}

// PacketBus handles reading/writing EO protocol packets over a connection.
type PacketBus struct {
	conn                     *Conn
	stateMu                  sync.Mutex
	Sequencer                *Sequencer
	ServerEncryptionMultiple int
	ClientEncryptionMultiple int
	needPong                 bool
	lastPingAt               time.Time
	pendingSequenceStart     int  // set when ping is sent, applied when pong is received
	hasPendingSequence       bool // true if pendingSequenceStart is set
}

type PingStartResult int

const (
	PingStarted PingStartResult = iota
	PingAwaitingPong
	PingTimedOut
)

func NewPacketBus(conn *Conn) *PacketBus {
	return &PacketBus{
		conn:      conn,
		Sequencer: NewSequencer(),
	}
}

// StartPing marks a ping as in-flight and stores its pending sequence start.
// If a ping is already in flight, it reports whether the caller should keep
// waiting for the outstanding pong or treat the connection as timed out.
func (pb *PacketBus) StartPing(now time.Time, timeout time.Duration, sequenceStart int) PingStartResult {
	pb.stateMu.Lock()
	defer pb.stateMu.Unlock()

	if pb.needPong {
		if timeout > 0 && !pb.lastPingAt.IsZero() && now.Sub(pb.lastPingAt) >= timeout {
			return PingTimedOut
		}

		return PingAwaitingPong
	}

	pb.needPong = true
	pb.lastPingAt = now
	pb.pendingSequenceStart = sequenceStart
	pb.hasPendingSequence = true
	return PingStarted
}

// CompletePong clears the outstanding ping marker.
// Used by tests to model ping-loop state transitions directly.
func (pb *PacketBus) CompletePong() {
	pb.stateMu.Lock()
	pb.needPong = false
	pb.lastPingAt = time.Time{}
	pb.stateMu.Unlock()
}

// HasPendingSequence reports whether a ping-driven sequence reset is waiting
// to be applied. Used by tests.
func (pb *PacketBus) HasPendingSequence() bool {
	pb.stateMu.Lock()
	defer pb.stateMu.Unlock()

	return pb.hasPendingSequence
}

// ConsumeSequence validates and advances packet sequencing under a single lock
// so ping-loop updates and receive-loop reads cannot race.
func (pb *PacketBus) ConsumeSequence(
	family eonet.PacketFamily,
	action eonet.PacketAction,
	clientSequence int,
	enforceSequence bool,
) error {
	pb.stateMu.Lock()
	defer pb.stateMu.Unlock()

	if family == eonet.PacketFamily_Init {
		pb.Sequencer.NextSequence()
		return nil
	}

	if family == eonet.PacketFamily_Connection && action == eonet.PacketAction_Ping && pb.hasPendingSequence {
		expectedSequence := pb.Sequencer.PeekNextSequenceWithStart(pb.pendingSequenceStart)
		if enforceSequence && clientSequence != expectedSequence {
			return fmt.Errorf("invalid sequence: got=%d expected=%d", clientSequence, expectedSequence)
		}

		pb.needPong = false
		pb.lastPingAt = time.Time{}
		pb.Sequencer.SetStart(pb.pendingSequenceStart)
		pb.Sequencer.NextSequence()
		pb.pendingSequenceStart = 0
		pb.hasPendingSequence = false
		return nil
	}

	expectedSequence := pb.Sequencer.NextSequence()
	if enforceSequence && clientSequence != expectedSequence {
		return fmt.Errorf("invalid sequence: got=%d expected=%d", clientSequence, expectedSequence)
	}

	return nil
}

// CurrentSequenceStart returns the active sequence start under lock.
func (pb *PacketBus) CurrentSequenceStart() int {
	pb.stateMu.Lock()
	defer pb.stateMu.Unlock()

	return pb.Sequencer.Start()
}

// Send writes a raw packet with the given family/action and payload bytes.
func (pb *PacketBus) Send(action eonet.PacketAction, family eonet.PacketFamily, payload []byte) error {
	packetSize := 2 + len(payload)
	lengthBytes := data.EncodeNumber(packetSize)

	bufp := packetBufPool.Get().(*[]byte)
	buf := (*bufp)[:0]

	buf = append(buf, lengthBytes[0], lengthBytes[1])
	buf = append(buf, byte(action), byte(family))
	buf = append(buf, payload...)

	if pb.ServerEncryptionMultiple != 0 {
		encrypted := EncryptPacket(buf[2:], pb.ServerEncryptionMultiple)
		copy(buf[2:], encrypted)
	}

	slog.Debug("packet send", "action", int(action), "family", int(family), "len", len(buf), "raw", fmt.Sprintf("%x", buf))
	err := pb.conn.WritePacket(buf)

	*bufp = buf
	packetBufPool.Put(bufp)

	return err
}

// SendPacket serializes an eolib Packet and sends it.
func (pb *PacketBus) SendPacket(pkt eonet.Packet) error {
	writer := data.NewEoWriter()
	if err := pkt.Serialize(writer); err != nil {
		return fmt.Errorf("serializing packet: %w", err)
	}
	return pb.Send(pkt.Action(), pkt.Family(), writer.Array())
}

// Recv reads the next packet from the connection.
// Returns the action, family, and a reader over the payload data.
func (pb *PacketBus) Recv() (eonet.PacketAction, eonet.PacketFamily, *data.EoReader, error) {
	packetBuf, err := pb.conn.ReadPacket()
	if err != nil {
		return 0, 0, nil, err
	}

	if len(packetBuf) < 2 {
		return 0, 0, nil, fmt.Errorf("packet too short: %d bytes", len(packetBuf))
	}

	// Decrypt if encryption is active
	if pb.ClientEncryptionMultiple != 0 {
		decrypted := DecryptPacket(packetBuf, pb.ClientEncryptionMultiple)
		copy(packetBuf, decrypted)
	}

	slog.Debug("packet recv", "len", len(packetBuf), "raw", fmt.Sprintf("%x", packetBuf))

	action := eonet.PacketAction(packetBuf[0])
	family := eonet.PacketFamily(packetBuf[1])

	reader := data.NewEoReader(packetBuf[2:])
	return action, family, reader, nil
}

// validForEncryption checks whether a packet should be encrypted/decrypted.
// Init packets (action=0xFF, family=0xFF) are never encrypted.
func validForEncryption(buf []byte) bool {
	return len(buf) > 2 && (buf[0] != 0xFF || buf[1] != 0xFF)
}

// EncryptPacket applies EO packet encryption.
func EncryptPacket(buf []byte, multiple int) []byte {
	if !validForEncryption(buf) {
		return buf
	}
	result, _ := encrypt.SwapMultiples(buf, multiple)
	result = encrypt.Interleave(result)
	result = encrypt.FlipMsb(result)
	return result
}

// DecryptPacket applies EO packet decryption (reverse of encrypt).
func DecryptPacket(buf []byte, multiple int) []byte {
	if !validForEncryption(buf) {
		return buf
	}
	result := encrypt.FlipMsb(buf)
	result = encrypt.Deinterleave(result)
	result, _ = encrypt.SwapMultiples(result, multiple)
	return result
}

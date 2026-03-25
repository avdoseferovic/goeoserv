package protocol

import (
	"fmt"
	"log/slog"
	"sync"

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
	Sequencer                *Sequencer
	ServerEncryptionMultiple int
	ClientEncryptionMultiple int
	NeedPong                 bool
	PendingSequenceStart     int  // set when ping is sent, applied when pong is received
	HasPendingSequence       bool // true if PendingSequenceStart is set
}

func NewPacketBus(conn *Conn) *PacketBus {
	return &PacketBus{
		conn:      conn,
		Sequencer: NewSequencer(),
	}
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
	return len(buf) > 2 && !(buf[0] == 0xFF && buf[1] == 0xFF)
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

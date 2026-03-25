package protocol

import (
	"fmt"

	"github.com/ethanmoffat/eolib-go/v3/data"
	"github.com/ethanmoffat/eolib-go/v3/encrypt"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

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

	buf := make([]byte, 0, 2+packetSize)
	buf = append(buf, lengthBytes[0], lengthBytes[1])
	buf = append(buf, byte(action), byte(family))
	buf = append(buf, payload...)

	if pb.ServerEncryptionMultiple != 0 {
		encrypted := EncryptPacket(buf[2:], pb.ServerEncryptionMultiple)
		copy(buf[2:], encrypted)
	}

	return pb.conn.WritePacket(buf)
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

	action := eonet.PacketAction(packetBuf[0])
	family := eonet.PacketFamily(packetBuf[1])

	reader := data.NewEoReader(packetBuf[2:])
	return action, family, reader, nil
}

// EncryptPacket applies EO packet encryption.
func EncryptPacket(buf []byte, multiple int) []byte {
	result, _ := encrypt.SwapMultiples(buf, multiple)
	result = encrypt.Interleave(result)
	result = encrypt.FlipMsb(result)
	return result
}

// DecryptPacket applies EO packet decryption (reverse of encrypt).
func DecryptPacket(buf []byte, multiple int) []byte {
	result := encrypt.FlipMsb(buf)
	result = encrypt.Deinterleave(result)
	result, _ = encrypt.SwapMultiples(result, multiple)
	return result
}

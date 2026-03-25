package protocol

import (
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/ethanmoffat/eolib-go/v3/data"
	"github.com/gorilla/websocket"
)

// ConnType distinguishes TCP from WebSocket connections.
type ConnType int

const (
	ConnTCP ConnType = iota
	ConnWebSocket
)

// Conn wraps either a raw TCP connection or a WebSocket connection,
// providing a unified interface for reading and writing EO packets.
type Conn struct {
	connType ConnType
	tcp      net.Conn
	ws       *websocket.Conn
	wsMu     sync.Mutex // websocket writes must be serialized
}

// NewTCPConn wraps a raw TCP connection.
func NewTCPConn(conn net.Conn) *Conn {
	return &Conn{connType: ConnTCP, tcp: conn}
}

// NewWebSocketConn wraps a gorilla/websocket connection.
func NewWebSocketConn(ws *websocket.Conn) *Conn {
	return &Conn{connType: ConnWebSocket, ws: ws}
}

// ReadPacket reads a single EO packet.
// For TCP: reads 2-byte length prefix, then the packet body.
// For WebSocket: reads one binary message, strips the 2-byte length prefix.
// Returns the raw packet bytes (action + family + payload), without the length prefix.
func (c *Conn) ReadPacket() ([]byte, error) {
	switch c.connType {
	case ConnTCP:
		// Read 2-byte length prefix
		lengthBuf := make([]byte, 2)
		if _, err := io.ReadFull(c.tcp, lengthBuf); err != nil {
			return nil, err
		}

		// Detect TLS ClientHello (0x16 0x03) — client is trying SSL on a plain TCP port
		if lengthBuf[0] == 0x16 && lengthBuf[1] == 0x03 {
			return nil, fmt.Errorf("TLS handshake detected on plain TCP port (client may need WebSocket port)")
		}

		packetLength := data.DecodeNumber(lengthBuf)
		if packetLength < 2 || packetLength > 65535 {
			return nil, fmt.Errorf("invalid packet length: %d", packetLength)
		}
		packetBuf := make([]byte, packetLength)
		if _, err := io.ReadFull(c.tcp, packetBuf); err != nil {
			return nil, err
		}
		return packetBuf, nil

	case ConnWebSocket:
		msgType, msg, err := c.ws.ReadMessage()
		if err != nil {
			return nil, err
		}
		if msgType != websocket.BinaryMessage {
			return nil, fmt.Errorf("expected binary message, got type %d", msgType)
		}
		// WS messages include the 2-byte length prefix — skip it
		if len(msg) < 4 { // 2 length + at least 2 (action+family)
			return nil, fmt.Errorf("websocket message too short: %d bytes", len(msg))
		}
		return msg[2:], nil

	default:
		return nil, fmt.Errorf("unknown connection type")
	}
}

// WritePacket writes a fully assembled packet (with 2-byte length prefix already prepended).
// For TCP: writes raw bytes.
// For WebSocket: writes as a binary message.
func (c *Conn) WritePacket(buf []byte) error {
	switch c.connType {
	case ConnTCP:
		_, err := c.tcp.Write(buf)
		return err

	case ConnWebSocket:
		c.wsMu.Lock()
		defer c.wsMu.Unlock()
		return c.ws.WriteMessage(websocket.BinaryMessage, buf)

	default:
		return fmt.Errorf("unknown connection type")
	}
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	switch c.connType {
	case ConnTCP:
		return c.tcp.Close()
	case ConnWebSocket:
		return c.ws.Close()
	default:
		return nil
	}
}

// RemoteAddr returns the remote address string.
func (c *Conn) RemoteAddr() string {
	switch c.connType {
	case ConnTCP:
		return c.tcp.RemoteAddr().String()
	case ConnWebSocket:
		return c.ws.RemoteAddr().String()
	default:
		return "unknown"
	}
}

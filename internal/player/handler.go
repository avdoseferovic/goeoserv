package player

import (
	"context"

	"github.com/avdo/goeoserv/internal/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

// EoReader is a type alias for convenience.
type EoReader = protocol.EoReader

// HandlerFunc is the signature for all packet handlers.
// ctx is derived from the player's connection lifetime — use it for DB queries.
type HandlerFunc func(ctx context.Context, p *Player, reader *EoReader) error

// handlers is the global handler registry keyed by [family][action].
var handlers = make(map[eonet.PacketFamily]map[eonet.PacketAction]HandlerFunc)

// Register adds a handler for the given packet family and action.
func Register(family eonet.PacketFamily, action eonet.PacketAction, fn HandlerFunc) {
	if handlers[family] == nil {
		handlers[family] = make(map[eonet.PacketAction]HandlerFunc)
	}
	handlers[family][action] = fn
}

// GetHandler looks up a handler for the given packet family and action.
func GetHandler(family eonet.PacketFamily, action eonet.PacketAction) HandlerFunc {
	if familyHandlers, ok := handlers[family]; ok {
		return familyHandlers[action]
	}
	return nil
}

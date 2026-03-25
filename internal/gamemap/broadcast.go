package gamemap

import (
	"log/slog"

	"github.com/avdo/goeoserv/internal/protocol"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// Broadcast sends a packet to all players on this map except excludeID.
// Collects player bus refs under lock, then sends outside the lock
// to avoid blocking map operations during network I/O.
func (m *GameMap) Broadcast(excludeID int, pkt eonet.Packet) {
	m.mu.RLock()
	buses := make([]*protocol.PacketBus, 0, len(m.players))
	for pid, ch := range m.players {
		if pid != excludeID {
			buses = append(buses, ch.Bus)
		}
	}
	m.mu.RUnlock()

	for _, bus := range buses {
		if err := bus.SendPacket(pkt); err != nil {
			slog.Debug("broadcast send error", "err", err)
		}
	}
}

// BroadcastToAdmins sends a packet to players with admin level >= minAdmin.
func (m *GameMap) BroadcastToAdmins(excludeID int, minAdmin int, pkt eonet.Packet) {
	m.mu.RLock()
	buses := make([]*protocol.PacketBus, 0, len(m.players))
	for pid, ch := range m.players {
		if pid != excludeID && ch.Admin >= minAdmin {
			buses = append(buses, ch.Bus)
		}
	}
	m.mu.RUnlock()

	for _, bus := range buses {
		_ = bus.SendPacket(pkt)
	}
}

// BroadcastToGuild sends a packet to players in the specified guild.
func (m *GameMap) BroadcastToGuild(excludeID int, guildTag string, pkt eonet.Packet) {
	m.mu.RLock()
	buses := make([]*protocol.PacketBus, 0, len(m.players))
	for pid, ch := range m.players {
		if pid != excludeID && ch.GuildTag == guildTag {
			buses = append(buses, ch.Bus)
		}
	}
	m.mu.RUnlock()

	for _, bus := range buses {
		_ = bus.SendPacket(pkt)
	}
}

func (m *GameMap) broadcastNpcPositions() {
	m.mu.RLock()
	if len(m.players) == 0 {
		m.mu.RUnlock()
		return
	}

	positions := make([]server.NpcUpdatePosition, 0, len(m.npcs))
	for _, npc := range m.npcs {
		if npc.Alive {
			positions = append(positions, server.NpcUpdatePosition{
				NpcIndex:  npc.Index,
				Coords:    eoproto.Coords{X: npc.X, Y: npc.Y},
				Direction: eoproto.Direction(npc.Direction),
			})
		}
	}

	if len(positions) == 0 {
		m.mu.RUnlock()
		return
	}

	buses := make([]*protocol.PacketBus, 0, len(m.players))
	for _, ch := range m.players {
		buses = append(buses, ch.Bus)
	}
	m.mu.RUnlock()

	pkt := &server.NpcPlayerServerPacket{Positions: positions}
	for _, bus := range buses {
		_ = bus.SendPacket(pkt)
	}
}

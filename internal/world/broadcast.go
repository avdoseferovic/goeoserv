package world

import (
	"strings"

	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

func (w *World) Broadcast(mapID, excludePlayerID int, pkt eonet.Packet) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.Broadcast(excludePlayerID, pkt)
}

func (w *World) BroadcastMap(mapID, excludePlayerID int, pkt any) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	if p, ok := pkt.(eonet.Packet); ok {
		m.Broadcast(excludePlayerID, p)
	}
}

func (w *World) BroadcastGlobal(excludePlayerID int, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok {
		return
	}
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	for _, m := range w.maps {
		m.Broadcast(excludePlayerID, p)
	}
}

func (w *World) BroadcastToAdmins(excludePlayerID int, minAdmin int, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok {
		return
	}
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	for _, m := range w.maps {
		m.BroadcastToAdmins(excludePlayerID, minAdmin, p)
	}
}

func (w *World) BroadcastToGuild(excludePlayerID int, guildTag string, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok || guildTag == "" {
		return
	}
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	for _, m := range w.maps {
		m.BroadcastToGuild(excludePlayerID, guildTag, p)
	}
}

func (w *World) BroadcastToParty(playerID int, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok {
		return
	}
	party := GetParty(playerID)
	if party != nil {
		party.BroadcastToParty(p)
	}
}

func (w *World) SendToPlayer(playerID int, pkt any) {
	p, ok := pkt.(eonet.Packet)
	if !ok {
		return
	}
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e != nil {
		_ = e.bus.SendPacket(p)
	}
}

func (w *World) SendTo(playerID int, pkt eonet.Packet) {
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e != nil {
		_ = e.bus.SendPacket(pkt)
	}
}

func (w *World) FindPlayerByName(name string) (int, bool) {
	w.playerMu.RLock()
	id, ok := w.nameIndex[strings.ToLower(name)]
	w.playerMu.RUnlock()
	return id, ok
}

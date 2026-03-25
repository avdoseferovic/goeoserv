package gamemap

import (
	"github.com/avdo/goeoserv/internal/protocol"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// Tick processes one game tick.
func (m *GameMap) Tick() {
	m.mu.Lock()
	m.tickCount++
	tc := m.tickCount
	m.mu.Unlock()

	m.TickNPCs(m.cfg.NPCs.ActRate)

	// HP/TP recovery
	if m.cfg.World.RecoverRate > 0 && tc%m.cfg.World.RecoverRate == 0 {
		m.tickRecovery()
	}

	// Timed spikes
	if m.cfg.World.SpikeRate > 0 && tc%m.cfg.World.SpikeRate == 0 {
		m.tickSpikes()
	}

	// HP/TP drain maps
	if m.cfg.World.DrainRate > 0 && tc%m.cfg.World.DrainRate == 0 {
		m.tickDrain()
	}

	// Door auto-close
	if m.cfg.Map.DoorCloseRate > 0 && tc%m.cfg.Map.DoorCloseRate == 0 {
		m.tickDoorClose()
	}
}

func (m *GameMap) tickRecovery() {
	type recoveryUpdate struct {
		bus    *protocol.PacketBus
		hp, tp int
	}

	m.mu.Lock()
	updates := make([]recoveryUpdate, 0, len(m.players))
	for _, ch := range m.players {
		changed := false
		if ch.HP < ch.MaxHP {
			ch.HP += ch.MaxHP / 20 // recover 5% of max HP
			if ch.HP > ch.MaxHP {
				ch.HP = ch.MaxHP
			}
			changed = true
		}
		if ch.TP < ch.MaxTP {
			ch.TP += ch.MaxTP / 20
			if ch.TP > ch.MaxTP {
				ch.TP = ch.MaxTP
			}
			changed = true
		}
		if changed {
			updates = append(updates, recoveryUpdate{bus: ch.Bus, hp: ch.HP, tp: ch.TP})
		}
	}
	m.mu.Unlock()

	for _, u := range updates {
		_ = u.bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: u.hp, Tp: u.tp})
	}
}

// tickSpikes applies damage to players standing on spike tiles.
func (m *GameMap) tickSpikes() {
	type spikeUpdate struct {
		bus    *protocol.PacketBus
		hp, tp int
	}

	m.mu.Lock()
	if len(m.players) == 0 {
		m.mu.Unlock()
		return
	}

	spikeDmgPct := m.cfg.World.SpikeDamage
	if spikeDmgPct <= 0 {
		spikeDmgPct = 0.1 // default 10%
	}

	var updates []spikeUpdate
	for _, ch := range m.players {
		spec, ok := m.tiles[[2]int{ch.X, ch.Y}]
		if !ok {
			continue
		}
		if spec != eomap.MapTileSpec_TimedSpikes && spec != eomap.MapTileSpec_Spikes {
			continue
		}
		damage := int(float64(ch.MaxHP) * spikeDmgPct)
		if damage < 1 {
			damage = 1
		}
		if damage >= ch.HP {
			damage = ch.HP - 1 // spikes don't kill
		}
		if damage <= 0 {
			continue
		}
		ch.HP -= damage
		updates = append(updates, spikeUpdate{bus: ch.Bus, hp: ch.HP, tp: ch.TP})
	}
	m.mu.Unlock()

	for _, u := range updates {
		_ = u.bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: u.hp, Tp: u.tp})
	}
}

// tickDrain applies HP or TP drain on drain-effect maps.
func (m *GameMap) tickDrain() {
	type drainUpdate struct {
		bus    *protocol.PacketBus
		hp, tp int
	}

	m.mu.Lock()
	if len(m.players) == 0 {
		m.mu.Unlock()
		return
	}

	timedEffect := m.emf.TimedEffect
	var updates []drainUpdate

	if timedEffect == eomap.MapTimedEffect_HpDrain {
		dmgPct := m.cfg.World.DrainHPDamage
		if dmgPct <= 0 {
			m.mu.Unlock()
			return
		}
		for _, ch := range m.players {
			damage := int(float64(ch.MaxHP) * dmgPct)
			if damage < 1 {
				damage = 1
			}
			if damage >= ch.HP {
				damage = ch.HP - 1
			}
			if damage <= 0 {
				continue
			}
			ch.HP -= damage
			updates = append(updates, drainUpdate{bus: ch.Bus, hp: ch.HP, tp: ch.TP})
		}
	}

	if timedEffect == eomap.MapTimedEffect_TpDrain {
		dmgPct := m.cfg.World.DrainTPDamage
		if dmgPct <= 0 {
			m.mu.Unlock()
			return
		}
		for _, ch := range m.players {
			if ch.TP <= 0 {
				continue
			}
			damage := int(float64(ch.MaxTP) * dmgPct)
			if damage < 1 {
				damage = 1
			}
			if damage >= ch.TP {
				damage = ch.TP - 1
			}
			if damage <= 0 {
				continue
			}
			ch.TP -= damage
			updates = append(updates, drainUpdate{bus: ch.Bus, hp: ch.HP, tp: ch.TP})
		}
	}
	m.mu.Unlock()

	for _, u := range updates {
		_ = u.bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: u.hp, Tp: u.tp})
	}
}

// tickDoorClose auto-closes any doors (placeholder — door state tracking not yet implemented).
func (m *GameMap) tickDoorClose() {
	// Door state tracking would require storing which doors are currently open
	// and broadcasting DoorCloseServerPacket when they expire.
	// For now this is a no-op until door state is tracked.
}

package gamemap

import (
	"fmt"
	"math/rand/v2"

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

	// Drop protection decay
	m.tickDropProtection()

	// Quake effects
	m.tickQuake()

	// Warp suck
	if m.cfg.World.WarpSuckRate > 0 {
		m.tickWarpSuck()
	}

	// Evacuate countdown
	m.tickEvacuate()
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

// tickDropProtection decrements protection timers on ground items.
func (m *GameMap) tickDropProtection() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.groundItems {
		if item.ProtectedTicks > 0 {
			item.ProtectedTicks--
		}
	}
}

// tickQuake triggers earthquake effects on maps with quake timed effects.
func (m *GameMap) tickQuake() {
	effect := m.emf.TimedEffect
	var quakeIdx int
	switch effect {
	case eomap.MapTimedEffect_Quake1:
		quakeIdx = 0
	case eomap.MapTimedEffect_Quake2:
		quakeIdx = 1
	case eomap.MapTimedEffect_Quake3:
		quakeIdx = 2
	case eomap.MapTimedEffect_Quake4:
		quakeIdx = 3
	default:
		return // not a quake map
	}

	if quakeIdx >= len(m.cfg.Map.Quakes) {
		return
	}
	qcfg := m.cfg.Map.Quakes[quakeIdx]

	m.mu.Lock()
	// Initialize random rate if not set
	if m.quakeRate == 0 {
		diff := qcfg.MaxTicks - qcfg.MinTicks
		if diff <= 0 {
			diff = 1
		}
		m.quakeRate = qcfg.MinTicks + rand.IntN(diff)
	}
	if m.quakeStrength == 0 {
		diff := qcfg.MaxStrength - qcfg.MinStrength
		if diff <= 0 {
			diff = 1
		}
		m.quakeStrength = qcfg.MinStrength + rand.IntN(diff)
	}

	m.quakeTicks++
	if m.quakeTicks < m.quakeRate {
		m.mu.Unlock()
		return
	}

	strength := m.quakeStrength
	m.quakeRate = 0
	m.quakeStrength = 0
	m.quakeTicks = 0

	buses := make([]*protocol.PacketBus, 0, len(m.players))
	for _, ch := range m.players {
		buses = append(buses, ch.Bus)
	}
	m.mu.Unlock()

	pkt := &server.EffectUseServerPacket{
		Effect: server.MapEffect_Quake,
		EffectData: &server.EffectUseEffectDataQuake{
			QuakeStrength: strength,
		},
	}
	for _, bus := range buses {
		_ = bus.SendPacket(pkt)
	}
}

// tickWarpSuck checks if players near warps should be sucked through them.
func (m *GameMap) tickWarpSuck() {
	m.mu.Lock()
	defer m.mu.Unlock()

	rate := m.cfg.World.WarpSuckRate
	for _, ch := range m.players {
		ch.WarpSuckTicks--
		if ch.WarpSuckTicks > 0 {
			continue
		}
		ch.WarpSuckTicks = rate * 8 // reset cooldown

		// Check adjacent tiles for warps
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				if dx == 0 && dy == 0 {
					continue
				}
				warp, ok := m.warps[[2]int{ch.X + dx, ch.Y + dy}]
				if !ok || warp.DestinationMap <= 0 {
					continue
				}
				// Found an adjacent warp — set pending warp
				ch.PendingWarp = &WarpDest{
					MapID: warp.DestinationMap,
					X:     warp.DestinationCoords.X,
					Y:     warp.DestinationCoords.Y,
				}
				_ = ch.Bus.SendPacket(&server.WarpRequestServerPacket{
					WarpType:     server.Warp_Local,
					MapId:        warp.DestinationMap,
					WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
				})
				return // only process one warp per tick per player
			}
		}
	}
}

// tickEvacuate handles the evacuation countdown and warnings.
func (m *GameMap) tickEvacuate() {
	m.mu.Lock()
	if m.EvacuateTicks <= 0 {
		m.mu.Unlock()
		return
	}

	m.EvacuateTicks--
	remaining := m.EvacuateTicks
	step := m.cfg.Evacuate.TimerStep * 8 // convert seconds to ticks

	// Broadcast warning at step intervals
	if step > 0 && remaining > 0 && remaining%step == 0 {
		secs := remaining / 8
		buses := make([]*protocol.PacketBus, 0, len(m.players))
		for _, ch := range m.players {
			buses = append(buses, ch.Bus)
		}
		m.mu.Unlock()

		for _, bus := range buses {
			_ = bus.SendPacket(&server.TalkServerServerPacket{
				Message: fmt.Sprintf("Map evacuation in %d seconds!", secs),
			})
		}
		return
	}

	// Evacuation complete — warp all non-admin players to jail
	if remaining <= 0 {
		m.EvacuateTicks = 0
		var toWarp []*MapCharacter
		for _, ch := range m.players {
			if ch.Admin < 1 {
				toWarp = append(toWarp, ch)
			}
		}
		m.mu.Unlock()

		for _, ch := range toWarp {
			ch.PendingWarp = &WarpDest{
				MapID: m.cfg.Jail.Map,
				X:     m.cfg.Jail.X,
				Y:     m.cfg.Jail.Y,
			}
			_ = ch.Bus.SendPacket(&server.WarpRequestServerPacket{
				WarpType:     server.Warp_Local,
				MapId:        m.cfg.Jail.Map,
				WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
			})
		}
		return
	}

	m.mu.Unlock()
}

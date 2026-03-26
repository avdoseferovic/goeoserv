package handlers

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"

	"github.com/avdo/goeoserv/internal/formula"
	"github.com/avdo/goeoserv/internal/player"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	worldpkg "github.com/avdo/goeoserv/internal/world"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Attack, eonet.PacketAction_Use, handleAttackUse)
}

func handleAttackUse(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.AttackUseClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize attack", "id", p.ID, "err", err)
		return nil
	}

	p.CharDirection = int(pkt.Direction)

	// Broadcast attack animation to other players
	p.World.BroadcastMap(p.MapID, p.ID, &server.AttackPlayerServerPacket{
		PlayerId:  p.ID,
		Direction: pkt.Direction,
	})

	// Determine weapon range (default 1 = melee)
	attackRange := 1
	for _, wr := range p.Cfg.Combat.WeaponRanges {
		if wr.Weapon == p.Equipment.Weapon {
			attackRange = wr.Range
			break
		}
	}

	// Scan tiles in attack direction for the first NPC in range
	dx, dy := 0, 0
	switch pkt.Direction {
	case eoproto.Direction_Down:
		dy = 1
	case eoproto.Direction_Left:
		dx = -1
	case eoproto.Direction_Up:
		dy = -1
	case eoproto.Direction_Right:
		dx = 1
	}

	npcIndex := -1
	targetX, targetY := p.CharX, p.CharY
	for i := range attackRange {
		_ = i
		targetX += dx
		targetY += dy
		if idx := p.World.GetNpcAt(p.MapID, targetX, targetY); idx >= 0 {
			npcIndex = idx
			break
		}
	}
	if npcIndex < 0 {
		return nil
	}

	// Look up NPC data for hit/damage calculation
	enfID := p.World.GetNpcEnfID(p.MapID, npcIndex)
	npcRec := pubdata.GetNpc(enfID)
	npcEvade := 0
	npcArmor := 0
	if npcRec != nil {
		npcEvade = npcRec.Evade
		npcArmor = npcRec.Armor
	}

	// Hit rate: clamped between 50-80%, based on player accuracy vs NPC evade
	// Formula from reoserv: min(0.8, max(0.5, accuracy / (target_evade * 2)))
	hitRate := 0.5
	if p.Accuracy+npcEvade > 0 {
		if npcEvade > 0 {
			hitRate = float64(p.Accuracy) / float64(npcEvade*2)
		} else {
			hitRate = 0.8
		}
	}
	hitRate = max(0.5, min(0.8, hitRate))

	// Roll for hit
	if rand.Float64() > hitRate {
		// Miss — send 0 damage
		return p.Bus.SendPacket(&server.NpcReplyServerPacket{
			PlayerId:        p.ID,
			PlayerDirection: pkt.Direction,
			NpcIndex:        npcIndex,
			Damage:          0,
			HpPercentage:    100,
		})
	}

	// Calculate base damage
	damage := p.MinDamage
	if p.MaxDamage > p.MinDamage {
		damage = p.MinDamage + rand.IntN(p.MaxDamage-p.MinDamage+1)
	}
	if damage < 1 {
		damage = 1
	}

	// Apply armor reduction (reoserv formula)
	if npcArmor > 0 && damage < npcArmor*2 {
		reduced := float64(damage) * math.Pow(float64(damage)/float64(npcArmor*2), 2)
		damage = int(reduced)
	}
	if damage < 1 {
		damage = 1
	}

	actualDmg, killed, hpPct := p.World.DamageNpc(p.MapID, npcIndex, p.ID, damage)
	if actualDmg == 0 {
		return nil
	}

	if killed {
		baseExp := 10
		if npcRec != nil {
			baseExp = npcRec.Experience
			if baseExp <= 0 {
				baseExp = 1
			}
		}
		if mult := p.Cfg.World.ExpMultiplier; mult > 1 {
			baseExp *= mult
		}

		// Drop items from NPC drop table
		for _, drop := range pubdata.GetNpcDrops(enfID) {
			if drop.Rate > 0 && rand.IntN(64000) < drop.Rate {
				amount := drop.MinAmount
				if drop.MaxAmount > drop.MinAmount {
					amount = drop.MinAmount + rand.IntN(drop.MaxAmount-drop.MinAmount+1)
				}
				if amount > 0 {
					p.World.DropItem(p.MapID, drop.ItemId, amount, targetX, targetY, p.ID)
				}
			}
		}

		killedData := server.NpcKilledData{
			KillerId:        p.ID,
			KillerDirection: pkt.Direction,
			NpcIndex:        npcIndex,
			Damage:          actualDmg,
		}

		// Broadcast kill to nearby players WITHOUT exp (they didn't earn it)
		p.World.BroadcastMap(p.MapID, p.ID, &server.NpcSpecServerPacket{
			NpcKilledData: killedData,
		})

		// Award exp to killer (and party members if in a party)
		awardNpcExp(p, baseExp, killedData)

		slog.Debug("npc killed", "player", p.ID, "npc_index", npcIndex, "exp", baseExp)
	} else {
		// NPC damaged — send damage notification
		return p.Bus.SendPacket(&server.NpcReplyServerPacket{
			PlayerId:        p.ID,
			PlayerDirection: pkt.Direction,
			NpcIndex:        npcIndex,
			Damage:          actualDmg,
			HpPercentage:    hpPct,
		})
	}

	return nil
}

// awardNpcExp awards experience to the killer and party members, sending the appropriate packets.
func awardNpcExp(p *player.Player, baseExp int, killedData server.NpcKilledData) {
	// Check if killer is in a party
	party := worldpkg.GetParty(p.ID)

	if party == nil {
		// Solo kill — all exp to killer
		giveExpToPlayer(p, baseExp, killedData)
		return
	}

	// Party kill — split exp among members on the same map
	members := party.GetMembersOnMap(p.MapID)
	if len(members) == 0 {
		giveExpToPlayer(p, baseExp, killedData)
		return
	}

	share := baseExp / len(members)
	if share < 1 {
		share = 1
	}

	// Give the killer their share (with the kill animation data)
	giveExpToPlayer(p, share, killedData)

	// Give party members their share
	for _, m := range members {
		if m.PlayerID == p.ID {
			continue
		}
		if memberPlayer, ok := m.Player.(*player.Player); ok && memberPlayer != nil {
			giveExpToPlayer(memberPlayer, share, killedData)
		}
	}
}

// giveExpToPlayer awards exp to a single player and sends the kill/levelup packet.
func giveExpToPlayer(p *player.Player, exp int, killedData server.NpcKilledData) {
	p.CharExp += exp
	totalExp := p.CharExp // send total accumulated exp, not just gained
	oldLevel := p.CharLevel
	newLevel := formula.LevelForExp(p.CharExp)
	leveledUp := newLevel > oldLevel
	if leveledUp {
		p.CharLevel = newLevel
		p.StatPoints += p.Cfg.World.StatPointsPerLvl
		p.SkillPoints += p.Cfg.World.SkillPointsPerLvl
		p.CalculateStats()
	}

	if leveledUp {
		_ = p.Bus.SendPacket(&server.NpcAcceptServerPacket{
			NpcKilledData: killedData,
			Experience:    &totalExp,
			LevelUp: &server.LevelUpStats{
				Level:       p.CharLevel,
				StatPoints:  p.StatPoints,
				SkillPoints: p.SkillPoints,
				MaxHp:       p.CharMaxHP,
				MaxTp:       p.CharMaxTP,
				MaxSp:       p.CharMaxSP,
			},
		})
	} else {
		_ = p.Bus.SendPacket(&server.NpcSpecServerPacket{
			NpcKilledData: killedData,
			Experience:    &totalExp,
		})
	}
}

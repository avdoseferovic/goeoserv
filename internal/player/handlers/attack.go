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
	eopub "github.com/ethanmoffat/eolib-go/v3/protocol/pub"
)

func init() {
	player.Register(eonet.PacketFamily_Attack, eonet.PacketAction_Use, handleAttackUse)
}

var (
	combatHitRoll    = rollCombatHit
	combatDamageRoll = rollCombatDamage
)

func handleAttackUse(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	if p.CharHP <= 0 {
		return nil
	}
	if p.World.HasCaptcha(p.ID) {
		return nil
	}
	if p.Cfg.Combat.EnforceWeight && p.Weight > p.MaxWeight {
		return nil
	}

	var pkt client.AttackUseClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize attack", "id", p.ID, "err", err)
		return nil
	}

	// Determine weapon range (default 1 = melee)
	attackRange := 1
	requiresArrows := false
	for _, wr := range p.Cfg.Combat.WeaponRanges {
		if wr.Weapon == p.Equipment.Weapon {
			attackRange = wr.Range
			requiresArrows = wr.Arrows
			break
		}
	}
	if requiresArrows {
		shield := pubdata.GetItem(p.Equipment.Shield)
		if shield == nil || shield.Subtype != eopub.ItemSubtype_Arrows {
			return nil
		}
	}

	p.CharDirection = int(pkt.Direction)

	// Broadcast attack animation to other players
	p.World.BroadcastMap(p.MapID, p.ID, &server.AttackPlayerServerPacket{
		PlayerId:  p.ID,
		Direction: pkt.Direction,
	})

	// Scan tiles in attack direction for the first valid PvP or NPC target in range.
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

	target := acquireAttackTarget(p, dx, dy, attackRange)
	if target.kind == attackTargetNone {
		return nil
	}
	if target.kind == attackTargetPlayer {
		handlePlayerMeleeAttack(p, target.playerID, pkt.Direction, target.x, target.y)
		return nil
	}

	npcIndex := target.npcIndex
	targetX, targetY := target.x, target.y

	// Look up NPC data for hit/damage calculation
	enfID := p.World.GetNpcEnfID(p.MapID, npcIndex)
	npcRec := pubdata.GetNpc(enfID)
	npcEvade := 0
	npcArmor := 0
	if npcRec != nil {
		npcEvade = npcRec.Evade
		npcArmor = npcRec.Armor
	}

	if !combatHitRoll(p.Accuracy, npcEvade) {
		// Miss — send 0 damage
		return p.Bus.SendPacket(&server.NpcReplyServerPacket{
			PlayerId:        p.ID,
			PlayerDirection: pkt.Direction,
			NpcIndex:        npcIndex,
			Damage:          0,
			HpPercentage:    p.World.GetNpcHpPercentage(p.MapID, npcIndex),
		})
	}

	damage := combatDamageRoll(p.MinDamage, p.MaxDamage, npcArmor)

	actualDmg, killed, hpPct := p.World.DamageNpc(p.MapID, npcIndex, p.ID, damage)
	if actualDmg == 0 {
		return nil
	}

	if killed {
		p.QuestProgress.RecordNpcKill(enfID)
		p.SaveCharacterAsync()

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

type playerMeleeAttackSnapshot struct {
	attackerID int
	mapID      int
	targetID   int
	direction  int
	targetX    int
	targetY    int
	minDamage  int
	maxDamage  int
	accuracy   int
}

type attackTargetKind int

const (
	attackTargetNone attackTargetKind = iota
	attackTargetPlayer
	attackTargetNPC
)

type attackTarget struct {
	kind     attackTargetKind
	playerID int
	npcIndex int
	x        int
	y        int
}

func acquireAttackTarget(attacker *player.Player, dx, dy, attackRange int) attackTarget {
	if attacker == nil || attacker.World == nil || attackRange < 1 {
		return attackTarget{}
	}

	targetX, targetY := attacker.CharX, attacker.CharY
	for range attackRange {
		targetX += dx
		targetY += dy

		if attacker.World.IsAttackTileBlocked(attacker.MapID, targetX, targetY) {
			return attackTarget{}
		}

		targetPlayerID := attacker.World.GetPlayerAt(attacker.MapID, targetX, targetY)
		if targetPlayerID != 0 {
			if attacker.World.CanPlayerAttackPlayer(attacker.MapID, attacker.ID, targetPlayerID) {
				return attackTarget{kind: attackTargetPlayer, playerID: targetPlayerID, x: targetX, y: targetY}
			}

			return attackTarget{}
		}

		if npcIndex := attacker.World.GetNpcAt(attacker.MapID, targetX, targetY); npcIndex >= 0 {
			return attackTarget{kind: attackTargetNPC, npcIndex: npcIndex, x: targetX, y: targetY}
		}
	}

	return attackTarget{}
}

func handlePlayerMeleeAttack(attacker *player.Player, targetPlayerID int, direction eoproto.Direction, targetX, targetY int) {
	if attacker.World == nil {
		return
	}
	if targetPlayerID <= 0 || targetPlayerID == attacker.ID {
		return
	}
	if attacker.World.HasCaptcha(targetPlayerID) {
		return
	}
	if !attacker.World.CanPlayerAttackPlayer(attacker.MapID, attacker.ID, targetPlayerID) {
		return
	}

	go resolvePlayerMeleeAttack(attacker.World, playerMeleeAttackSnapshot{
		attackerID: attacker.ID,
		mapID:      attacker.MapID,
		targetID:   targetPlayerID,
		direction:  int(direction),
		targetX:    targetX,
		targetY:    targetY,
		minDamage:  attacker.MinDamage,
		maxDamage:  attacker.MaxDamage,
		accuracy:   attacker.Accuracy,
	})
}

func resolvePlayerMeleeAttack(world player.WorldInterface, attack playerMeleeAttackSnapshot) {
	target := world.GetPlayerSession(attack.targetID)
	if target == nil {
		return
	}

	target.Mu.Lock()
	defer target.Mu.Unlock()

	if target.State != player.StateInGame || target.World == nil {
		return
	}
	if target.ID == attack.attackerID || target.MapID != attack.mapID {
		return
	}
	if !world.CanPlayerAttackPlayer(attack.mapID, attack.attackerID, target.ID) {
		return
	}
	if target.CharX != attack.targetX || target.CharY != attack.targetY {
		return
	}
	if world.HasCaptcha(target.ID) {
		return
	}

	if !combatHitRoll(attack.accuracy, target.Evade) {
		world.BroadcastMap(attack.mapID, -1, &server.AvatarReplyServerPacket{
			PlayerId:     attack.attackerID,
			VictimId:     target.ID,
			Damage:       0,
			Direction:    eoproto.Direction(attack.direction),
			HpPercentage: hpPercentage(target.CharHP, target.CharMaxHP),
			Dead:         false,
		})
		return
	}

	damage := combatDamageRoll(attack.minDamage, attack.maxDamage, target.Armor)
	if damage <= 0 || target.CharHP <= 0 {
		return
	}

	actualDamage := damage
	if actualDamage > target.CharHP {
		actualDamage = target.CharHP
	}
	target.CharHP -= actualDamage
	if target.CharHP < 0 {
		target.CharHP = 0
	}
	remainingHpPct := hpPercentage(target.CharHP, target.CharMaxHP)
	world.UpdatePlayerVitals(target.MapID, target.ID, target.CharHP, target.CharTP)
	world.BroadcastMap(attack.mapID, -1, &server.AvatarReplyServerPacket{
		PlayerId:     attack.attackerID,
		VictimId:     target.ID,
		Damage:       actualDamage,
		Direction:    eoproto.Direction(attack.direction),
		HpPercentage: remainingHpPct,
		Dead:         target.CharHP == 0,
	})
	_ = target.Bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: target.CharHP, Tp: target.CharTP})

	if target.CharHP > 0 {
		return
	}

	if world.HandlePlayerDefeat(target.MapID, attack.attackerID, target.ID, attack.direction) {
		return
	}
	target.Die()
}

func rollWeaponDamage(minDamage, maxDamage int) int {
	damage := minDamage
	if maxDamage > minDamage {
		damage = minDamage + rand.IntN(maxDamage-minDamage+1)
	}
	if damage < 1 {
		return 1
	}
	return damage
}

func rollCombatDamage(minDamage, maxDamage, armor int) int {
	return reduceDamageByArmor(rollWeaponDamage(minDamage, maxDamage), armor)
}

func rollCombatHit(accuracy, evade int) bool {
	return rand.Float64() <= combatHitRate(accuracy, evade)
}

func combatHitRate(accuracy, evade int) float64 {
	hitRate := 0.5
	if accuracy+evade > 0 {
		if evade > 0 {
			hitRate = float64(accuracy) / float64(evade*2)
		} else {
			hitRate = 0.8
		}
	}
	return max(0.5, min(0.8, hitRate))
}

func hpPercentage(currentHP, maxHP int) int {
	if maxHP <= 0 {
		return 0
	}

	hpPct := currentHP * 100 / maxHP
	if hpPct < 0 {
		return 0
	}
	if hpPct > 100 {
		return 100
	}
	return hpPct
}

func reduceDamageByArmor(damage, armor int) int {
	if damage < 1 {
		return 1
	}
	if armor > 0 && damage < armor*2 {
		reduced := float64(damage) * math.Pow(float64(damage)/float64(armor*2), 2)
		damage = int(reduced)
	}
	if damage < 1 {
		return 1
	}
	return damage
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

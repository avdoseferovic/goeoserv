package handlers

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/avdo/goeoserv/internal/player"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	worldpkg "github.com/avdo/goeoserv/internal/world"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

const (
	spellCastWindow   = 5 * time.Second
	spellTargetRange  = 5
	groupSpellRange   = 5
	spellHealBase     = 8
	groupHealDivisor  = 4
	spellDamageBase   = 5
	spellDamageSpread = 2
)

func init() {
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_Request, handleSpellRequest)
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_TargetSelf, handleSpellTargetSelf)
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_TargetOther, handleSpellTargetOther)
	player.Register(eonet.PacketFamily_Spell, eonet.PacketAction_TargetGroup, handleSpellTargetGroup)
}

// handleSpellRequest begins spell casting and records the chant timestamp.
func handleSpellRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell request", "id", p.ID, "err", err)
		return nil
	}

	if !canStartSpellCast(p, pkt.SpellId, pkt.Timestamp) {
		return rejectSpellCast(p)
	}

	p.PendingSpell = &player.SpellCastState{
		ID:        pkt.SpellId,
		Timestamp: pkt.Timestamp,
		StartedAt: time.Now(),
	}

	p.World.BroadcastMap(p.MapID, -1, &server.SpellRequestServerPacket{
		PlayerId: p.ID,
		SpellId:  pkt.SpellId,
	})

	return nil
}

// handleSpellTargetSelf handles self-targeted spells (heals).
func handleSpellTargetSelf(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellTargetSelfClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell target self", "id", p.ID, "err", err)
		return nil
	}

	p.CharDirection = int(pkt.Direction)

	spellLevel, tpCost, ok := validateSelfOrGroupSpellCast(p, pkt.SpellId, pkt.Timestamp)
	if !ok {
		return rejectSpellCast(p)
	}

	healAmount := spellHealAmount(p, spellLevel)
	actualHeal := p.GainHP(healAmount)
	p.CharTP -= tpCost
	finishSpellCast(p, pkt.Timestamp)
	p.World.UpdatePlayerVitals(p.MapID, p.ID, p.CharHP, p.CharTP)

	hp := p.CharHP
	tp := p.CharTP
	p.World.BroadcastMap(p.MapID, -1, &server.SpellTargetSelfServerPacket{
		PlayerId:     p.ID,
		SpellId:      pkt.SpellId,
		SpellHealHp:  actualHeal,
		HpPercentage: hpPercentage(p.CharHP, p.CharMaxHP),
		Hp:           &hp,
		Tp:           &tp,
	})

	return nil
}

// handleSpellTargetOther handles targeted spells against players or NPCs.
func handleSpellTargetOther(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellTargetOtherClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell target other", "id", p.ID, "err", err)
		return nil
	}

	spellLevel, tpCost, ok := validateTargetedSpellCast(p, pkt.SpellId, pkt.PreviousTimestamp, pkt.Timestamp)
	if !ok {
		return rejectSpellCast(p)
	}

	switch pkt.TargetType {
	case client.SpellTarget_Player:
		return castSpellOnPlayer(p, pkt, spellLevel, tpCost)
	case client.SpellTarget_Npc:
		return castSpellOnNpc(p, pkt, spellLevel, tpCost)
	default:
		return rejectSpellCast(p)
	}
}

// handleSpellTargetGroup handles group heal spells.
func handleSpellTargetGroup(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.SpellTargetGroupClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize spell target group", "id", p.ID, "err", err)
		return nil
	}

	spellLevel, tpCost, ok := validateSelfOrGroupSpellCast(p, pkt.SpellId, pkt.Timestamp)
	if !ok {
		return rejectSpellCast(p)
	}

	targets := collectGroupHealTargets(p)
	if len(targets) == 0 {
		targets = []*player.Player{p}
	}

	healAmount := max(1, spellHealAmount(p, spellLevel)-max(1, spellLevel/groupHealDivisor))
	players := make([]server.GroupHealTargetPlayer, 0, len(targets))
	for _, target := range targets {
		if target == nil || target.CharHP <= 0 {
			continue
		}

		target.GainHP(healAmount)
		p.World.UpdatePlayerVitals(target.MapID, target.ID, target.CharHP, target.CharTP)
		_ = target.Bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: target.CharHP, Tp: target.CharTP})
		players = append(players, server.GroupHealTargetPlayer{
			PlayerId:     target.ID,
			HpPercentage: hpPercentage(target.CharHP, target.CharMaxHP),
			Hp:           target.CharHP,
		})
	}

	p.CharTP -= tpCost
	finishSpellCast(p, pkt.Timestamp)
	p.World.UpdatePlayerVitals(p.MapID, p.ID, p.CharHP, p.CharTP)

	p.World.BroadcastMap(p.MapID, -1, &server.SpellTargetGroupServerPacket{
		SpellId:     pkt.SpellId,
		CasterId:    p.ID,
		CasterTp:    p.CharTP,
		SpellHealHp: healAmount,
		Players:     players,
	})

	return nil
}

func canStartSpellCast(p *player.Player, spellID, timestamp int) bool {
	if !canUseSpell(p, spellID) {
		return false
	}
	if timestamp <= 0 || timestamp <= p.LastSpellCast {
		return false
	}
	if p.CharTP < spellTpCost(p.GetSpellLevel(spellID)) {
		return false
	}

	clearExpiredPendingSpell(p)
	return p.PendingSpell == nil
}

func validateSelfOrGroupSpellCast(p *player.Player, spellID, timestamp int) (int, int, bool) {
	spellLevel, tpCost, pending, ok := validateSpellCastStart(p, spellID)
	if !ok {
		return 0, 0, false
	}
	if timestamp != pending.Timestamp {
		return 0, 0, false
	}
	return spellLevel, tpCost, true
}

func validateTargetedSpellCast(p *player.Player, spellID, previousTimestamp, timestamp int) (int, int, bool) {
	spellLevel, tpCost, pending, ok := validateSpellCastStart(p, spellID)
	if !ok {
		return 0, 0, false
	}
	if previousTimestamp != pending.Timestamp || timestamp <= previousTimestamp {
		return 0, 0, false
	}
	return spellLevel, tpCost, true
}

func validateSpellCastStart(p *player.Player, spellID int) (int, int, *player.SpellCastState, bool) {
	if !canUseSpell(p, spellID) {
		return 0, 0, nil, false
	}

	clearExpiredPendingSpell(p)
	pending := p.PendingSpell
	if pending == nil || pending.ID != spellID {
		return 0, 0, nil, false
	}

	spellLevel := p.GetSpellLevel(spellID)
	tpCost := spellTpCost(spellLevel)
	if p.CharTP < tpCost {
		return 0, 0, nil, false
	}

	return spellLevel, tpCost, pending, true
}

func clearExpiredPendingSpell(p *player.Player) {
	if p.PendingSpell == nil {
		return
	}
	if time.Since(p.PendingSpell.StartedAt) <= spellCastWindow {
		return
	}
	p.PendingSpell = nil
}

func finishSpellCast(p *player.Player, timestamp int) {
	p.PendingSpell = nil
	p.LastSpellCast = timestamp
}

func rejectSpellCast(p *player.Player) error {
	p.PendingSpell = nil
	return p.Bus.SendPacket(&server.SpellErrorServerPacket{})
}

func canUseSpell(p *player.Player, spellID int) bool {
	if p == nil || p.World == nil {
		return false
	}
	if p.CharHP <= 0 {
		return false
	}
	if p.World.HasCaptcha(p.ID) {
		return false
	}
	if p.Cfg.Combat.EnforceWeight && p.Weight > p.MaxWeight {
		return false
	}
	return p.GetSpellLevel(spellID) > 0
}

func spellTpCost(spellLevel int) int {
	return max(spellLevel, 1) * 5
}

func spellHealAmount(p *player.Player, spellLevel int) int {
	if p == nil {
		return max(spellLevel, 1) * spellHealBase
	}

	healAmount := spellLevel*spellHealBase + p.Stats.Wis*2 + p.Stats.Intl
	if healAmount < 1 {
		return 1
	}
	return healAmount
}

func spellDamageAmount(p *player.Player, spellLevel, armor int) int {
	if p == nil {
		return 1
	}

	minDamage := spellLevel*spellDamageBase + p.Stats.Intl + max(0, p.Stats.Wis/2)
	maxDamage := minDamage + max(spellDamageSpread, p.Stats.Intl+p.Stats.Wis)
	return reduceDamageByArmor(rollWeaponDamage(minDamage, maxDamage), armor)
}

func castSpellOnPlayer(p *player.Player, pkt client.SpellTargetOtherClientPacket, spellLevel, tpCost int) error {
	target := p.World.GetPlayerSession(pkt.VictimId)
	if !isValidSpellPlayerTarget(p, target) {
		return rejectSpellCast(p)
	}

	p.CharDirection = directionToTarget(p.CharX, p.CharY, target.CharX, target.CharY, p.CharDirection)
	healAmount := spellHealAmount(p, spellLevel)
	actualHeal := target.GainHP(healAmount)
	p.CharTP -= tpCost
	finishSpellCast(p, pkt.Timestamp)
	p.World.UpdatePlayerVitals(p.MapID, p.ID, p.CharHP, p.CharTP)
	p.World.UpdatePlayerVitals(target.MapID, target.ID, target.CharHP, target.CharTP)
	_ = p.Bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: p.CharHP, Tp: p.CharTP})
	_ = target.Bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: target.CharHP, Tp: target.CharTP})

	p.World.BroadcastMap(p.MapID, -1, &server.SpellTargetOtherServerPacket{
		VictimId:        target.ID,
		CasterId:        p.ID,
		CasterDirection: eoproto.Direction(p.CharDirection),
		SpellId:         pkt.SpellId,
		SpellHealHp:     actualHeal,
		HpPercentage:    hpPercentage(target.CharHP, target.CharMaxHP),
	})

	return nil
}

func castSpellOnNpc(p *player.Player, pkt client.SpellTargetOtherClientPacket, spellLevel, tpCost int) error {
	npcX, npcY, ok := findNpcTargetInRange(p, pkt.VictimId, spellTargetRange)
	if !ok {
		return rejectSpellCast(p)
	}

	enfID := p.World.GetNpcEnfID(p.MapID, pkt.VictimId)
	if enfID <= 0 {
		return rejectSpellCast(p)
	}

	npcRec := pubdata.GetNpc(enfID)
	npcArmor := 0
	if npcRec != nil {
		npcArmor = npcRec.Armor
	}

	damage := spellDamageAmount(p, spellLevel, npcArmor)
	actualDamage, killed, hpPct := p.World.DamageNpc(p.MapID, pkt.VictimId, p.ID, damage)
	if actualDamage <= 0 {
		return rejectSpellCast(p)
	}

	p.CharDirection = directionToTarget(p.CharX, p.CharY, npcX, npcY, p.CharDirection)
	p.CharTP -= tpCost
	finishSpellCast(p, pkt.Timestamp)
	p.World.UpdatePlayerVitals(p.MapID, p.ID, p.CharHP, p.CharTP)

	attackerPacket := &server.CastReplyServerPacket{
		SpellId:         pkt.SpellId,
		CasterId:        p.ID,
		CasterDirection: eoproto.Direction(p.CharDirection),
		NpcIndex:        pkt.VictimId,
		Damage:          actualDamage,
		HpPercentage:    hpPct,
		CasterTp:        &p.CharTP,
	}
	observerPacket := &server.CastReplyServerPacket{
		SpellId:         pkt.SpellId,
		CasterId:        p.ID,
		CasterDirection: eoproto.Direction(p.CharDirection),
		NpcIndex:        pkt.VictimId,
		Damage:          actualDamage,
		HpPercentage:    hpPct,
	}
	_ = p.Bus.SendPacket(attackerPacket)
	p.World.BroadcastMap(p.MapID, p.ID, observerPacket)

	if !killed {
		return nil
	}

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

	for _, drop := range pubdata.GetNpcDrops(enfID) {
		if drop.Rate <= 0 || rand.IntN(64000) >= drop.Rate {
			continue
		}

		amount := drop.MinAmount
		if drop.MaxAmount > drop.MinAmount {
			amount = drop.MinAmount + rand.IntN(drop.MaxAmount-drop.MinAmount+1)
		}
		if amount > 0 {
			p.World.DropItem(p.MapID, drop.ItemId, amount, npcX, npcY, p.ID)
		}
	}

	killedData := server.NpcKilledData{
		KillerId:        p.ID,
		KillerDirection: eoproto.Direction(p.CharDirection),
		NpcIndex:        pkt.VictimId,
		Damage:          actualDamage,
	}
	p.QuestProgress.RecordNpcKill(enfID)
	_ = p.SaveCharacter()
	p.World.BroadcastMap(p.MapID, p.ID, &server.NpcSpecServerPacket{NpcKilledData: killedData})
	awardNpcExp(p, baseExp, killedData)

	return nil
}

func isValidSpellPlayerTarget(caster, target *player.Player) bool {
	if caster == nil || target == nil || caster.World == nil {
		return false
	}
	if target.ID == caster.ID {
		return false
	}
	if target.State != player.StateInGame || target.World == nil {
		return false
	}
	if target.MapID != caster.MapID || target.CharHP <= 0 {
		return false
	}
	return caster.DistanceTo(target.CharX, target.CharY) <= spellTargetRange
}

func collectGroupHealTargets(caster *player.Player) []*player.Player {
	if caster == nil {
		return nil
	}

	party := worldpkg.GetParty(caster.ID)
	if party == nil {
		return []*player.Player{caster}
	}

	members := party.GetMembersOnMap(caster.MapID)
	targets := make([]*player.Player, 0, len(members))
	for _, member := range members {
		target, ok := member.Player.(*player.Player)
		if !ok || target == nil {
			continue
		}
		if target.State != player.StateInGame || target.CharHP <= 0 {
			continue
		}
		if caster.DistanceTo(target.CharX, target.CharY) > groupSpellRange {
			continue
		}
		targets = append(targets, target)
	}

	return targets
}

func findNpcTargetInRange(caster *player.Player, npcIndex, maxRange int) (int, int, bool) {
	if caster == nil || caster.World == nil || npcIndex < 0 || maxRange < 0 {
		return 0, 0, false
	}

	for y := caster.CharY - maxRange; y <= caster.CharY+maxRange; y++ {
		for x := caster.CharX - maxRange; x <= caster.CharX+maxRange; x++ {
			if caster.DistanceTo(x, y) > maxRange {
				continue
			}
			if caster.World.GetNpcAt(caster.MapID, x, y) != npcIndex {
				continue
			}
			return x, y, true
		}
	}

	return 0, 0, false
}

func directionToTarget(fromX, fromY, targetX, targetY, fallback int) int {
	dx := targetX - fromX
	dy := targetY - fromY
	if dx == 0 && dy == 0 {
		return fallback
	}

	if abs(dx) >= abs(dy) {
		if dx < 0 {
			return int(eoproto.Direction_Left)
		}
		return int(eoproto.Direction_Right)
	}
	if dy < 0 {
		return int(eoproto.Direction_Up)
	}
	return int(eoproto.Direction_Down)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

package handlers

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/protocol"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	"github.com/ethanmoffat/eolib-go/v3/data"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
	eopub "github.com/ethanmoffat/eolib-go/v3/protocol/pub"
)

func TestAcquireAttackTargetReturnsAttackablePlayer(t *testing.T) {
	world := &attackTargetTestWorld{
		playersAt:       map[[3]int]int{{1, 6, 5}: 2},
		attackablePairs: map[[3]int]bool{{1, 1, 2}: true},
		playerSessions:  map[int]*player.Player{},
	}
	attacker := &player.Player{ID: 1, State: player.StateInGame, MapID: 1, CharX: 5, CharY: 5, CharHP: 10, World: world}
	world.playerSessions[attacker.ID] = attacker
	world.playerSessions[2] = &player.Player{ID: 2, State: player.StateInGame, MapID: 1, World: world, CharHP: 10}

	target := acquireAttackTarget(attacker, 1, 0, 3)

	if target.kind != attackTargetPlayer {
		t.Fatalf("expected player target, got kind %v", target.kind)
	}
	if target.playerID != 2 {
		t.Fatalf("expected player 2, got %d", target.playerID)
	}
	if target.x != 6 || target.y != 5 {
		t.Fatalf("expected target at (6,5), got (%d,%d)", target.x, target.y)
	}
}

func TestAcquireAttackTargetStopsAtProtectedPlayer(t *testing.T) {
	world := &attackTargetTestWorld{
		playersAt: map[[3]int]int{{1, 6, 5}: 2},
		npcsAt:    map[[3]int]int{{1, 7, 5}: 9},
	}
	attacker := &player.Player{ID: 1, State: player.StateInGame, MapID: 1, CharX: 5, CharY: 5, CharHP: 10, World: world}

	target := acquireAttackTarget(attacker, 1, 0, 3)

	if target.kind != attackTargetNone {
		t.Fatalf("expected no target when protected player blocks the line, got kind %v", target.kind)
	}
	if target.npcIndex != 0 || target.playerID != 0 {
		t.Fatalf("expected no player or npc target, got player=%d npc=%d", target.playerID, target.npcIndex)
	}
}

func TestAcquireAttackTargetStopsAtNonAttackablePlayerBeforeOtherPlayer(t *testing.T) {
	world := &attackTargetTestWorld{
		playersAt:       map[[3]int]int{{1, 6, 5}: 2, {1, 7, 5}: 3},
		attackablePairs: map[[3]int]bool{{1, 1, 3}: true},
		playerSessions:  map[int]*player.Player{},
	}
	attacker := &player.Player{ID: 1, MapID: 1, CharX: 5, CharY: 5, World: world}
	world.playerSessions[attacker.ID] = attacker
	world.playerSessions[2] = &player.Player{ID: 2, State: player.StateInGame, MapID: 1, World: world, CharHP: 10}
	world.playerSessions[3] = &player.Player{ID: 3, State: player.StateInGame, MapID: 1, World: world, CharHP: 10}

	target := acquireAttackTarget(attacker, 1, 0, 3)

	if target.kind != attackTargetNone {
		t.Fatalf("expected no target when protected player blocks another player, got kind %v", target.kind)
	}
}

func TestHandleAttackUseRoutesToPlayerTarget(t *testing.T) {
	world := &attackTargetTestWorld{
		playersAt:             map[[3]int]int{{1, 6, 5}: 2},
		attackablePairs:       map[[3]int]bool{{1, 1, 2}: true},
		playerSessions:        map[int]*player.Player{},
		getPlayerSessionCalls: make(chan int, 1),
	}
	target := &player.Player{ID: 2, State: player.StateInGame, World: world, MapID: 1, CharHP: 10}
	world.playerSessions[target.ID] = target
	attacker := newAttackTestPlayer(world)

	err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
	if err != nil {
		t.Fatalf("handleAttackUse returned error: %v", err)
	}

	if attacker.CharDirection != int(eoproto.Direction_Right) {
		t.Fatalf("expected attacker direction %d, got %d", eoproto.Direction_Right, attacker.CharDirection)
	}
	if len(world.broadcastPackets) == 0 {
		t.Fatal("expected attack animation broadcast")
	}
	attackPkt, ok := world.broadcastPackets[0].(*server.AttackPlayerServerPacket)
	if !ok {
		t.Fatalf("expected first broadcast to be AttackPlayerServerPacket, got %T", world.broadcastPackets[0])
	}
	if int(attackPkt.Direction) != int(eoproto.Direction_Right) {
		t.Fatalf("expected attack direction %d, got %d", eoproto.Direction_Right, attackPkt.Direction)
	}

	select {
	case playerID := <-world.getPlayerSessionCalls:
		if playerID != target.ID {
			t.Fatalf("expected player session lookup for %d, got %d", target.ID, playerID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected player attack path to resolve target session")
	}

	if world.damageNpcCalls != 0 {
		t.Fatalf("expected player attack path, got %d npc damage calls", world.damageNpcCalls)
	}
}

func TestHandleAttackUsePlayerSpecificBlockersPreventNpcFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		targetState player.ClientState
		targetHP    int
		hasCaptcha  bool
		attackable  bool
	}{
		{name: "dead player", targetState: player.StateInGame, targetHP: 0},
		{name: "captcha player", targetState: player.StateInGame, targetHP: 10, hasCaptcha: true, attackable: true},
		{name: "non-ingame player", targetState: player.StateLoggedIn, targetHP: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &attackTargetTestWorld{
				playersAt:             map[[3]int]int{{1, 6, 5}: 2},
				npcsAt:                map[[3]int]int{{1, 7, 5}: 9},
				attackablePairs:       map[[3]int]bool{{1, 1, 2}: tt.attackable},
				captchas:              map[int]bool{2: tt.hasCaptcha},
				playerSessions:        map[int]*player.Player{},
				getPlayerSessionCalls: make(chan int, 1),
			}
			world.playerSessions[2] = &player.Player{ID: 2, State: tt.targetState, World: world, MapID: 1, CharHP: tt.targetHP}
			attacker := newAttackTestPlayer(world)

			err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
			if err != nil {
				t.Fatalf("handleAttackUse returned error: %v", err)
			}

			if world.damageNpcCalls != 0 {
				t.Fatalf("expected player-specific blocker to stop npc targeting, got %d npc damage calls", world.damageNpcCalls)
			}
			if len(world.getPlayerSessionCalls) != 0 {
				t.Fatalf("expected blocker to stop before async player resolution, got %d lookups", len(world.getPlayerSessionCalls))
			}
		})
	}
}

func TestHandleAttackUseAttackerGateFailuresStopBeforeAnimation(t *testing.T) {
	tests := []struct {
		name          string
		state         player.ClientState
		charHP        int
		hasCaptcha    bool
		enforceWeight bool
		weight        int
		maxWeight     int
	}{
		{name: "dead attacker", state: player.StateInGame, charHP: 0},
		{name: "attacker not in game", state: player.StateLoggedIn, charHP: 10},
		{name: "attacker captcha blocked", state: player.StateInGame, charHP: 10, hasCaptcha: true},
		{name: "attacker overweight", state: player.StateInGame, charHP: 10, enforceWeight: true, weight: 11, maxWeight: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &attackTargetTestWorld{
				playersAt:             map[[3]int]int{{1, 6, 5}: 2},
				npcsAt:                map[[3]int]int{{1, 6, 5}: 9},
				attackablePairs:       map[[3]int]bool{{1, 1, 2}: true},
				captchas:              map[int]bool{1: tt.hasCaptcha},
				playerSessions:        map[int]*player.Player{},
				getPlayerSessionCalls: make(chan int, 1),
			}

			attacker := newAttackTestPlayer(world)
			attacker.State = tt.state
			attacker.CharHP = tt.charHP
			attacker.Cfg = &config.Config{Combat: config.Combat{EnforceWeight: tt.enforceWeight}}
			attacker.Weight = tt.weight
			attacker.MaxWeight = tt.maxWeight

			err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
			if err != nil {
				t.Fatalf("handleAttackUse returned error: %v", err)
			}

			if attacker.CharDirection != 0 {
				t.Fatalf("expected attacker direction to remain unchanged, got %d", attacker.CharDirection)
			}
			if len(world.broadcastPackets) != 0 {
				t.Fatalf("expected no broadcasts, got %d", len(world.broadcastPackets))
			}
			if world.damageNpcCalls != 0 {
				t.Fatalf("expected no npc damage calls, got %d", world.damageNpcCalls)
			}
			if len(world.getPlayerSessionCalls) != 0 {
				t.Fatalf("expected no player session lookups, got %d", len(world.getPlayerSessionCalls))
			}
		})
	}
}

func TestHandleAttackUseUsesConfiguredRangeForPlayerTarget(t *testing.T) {
	world := &attackTargetTestWorld{
		playersAt:             map[[3]int]int{{1, 8, 5}: 2},
		attackablePairs:       map[[3]int]bool{{1, 1, 2}: true},
		playerSessions:        map[int]*player.Player{},
		getPlayerSessionCalls: make(chan int, 1),
	}
	target := &player.Player{ID: 2, State: player.StateInGame, World: world, MapID: 1, CharHP: 10}
	world.playerSessions[target.ID] = target
	attacker := newAttackTestPlayer(world)
	attacker.Cfg = &config.Config{Combat: config.Combat{WeaponRanges: []config.WeaponRange{{Weapon: 365, Range: 3}}}}
	attacker.Equipment.Weapon = 365

	err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
	if err != nil {
		t.Fatalf("handleAttackUse returned error: %v", err)
	}

	select {
	case playerID := <-world.getPlayerSessionCalls:
		if playerID != target.ID {
			t.Fatalf("expected player session lookup for %d, got %d", target.ID, playerID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected configured weapon range to reach distant player target")
	}
}

func TestHandleAttackUseUsesConfiguredRangeForNpcTarget(t *testing.T) {
	stubAttackCombatRolls(t, true)

	world := &attackTargetTestWorld{
		npcsAt: map[[3]int]int{{1, 8, 5}: 9},
	}
	attacker := newAttackTestPlayer(world)
	attacker.Cfg = &config.Config{Combat: config.Combat{WeaponRanges: []config.WeaponRange{{Weapon: 365, Range: 3}}}}
	attacker.Equipment.Weapon = 365

	err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
	if err != nil {
		t.Fatalf("handleAttackUse returned error: %v", err)
	}

	if attacker.CharDirection != int(eoproto.Direction_Right) {
		t.Fatalf("expected attacker direction %d, got %d", eoproto.Direction_Right, attacker.CharDirection)
	}
	if len(world.broadcastPackets) == 0 {
		t.Fatal("expected ranged npc attack animation broadcast")
	}
	if world.damageNpcCalls != 1 {
		t.Fatalf("expected configured weapon range to reach npc target once, got %d damage calls", world.damageNpcCalls)
	}
	if len(world.getPlayerSessionCalls) != 0 {
		t.Fatalf("expected npc attack path to avoid player session lookups, got %d", len(world.getPlayerSessionCalls))
	}
	if world.lastDamageNpcCall.npcIndex != 9 {
		t.Fatalf("expected npc target 9, got %d", world.lastDamageNpcCall.npcIndex)
	}
}

func TestHandleAttackUseArrowRequiredWeaponWithoutArrowsAgainstNpcDoesNothing(t *testing.T) {
	world := &attackTargetTestWorld{
		npcsAt: map[[3]int]int{{1, 8, 5}: 9},
	}
	attacker := newAttackTestPlayer(world)
	attacker.Cfg = &config.Config{Combat: config.Combat{WeaponRanges: []config.WeaponRange{{Weapon: 297, Range: 3, Arrows: true}}}}
	attacker.Equipment.Weapon = 297
	attacker.Equipment.Shield = 0

	err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
	if err != nil {
		t.Fatalf("handleAttackUse returned error: %v", err)
	}

	if attacker.CharDirection != 0 {
		t.Fatalf("expected missing npc ammo to leave direction unchanged, got %d", attacker.CharDirection)
	}
	if len(world.broadcastPackets) != 0 {
		t.Fatalf("expected missing npc ammo to stop before broadcasting attack animation, got %d broadcasts", len(world.broadcastPackets))
	}
	if world.damageNpcCalls != 0 {
		t.Fatalf("expected missing npc ammo to stop before npc damage, got %d npc damage calls", world.damageNpcCalls)
	}
	if len(world.getPlayerSessionCalls) != 0 {
		t.Fatalf("expected missing npc ammo to stop before target resolution, got %d lookups", len(world.getPlayerSessionCalls))
	}
}

func TestHandleAttackUseArrowRequiredWeaponWithoutArrowsDoesNothing(t *testing.T) {
	setAttackTestItemDB(t, map[int]eopub.ItemSubtype{11: eopub.ItemSubtype_None})

	world := &attackTargetTestWorld{
		playersAt:             map[[3]int]int{{1, 8, 5}: 2},
		attackablePairs:       map[[3]int]bool{{1, 1, 2}: true},
		playerSessions:        map[int]*player.Player{},
		getPlayerSessionCalls: make(chan int, 1),
	}
	world.playerSessions[2] = &player.Player{ID: 2, State: player.StateInGame, World: world, MapID: 1, CharHP: 10}
	attacker := newAttackTestPlayer(world)
	attacker.Cfg = &config.Config{Combat: config.Combat{WeaponRanges: []config.WeaponRange{{Weapon: 297, Range: 3, Arrows: true}}}}
	attacker.Equipment.Weapon = 297
	attacker.Equipment.Shield = 11

	err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
	if err != nil {
		t.Fatalf("handleAttackUse returned error: %v", err)
	}

	if attacker.CharDirection != 0 {
		t.Fatalf("expected missing arrows to leave direction unchanged, got %d", attacker.CharDirection)
	}
	if len(world.broadcastPackets) != 0 {
		t.Fatalf("expected missing arrows to stop before broadcasting attack animation, got %d broadcasts", len(world.broadcastPackets))
	}
	if len(world.getPlayerSessionCalls) != 0 {
		t.Fatalf("expected missing arrows to stop before target resolution, got %d lookups", len(world.getPlayerSessionCalls))
	}
	if world.damageNpcCalls != 0 {
		t.Fatalf("expected missing arrows to stop before npc damage, got %d npc damage calls", world.damageNpcCalls)
	}
}

func TestHandleAttackUseArrowRequiredWeaponWithArrowsEquippedProceeds(t *testing.T) {
	setAttackTestItemDB(t, map[int]eopub.ItemSubtype{10: eopub.ItemSubtype_Arrows})

	world := &attackTargetTestWorld{
		playersAt:             map[[3]int]int{{1, 8, 5}: 2},
		attackablePairs:       map[[3]int]bool{{1, 1, 2}: true},
		playerSessions:        map[int]*player.Player{},
		getPlayerSessionCalls: make(chan int, 1),
	}
	target := &player.Player{ID: 2, State: player.StateInGame, World: world, MapID: 1, CharHP: 10}
	world.playerSessions[target.ID] = target
	attacker := newAttackTestPlayer(world)
	attacker.Cfg = &config.Config{Combat: config.Combat{WeaponRanges: []config.WeaponRange{{Weapon: 297, Range: 3, Arrows: true}}}}
	attacker.Equipment.Weapon = 297
	attacker.Equipment.Shield = 10

	err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
	if err != nil {
		t.Fatalf("handleAttackUse returned error: %v", err)
	}

	if attacker.CharDirection != int(eoproto.Direction_Right) {
		t.Fatalf("expected attacker direction %d, got %d", eoproto.Direction_Right, attacker.CharDirection)
	}
	if len(world.broadcastPackets) == 0 {
		t.Fatal("expected ranged attack animation broadcast when arrows are equipped")
	}

	select {
	case playerID := <-world.getPlayerSessionCalls:
		if playerID != target.ID {
			t.Fatalf("expected player session lookup for %d, got %d", target.ID, playerID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected arrow-equipped ranged attack to resolve player target")
	}
}

func TestHandleAttackUseNpcKillDoesNotDeadlockWhilePlayerMutexHeld(t *testing.T) {
	stubAttackCombatRolls(t, true)

	world := &attackTargetTestWorld{
		npcsAt:                map[[3]int]int{{1, 6, 5}: 9},
		damageNpcResult:       attackNpcDamageResult{actualDamage: 5, killed: true},
		getPlayerSessionCalls: make(chan int, 1),
	}
	attacker := newAttackTestPlayer(world)
	attacker.QuestProgress.SetQuestState(1, "Begin")
	bus, packets := newAttackTestPacketBus(t)
	attacker.Bus = bus

	done := make(chan error, 1)
	go func() {
		attacker.Mu.Lock()
		defer attacker.Mu.Unlock()
		done <- handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("handleAttackUse returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("handleAttackUse deadlocked after killing npc")
	}

	pkt := waitForAttackPacket(t, packets)
	if pkt.family != eonet.PacketFamily_Npc {
		t.Fatalf("expected npc packet after kill, got family=%d action=%d", pkt.family, pkt.action)
	}
	if pkt.action != eonet.PacketAction_Spec && pkt.action != eonet.PacketAction_Accept {
		t.Fatalf("expected npc spec/accept packet after kill, got family=%d action=%d", pkt.family, pkt.action)
	}
	if world.damageNpcCalls != 1 {
		t.Fatalf("expected one npc damage call, got %d", world.damageNpcCalls)
	}
	if attacker.CharExp == 0 {
		t.Fatal("expected npc kill to award experience")
	}
	questState := attacker.QuestProgress.ActiveQuests[1]
	if questState == nil || questState.NpcKills[0] != 1 {
		t.Fatal("expected npc kill progress to be recorded")
	}
	if attacker.CharDirection != int(eoproto.Direction_Right) {
		t.Fatalf("expected attacker direction %d, got %d", eoproto.Direction_Right, attacker.CharDirection)
	}
	if len(world.broadcastPackets) == 0 {
		t.Fatal("expected attack animation broadcast before kill packet")
	}
}

func TestHandleAttackUseNpcMissUsesNpcEvadeAndSendsMissPacket(t *testing.T) {
	setAttackTestNpcDB(t, map[int]eopub.EnfRecord{1: {Evade: 7}})

	world := &attackTargetTestWorld{
		npcsAt:          map[[3]int]int{{1, 6, 5}: 9},
		npcEnfIDByIndex: map[int]int{9: 1},
		npcHpPercentage: 63,
		playerSessions:  map[int]*player.Player{},
	}
	attacker := newAttackTestPlayer(world)
	b, packets := newAttackTestPacketBus(t)
	attacker.Bus = b
	attacker.Accuracy = 11

	prevHitRoll := combatHitRoll
	t.Cleanup(func() { combatHitRoll = prevHitRoll })
	observedAccuracy := 0
	observedEvade := 0
	combatHitRoll = func(accuracy, evade int) bool {
		observedAccuracy = accuracy
		observedEvade = evade
		return false
	}

	err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
	if err != nil {
		t.Fatalf("handleAttackUse returned error: %v", err)
	}

	if observedAccuracy != 11 || observedEvade != 7 {
		t.Fatalf("expected hit roll inputs (11,7), got (%d,%d)", observedAccuracy, observedEvade)
	}
	if world.damageNpcCalls != 0 {
		t.Fatalf("expected npc miss to skip damage call, got %d", world.damageNpcCalls)
	}

	pkt := waitForAttackPacket(t, packets)
	wantReplyPacket := server.NpcReplyServerPacket{}
	if pkt.action != wantReplyPacket.Action() || pkt.family != wantReplyPacket.Family() {
		t.Fatalf("expected npc reply packet, got family=%d action=%d", pkt.family, pkt.action)
	}

	var reply server.NpcReplyServerPacket
	if err := reply.Deserialize(data.NewEoReader(pkt.payload)); err != nil {
		t.Fatalf("deserialize npc miss reply: %v", err)
	}
	if reply.NpcIndex != 9 || reply.Damage != 0 || reply.HpPercentage != 63 {
		t.Fatalf("unexpected miss reply: %+v", reply)
	}
}

func TestHandleAttackUseNpcHitUsesNpcArmorInDamageRoll(t *testing.T) {
	setAttackTestNpcDB(t, map[int]eopub.EnfRecord{1: {Armor: 9}})

	world := &attackTargetTestWorld{
		npcsAt:          map[[3]int]int{{1, 6, 5}: 9},
		npcEnfIDByIndex: map[int]int{9: 1},
		damageNpcResult: attackNpcDamageResult{actualDamage: 4, hpPct: 55},
		playerSessions:  map[int]*player.Player{},
	}
	attacker := newAttackTestPlayer(world)
	b, packets := newAttackTestPacketBus(t)
	attacker.Bus = b
	attacker.MinDamage = 3
	attacker.MaxDamage = 8

	prevHitRoll := combatHitRoll
	prevDamageRoll := combatDamageRoll
	t.Cleanup(func() {
		combatHitRoll = prevHitRoll
		combatDamageRoll = prevDamageRoll
	})

	observedMin := 0
	observedMax := 0
	observedArmor := 0
	combatHitRoll = func(int, int) bool { return true }
	combatDamageRoll = func(minDamage, maxDamage, armor int) int {
		observedMin = minDamage
		observedMax = maxDamage
		observedArmor = armor
		return 4
	}

	err := handleAttackUse(context.Background(), attacker, newAttackUseReader(t, eoproto.Direction_Right))
	if err != nil {
		t.Fatalf("handleAttackUse returned error: %v", err)
	}

	if observedMin != 3 || observedMax != 8 || observedArmor != 9 {
		t.Fatalf("expected damage roll inputs (3,8,9), got (%d,%d,%d)", observedMin, observedMax, observedArmor)
	}
	if world.damageNpcCalls != 1 {
		t.Fatalf("expected one npc damage call, got %d", world.damageNpcCalls)
	}
	if world.lastDamageNpcCall.damage != 4 {
		t.Fatalf("expected armor-aware damage roll 4, got %d", world.lastDamageNpcCall.damage)
	}

	pkt := waitForAttackPacket(t, packets)
	wantReplyPacket := server.NpcReplyServerPacket{}
	if pkt.action != wantReplyPacket.Action() || pkt.family != wantReplyPacket.Family() {
		t.Fatalf("expected npc reply packet, got family=%d action=%d", pkt.family, pkt.action)
	}

	var reply server.NpcReplyServerPacket
	if err := reply.Deserialize(data.NewEoReader(pkt.payload)); err != nil {
		t.Fatalf("deserialize npc hit reply: %v", err)
	}
	if reply.Damage != 4 || reply.HpPercentage != 55 {
		t.Fatalf("unexpected npc hit reply: %+v", reply)
	}
}

func newAttackTestPlayer(world *attackTargetTestWorld) *player.Player {
	attacker := &player.Player{
		ID:            1,
		State:         player.StateInGame,
		MapID:         1,
		CharX:         5,
		CharY:         5,
		CharHP:        10,
		World:         world,
		Cfg:           &config.Config{},
		MinDamage:     1,
		MaxDamage:     1,
		Accuracy:      1,
		QuestProgress: player.NewQuestProgress(),
	}
	if world.playerSessions != nil {
		world.playerSessions[attacker.ID] = attacker
	}
	return attacker
}

func newAttackUseReader(t *testing.T, direction eoproto.Direction) *player.EoReader {
	t.Helper()

	writer := data.NewEoWriter()
	pkt := client.AttackUseClientPacket{Direction: direction, Timestamp: 123}
	if err := pkt.Serialize(writer); err != nil {
		t.Fatalf("serialize attack packet: %v", err)
	}

	return data.NewEoReader(writer.Array())
}

func setAttackTestItemDB(t *testing.T, subtypes map[int]eopub.ItemSubtype) {
	t.Helper()

	prev := pubdata.ItemDB
	t.Cleanup(func() {
		pubdata.ItemDB = prev
	})

	maxID := 0
	for itemID := range subtypes {
		if itemID > maxID {
			maxID = itemID
		}
	}

	itemDB := &eopub.Eif{Items: make([]eopub.EifRecord, maxID)}
	for itemID, subtype := range subtypes {
		itemDB.Items[itemID-1] = eopub.EifRecord{Subtype: subtype}
	}
	pubdata.ItemDB = itemDB
}

func setAttackTestNpcDB(t *testing.T, npcs map[int]eopub.EnfRecord) {
	t.Helper()

	prev := pubdata.NpcDB
	t.Cleanup(func() {
		pubdata.NpcDB = prev
	})

	maxID := 0
	for npcID := range npcs {
		if npcID > maxID {
			maxID = npcID
		}
	}

	npcDB := &eopub.Enf{Npcs: make([]eopub.EnfRecord, maxID)}
	for npcID, record := range npcs {
		npcDB.Npcs[npcID-1] = record
	}
	pubdata.NpcDB = npcDB
}

func stubAttackCombatRolls(t *testing.T, hit bool) {
	t.Helper()

	prevHitRoll := combatHitRoll
	prevDamageRoll := combatDamageRoll
	t.Cleanup(func() {
		combatHitRoll = prevHitRoll
		combatDamageRoll = prevDamageRoll
	})

	combatHitRoll = func(int, int) bool { return hit }
	combatDamageRoll = func(minDamage, _, _ int) int { return minDamage }
}

type attackTargetTestWorld struct {
	playersAt             map[[3]int]int
	npcsAt                map[[3]int]int
	blockedTiles          map[[3]int]bool
	attackablePairs       map[[3]int]bool
	captchas              map[int]bool
	playerSessions        map[int]*player.Player
	npcEnfIDByIndex       map[int]int
	npcHpPercentage       int
	broadcastPackets      []any
	damageNpcCalls        int
	lastDamageNpcCall     attackNpcDamageCall
	damageNpcResult       attackNpcDamageResult
	getPlayerSessionCalls chan int
}

type attackNpcDamageCall struct {
	mapID    int
	npcIndex int
	playerID int
	damage   int
}

type attackNpcDamageResult struct {
	actualDamage int
	killed       bool
	hpPct        int
}

func (w *attackTargetTestWorld) tileKey(mapID, x, y int) [3]int {
	return [3]int{mapID, x, y}
}

func (w *attackTargetTestWorld) EnterMap(int, any)                     {}
func (w *attackTargetTestWorld) BindPlayerSession(int, *player.Player) {}
func (w *attackTargetTestWorld) LeaveMap(int, int)                     {}
func (w *attackTargetTestWorld) Walk(int, int, int, [2]int)            {}
func (w *attackTargetTestWorld) Face(int, int, int)                    {}
func (w *attackTargetTestWorld) BroadcastMap(_ int, _ int, pkt any) {
	w.broadcastPackets = append(w.broadcastPackets, pkt)
}
func (w *attackTargetTestWorld) BroadcastGlobal(int, any)            {}
func (w *attackTargetTestWorld) BroadcastToAdmins(int, int, any)     {}
func (w *attackTargetTestWorld) SendToPlayer(int, any)               {}
func (w *attackTargetTestWorld) FindPlayerByName(string) (int, bool) { return 0, false }
func (w *attackTargetTestWorld) GetNearbyInfo(int) any               { return nil }

func (w *attackTargetTestWorld) DamageNpc(mapID, npcIndex, playerID, damage int) (int, bool, int) {
	w.damageNpcCalls++
	w.lastDamageNpcCall = attackNpcDamageCall{mapID: mapID, npcIndex: npcIndex, playerID: playerID, damage: damage}
	return w.damageNpcResult.actualDamage, w.damageNpcResult.killed, w.damageNpcResult.hpPct
}
func (w *attackTargetTestWorld) GetNpcHpPercentage(int, int) int           { return w.npcHpPercentage }
func (w *attackTargetTestWorld) DropItem(int, int, int, int, int, int) int { return 0 }
func (w *attackTargetTestWorld) PickupItem(int, int, int) (int, int, bool) { return 0, 0, false }
func (w *attackTargetTestWorld) GetPlayerBus(int) any                      { return nil }
func (w *attackTargetTestWorld) GetPlayerSession(playerID int) *player.Player {
	if w.getPlayerSessionCalls != nil {
		select {
		case w.getPlayerSessionCalls <- playerID:
		default:
		}
	}
	return w.playerSessions[playerID]
}
func (w *attackTargetTestWorld) IsPkMap(int) bool                           { return false }
func (w *attackTargetTestWorld) HandlePlayerDefeat(int, int, int, int) bool { return false }
func (w *attackTargetTestWorld) UpdatePlayerVitals(int, int, int, int)      {}
func (w *attackTargetTestWorld) UpdatePlayerCombatStats(int, int, int, int) {}
func (w *attackTargetTestWorld) UpdatePlayerCombatSnapshot(int, int, int, int, int, int, int, int) {
}
func (w *attackTargetTestWorld) OnlinePlayerCount() int                               { return 0 }
func (w *attackTargetTestWorld) IsLoggedIn(int) bool                                  { return false }
func (w *attackTargetTestWorld) AddLoggedInAccount(int)                               {}
func (w *attackTargetTestWorld) GetOnlinePlayers() any                                { return nil }
func (w *attackTargetTestWorld) WarpPlayer(int, int, int, int, int) any               { return nil }
func (w *attackTargetTestWorld) GetPendingWarp(int, int) (int, int, int, bool)        { return 0, 0, 0, false }
func (w *attackTargetTestWorld) SetPendingWarp(int, int, int, int, int)               {}
func (w *attackTargetTestWorld) GetPlayerName(int) string                             { return "" }
func (w *attackTargetTestWorld) GetPlayerPosition(int) any                            { return nil }
func (w *attackTargetTestWorld) UpdateMapEquipment(int, int, int, int, int, int, int) {}
func (w *attackTargetTestWorld) BroadcastToGuild(int, string, any)                    {}
func (w *attackTargetTestWorld) BroadcastToParty(int, any)                            {}
func (w *attackTargetTestWorld) GetNpcEnfID(_ int, npcIndex int) int {
	if w.npcEnfIDByIndex == nil {
		return 0
	}
	return w.npcEnfIDByIndex[npcIndex]
}
func (w *attackTargetTestWorld) OpenDoor(int, int, int, int) bool            { return false }
func (w *attackTargetTestWorld) SetMutedUntil(int, time.Time)                {}
func (w *attackTargetTestWorld) ClearMuted(int)                              {}
func (w *attackTargetTestWorld) GetMutedUntil(int) (time.Time, bool)         { return time.Time{}, false }
func (w *attackTargetTestWorld) IsMuted(int) bool                            { return false }
func (w *attackTargetTestWorld) StartCaptcha(int, int) bool                  { return false }
func (w *attackTargetTestWorld) RefreshCaptcha(int) bool                     { return false }
func (w *attackTargetTestWorld) VerifyCaptcha(int, string) (int, bool)       { return 0, false }
func (w *attackTargetTestWorld) HasCaptcha(playerID int) bool                { return w.captchas[playerID] }
func (w *attackTargetTestWorld) GetChestItems(int, int, int) any             { return nil }
func (w *attackTargetTestWorld) AddChestItem(int, int, int, int, int) any    { return nil }
func (w *attackTargetTestWorld) TakeChestItem(int, int, int, int) (int, any) { return 0, nil }
func (w *attackTargetTestWorld) StartEvacuate(int, int)                      {}
func (w *attackTargetTestWorld) TryStartJukebox(int, int) bool               { return false }

func (w *attackTargetTestWorld) GetNpcAt(mapID, x, y int) int {
	if npcIndex, ok := w.npcsAt[w.tileKey(mapID, x, y)]; ok {
		return npcIndex
	}
	return -1
}

func (w *attackTargetTestWorld) GetPlayerAt(mapID, x, y int) int {
	return w.playersAt[w.tileKey(mapID, x, y)]
}

func (w *attackTargetTestWorld) IsAttackTileBlocked(mapID, x, y int) bool {
	return w.blockedTiles[w.tileKey(mapID, x, y)]
}

func (w *attackTargetTestWorld) CanPlayerAttackPlayer(mapID, attackerID, targetID int) bool {
	if attackerID <= 0 || targetID <= 0 || attackerID == targetID {
		return false
	}

	attacker := w.playerSessions[attackerID]
	target := w.playerSessions[targetID]
	if attacker == nil || target == nil {
		return false
	}
	if attacker.State != player.StateInGame || target.State != player.StateInGame {
		return false
	}
	if attacker.World == nil || target.World == nil {
		return false
	}
	if attacker.MapID != mapID || target.MapID != mapID {
		return false
	}
	if attacker.CharHP <= 0 || target.CharHP <= 0 {
		return false
	}

	return w.attackablePairs[[3]int{mapID, attackerID, targetID}]
}

type attackTestPacket struct {
	action  eonet.PacketAction
	family  eonet.PacketFamily
	payload []byte
}

func newAttackTestPacketBus(t *testing.T) (*protocol.PacketBus, <-chan attackTestPacket) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	bus := protocol.NewPacketBus(protocol.NewTCPConn(clientConn))
	receiverConn := protocol.NewTCPConn(serverConn)
	packets := make(chan attackTestPacket, 4)

	go func() {
		defer close(packets)
		for {
			rawPacket, err := receiverConn.ReadPacket()
			if err != nil {
				return
			}
			packets <- attackTestPacket{
				action:  eonet.PacketAction(rawPacket[0]),
				family:  eonet.PacketFamily(rawPacket[1]),
				payload: append([]byte(nil), rawPacket[2:]...),
			}
		}
	}()

	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})

	return bus, packets
}

func waitForAttackPacket(t *testing.T, packets <-chan attackTestPacket) attackTestPacket {
	t.Helper()

	select {
	case pkt, ok := <-packets:
		if !ok {
			t.Fatal("expected packet from attack handler bus")
		}
		return pkt
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for attack handler packet")
		return attackTestPacket{}
	}
}

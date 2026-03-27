package handlers

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/protocol"
	"github.com/ethanmoffat/eolib-go/v3/data"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func TestHandleSpellTargetOtherRoutesNpcTargetThroughHandlers(t *testing.T) {
	world := &spellTargetTestWorld{
		attackTargetTestWorld: attackTargetTestWorld{
			npcsAt:         map[[3]int]int{{1, 7, 5}: 9},
			playerSessions: map[int]*player.Player{},
		},
		npcEnfIDs:       map[[2]int]int{{1, 9}: 1},
		damageNpcResult: spellNpcDamageResult{actualDamage: 12, hpPct: 80},
	}
	bus, sentPackets := newSpellTestPacketBus(t)
	caster := newSpellTestPlayer(world, bus)
	caster.Spells = append(caster.Spells, player.SpellState{ID: 8, Level: 2})

	if err := handleSpellRequest(context.Background(), caster, newSpellRequestReader(t, 7, 100)); err != nil {
		t.Fatalf("handleSpellRequest returned error: %v", err)
	}

	if err := handleSpellTargetOther(context.Background(), caster, newSpellTargetOtherReader(t, client.SpellTarget_Npc, 100, 7, 9, 101)); err != nil {
		t.Fatalf("handleSpellTargetOther returned error: %v", err)
	}

	if world.damageNpcCalls != 1 {
		t.Fatalf("expected one npc damage call, got %d", world.damageNpcCalls)
	}
	if world.lastDamageNpcCall.npcIndex != 9 {
		t.Fatalf("expected npc target 9, got %d", world.lastDamageNpcCall.npcIndex)
	}
	if world.lastDamageNpcCall.playerID != caster.ID {
		t.Fatalf("expected caster %d to damage npc, got %d", caster.ID, world.lastDamageNpcCall.playerID)
	}
	if caster.CharDirection != int(eoproto.Direction_Right) {
		t.Fatalf("expected caster direction %d, got %d", eoproto.Direction_Right, caster.CharDirection)
	}
	if caster.CharTP != 25 {
		t.Fatalf("expected caster TP 25 after spell cost, got %d", caster.CharTP)
	}
	if caster.PendingSpell != nil {
		t.Fatal("expected pending spell to clear after successful npc cast")
	}
	if caster.LastSpellCast != 101 {
		t.Fatalf("expected last spell cast timestamp 101, got %d", caster.LastSpellCast)
	}

	attackerPacket := waitForSpellPacket(t, sentPackets)
	assertPacketIdentity(t, attackerPacket, server.CastReplyServerPacket{}.Action(), server.CastReplyServerPacket{}.Family())

	var castReply server.CastReplyServerPacket
	if err := castReply.Deserialize(data.NewEoReader(attackerPacket.payload)); err != nil {
		t.Fatalf("deserialize cast reply: %v", err)
	}
	if castReply.SpellId != 7 {
		t.Fatalf("expected spell id 7, got %d", castReply.SpellId)
	}
	if castReply.NpcIndex != 9 {
		t.Fatalf("expected npc index 9, got %d", castReply.NpcIndex)
	}
	if castReply.Damage != 12 {
		t.Fatalf("expected actual damage 12, got %d", castReply.Damage)
	}
	if castReply.CasterTp == nil || *castReply.CasterTp != 25 {
		t.Fatalf("expected caster TP payload 25, got %+v", castReply.CasterTp)
	}

	if len(world.broadcastPackets) != 2 {
		t.Fatalf("expected spell request and cast reply broadcasts, got %d", len(world.broadcastPackets))
	}
	if _, ok := world.broadcastPackets[0].(*server.SpellRequestServerPacket); !ok {
		t.Fatalf("expected first broadcast to be SpellRequestServerPacket, got %T", world.broadcastPackets[0])
	}
	observerReply, ok := world.broadcastPackets[1].(*server.CastReplyServerPacket)
	if !ok {
		t.Fatalf("expected second broadcast to be CastReplyServerPacket, got %T", world.broadcastPackets[1])
	}
	if observerReply.CasterTp != nil {
		t.Fatalf("expected observer cast reply to omit caster TP, got %+v", observerReply.CasterTp)
	}

	if len(world.updatePlayerVitalsCalls) != 1 {
		t.Fatalf("expected one vitals update, got %d", len(world.updatePlayerVitalsCalls))
	}
	vitals := world.updatePlayerVitalsCalls[0]
	if vitals.playerID != caster.ID || vitals.tp != 25 {
		t.Fatalf("expected caster vitals update with TP 25, got %+v", vitals)
	}
}

func TestHandleSpellTargetOtherRejectsOutOfRangeNpcTarget(t *testing.T) {
	world := &spellTargetTestWorld{
		attackTargetTestWorld: attackTargetTestWorld{
			npcsAt:         map[[3]int]int{{1, 11, 5}: 9},
			playerSessions: map[int]*player.Player{},
		},
	}
	bus, sentPackets := newSpellTestPacketBus(t)
	caster := newSpellTestPlayer(world, bus)

	if err := handleSpellRequest(context.Background(), caster, newSpellRequestReader(t, 7, 100)); err != nil {
		t.Fatalf("handleSpellRequest returned error: %v", err)
	}

	if err := handleSpellTargetOther(context.Background(), caster, newSpellTargetOtherReader(t, client.SpellTarget_Npc, 100, 7, 9, 101)); err != nil {
		t.Fatalf("handleSpellTargetOther returned error: %v", err)
	}

	if world.damageNpcCalls != 0 {
		t.Fatalf("expected invalid npc target to stop before damage, got %d damage calls", world.damageNpcCalls)
	}
	if caster.CharTP != 40 {
		t.Fatalf("expected rejected spell to preserve TP 40, got %d", caster.CharTP)
	}
	if caster.PendingSpell != nil {
		t.Fatal("expected rejected spell to clear pending chant state")
	}
	if caster.LastSpellCast != 0 {
		t.Fatalf("expected rejected spell to leave last cast timestamp unchanged, got %d", caster.LastSpellCast)
	}
	if len(world.broadcastPackets) != 1 {
		t.Fatalf("expected only spell request broadcast before rejection, got %d", len(world.broadcastPackets))
	}
	if len(world.updatePlayerVitalsCalls) != 0 {
		t.Fatalf("expected rejected spell to avoid vitals updates, got %d", len(world.updatePlayerVitalsCalls))
	}

	rejectionPacket := waitForSpellPacket(t, sentPackets)
	assertPacketIdentity(t, rejectionPacket, server.SpellErrorServerPacket{}.Action(), server.SpellErrorServerPacket{}.Family())
	if err := (&server.SpellErrorServerPacket{}).Deserialize(data.NewEoReader(rejectionPacket.payload)); err != nil {
		t.Fatalf("deserialize spell error: %v", err)
	}
}

func TestHandleSpellTargetOtherRejectsNpcTargetWhenPreviousTimestampMismatches(t *testing.T) {
	world := &spellTargetTestWorld{
		attackTargetTestWorld: attackTargetTestWorld{
			npcsAt:         map[[3]int]int{{1, 7, 5}: 9},
			playerSessions: map[int]*player.Player{},
		},
	}
	bus, sentPackets := newSpellTestPacketBus(t)
	caster := newSpellTestPlayer(world, bus)

	if err := handleSpellRequest(context.Background(), caster, newSpellRequestReader(t, 7, 100)); err != nil {
		t.Fatalf("handleSpellRequest returned error: %v", err)
	}

	if err := handleSpellTargetOther(context.Background(), caster, newSpellTargetOtherReader(t, client.SpellTarget_Npc, 99, 7, 9, 101)); err != nil {
		t.Fatalf("handleSpellTargetOther returned error: %v", err)
	}

	assertNpcSpellRejectionState(t, world, caster)

	rejectionPacket := waitForSpellPacket(t, sentPackets)
	assertPacketIdentity(t, rejectionPacket, server.SpellErrorServerPacket{}.Action(), server.SpellErrorServerPacket{}.Family())
	if err := (&server.SpellErrorServerPacket{}).Deserialize(data.NewEoReader(rejectionPacket.payload)); err != nil {
		t.Fatalf("deserialize spell error: %v", err)
	}
}

func TestHandleSpellTargetOtherRejectsNpcTargetWhenPendingSpellExpired(t *testing.T) {
	world := &spellTargetTestWorld{
		attackTargetTestWorld: attackTargetTestWorld{
			npcsAt:         map[[3]int]int{{1, 7, 5}: 9},
			playerSessions: map[int]*player.Player{},
		},
	}
	bus, sentPackets := newSpellTestPacketBus(t)
	caster := newSpellTestPlayer(world, bus)

	if err := handleSpellRequest(context.Background(), caster, newSpellRequestReader(t, 7, 100)); err != nil {
		t.Fatalf("handleSpellRequest returned error: %v", err)
	}
	caster.PendingSpell.StartedAt = time.Now().Add(-spellCastWindow - time.Millisecond)

	if err := handleSpellTargetOther(context.Background(), caster, newSpellTargetOtherReader(t, client.SpellTarget_Npc, 100, 7, 9, 101)); err != nil {
		t.Fatalf("handleSpellTargetOther returned error: %v", err)
	}

	assertNpcSpellRejectionState(t, world, caster)

	rejectionPacket := waitForSpellPacket(t, sentPackets)
	assertPacketIdentity(t, rejectionPacket, server.SpellErrorServerPacket{}.Action(), server.SpellErrorServerPacket{}.Family())
	if err := (&server.SpellErrorServerPacket{}).Deserialize(data.NewEoReader(rejectionPacket.payload)); err != nil {
		t.Fatalf("deserialize spell error: %v", err)
	}
}

func TestHandleSpellTargetOtherRejectsNpcTargetWhenRequestedSpellDiffersFromPendingChant(t *testing.T) {
	world := &spellTargetTestWorld{
		attackTargetTestWorld: attackTargetTestWorld{
			npcsAt:         map[[3]int]int{{1, 7, 5}: 9},
			playerSessions: map[int]*player.Player{},
		},
	}
	bus, sentPackets := newSpellTestPacketBus(t)
	caster := newSpellTestPlayer(world, bus)

	if err := handleSpellRequest(context.Background(), caster, newSpellRequestReader(t, 7, 100)); err != nil {
		t.Fatalf("handleSpellRequest returned error: %v", err)
	}

	if err := handleSpellTargetOther(context.Background(), caster, newSpellTargetOtherReader(t, client.SpellTarget_Npc, 100, 8, 9, 101)); err != nil {
		t.Fatalf("handleSpellTargetOther returned error: %v", err)
	}

	assertNpcSpellRejectionState(t, world, caster)

	rejectionPacket := waitForSpellPacket(t, sentPackets)
	assertPacketIdentity(t, rejectionPacket, server.SpellErrorServerPacket{}.Action(), server.SpellErrorServerPacket{}.Family())
	if err := (&server.SpellErrorServerPacket{}).Deserialize(data.NewEoReader(rejectionPacket.payload)); err != nil {
		t.Fatalf("deserialize spell error: %v", err)
	}
}

func assertNpcSpellRejectionState(t *testing.T, world *spellTargetTestWorld, caster *player.Player) {
	t.Helper()

	if world.damageNpcCalls != 0 {
		t.Fatalf("expected invalid npc target to stop before damage, got %d damage calls", world.damageNpcCalls)
	}
	if caster.CharTP != 40 {
		t.Fatalf("expected rejected spell to preserve TP 40, got %d", caster.CharTP)
	}
	if caster.PendingSpell != nil {
		t.Fatal("expected rejected spell to clear pending chant state")
	}
	if caster.LastSpellCast != 0 {
		t.Fatalf("expected rejected spell to leave last cast timestamp unchanged, got %d", caster.LastSpellCast)
	}
	if len(world.broadcastPackets) != 1 {
		t.Fatalf("expected only spell request broadcast before rejection, got %d", len(world.broadcastPackets))
	}
	if len(world.updatePlayerVitalsCalls) != 0 {
		t.Fatalf("expected rejected spell to avoid vitals updates, got %d", len(world.updatePlayerVitalsCalls))
	}
}

type spellTargetTestWorld struct {
	attackTargetTestWorld
	npcEnfIDs               map[[2]int]int
	damageNpcResult         spellNpcDamageResult
	updatePlayerVitalsCalls []spellVitalsUpdateCall
}

type spellNpcDamageResult struct {
	actualDamage int
	killed       bool
	hpPct        int
}

type spellVitalsUpdateCall struct {
	mapID    int
	playerID int
	hp       int
	tp       int
}

func (w *spellTargetTestWorld) DamageNpc(mapID, npcIndex, playerID, damage int) (int, bool, int) {
	w.attackTargetTestWorld.DamageNpc(mapID, npcIndex, playerID, damage)
	return w.damageNpcResult.actualDamage, w.damageNpcResult.killed, w.damageNpcResult.hpPct
}

func (w *spellTargetTestWorld) GetNpcEnfID(mapID, npcIndex int) int {
	if w.npcEnfIDs == nil {
		return 0
	}
	return w.npcEnfIDs[[2]int{mapID, npcIndex}]
}

func (w *spellTargetTestWorld) UpdatePlayerVitals(mapID, playerID, hp, tp int) {
	w.updatePlayerVitalsCalls = append(w.updatePlayerVitalsCalls, spellVitalsUpdateCall{mapID: mapID, playerID: playerID, hp: hp, tp: tp})
}

type spellTestPacket struct {
	action  eonet.PacketAction
	family  eonet.PacketFamily
	payload []byte
}

func newSpellTestPlayer(world player.WorldInterface, bus *protocol.PacketBus) *player.Player {
	p := &player.Player{
		ID:        1,
		State:     player.StateInGame,
		MapID:     1,
		CharX:     5,
		CharY:     5,
		CharHP:    30,
		CharMaxHP: 30,
		CharTP:    40,
		CharMaxTP: 40,
		World:     world,
		Bus:       bus,
		Cfg:       &config.Config{},
		Stats:     player.CharacterStats{Intl: 4, Wis: 2},
		Spells:    []player.SpellState{{ID: 7, Level: 3}},
		QuestProgress: &player.QuestProgressTracker{
			ActiveQuests:    map[int]*player.QuestState{},
			CompletedQuests: map[int]bool{},
		},
	}

	if spellWorld, ok := world.(*spellTargetTestWorld); ok && spellWorld.playerSessions != nil {
		spellWorld.playerSessions[p.ID] = p
	}

	return p
}

func newSpellRequestReader(t *testing.T, spellID, timestamp int) *player.EoReader {
	t.Helper()

	writer := data.NewEoWriter()
	pkt := client.SpellRequestClientPacket{SpellId: spellID, Timestamp: timestamp}
	if err := pkt.Serialize(writer); err != nil {
		t.Fatalf("serialize spell request packet: %v", err)
	}

	return data.NewEoReader(writer.Array())
}

func newSpellTargetOtherReader(t *testing.T, targetType client.SpellTargetType, previousTimestamp, spellID, victimID, timestamp int) *player.EoReader {
	t.Helper()

	writer := data.NewEoWriter()
	pkt := client.SpellTargetOtherClientPacket{
		TargetType:        targetType,
		PreviousTimestamp: previousTimestamp,
		SpellId:           spellID,
		VictimId:          victimID,
		Timestamp:         timestamp,
	}
	if err := pkt.Serialize(writer); err != nil {
		t.Fatalf("serialize spell target packet: %v", err)
	}

	return data.NewEoReader(writer.Array())
}

func newSpellTestPacketBus(t *testing.T) (*protocol.PacketBus, <-chan spellTestPacket) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	bus := protocol.NewPacketBus(protocol.NewTCPConn(clientConn))
	receiverConn := protocol.NewTCPConn(serverConn)
	packets := make(chan spellTestPacket, 4)

	go func() {
		defer close(packets)
		for {
			rawPacket, err := receiverConn.ReadPacket()
			if err != nil {
				return
			}
			packets <- spellTestPacket{
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

func waitForSpellPacket(t *testing.T, packets <-chan spellTestPacket) spellTestPacket {
	t.Helper()

	pkt, ok := <-packets
	if !ok {
		t.Fatal("expected packet from spell handler bus")
	}
	return pkt
}

func assertPacketIdentity(t *testing.T, pkt spellTestPacket, wantAction eonet.PacketAction, wantFamily eonet.PacketFamily) {
	t.Helper()

	if pkt.action != wantAction || pkt.family != wantFamily {
		t.Fatalf("expected packet (%d,%d), got (%d,%d)", wantAction, wantFamily, pkt.action, pkt.family)
	}
}

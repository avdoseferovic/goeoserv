package world

import (
	"fmt"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/player"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

type arenaPhase int

const (
	arenaPhaseWaiting arenaPhase = iota
	arenaPhaseCountdown
	arenaPhaseActive
)

type arenaParticipant struct {
	playerID int
	name     string
	kills    int
	queueX   int
	queueY   int
	launchX  int
	launchY  int
}

type arenaState struct {
	mapID          int
	config         *config.Arena
	phase          arenaPhase
	countdownTicks int
	participants   map[int]*arenaParticipant
}

func newArenaState(mapID int, arenaConfig *config.Arena) *arenaState {
	return &arenaState{mapID: mapID, config: arenaConfig, participants: make(map[int]*arenaParticipant)}
}

func (a *arenaState) countdownTicksForNextRound() int {
	if a == nil || a.config == nil {
		return 5 * 8
	}

	return a.config.CountdownTicks()
}

func (a *arenaState) minimumPlayersToStart() int {
	if a == nil || a.config == nil {
		return 2
	}

	return a.config.StartPlayerThreshold()
}

func (a *arenaState) participantLimit() int {
	if a == nil || a.config == nil {
		return 0
	}

	return a.config.ParticipantLimit()
}

func (a *arenaState) usesQueueSpawns() bool {
	return a != nil && a.config != nil && a.config.UsesQueueSpawns()
}

func (a *arenaState) queueSpawnAt(x, y int) *config.ArenaSpawn {
	if a == nil || a.config == nil {
		return nil
	}

	return a.config.QueueSpawnAt(x, y)
}

func (w *World) CanPlayerAttackPlayer(mapID, attackerID, targetID int) bool {
	if attackerID <= 0 || targetID <= 0 || attackerID == targetID {
		return false
	}
	attacker := w.GetPlayerSession(attackerID)
	target := w.GetPlayerSession(targetID)
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
	if partyMembersFightEachOther(attackerID, targetID) {
		return false
	}

	w.arenaMu.Lock()
	defer w.arenaMu.Unlock()

	arena := w.arenas[mapID]
	if arena == nil {
		return w.IsPkMap(mapID)
	}
	if arena.phase != arenaPhaseActive {
		return false
	}
	_, attackerJoined := arena.participants[attackerID]
	_, targetJoined := arena.participants[targetID]
	return attackerJoined && targetJoined
}

func partyMembersFightEachOther(attackerID, targetID int) bool {
	party := GetParty(attackerID)
	if party == nil {
		return false
	}

	party.mu.RLock()
	defer party.mu.RUnlock()
	for _, member := range party.Members {
		if member.PlayerID == targetID {
			return true
		}
	}
	return false
}

func (w *World) HandlePlayerDefeat(mapID, attackerID, targetID, direction int) bool {
	target := w.GetPlayerSession(targetID)
	if target == nil {
		return false
	}

	winnerID, winnerName, winnerKills, killerName, victimName, arenaKill := w.recordArenaDefeat(mapID, attackerID, targetID)
	if !arenaKill {
		return false
	}

	if target.CharMaxHP > 0 {
		target.CharHP = target.CharMaxHP
	}
	spawnMap := target.Cfg.Rescue.Map
	spawnX := target.Cfg.Rescue.X
	spawnY := target.Cfg.Rescue.Y
	if spawnMap <= 0 {
		spawnMap = target.Cfg.NewCharacter.SpawnMap
		spawnX = target.Cfg.NewCharacter.SpawnX
		spawnY = target.Cfg.NewCharacter.SpawnY
	}
	target.PendingWarp = &player.PendingWarp{MapID: spawnMap, X: spawnX, Y: spawnY}
	w.UpdatePlayerVitals(target.MapID, target.ID, target.CharHP, target.CharTP)
	_ = target.Bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: target.CharHP, Tp: target.CharTP})
	_ = target.Bus.SendPacket(&server.WarpRequestServerPacket{
		WarpType:     server.Warp_Local,
		MapId:        target.PendingWarp.MapID,
		WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
	})

	w.BroadcastMap(mapID, -1, &server.ArenaSpecServerPacket{
		PlayerId:   attackerID,
		Direction:  eoproto.Direction(direction),
		KillsCount: winnerKills,
		KillerName: killerName,
		VictimName: victimName,
	})

	if winnerID == 0 {
		return true
	}

	w.BroadcastMap(mapID, -1, &server.ArenaAcceptServerPacket{
		WinnerName: winnerName,
		KillsCount: winnerKills,
		KillerName: killerName,
		VictimName: victimName,
	})
	w.BroadcastMap(mapID, -1, &server.TalkServerServerPacket{Message: fmt.Sprintf("%s wins the arena!", winnerName)})
	w.refillArenaFromMap(mapID)
	return true
}

func (w *World) joinArenaIfEligible(mapID, playerID int, playerName string) {
	if playerID <= 0 || playerName == "" {
		return
	}

	m := w.getMap(mapID)
	if m == nil {
		return
	}

	position := m.GetPlayerPosition(playerID)
	if position == nil {
		return
	}

	w.arenaMu.Lock()
	arena := w.arenas[mapID]
	if arena == nil {
		w.arenaMu.Unlock()
		return
	}

	if _, joined := arena.participants[playerID]; joined {
		w.arenaMu.Unlock()
		return
	}

	spawn := arena.queueSpawnAt(position.X, position.Y)
	if arena.usesQueueSpawns() && spawn == nil {
		w.arenaMu.Unlock()
		return
	}

	if arena.phase == arenaPhaseActive {
		w.arenaMu.Unlock()
		w.SendToPlayer(playerID, &server.ArenaDropServerPacket{})
		w.SendToPlayer(playerID, &server.TalkServerServerPacket{Message: "Arena match in progress. You will join the next round."})
		return
	}

	participantLimit := arena.participantLimit()
	if participantLimit > 0 && len(arena.participants) >= participantLimit {
		w.arenaMu.Unlock()
		w.SendToPlayer(playerID, &server.ArenaDropServerPacket{})
		w.SendToPlayer(playerID, &server.TalkServerServerPacket{Message: "Arena queue is full."})
		return
	}

	participant := &arenaParticipant{playerID: playerID, name: playerName}
	if spawn != nil {
		participant.queueX = spawn.From.X
		participant.queueY = spawn.From.Y
		participant.launchX = spawn.To.X
		participant.launchY = spawn.To.Y
	}
	arena.participants[playerID] = participant
	playerCount := len(arena.participants)
	countdownSeconds := arena.countdownTicksForNextRound() / 8
	shouldStartCountdown := arena.phase == arenaPhaseWaiting && playerCount >= arena.minimumPlayersToStart()
	if shouldStartCountdown {
		arena.phase = arenaPhaseCountdown
		arena.countdownTicks = arena.countdownTicksForNextRound()
	}
	w.arenaMu.Unlock()

	w.SendToPlayer(playerID, &server.TalkServerServerPacket{Message: "You joined the arena."})
	if shouldStartCountdown {
		w.BroadcastMap(mapID, -1, &server.TalkServerServerPacket{Message: fmt.Sprintf("Arena starts in %d seconds.", countdownSeconds)})
		return
	}
	if playerCount == 1 {
		w.BroadcastMap(mapID, -1, &server.TalkServerServerPacket{Message: "Arena waiting for challengers."})
	}
}

func (w *World) syncArenaParticipation(mapID, playerID int, playerName string) {
	if !w.arenaUsesQueueSpawns(mapID) {
		w.joinArenaIfEligible(mapID, playerID, playerName)
		return
	}

	if w.removeArenaQueueParticipantIfMoved(mapID, playerID) {
		w.SendToPlayer(playerID, &server.TalkServerServerPacket{Message: "You left the arena queue."})
	}

	w.joinArenaIfEligible(mapID, playerID, playerName)
}

func (w *World) leaveArena(mapID, playerID int) {
	winnerID, winnerName, winnerKills, killerName, victimName, left := w.removeArenaParticipant(mapID, playerID)
	if !left {
		return
	}

	if winnerID == 0 {
		return
	}

	w.BroadcastMap(mapID, -1, &server.ArenaAcceptServerPacket{
		WinnerName: winnerName,
		KillsCount: winnerKills,
		KillerName: killerName,
		VictimName: victimName,
	})
	w.BroadcastMap(mapID, -1, &server.TalkServerServerPacket{Message: fmt.Sprintf("%s wins the arena!", winnerName)})
	w.refillArenaFromMap(mapID)
}

func (w *World) tickArenas() {
	type arenaLaunch struct {
		playerID int
		x        int
		y        int
	}

	type arenaTickEvent struct {
		mapID        int
		countdownMsg string
		startCount   int
		launches     []arenaLaunch
	}

	w.arenaMu.Lock()
	events := make([]arenaTickEvent, 0, len(w.arenas))
	for _, arena := range w.arenas {
		if arena.phase != arenaPhaseCountdown {
			continue
		}
		if len(arena.participants) < arena.minimumPlayersToStart() {
			arena.phase = arenaPhaseWaiting
			arena.countdownTicks = 0
			continue
		}

		arena.countdownTicks--
		if arena.countdownTicks <= 0 {
			arena.phase = arenaPhaseActive
			event := arenaTickEvent{mapID: arena.mapID, startCount: len(arena.participants)}
			if arena.usesQueueSpawns() {
				event.launches = make([]arenaLaunch, 0, len(arena.participants))
				for _, participant := range arena.participants {
					event.launches = append(event.launches, arenaLaunch{
						playerID: participant.playerID,
						x:        participant.launchX,
						y:        participant.launchY,
					})
				}
			}
			events = append(events, event)
			continue
		}
		if arena.countdownTicks%8 == 0 {
			events = append(events, arenaTickEvent{mapID: arena.mapID, countdownMsg: fmt.Sprintf("Arena starts in %d seconds.", arena.countdownTicks/8)})
		}
	}
	w.arenaMu.Unlock()

	for _, event := range events {
		if event.countdownMsg != "" {
			w.BroadcastMap(event.mapID, -1, &server.TalkServerServerPacket{Message: event.countdownMsg})
			continue
		}
		for _, launch := range event.launches {
			w.SetPendingWarp(event.mapID, launch.playerID, event.mapID, launch.x, launch.y)
			w.SendToPlayer(launch.playerID, &server.WarpRequestServerPacket{
				WarpType:     server.Warp_Local,
				MapId:        event.mapID,
				WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
			})
		}
		w.BroadcastMap(event.mapID, -1, &server.ArenaUseServerPacket{PlayersCount: event.startCount})
		w.BroadcastMap(event.mapID, -1, &server.TalkServerServerPacket{Message: "Arena fight!"})
	}
}

func (w *World) recordArenaDefeat(mapID, attackerID, targetID int) (winnerID int, winnerName string, winnerKills int, killerName string, victimName string, ok bool) {
	w.arenaMu.Lock()
	defer w.arenaMu.Unlock()

	arena := w.arenas[mapID]
	if arena == nil || arena.phase != arenaPhaseActive {
		return 0, "", 0, "", "", false
	}

	killer := arena.participants[attackerID]
	victim := arena.participants[targetID]
	if killer == nil || victim == nil {
		return 0, "", 0, "", "", false
	}

	killer.kills++
	killerName = killer.name
	victimName = victim.name
	winnerKills = killer.kills
	delete(arena.participants, targetID)

	if len(arena.participants) != 1 {
		return 0, "", winnerKills, killerName, victimName, true
	}

	for id, participant := range arena.participants {
		winnerID = id
		winnerName = participant.name
		winnerKills = participant.kills
		break
	}
	arena.phase = arenaPhaseWaiting
	arena.countdownTicks = 0
	return winnerID, winnerName, winnerKills, killerName, victimName, true
}

func (w *World) removeArenaParticipant(mapID, playerID int) (winnerID int, winnerName string, winnerKills int, killerName string, victimName string, ok bool) {
	w.arenaMu.Lock()
	defer w.arenaMu.Unlock()

	arena := w.arenas[mapID]
	if arena == nil {
		return 0, "", 0, "", "", false
	}

	participant := arena.participants[playerID]
	if participant == nil {
		return 0, "", 0, "", "", false
	}

	victimName = participant.name
	delete(arena.participants, playerID)

	if len(arena.participants) < arena.minimumPlayersToStart() && arena.phase == arenaPhaseCountdown {
		arena.phase = arenaPhaseWaiting
		arena.countdownTicks = 0
	}
	if len(arena.participants) == 0 {
		arena.phase = arenaPhaseWaiting
		arena.countdownTicks = 0
		return 0, "", 0, "", victimName, true
	}
	if arena.phase != arenaPhaseActive || len(arena.participants) != 1 {
		return 0, "", 0, "", victimName, true
	}

	for id, remaining := range arena.participants {
		winnerID = id
		winnerName = remaining.name
		winnerKills = remaining.kills
		killerName = remaining.name
		break
	}
	arena.phase = arenaPhaseWaiting
	arena.countdownTicks = 0
	return winnerID, winnerName, winnerKills, killerName, victimName, true
}

func (w *World) refillArenaFromMap(mapID int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	for _, playerID := range m.PlayerIDs() {
		w.syncArenaParticipation(mapID, playerID, w.GetPlayerName(playerID))
	}
}

func (w *World) removeArenaQueueParticipantIfMoved(mapID, playerID int) bool {
	m := w.getMap(mapID)
	if m == nil {
		return false
	}

	position := m.GetPlayerPosition(playerID)
	if position == nil {
		return false
	}

	w.arenaMu.Lock()
	defer w.arenaMu.Unlock()

	arena := w.arenas[mapID]
	if arena == nil || !arena.usesQueueSpawns() || arena.phase == arenaPhaseActive {
		return false
	}

	participant := arena.participants[playerID]
	if participant == nil {
		return false
	}
	if participant.queueX == position.X && participant.queueY == position.Y {
		return false
	}

	delete(arena.participants, playerID)
	if len(arena.participants) < arena.minimumPlayersToStart() && arena.phase == arenaPhaseCountdown {
		arena.phase = arenaPhaseWaiting
		arena.countdownTicks = 0
	}
	if len(arena.participants) == 0 {
		arena.phase = arenaPhaseWaiting
		arena.countdownTicks = 0
	}

	return true
}

func (w *World) arenaUsesQueueSpawns(mapID int) bool {
	w.arenaMu.Lock()
	defer w.arenaMu.Unlock()

	arena := w.arenas[mapID]
	return arena != nil && arena.usesQueueSpawns()
}

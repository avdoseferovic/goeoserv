package world

import (
	"math/rand/v2"
	"strings"
	"time"

	"github.com/avdo/goeoserv/internal/deep"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
)

func (w *World) RegisterPlayer(playerID, mapID int, name string, bus *protocol.PacketBus) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	entry := w.playerIndex[playerID]
	if entry == nil {
		entry = &playerEntry{}
		w.playerIndex[playerID] = entry
	}
	entry.mapID = mapID
	entry.name = name
	entry.bus = bus
	if name != "" {
		w.nameIndex[strings.ToLower(name)] = playerID
	}
}

func (w *World) BindPlayerSession(playerID int, session *player.Player) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	entry := w.playerIndex[playerID]
	if entry == nil {
		entry = &playerEntry{}
		w.playerIndex[playerID] = entry
	}
	entry.session = session
}

func (w *World) UnregisterPlayer(playerID int) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	if e, ok := w.playerIndex[playerID]; ok {
		if e.name != "" {
			delete(w.nameIndex, strings.ToLower(e.name))
		}
		delete(w.playerIndex, playerID)
	}
	delete(w.muteUntil, playerID)
	delete(w.captchas, playerID)
}

func (w *World) SetMutedUntil(playerID int, until time.Time) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	w.muteUntil[playerID] = until
}

func (w *World) ClearMuted(playerID int) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	delete(w.muteUntil, playerID)
}

func (w *World) GetMutedUntil(playerID int) (time.Time, bool) {
	w.playerMu.RLock()
	defer w.playerMu.RUnlock()
	until, ok := w.muteUntil[playerID]
	return until, ok
}

func (w *World) IsMuted(playerID int) bool {
	w.playerMu.RLock()
	until, ok := w.muteUntil[playerID]
	w.playerMu.RUnlock()
	return ok && time.Now().Before(until)
}

func (w *World) HasCaptcha(playerID int) bool {
	w.playerMu.RLock()
	defer w.playerMu.RUnlock()
	_, ok := w.captchas[playerID]
	return ok
}

func randomCaptcha() string {
	b := make([]byte, 5)
	for i := range b {
		b[i] = byte(rand.IntN(26) + 65)
	}
	return string(b)
}

func (w *World) StartCaptcha(playerID int, reward int) bool {
	w.playerMu.Lock()
	entry := w.playerIndex[playerID]
	if entry == nil || entry.bus == nil {
		w.playerMu.Unlock()
		return false
	}
	challenge := randomCaptcha()
	w.captchas[playerID] = &captchaState{challenge: challenge, reward: reward}
	bus := entry.bus
	w.playerMu.Unlock()
	payload, err := deep.SerializeCaptchaOpen(1, reward, challenge)
	if err != nil {
		return false
	}
	return bus.Send(eonet.PacketAction_Open, deep.FamilyCaptcha, payload) == nil
}

func (w *World) RefreshCaptcha(playerID int) bool {
	w.playerMu.Lock()
	entry := w.playerIndex[playerID]
	state := w.captchas[playerID]
	if entry == nil || entry.bus == nil || state == nil {
		w.playerMu.Unlock()
		return false
	}
	state.challenge = randomCaptcha()
	state.attempts = 0
	challenge := state.challenge
	bus := entry.bus
	w.playerMu.Unlock()
	payload, err := deep.SerializeCaptchaAgree(1, challenge)
	if err != nil {
		return false
	}
	return bus.Send(eonet.PacketAction_Agree, deep.FamilyCaptcha, payload) == nil
}

func (w *World) VerifyCaptcha(playerID int, value string) (reward int, solved bool) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	state := w.captchas[playerID]
	if state == nil {
		return 0, false
	}
	state.attempts++
	if strings.EqualFold(strings.TrimSpace(value), state.challenge) {
		reward = state.reward
		delete(w.captchas, playerID)
		return reward, true
	}
	if state.attempts > 5 {
		delete(w.captchas, playerID)
	}
	return 0, false
}

func (w *World) UpdatePlayerMap(playerID, mapID int) {
	w.playerMu.Lock()
	defer w.playerMu.Unlock()
	if e, ok := w.playerIndex[playerID]; ok {
		e.mapID = mapID
	}
}

func (w *World) IsLoggedIn(accountID int) bool {
	w.accountMu.RLock()
	defer w.accountMu.RUnlock()
	return w.loggedAccounts[accountID]
}

func (w *World) AddLoggedInAccount(accountID int) {
	w.accountMu.Lock()
	defer w.accountMu.Unlock()
	w.loggedAccounts[accountID] = true
}

func (w *World) RemoveLoggedInAccount(accountID int) {
	w.accountMu.Lock()
	defer w.accountMu.Unlock()
	delete(w.loggedAccounts, accountID)
}

func (w *World) GetPlayerBus(playerID int) any {
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e != nil {
		return e.bus
	}
	return nil
}

func (w *World) GetPlayerSession(playerID int) *player.Player {
	w.playerMu.RLock()
	defer w.playerMu.RUnlock()
	if e := w.playerIndex[playerID]; e != nil {
		return e.session
	}
	return nil
}

func (w *World) GetPlayerName(playerID int) string {
	w.playerMu.RLock()
	e := w.playerIndex[playerID]
	w.playerMu.RUnlock()
	if e != nil {
		return e.name
	}
	return ""
}

func (w *World) OnlinePlayerCount() int {
	w.playerMu.RLock()
	defer w.playerMu.RUnlock()
	return len(w.playerIndex)
}

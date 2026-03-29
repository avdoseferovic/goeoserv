package world

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/avdo/goeoserv/internal/config"
	"github.com/avdo/goeoserv/internal/db"
	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/protocol"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	"github.com/ethanmoffat/eolib-go/v3/data"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
)

type playerEntry struct {
	mapID   int
	name    string
	bus     *protocol.PacketBus
	session *player.Player
}

type captchaState struct {
	challenge string
	reward    int
	attempts  int
}

type World struct {
	mapMu sync.RWMutex
	maps  map[int]*gamemap.GameMap
	ticks int

	arenaMu sync.Mutex
	arenas  map[int]*arenaState

	accountMu      sync.RWMutex
	loggedAccounts map[int]bool

	playerMu    sync.RWMutex
	playerIndex map[int]*playerEntry
	nameIndex   map[string]int
	muteUntil   map[int]time.Time
	captchas    map[int]*captchaState

	cfg *config.Config
	db  *db.Database
}

func New(cfg *config.Config, database *db.Database) *World {
	return &World{
		maps:           make(map[int]*gamemap.GameMap),
		arenas:         make(map[int]*arenaState),
		loggedAccounts: make(map[int]bool),
		playerIndex:    make(map[int]*playerEntry),
		nameIndex:      make(map[string]int),
		muteUntil:      make(map[int]time.Time),
		captchas:       make(map[int]*captchaState),
		cfg:            cfg,
		db:             database,
	}
}

func (w *World) getMap(mapID int) *gamemap.GameMap {
	w.mapMu.RLock()
	m := w.maps[mapID]
	w.mapMu.RUnlock()
	return m
}

func (w *World) GetMap(mapID int) *gamemap.GameMap {
	return w.getMap(mapID)
}

func (w *World) LoadMaps() error {
	mapDir := "data/maps"
	entries, err := os.ReadDir(mapDir)
	if err != nil {
		return fmt.Errorf("reading map directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".emf") {
			continue
		}

		mapPath := filepath.Join(mapDir, entry.Name())
		mapData, err := os.ReadFile(mapPath)
		if err != nil {
			slog.Warn("failed to read map file", "path", mapPath, "err", err)
			continue
		}

		reader := data.NewEoReader(mapData)
		var emf eomap.Emf
		if err := emf.Deserialize(reader); err != nil {
			slog.Warn("failed to deserialize map", "path", mapPath, "err", err)
			continue
		}

		baseName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		mapID, err := strconv.Atoi(baseName)
		if err != nil {
			slog.Warn("invalid map filename", "name", entry.Name())
			continue
		}

		gm := gamemap.New(mapID, &emf, w.cfg)
		gm.SpawnNPCs(w.cfg.NPCs.InstantSpawn)
		w.maps[mapID] = gm
		if arenaConfig := w.cfg.Arenas.MapConfig(mapID); arenaConfig != nil {
			w.arenas[mapID] = newArenaState(mapID, arenaConfig)
		} else if gm.HasArena() {
			w.arenas[mapID] = newArenaState(mapID, nil)
		}
		count++
	}

	slog.Info("maps loaded", "count", count)
	return nil
}

func (w *World) InitNpcStats() {
	w.mapMu.RLock()
	defer w.mapMu.RUnlock()
	for _, m := range w.maps {
		m.InitNpcStats(func(npcID int) int {
			rec := pubdata.GetNpc(npcID)
			if rec != nil {
				return rec.Hp
			}
			return 1
		})
	}
}

func (w *World) EnterMap(mapID int, charInfo any) {
	m := w.getMap(mapID)
	if m == nil {
		slog.Warn("map not found for enter", "map_id", mapID)
		return
	}
	if mc, ok := charInfo.(*gamemap.MapCharacter); ok {
		m.Enter(mc)
		w.RegisterPlayer(mc.PlayerID, mapID, mc.Name, mc.Bus)
		w.syncArenaParticipation(mapID, mc.PlayerID, mc.Name)
	}
}

func (w *World) LeaveMap(mapID, playerID int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.Leave(playerID)
	w.leaveArena(mapID, playerID)
	w.UnregisterPlayer(playerID)
}

func (w *World) Walk(mapID, playerID int, direction int, coords [2]int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.Walk(playerID, direction, coords)
	if w.arenaUsesQueueSpawns(mapID) {
		w.syncArenaParticipation(mapID, playerID, w.GetPlayerName(playerID))
	}
}

func (w *World) Face(mapID, playerID int, direction int) {
	m := w.getMap(mapID)
	if m == nil {
		return
	}
	m.Face(playerID, direction)
}

func (w *World) RunTickLoop(ctx context.Context) {
	tickRate := time.Duration(w.cfg.World.TickRate) * time.Millisecond
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick()
		}
	}
}

func (w *World) tick() {
	w.ticks++
	shouldAutoPickup := w.cfg.AutoPickup.Enabled && w.cfg.AutoPickup.Rate > 0 && w.ticks%w.cfg.AutoPickup.Rate == 0

	w.mapMu.RLock()
	for _, m := range w.maps {
		m.Tick()
		w.syncMapPlayerVitals(m)
		if shouldAutoPickup {
			w.tickAutoPickup(m)
		}
	}
	w.mapMu.RUnlock()

	w.tickArenas()

	delayTicks := w.cfg.Marriage.CeremonyStartDelaySeconds * 8
	if delayTicks <= 0 {
		delayTicks = 160
	}
	TickWeddings(delayTicks)
}

func (w *World) syncMapPlayerVitals(m *gamemap.GameMap) {
	if m == nil {
		return
	}

	for _, vitals := range m.GetPlayerVitalsSnapshot() {
		target := w.GetPlayerSession(vitals.PlayerID)
		if target == nil {
			continue
		}

		shouldDie := false

		target.Mu.Lock()
		if target.State != player.StateInGame || target.MapID != m.ID {
			target.Mu.Unlock()
			continue
		}

		target.CharHP = vitals.HP
		target.CharTP = vitals.TP
		shouldDie = target.CharHP <= 0

		if shouldDie {
			target.Die()
		}
		target.Mu.Unlock()
	}
}

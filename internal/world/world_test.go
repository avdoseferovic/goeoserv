package world

import (
	"testing"

	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
	eopub "github.com/ethanmoffat/eolib-go/v3/protocol/pub"
)

func TestCalculateStatsUpdatesLiveMapSnapshot(t *testing.T) {
	setWorldTestItemDB(t, map[int]eopub.EifRecord{
		1: {
			Hp:    7,
			Tp:    5,
			Armor: 4,
			Evade: 3,
			Con:   2,
			Wis:   1,
		},
	})

	w := New(nil, nil)
	m := gamemap.New(1, &eomap.Emf{}, nil)
	w.maps[1] = m

	w.EnterMap(1, &gamemap.MapCharacter{PlayerID: 7, Name: "regression", MapID: 1})

	p := &player.Player{
		ID:        7,
		World:     w,
		MapID:     1,
		CharLevel: 4,
		CharHP:    40,
		CharTP:    30,
		Stats: player.CharacterStats{
			Agi: 8,
			Con: 6,
			Wis: 5,
		},
	}

	p.CalculateStats()

	baseline := onlyCharacterMapInfo(t, m)
	if baseline.Hp != 40 || baseline.MaxHp != 48 || baseline.Tp != 30 || baseline.MaxTp != 45 {
		t.Fatalf("baseline snapshot = hp/maxHP/tp/maxTP %d/%d/%d/%d, want 40/48/30/45", baseline.Hp, baseline.MaxHp, baseline.Tp, baseline.MaxTp)
	}

	p.Stats.Agi = 12
	p.Stats.Con = 9
	p.Stats.Wis = 7
	p.Equipment.Armor = 1
	p.CharHP = 999
	p.CharTP = 999

	p.CalculateStats()

	updated := onlyCharacterMapInfo(t, m)
	if updated.Hp != 70 || updated.MaxHp != 70 || updated.Tp != 59 || updated.MaxTp != 59 {
		t.Fatalf("updated snapshot = hp/maxHP/tp/maxTP %d/%d/%d/%d, want 70/70/59/59", updated.Hp, updated.MaxHp, updated.Tp, updated.MaxTp)
	}

	removed := m.RemoveAndReturn(7)
	if removed == nil {
		t.Fatal("expected map character to exist")
	}
	if removed.Armor != 9 || removed.Evade != 9 {
		t.Fatalf("updated combat snapshot = armor/evade %d/%d, want 9/9", removed.Armor, removed.Evade)
	}
}

func onlyCharacterMapInfo(t *testing.T, m *gamemap.GameMap) gamemapCharacterView {
	t.Helper()

	nearby := m.GetNearbyInfo()
	if len(nearby.Characters) != 1 {
		t.Fatalf("expected 1 character on map, got %d", len(nearby.Characters))
	}

	ch := nearby.Characters[0]
	return gamemapCharacterView{Hp: ch.Hp, MaxHp: ch.MaxHp, Tp: ch.Tp, MaxTp: ch.MaxTp}
}

type gamemapCharacterView struct {
	Hp, MaxHp int
	Tp, MaxTp int
}

func setWorldTestItemDB(t *testing.T, items map[int]eopub.EifRecord) {
	t.Helper()

	prev := pubdata.ItemDB
	t.Cleanup(func() {
		pubdata.ItemDB = prev
	})

	maxID := 0
	for itemID := range items {
		if itemID > maxID {
			maxID = itemID
		}
	}

	itemDB := &eopub.Eif{Items: make([]eopub.EifRecord, maxID)}
	for itemID, record := range items {
		itemDB.Items[itemID-1] = record
	}

	pubdata.ItemDB = itemDB
}

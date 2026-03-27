package gamemap

import (
	"testing"

	"github.com/avdo/goeoserv/internal/config"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	eomap "github.com/ethanmoffat/eolib-go/v3/protocol/map"
	eopub "github.com/ethanmoffat/eolib-go/v3/protocol/pub"
)

func TestNpcNextChaseStepLockedRoutesAroundWalls(t *testing.T) {
	m := New(1, &eomap.Emf{
		Width:  5,
		Height: 3,
		TileSpecRows: []eomap.MapTileSpecRow{{
			Y: 1,
			Tiles: []eomap.MapTileSpecRowTile{{
				X:        2,
				TileSpec: eomap.MapTileSpec_Wall,
			}, {
				X:        3,
				TileSpec: eomap.MapTileSpec_Wall,
			}},
		}},
	}, &config.Config{NPCs: config.NPCs{ChaseDistance: 8}})

	npc := &NpcState{Index: 0, ID: 1, X: 1, Y: 1, Alive: true}
	target := &MapCharacter{PlayerID: 7, X: 4, Y: 1, HP: 10}

	m.npcs = []*NpcState{npc}
	m.players = map[int]*MapCharacter{target.PlayerID: target}

	nextX, nextY, dir, ok := m.npcNextChaseStepLocked(npc, target)
	if !ok {
		t.Fatal("expected NPC to find a chase step around the wall")
	}
	if nextX != 1 || nextY != 2 {
		t.Fatalf("next chase step = (%d,%d), want (1,2)", nextX, nextY)
	}
	if dir != 0 {
		t.Fatalf("first chase direction = %d, want 0 (down)", dir)
	}
}

func TestNpcAttackLockedRejectsDiagonalTargets(t *testing.T) {
	setNpcTestDB(t, map[int]eopub.EnfRecord{1: {MinDamage: 5, MaxDamage: 5}})

	m := New(1, &eomap.Emf{Width: 3, Height: 3}, &config.Config{NPCs: config.NPCs{ChaseDistance: 8}})
	npc := &NpcState{Index: 0, ID: 1, X: 1, Y: 1, Alive: true}
	target := &MapCharacter{PlayerID: 7, X: 2, Y: 2, HP: 10, MaxHP: 10}

	attack, ok := m.npcAttackLocked(npc, target)
	if ok {
		t.Fatalf("expected diagonal attack to fail, got %#v", attack)
	}
	if target.HP != 10 {
		t.Fatalf("target HP = %d after diagonal attack, want 10", target.HP)
	}
	if npcIsOrthogonallyAdjacent(npc.X, npc.Y, target.X, target.Y) {
		t.Fatal("expected diagonal tiles to not count as melee adjacency")
	}
}

func setNpcTestDB(t *testing.T, npcs map[int]eopub.EnfRecord) {
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

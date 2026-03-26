package player

import (
	"testing"

	"github.com/avdo/goeoserv/internal/config"
)

// testCfg returns a minimal config for tests.
func testCfg() *config.Config {
	return &config.Config{}
}

func TestRemoveItem(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		inventory map[int]int
		itemID    int
		amount    int
		wantOK    bool
		wantQty   int // expected remaining qty (0 means deleted)
	}{
		{
			name:      "sufficient stock",
			inventory: map[int]int{10: 5},
			itemID:    10,
			amount:    3,
			wantOK:    true,
			wantQty:   2,
		},
		{
			name:      "exact amount removes entry",
			inventory: map[int]int{10: 5},
			itemID:    10,
			amount:    5,
			wantOK:    true,
			wantQty:   0,
		},
		{
			name:      "insufficient stock",
			inventory: map[int]int{10: 2},
			itemID:    10,
			amount:    5,
			wantOK:    false,
			wantQty:   2,
		},
		{
			name:      "item not in inventory",
			inventory: map[int]int{},
			itemID:    99,
			amount:    1,
			wantOK:    false,
			wantQty:   0,
		},
		{
			name:      "zero amount succeeds",
			inventory: map[int]int{10: 5},
			itemID:    10,
			amount:    0,
			wantOK:    true,
			wantQty:   5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Player{Cfg: testCfg(), Inventory: tt.inventory}
			got := p.RemoveItem(tt.itemID, tt.amount)
			if got != tt.wantOK {
				t.Errorf("RemoveItem() = %v, want %v", got, tt.wantOK)
			}
			if tt.wantQty == 0 {
				if _, exists := p.Inventory[tt.itemID]; exists {
					t.Errorf("expected item %d to be deleted from inventory", tt.itemID)
				}
			} else if p.Inventory[tt.itemID] != tt.wantQty {
				t.Errorf("remaining qty = %d, want %d", p.Inventory[tt.itemID], tt.wantQty)
			}
		})
	}
}

func TestAddItem(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		initial map[int]int
		itemID  int
		amount  int
		wantQty int
	}{
		{
			name:    "add to empty inventory",
			initial: map[int]int{},
			itemID:  10,
			amount:  5,
			wantQty: 5,
		},
		{
			name:    "add to existing stack",
			initial: map[int]int{10: 3},
			itemID:  10,
			amount:  7,
			wantQty: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Player{Cfg: testCfg(), Inventory: tt.initial}
			p.AddItem(tt.itemID, tt.amount)
			if p.Inventory[tt.itemID] != tt.wantQty {
				t.Errorf("qty = %d, want %d", p.Inventory[tt.itemID], tt.wantQty)
			}
		})
	}
}

func TestGainHP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		hp       int
		maxHP    int
		amount   int
		wantGain int
		wantHP   int
	}{
		{
			name: "partial heal",
			hp:   50, maxHP: 100,
			amount:   30,
			wantGain: 30,
			wantHP:   80,
		},
		{
			name: "heal clamped to max",
			hp:   90, maxHP: 100,
			amount:   50,
			wantGain: 10,
			wantHP:   100,
		},
		{
			name: "already at max",
			hp:   100, maxHP: 100,
			amount:   10,
			wantGain: 0,
			wantHP:   100,
		},
		{
			name: "zero amount",
			hp:   50, maxHP: 100,
			amount:   0,
			wantGain: 0,
			wantHP:   50,
		},
		{
			name: "negative amount",
			hp:   50, maxHP: 100,
			amount:   -10,
			wantGain: 0,
			wantHP:   50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Player{Cfg: testCfg(), CharHP: tt.hp, CharMaxHP: tt.maxHP}
			gain := p.GainHP(tt.amount)
			if gain != tt.wantGain {
				t.Errorf("GainHP() = %d, want %d", gain, tt.wantGain)
			}
			if p.CharHP != tt.wantHP {
				t.Errorf("CharHP = %d, want %d", p.CharHP, tt.wantHP)
			}
		})
	}
}

func TestGainTP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		tp       int
		maxTP    int
		amount   int
		wantGain int
		wantTP   int
	}{
		{
			name: "partial restore",
			tp:   20, maxTP: 100,
			amount:   30,
			wantGain: 30,
			wantTP:   50,
		},
		{
			name: "clamped to max",
			tp:   95, maxTP: 100,
			amount:   50,
			wantGain: 5,
			wantTP:   100,
		},
		{
			name: "already at max",
			tp:   100, maxTP: 100,
			amount:   10,
			wantGain: 0,
			wantTP:   100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Player{Cfg: testCfg(), CharTP: tt.tp, CharMaxTP: tt.maxTP}
			gain := p.GainTP(tt.amount)
			if gain != tt.wantGain {
				t.Errorf("GainTP() = %d, want %d", gain, tt.wantGain)
			}
			if p.CharTP != tt.wantTP {
				t.Errorf("CharTP = %d, want %d", p.CharTP, tt.wantTP)
			}
		})
	}
}

func TestGetSpellLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		spells  []SpellState
		spellID int
		want    int
	}{
		{
			name:    "known spell",
			spells:  []SpellState{{ID: 1, Level: 3}, {ID: 5, Level: 7}},
			spellID: 5,
			want:    7,
		},
		{
			name:    "unknown spell returns 0",
			spells:  []SpellState{{ID: 1, Level: 3}},
			spellID: 99,
			want:    0,
		},
		{
			name:    "empty spell list",
			spells:  nil,
			spellID: 1,
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &Player{Cfg: testCfg(), Spells: tt.spells}
			got := p.GetSpellLevel(tt.spellID)
			if got != tt.want {
				t.Errorf("GetSpellLevel(%d) = %d, want %d", tt.spellID, got, tt.want)
			}
		})
	}
}

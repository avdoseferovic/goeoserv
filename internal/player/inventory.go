package player

// RemoveItem deducts amount from inventory. Returns false if insufficient.
func (p *Player) RemoveItem(itemID, amount int) bool {
	if p.Inventory[itemID] < amount {
		return false
	}
	p.Inventory[itemID] -= amount
	if p.Inventory[itemID] <= 0 {
		delete(p.Inventory, itemID)
	}
	return true
}

// AddItem adds amount to inventory.
func (p *Player) AddItem(itemID, amount int) {
	p.Inventory[itemID] += amount
}

// GainHP heals the player, clamping to max. Returns actual gain.
func (p *Player) GainHP(amount int) int {
	if amount <= 0 || p.CharHP >= p.CharMaxHP {
		return 0
	}
	gain := amount
	p.CharHP += gain
	if p.CharHP > p.CharMaxHP {
		gain -= (p.CharHP - p.CharMaxHP)
		p.CharHP = p.CharMaxHP
	}
	return gain
}

// GainTP restores TP, clamping to max. Returns actual gain.
func (p *Player) GainTP(amount int) int {
	if amount <= 0 || p.CharTP >= p.CharMaxTP {
		return 0
	}
	gain := amount
	p.CharTP += gain
	if p.CharTP > p.CharMaxTP {
		gain -= (p.CharTP - p.CharMaxTP)
		p.CharTP = p.CharMaxTP
	}
	return gain
}

// GetSpellLevel returns the level of a learned spell, or 0 if not known.
func (p *Player) GetSpellLevel(spellID int) int {
	for _, s := range p.Spells {
		if s.ID == spellID {
			return s.Level
		}
	}
	return 0
}

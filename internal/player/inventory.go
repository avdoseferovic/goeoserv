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

// AddItem adds amount to inventory, capped by Limits.MaxItem.
func (p *Player) AddItem(itemID, amount int) {
	p.Inventory[itemID] += amount
	if maxItem := p.Cfg.Limits.MaxItem; maxItem > 0 && p.Inventory[itemID] > maxItem {
		p.Inventory[itemID] = maxItem
	}
}

// DistanceTo returns the Manhattan distance between the player and a point.
func (p *Player) DistanceTo(x, y int) int {
	dx := p.CharX - x
	dy := p.CharY - y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return max(dx, dy)
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

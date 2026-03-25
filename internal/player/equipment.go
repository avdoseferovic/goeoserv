package player

import (
	pubdata "github.com/avdo/goeoserv/internal/pub"
	eopub "github.com/ethanmoffat/eolib-go/v3/protocol/pub"
)

// Equipment holds all equipped item IDs.
type Equipment struct {
	Boots     int
	Accessory int
	Gloves    int
	Belt      int
	Armor     int
	Necklace  int
	Hat       int
	Shield    int
	Weapon    int
	Ring      [2]int
	Armlet    [2]int
	Bracer    [2]int
}

// SlotForItemType returns which equipment field an item type maps to.
// subLoc 0 or 1 for dual-slot items (ring, armlet, bracer).
func (e *Equipment) Equip(itemType eopub.ItemType, itemID, subLoc int) (oldItemID int) {
	switch itemType {
	case eopub.Item_Weapon:
		oldItemID = e.Weapon
		e.Weapon = itemID
	case eopub.Item_Shield:
		oldItemID = e.Shield
		e.Shield = itemID
	case eopub.Item_Armor:
		oldItemID = e.Armor
		e.Armor = itemID
	case eopub.Item_Hat:
		oldItemID = e.Hat
		e.Hat = itemID
	case eopub.Item_Boots:
		oldItemID = e.Boots
		e.Boots = itemID
	case eopub.Item_Gloves:
		oldItemID = e.Gloves
		e.Gloves = itemID
	case eopub.Item_Accessory:
		oldItemID = e.Accessory
		e.Accessory = itemID
	case eopub.Item_Belt:
		oldItemID = e.Belt
		e.Belt = itemID
	case eopub.Item_Necklace:
		oldItemID = e.Necklace
		e.Necklace = itemID
	case eopub.Item_Ring:
		idx := subLoc
		if idx < 0 || idx > 1 {
			idx = 0
		}
		oldItemID = e.Ring[idx]
		e.Ring[idx] = itemID
	case eopub.Item_Armlet:
		idx := subLoc
		if idx < 0 || idx > 1 {
			idx = 0
		}
		oldItemID = e.Armlet[idx]
		e.Armlet[idx] = itemID
	case eopub.Item_Bracer:
		idx := subLoc
		if idx < 0 || idx > 1 {
			idx = 0
		}
		oldItemID = e.Bracer[idx]
		e.Bracer[idx] = itemID
	}
	return
}

// Unequip removes an item from the given slot by item type and subLoc.
func (e *Equipment) Unequip(itemType eopub.ItemType, subLoc int) (removedID int) {
	switch itemType {
	case eopub.Item_Weapon:
		removedID = e.Weapon
		e.Weapon = 0
	case eopub.Item_Shield:
		removedID = e.Shield
		e.Shield = 0
	case eopub.Item_Armor:
		removedID = e.Armor
		e.Armor = 0
	case eopub.Item_Hat:
		removedID = e.Hat
		e.Hat = 0
	case eopub.Item_Boots:
		removedID = e.Boots
		e.Boots = 0
	case eopub.Item_Gloves:
		removedID = e.Gloves
		e.Gloves = 0
	case eopub.Item_Accessory:
		removedID = e.Accessory
		e.Accessory = 0
	case eopub.Item_Belt:
		removedID = e.Belt
		e.Belt = 0
	case eopub.Item_Necklace:
		removedID = e.Necklace
		e.Necklace = 0
	case eopub.Item_Ring:
		idx := subLoc
		if idx < 0 || idx > 1 {
			idx = 0
		}
		removedID = e.Ring[idx]
		e.Ring[idx] = 0
	case eopub.Item_Armlet:
		idx := subLoc
		if idx < 0 || idx > 1 {
			idx = 0
		}
		removedID = e.Armlet[idx]
		e.Armlet[idx] = 0
	case eopub.Item_Bracer:
		idx := subLoc
		if idx < 0 || idx > 1 {
			idx = 0
		}
		removedID = e.Bracer[idx]
		e.Bracer[idx] = 0
	}
	return
}

// FindItemType returns the item type for a given equipped item ID, searching all slots.
func (e *Equipment) FindItemType(itemID int) (eopub.ItemType, int) {
	if e.Weapon == itemID {
		return eopub.Item_Weapon, 0
	}
	if e.Shield == itemID {
		return eopub.Item_Shield, 0
	}
	if e.Armor == itemID {
		return eopub.Item_Armor, 0
	}
	if e.Hat == itemID {
		return eopub.Item_Hat, 0
	}
	if e.Boots == itemID {
		return eopub.Item_Boots, 0
	}
	if e.Gloves == itemID {
		return eopub.Item_Gloves, 0
	}
	if e.Accessory == itemID {
		return eopub.Item_Accessory, 0
	}
	if e.Belt == itemID {
		return eopub.Item_Belt, 0
	}
	if e.Necklace == itemID {
		return eopub.Item_Necklace, 0
	}
	for i := 0; i < 2; i++ {
		if e.Ring[i] == itemID {
			return eopub.Item_Ring, i
		}
		if e.Armlet[i] == itemID {
			return eopub.Item_Armlet, i
		}
		if e.Bracer[i] == itemID {
			return eopub.Item_Bracer, i
		}
	}
	return eopub.Item_General, 0
}

// ForEachID calls fn for each non-zero equipped item ID, avoiding allocation.
func (e *Equipment) ForEachID(fn func(itemID int)) {
	for _, id := range [...]int{
		e.Boots, e.Accessory, e.Gloves, e.Belt,
		e.Armor, e.Necklace, e.Hat, e.Shield, e.Weapon,
		e.Ring[0], e.Ring[1], e.Armlet[0], e.Armlet[1],
		e.Bracer[0], e.Bracer[1],
	} {
		if id != 0 {
			fn(id)
		}
	}
}

// IsEquipable returns true if the item type is an equipment type.
func IsEquipable(t eopub.ItemType) bool {
	switch t {
	case eopub.Item_Weapon, eopub.Item_Shield, eopub.Item_Armor, eopub.Item_Hat,
		eopub.Item_Boots, eopub.Item_Gloves, eopub.Item_Accessory, eopub.Item_Belt,
		eopub.Item_Necklace, eopub.Item_Ring, eopub.Item_Armlet, eopub.Item_Bracer:
		return true
	}
	return false
}

// CalculateStats recalculates derived stats from base stats + equipment + class.
func (p *Player) CalculateStats() {
	// Start with base stats
	adjStr := p.Stats.Str
	adjIntl := p.Stats.Intl
	adjWis := p.Stats.Wis
	adjAgi := p.Stats.Agi
	adjCon := p.Stats.Con
	adjCha := p.Stats.Cha

	// Add class bonuses
	if p.ClassID > 0 {
		class := pubdata.GetClass(p.ClassID)
		if class != nil {
			adjStr += class.Str
			adjIntl += class.Intl
			adjWis += class.Wis
			adjAgi += class.Agi
			adjCon += class.Con
			adjCha += class.Cha
		}
	}

	// Equipment bonuses
	var eqHP, eqTP int
	var eqMinDmg, eqMaxDmg, eqAccuracy, eqEvade, eqArmor int
	totalWeight := 0

	p.Equipment.ForEachID(func(itemID int) {
		item := pubdata.GetItem(itemID)
		if item == nil {
			return
		}
		totalWeight += item.Weight
		eqHP += item.Hp
		eqTP += item.Tp
		eqMinDmg += item.MinDamage
		eqMaxDmg += item.MaxDamage
		eqAccuracy += item.Accuracy
		eqEvade += item.Evade
		eqArmor += item.Armor
		adjStr += item.Str
		adjIntl += item.Intl
		adjWis += item.Wis
		adjAgi += item.Agi
		adjCon += item.Con
		adjCha += item.Cha
	})

	// Inventory weight
	for itemID, amount := range p.Inventory {
		item := pubdata.GetItem(itemID)
		if item != nil {
			totalWeight += item.Weight * amount
		}
	}

	// Formula-based stats
	maxHP := 10 + adjCon*3 + p.CharLevel*5 + eqHP
	maxTP := 10 + adjWis*3 + p.CharLevel*5 + eqTP
	maxSP := 20 + p.CharLevel*2
	maxWeight := 70 + adjStr

	// Derived combat stats
	minDmg := adjStr/2 + eqMinDmg
	maxDmg := adjStr + eqMaxDmg
	accuracy := adjAgi/2 + eqAccuracy
	evade := adjAgi/2 + eqEvade
	armor := adjCon/2 + eqArmor

	if minDmg < 1 {
		minDmg = 1
	}
	if maxDmg < 1 {
		maxDmg = 1
	}
	if maxHP > 64000 {
		maxHP = 64000
	}
	if maxTP > 64000 {
		maxTP = 64000
	}

	p.CharMaxHP = maxHP
	p.CharMaxTP = maxTP
	p.CharMaxSP = maxSP
	p.MaxWeight = maxWeight
	p.Weight = totalWeight
	p.MinDamage = minDmg
	p.MaxDamage = maxDmg
	p.Accuracy = accuracy
	p.Evade = evade
	p.Armor = armor

	// Clamp current HP/TP
	if p.CharHP > p.CharMaxHP {
		p.CharHP = p.CharMaxHP
	}
	if p.CharTP > p.CharMaxTP {
		p.CharTP = p.CharMaxTP
	}
}

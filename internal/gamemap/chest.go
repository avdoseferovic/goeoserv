package gamemap

// Chest holds items at a specific map tile.
type Chest struct {
	Items []ChestItem
}

// ChestItem is a single item stack in a chest.
type ChestItem struct {
	ItemID int
	Amount int
}

// GetChestItems returns the items in a chest at the given coordinates.
func (m *GameMap) GetChestItems(x, y int) []ChestItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	chest := m.chests[[2]int{x, y}]
	if chest == nil {
		return nil
	}
	result := make([]ChestItem, len(chest.Items))
	copy(result, chest.Items)
	return result
}

// AddChestItem adds an item to a chest. Returns the updated item list.
func (m *GameMap) AddChestItem(x, y, itemID, amount int) []ChestItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	chest := m.chests[[2]int{x, y}]
	if chest == nil {
		return nil
	}
	for i := range chest.Items {
		if chest.Items[i].ItemID == itemID {
			// Enforce max chest item limit
			if maxChest := m.cfg.Limits.MaxChest; maxChest > 0 && chest.Items[i].Amount+amount > maxChest {
				return nil
			}
			chest.Items[i].Amount += amount
			result := make([]ChestItem, len(chest.Items))
			copy(result, chest.Items)
			return result
		}
	}
	if len(chest.Items) >= m.cfg.Chest.Slots {
		return nil // chest full
	}
	chest.Items = append(chest.Items, ChestItem{ItemID: itemID, Amount: amount})
	result := make([]ChestItem, len(chest.Items))
	copy(result, chest.Items)
	return result
}

// TakeChestItem removes an item from a chest. Returns (amount taken, updated items).
func (m *GameMap) TakeChestItem(x, y, itemID int) (int, []ChestItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	chest := m.chests[[2]int{x, y}]
	if chest == nil {
		return 0, nil
	}
	for i := range chest.Items {
		if chest.Items[i].ItemID == itemID {
			amount := chest.Items[i].Amount
			chest.Items = append(chest.Items[:i], chest.Items[i+1:]...)
			result := make([]ChestItem, len(chest.Items))
			copy(result, chest.Items)
			return amount, result
		}
	}
	return 0, nil
}

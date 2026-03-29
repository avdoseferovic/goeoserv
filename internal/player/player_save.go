package player

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
)

// SaveCharacter persists the current character state to the database.
// It saves position, stats, and inventory in a single transaction.
// Uses context.Background() since this may be called during disconnect/save loops.
func (p *Player) SaveCharacter() error {
	p.Mu.Lock()
	defer p.Mu.Unlock()

	if p.CharacterID == nil {
		return nil
	}
	charID := *p.CharacterID
	ctx := context.Background() // intentional: saves run outside request scope

	tx, err := p.DB.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			slog.Warn("failed to rollback transaction", "err", err)
		}
	}()
	_, err = tx.ExecContext(ctx,
		`UPDATE characters SET
			map = ?, x = ?, y = ?, direction = ?,
			level = ?, experience = ?, hp = ?, tp = ?,
			class = ?, race = ?,
			strength = ?, intelligence = ?, wisdom = ?,
			agility = ?, constitution = ?, charisma = ?,
			stat_points = ?, skill_points = ?,
			admin_level = ?, gold_bank = ?,
			boots = ?, accessory = ?, gloves = ?, belt = ?,
			armor = ?, necklace = ?, hat = ?, shield = ?, weapon = ?,
			ring = ?, ring2 = ?, armlet = ?, armlet2 = ?,
			bracer = ?, bracer2 = ?
		WHERE id = ?`,
		p.MapID, p.CharX, p.CharY, p.CharDirection,
		p.CharLevel, p.CharExp, p.CharHP, p.CharTP,
		p.ClassID, p.CharSkin,
		p.Stats.Str, p.Stats.Intl, p.Stats.Wis,
		p.Stats.Agi, p.Stats.Con, p.Stats.Cha,
		p.StatPoints, p.SkillPoints,
		p.CharAdmin, p.GoldBank,
		p.Equipment.Boots, p.Equipment.Accessory, p.Equipment.Gloves, p.Equipment.Belt,
		p.Equipment.Armor, p.Equipment.Necklace, p.Equipment.Hat, p.Equipment.Shield, p.Equipment.Weapon,
		p.Equipment.Ring[0], p.Equipment.Ring[1], p.Equipment.Armlet[0], p.Equipment.Armlet[1],
		p.Equipment.Bracer[0], p.Equipment.Bracer[1],
		charID,
	)
	if err != nil {
		return fmt.Errorf("update character: %w", err)
	}

	// Save inventory: delete all and re-insert
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_inventory WHERE character_id = ?", charID); err != nil {
		return fmt.Errorf("clear inventory: %w", err)
	}
	for itemID, qty := range p.Inventory {
		if qty <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO character_inventory (character_id, item_id, quantity) VALUES (?, ?, ?)",
			charID, itemID, qty,
		); err != nil {
			return fmt.Errorf("insert inventory item %d: %w", itemID, err)
		}
	}

	// Save spells: delete all and re-insert
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_spells WHERE character_id = ?", charID); err != nil {
		return fmt.Errorf("clear spells: %w", err)
	}
	for _, sp := range p.Spells {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO character_spells (character_id, spell_id, level) VALUES (?, ?, ?)",
			charID, sp.ID, sp.Level,
		); err != nil {
			return fmt.Errorf("insert spell %d: %w", sp.ID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM character_quest_progress WHERE character_id = ?", charID); err != nil {
		return fmt.Errorf("clear quest progress: %w", err)
	}
	for questID, qs := range p.QuestProgress.ActiveQuests {
		npcKills := map[string]int{}
		for npcID, count := range qs.NpcKills {
			npcKills[fmt.Sprintf("%d", npcID)] = count
		}
		payload, err := json.Marshal(map[string]any{"state_name": qs.StateName, "npc_kills": npcKills})
		if err != nil {
			return fmt.Errorf("marshal active quest %d: %w", questID, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO character_quest_progress (character_id, quest_id, state, npc_kills, player_kills, completions) VALUES (?, ?, ?, ?, ?, ?)",
			charID, questID, 0, string(payload), 0, 0,
		); err != nil {
			return fmt.Errorf("insert active quest %d: %w", questID, err)
		}
	}
	for questID := range p.QuestProgress.CompletedQuests {
		payload, err := json.Marshal(map[string]any{"state_name": "done", "npc_kills": map[string]int{}})
		if err != nil {
			return fmt.Errorf("marshal completed quest %d: %w", questID, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO character_quest_progress (character_id, quest_id, state, npc_kills, player_kills, done_at, completions) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)",
			charID, questID, 1, string(payload), 0, 1,
		); err != nil {
			return fmt.Errorf("insert completed quest %d: %w", questID, err)
		}
	}

	return tx.Commit()
}

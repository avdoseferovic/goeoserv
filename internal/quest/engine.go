package quest

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// QuestDB holds all loaded quests indexed by quest ID.
var QuestDB = make(map[int]*Quest)

// LoadQuests loads all .eqf files from the given directory.
func LoadQuests(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading quest directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".eqf") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("failed to read quest file", "path", path, "err", err)
			continue
		}

		// Extract quest ID from filename (e.g., "00001.eqf" -> 1)
		baseName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		questID, err := strconv.Atoi(baseName)
		if err != nil {
			slog.Warn("invalid quest filename", "name", entry.Name())
			continue
		}

		quest, err := Parse(questID, string(content))
		if err != nil {
			slog.Warn("failed to parse quest", "path", path, "err", err)
			continue
		}

		QuestDB[questID] = quest
		count++
	}

	slog.Info("quests loaded", "count", count)
	return nil
}

// PlayerQuestState tracks a player's progress in a single quest.
type PlayerQuestState struct {
	QuestID    int
	StateName  string
	NpcKills   map[int]int // npcID -> kill count
	ItemsGiven map[int]int // itemID -> count given
}

// PlayerQuestProgress tracks all quest progress for a player.
type PlayerQuestProgress struct {
	ActiveQuests    map[int]*PlayerQuestState // questID -> state
	CompletedQuests map[int]bool              // questID -> completed
}

// NewPlayerQuestProgress creates a new empty quest progress tracker.
func NewPlayerQuestProgress() *PlayerQuestProgress {
	return &PlayerQuestProgress{
		ActiveQuests:    make(map[int]*PlayerQuestState),
		CompletedQuests: make(map[int]bool),
	}
}

// GetQuestState returns the current state name for a quest, or "Begin" if not started.
func (p *PlayerQuestProgress) GetQuestState(questID int) string {
	if qs, ok := p.ActiveQuests[questID]; ok {
		return qs.StateName
	}
	return "Begin"
}

// SetQuestState updates the player's state for a quest.
func (p *PlayerQuestProgress) SetQuestState(questID int, stateName string) {
	if _, ok := p.ActiveQuests[questID]; !ok {
		p.ActiveQuests[questID] = &PlayerQuestState{
			QuestID:    questID,
			StateName:  stateName,
			NpcKills:   make(map[int]int),
			ItemsGiven: make(map[int]int),
		}
	} else {
		p.ActiveQuests[questID].StateName = stateName
	}
}

// CompleteQuest marks a quest as completed.
func (p *PlayerQuestProgress) CompleteQuest(questID int) {
	delete(p.ActiveQuests, questID)
	p.CompletedQuests[questID] = true
}

// ProcessRule checks if a rule condition is met and returns the goto state.
// Returns ("", false) if the rule doesn't apply.
func ProcessRule(rule Rule, npcInputChoice int) (string, bool) {
	lower := strings.ToLower(rule.Name)
	switch lower {
	case "inputnpc":
		// InputNpc(choice_id) — player selected a dialog option
		if len(rule.Args) > 0 && !rule.Args[0].IsStr && rule.Args[0].IntVal == npcInputChoice {
			return rule.Goto, true
		}
	case "talkedtonpc":
		// TalkedToNpc(npc_id) — always true when talking to the NPC
		return rule.Goto, true
	case "killednpcs":
		// KilledNpcs(npc_id, count) — TODO: check kill count
		return "", false
	case "gotitems":
		// GotItems(item_id, count) — TODO: check inventory
		return "", false
	case "always":
		return rule.Goto, true
	}
	return "", false
}

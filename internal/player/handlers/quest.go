package handlers

import (
	"context"
	"log/slog"
	"strings"

	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/quest"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Quest, eonet.PacketAction_Use, handleQuestUse)
	player.Register(eonet.PacketFamily_Quest, eonet.PacketAction_Accept, handleQuestAccept)
	player.Register(eonet.PacketFamily_Quest, eonet.PacketAction_List, handleQuestList)
}

// handleQuestUse — player talks to a quest NPC
func handleQuestUse(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.QuestUseClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize quest use", "id", p.ID, "err", err)
		return nil
	}

	q, ok := quest.QuestDB[pkt.QuestId]
	if !ok {
		return nil
	}

	// Get player's current state for this quest
	stateName := p.QuestProgress.GetQuestState(pkt.QuestId)
	state := q.GetState(stateName)
	if state == nil {
		return nil
	}

	sessionID := p.GenerateSessionID()

	return sendQuestDialog(p, q, state, pkt.QuestId, sessionID)
}

// handleQuestAccept — player responds to a quest dialog
func handleQuestAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.QuestAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize quest accept", "id", p.ID, "err", err)
		return nil
	}

	q, ok := quest.QuestDB[pkt.QuestId]
	if !ok {
		return nil
	}

	stateName := p.QuestProgress.GetQuestState(pkt.QuestId)
	state := q.GetState(stateName)
	if state == nil {
		return nil
	}

	// Determine which input the player selected
	actionID := 0
	if pkt.ReplyTypeData != nil {
		if link, ok := pkt.ReplyTypeData.(*client.QuestAcceptReplyTypeDataLink); ok {
			actionID = link.Action
		}
	}

	// Build quest context for rule evaluation
	questCtx := &quest.QuestPlayerContext{
		Inventory: p.Inventory,
		NpcKills:  make(map[int]int),
	}
	if qs, ok := p.QuestProgress.ActiveQuests[pkt.QuestId]; ok {
		_ = qs // NpcKills tracked at quest progress level if needed
	}

	// Process rules to find the next state
	for _, rule := range state.Rules {
		nextState, matched := quest.ProcessRuleWithContext(rule, actionID, questCtx)
		if matched {
			if strings.EqualFold(nextState, "goreset") || strings.EqualFold(nextState, "done") {
				// Quest reset or completed
				if strings.EqualFold(nextState, "done") {
					p.QuestProgress.CompleteQuest(pkt.QuestId)
					// Execute reward actions from the current state
					executeQuestRewards(p, state)
				} else {
					p.QuestProgress.SetQuestState(pkt.QuestId, "Begin")
				}
				return nil
			}

			p.QuestProgress.SetQuestState(pkt.QuestId, nextState)

			// Show the next state's dialog
			newState := q.GetState(nextState)
			if newState != nil {
				sessionID := p.GenerateSessionID()
				return sendQuestDialog(p, q, newState, pkt.QuestId, sessionID)
			}
			return nil
		}
	}

	return nil
}

// handleQuestList — player views quest progress or history
func handleQuestList(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.QuestListClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize quest list", "id", p.ID, "err", err)
		return nil
	}

	switch pkt.Page {
	case eonet.QuestPage_Progress:
		var entries []server.QuestProgressEntry
		for questID, qs := range p.QuestProgress.ActiveQuests {
			q := quest.QuestDB[questID]
			if q == nil {
				continue
			}
			state := q.GetState(qs.StateName)
			desc := ""
			if state != nil {
				desc = state.Description
			}
			entries = append(entries, server.QuestProgressEntry{
				Name:        q.Name,
				Description: desc,
			})
		}

		return p.Bus.SendPacket(&server.QuestListServerPacket{
			Page:        eonet.QuestPage_Progress,
			QuestsCount: len(entries),
			PageData: &server.QuestListPageDataProgress{
				QuestProgressEntries: entries,
			},
		})

	case eonet.QuestPage_History:
		var entries []string
		for questID := range p.QuestProgress.CompletedQuests {
			q := quest.QuestDB[questID]
			if q != nil {
				entries = append(entries, q.Name)
			}
		}

		return p.Bus.SendPacket(&server.QuestListServerPacket{
			Page:        eonet.QuestPage_History,
			QuestsCount: len(entries),
			PageData: &server.QuestListPageDataHistory{
				CompletedQuests: entries,
			},
		})
	}

	return nil
}

func sendQuestDialog(p *player.Player, q *quest.Quest, state *quest.State, questID, sessionID int) error {
	var dialogEntries []server.DialogEntry
	var questEntries []server.DialogQuestEntry

	for _, action := range state.Actions {
		lower := strings.ToLower(action.Name)
		switch lower {
		case "addnpctext":
			if len(action.Args) >= 2 && action.Args[1].IsStr {
				dialogEntries = append(dialogEntries, server.DialogEntry{
					EntryType: server.DialogEntry_Text,
					Line:      action.Args[1].StrVal,
				})
			}
		case "addnpcinput":
			if len(action.Args) >= 3 && action.Args[2].IsStr {
				linkID := 0
				if len(action.Args) >= 2 && !action.Args[1].IsStr {
					linkID = action.Args[1].IntVal
				}
				dialogEntries = append(dialogEntries, server.DialogEntry{
					EntryType:     server.DialogEntry_Link,
					EntryTypeData: &server.EntryTypeDataLink{LinkId: linkID},
					Line:          action.Args[2].StrVal,
				})
			}
		}
	}

	return p.Bus.SendPacket(&server.QuestDialogServerPacket{
		BehaviorId:    0,
		QuestId:       questID,
		SessionId:     sessionID,
		DialogId:      0,
		QuestEntries:  questEntries,
		DialogEntries: dialogEntries,
	})
}

// executeQuestRewards processes reward actions from a quest state (GiveItem, GiveExp, etc).
func executeQuestRewards(p *player.Player, state *quest.State) {
	for _, action := range state.Actions {
		lower := strings.ToLower(action.Name)
		switch lower {
		case "giveitem":
			// GiveItem(item_id, amount)
			if len(action.Args) >= 2 && !action.Args[0].IsStr && !action.Args[1].IsStr {
				itemID := action.Args[0].IntVal
				amount := action.Args[1].IntVal
				p.Inventory[itemID] += amount
			}
		case "giveexp":
			// GiveExp(amount)
			if len(action.Args) >= 1 && !action.Args[0].IsStr {
				p.CharExp += action.Args[0].IntVal
			}
		case "removeitem":
			// RemoveItem(item_id, amount)
			if len(action.Args) >= 2 && !action.Args[0].IsStr && !action.Args[1].IsStr {
				p.RemoveItem(action.Args[0].IntVal, action.Args[1].IntVal)
			}
		case "setclass":
			// SetClass(class_id) - placeholder
		case "setrace":
			// SetRace(race_id) - placeholder
		}
	}
}

package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/avdo/goeoserv/internal/deep"
	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_AdminInteract, eonet.PacketAction_Take, handleAdminInteractTake)
	player.Register(eonet.PacketFamily_AdminInteract, eonet.PacketAction_Tell, handleAdminInteractTell)
	player.Register(eonet.PacketFamily_AdminInteract, eonet.PacketAction_Report, handleAdminInteractReport)
}

func handleAdminInteractTell(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.AdminInteractTellClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize admin interact tell", "id", p.ID, "err", err)
		return nil
	}

	message := strings.TrimSpace(pkt.Message)
	if message == "" {
		return nil
	}

	p.World.BroadcastToAdmins(0, 1, &server.AdminInteractReplyServerPacket{
		MessageType: server.AdminMessage_Message,
		MessageTypeData: &server.AdminInteractReplyMessageTypeDataMessage{
			PlayerName: p.CharName,
			Message:    message,
		},
	})
	return nil
}

func handleAdminInteractReport(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.AdminInteractReportClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize admin interact report", "id", p.ID, "err", err)
		return nil
	}

	message := strings.TrimSpace(pkt.Message)
	reportee := strings.TrimSpace(pkt.Reportee)
	if message == "" || reportee == "" {
		return nil
	}

	p.World.BroadcastToAdmins(0, 1, &server.AdminInteractReplyServerPacket{
		MessageType: server.AdminMessage_Report,
		MessageTypeData: &server.AdminInteractReplyMessageTypeDataReport{
			PlayerName:   p.CharName,
			Message:      message,
			ReporteeName: reportee,
		},
	})
	p.World.BroadcastToAdmins(0, 1, &server.TalkServerServerPacket{
		Message: formatAdminReportSummary(p, reportee, message),
	})
	return nil
}

func formatAdminReportSummary(p *player.Player, reportee string, message string) string {
	summary := fmt.Sprintf("Report: %s reported %s - %s", p.CharName, reportee, message)
	if p.World == nil {
		return summary
	}

	reporteeID, found := p.World.FindPlayerByName(strings.ToLower(reportee))
	if !found {
		return summary + " [target offline]"
	}

	pos, _ := p.World.GetPlayerPosition(reporteeID).(*gamemap.PlayerPosition)
	if pos == nil {
		return summary + " [target online]"
	}

	return fmt.Sprintf("%s [target online: map %d (%d, %d)]", summary, pos.MapID, pos.X, pos.Y)
}

func handleAdminInteractTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil || !p.IsDeep() || !p.Cfg.World.InfoRevealsDrops {
		return nil
	}
	req, err := deep.DeserializeAdminInteractTake(reader)
	if err != nil {
		return nil
	}
	var lines []deep.DialogLine
	switch req.LookupType {
	case deep.LookupTypeItem:
		lines = buildItemDropLookup(req.ID)
	case deep.LookupTypeNpc:
		lines = buildNpcDropLookup(req.ID)
	default:
		return nil
	}
	if len(lines) == 0 {
		return nil
	}
	payload, err := deep.SerializeAdminInteractAdd(lines)
	if err != nil {
		return nil
	}
	return p.Bus.Send(eonet.PacketAction_Add, eonet.PacketFamily_AdminInteract, payload)
}

func buildItemDropLookup(itemID int) []deep.DialogLine {
	if pubdata.DropDB == nil {
		return nil
	}
	lines := []deep.DialogLine{{Left: " ", Right: ""}, {Left: "Drops:", Right: ""}}
	for _, npc := range pubdata.DropDB.Npcs {
		for _, drop := range npc.Drops {
			if drop.ItemId != itemID || drop.MinAmount <= 0 || drop.MaxAmount <= 0 {
				continue
			}
			npcRec := pubdata.GetNpc(npc.NpcId)
			if npcRec == nil {
				continue
			}
			lines = append(lines, deep.DialogLine{
				Left:  npcRec.Name,
				Right: dropRateString(drop.Rate),
			})
		}
	}
	if len(lines) <= 2 {
		return nil
	}
	return lines
}

func buildNpcDropLookup(npcID int) []deep.DialogLine {
	drops := pubdata.GetNpcDrops(npcID)
	if len(drops) == 0 {
		return nil
	}
	lines := []deep.DialogLine{{Left: " ", Right: ""}, {Left: "Drops:", Right: ""}}
	for _, drop := range drops {
		if drop.MinAmount <= 0 || drop.MaxAmount <= 0 {
			continue
		}
		item := pubdata.GetItem(drop.ItemId)
		if item == nil {
			continue
		}
		lines = append(lines, deep.DialogLine{
			Left:  item.Name,
			Right: dropRateString(drop.Rate),
		})
	}
	if len(lines) <= 2 {
		return nil
	}
	return lines
}

func dropRateString(rate int) string {
	pct := float64(rate) / 64000.0 * 100.0
	s := strconv.FormatFloat(pct, 'f', 2, 64)
	return s + "%"
}

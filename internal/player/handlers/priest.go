package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/protocol"
	"github.com/avdo/goeoserv/internal/world"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Open, handlePriestOpen)
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Request, handlePriestRequest)
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Accept, handlePriestAccept)
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Use, handlePriestUse)
}

func handlePriestOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.PriestOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize priest open", "id", p.ID, "err", err)
		return nil
	}
	_ = pkt

	return p.Bus.SendPacket(&server.PriestOpenServerPacket{SessionId: p.GenerateSessionID()})
}

func handlePriestRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil || p.CharacterID == nil {
		return nil
	}

	var pkt client.PriestRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize priest request", "id", p.ID, "err", err)
		return nil
	}

	if !p.TakeAndValidateSessionID(pkt.SessionId) {
		return nil
	}

	partnerName := strings.TrimSpace(strings.ToLower(pkt.Name))
	if partnerName == "" || partnerName == strings.ToLower(p.CharName) {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_PartnerNotPresent})
	}

	partnerID, found := p.World.FindPlayerByName(partnerName)
	if !found {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_PartnerNotPresent})
	}

	partnerPos, _ := p.World.GetPlayerPosition(partnerID).(*gamemap.PlayerPosition)
	if partnerPos == nil || partnerPos.MapID != p.MapID {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_PartnerNotPresent})
	}

	partnerBus, _ := p.World.GetPlayerBus(partnerID).(*protocol.PacketBus)
	if partnerBus == nil {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_PartnerNotPresent})
	}

	playerStatus, err := loadCharacterMarriageStatusByID(ctx, p, *p.CharacterID)
	if err != nil {
		return nil
	}
	partnerStatus, err := loadCharacterMarriageStatusByName(ctx, p, partnerName)
	if errors.Is(err, sql.ErrNoRows) {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_PartnerNotPresent})
	}
	if err != nil {
		return nil
	}
	if playerStatus.Level < p.Cfg.Marriage.MinLevel || partnerStatus.Level < p.Cfg.Marriage.MinLevel {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_LowLevel})
	}
	if playerStatus.Partner != "" || partnerStatus.Partner != "" {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_PartnerAlreadyMarried})
	}
	if !strings.EqualFold(playerStatus.Fiance, partnerStatus.Name) || !strings.EqualFold(partnerStatus.Fiance, p.CharName) {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_NoPermission})
	}
	if world.GetWedding(p.MapID) != nil {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_Busy})
	}

	if !world.StartWedding(p.MapID, p.ID, partnerID, 0, p.Bus, partnerBus) {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_Busy})
	}

	if err := partnerBus.SendPacket(&server.PriestRequestServerPacket{
		SessionId:   1,
		PartnerName: p.CharName,
	}); err != nil {
		world.EndWedding(p.MapID)
		return nil
	}

	_ = p.Bus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding request sent. Waiting for your partner to accept."})
	_ = partnerBus.SendPacket(&server.TalkServerServerPacket{Message: p.CharName + " is waiting for your answer at the priest."})
	return nil
}

func handlePriestAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.PriestAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize priest accept", "id", p.ID, "err", err)
		return nil
	}
	_ = pkt

	if !world.AcceptWedding(p.MapID, p.ID) {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_Busy})
	}
	if playerID, partnerID, ok := world.Participants(p.MapID); ok {
		_ = p.Bus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony accepted. Listen for the priest's vows."})
		if playerID == p.ID {
			if partnerBus, _ := p.World.GetPlayerBus(partnerID).(*protocol.PacketBus); partnerBus != nil {
				_ = partnerBus.SendPacket(&server.TalkServerServerPacket{Message: p.CharName + " accepted the wedding ceremony."})
			}
		} else if playerBus, _ := p.World.GetPlayerBus(playerID).(*protocol.PacketBus); playerBus != nil {
			_ = playerBus.SendPacket(&server.TalkServerServerPacket{Message: p.CharName + " accepted the wedding ceremony."})
		}
	}
	p.GenerateSessionID()
	return nil
}

func handlePriestUse(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.PriestUseClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize priest use", "id", p.ID, "err", err)
		return nil
	}
	_ = pkt

	if !world.RespondIDo(p.MapID, p.ID) {
		return p.Bus.SendPacket(&server.PriestReplyServerPacket{ReplyCode: server.PriestReply_Busy})
	}
	playerID, partnerID, ok := world.BeginWeddingFinalization(p.MapID)
	if !ok {
		return nil
	}
	playerName := p.World.GetPlayerName(playerID)
	partnerName := p.World.GetPlayerName(partnerID)
	partnerBus, _ := p.World.GetPlayerBus(partnerID).(*protocol.PacketBus)
	if playerName == "" || partnerName == "" || partnerBus == nil {
		_ = p.Bus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
		if partnerBus != nil {
			_ = partnerBus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
		}
		world.EndWedding(p.MapID)
		return nil
	}
	tx, err := p.DB.BeginTx(ctx)
	if err != nil {
		world.EndWedding(p.MapID)
		return nil
	}
	defer tx.Rollback() //nolint:errcheck
	if err := execRelationshipUpdate(ctx, tx, `UPDATE characters SET fiance = '', partner = ? WHERE LOWER(name) = ?`, partnerName, strings.ToLower(playerName)); err != nil {
		_ = p.Bus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
		_ = partnerBus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
		world.EndWedding(p.MapID)
		return nil
	}
	if err := execRelationshipUpdate(ctx, tx, `UPDATE characters SET fiance = '', partner = ? WHERE LOWER(name) = ?`, playerName, strings.ToLower(partnerName)); err != nil {
		_ = p.Bus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
		_ = partnerBus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
		world.EndWedding(p.MapID)
		return nil
	}
	if err := tx.Commit(); err != nil {
		_ = p.Bus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
		_ = partnerBus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
		world.EndWedding(p.MapID)
		return nil
	}
	world.EndWedding(p.MapID)
	_ = p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_Success, ReplyCodeData: &server.MarriageReplyReplyCodeDataSuccess{GoldAmount: p.Inventory[1]}})
	if partnerBus != nil {
		_ = partnerBus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_Success, ReplyCodeData: &server.MarriageReplyReplyCodeDataSuccess{GoldAmount: 0}})
	}
	return nil
}

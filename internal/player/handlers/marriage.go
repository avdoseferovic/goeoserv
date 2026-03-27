package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Marriage, eonet.PacketAction_Open, handleMarriageOpen)
	player.Register(eonet.PacketFamily_Marriage, eonet.PacketAction_Request, handleMarriageRequest)
}

func handleMarriageOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.MarriageOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize marriage open", "id", p.ID, "err", err)
		return nil
	}
	_ = pkt

	return p.Bus.SendPacket(&server.MarriageOpenServerPacket{SessionId: p.GenerateSessionID()})
}

func handleMarriageRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}

	var pkt client.MarriageRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize marriage request", "id", p.ID, "err", err)
		return nil
	}

	if !p.TakeAndValidateSessionID(pkt.SessionId) {
		return nil
	}

	partnerName := strings.TrimSpace(strings.ToLower(pkt.Name))
	if partnerName == "" {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_WrongName})
	}

	switch pkt.RequestType {
	case client.MarriageRequest_Divorce:
		return handleMarriageDivorce(ctx, p, partnerName)
	case client.MarriageRequest_MarriageApproval:
		return handleMarriageApproval(ctx, p, partnerName)
	default:
		return nil
	}
}

func handleMarriageApproval(ctx context.Context, p *player.Player, partnerName string) error {
	if strings.EqualFold(partnerName, p.CharName) {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_WrongName})
	}

	currentStatus, err := loadCharacterMarriageStatusByID(ctx, p, *p.CharacterID)
	if err != nil {
		return nil
	}
	if currentStatus.Partner != "" {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_AlreadyMarried})
	}
	if currentStatus.Fiance != "" && !strings.EqualFold(currentStatus.Fiance, partnerName) {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_WrongName})
	}

	partnerStatus, err := loadCharacterMarriageStatusByName(ctx, p, partnerName)
	if errors.Is(err, sql.ErrNoRows) {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_WrongName})
	}
	if err != nil {
		return nil
	}
	if strings.EqualFold(partnerStatus.Name, p.CharName) {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_WrongName})
	}
	if partnerStatus.Partner != "" {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_AlreadyMarried})
	}
	if partnerStatus.Fiance != "" && !strings.EqualFold(partnerStatus.Fiance, p.CharName) {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_WrongName})
	}
	if cost := p.Cfg.Marriage.ApprovalCost; cost > 0 && !p.RemoveItem(1, cost) {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_NotEnoughGold})
	}
	tx, err := p.DB.BeginTx(ctx)
	if err != nil {
		return nil
	}
	defer tx.Rollback() //nolint:errcheck
	if err := execRelationshipUpdate(ctx, tx, `UPDATE characters SET fiance = ? WHERE id = ?`, partnerStatus.Name, *p.CharacterID); err != nil {
		return nil
	}
	if err := execRelationshipUpdate(ctx, tx, `UPDATE characters SET fiance = ? WHERE LOWER(name) = ?`, p.CharName, partnerName); err != nil {
		return nil
	}
	if err := tx.Commit(); err != nil {
		return nil
	}
	return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_Success, ReplyCodeData: &server.MarriageReplyReplyCodeDataSuccess{GoldAmount: p.Inventory[1]}})
}

func handleMarriageDivorce(ctx context.Context, p *player.Player, partnerName string) error {
	currentStatus, err := loadCharacterMarriageStatusByID(ctx, p, *p.CharacterID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		slog.Error("failed to load current partner", "id", p.ID, "err", err)
		return nil
	}

	if !strings.EqualFold(currentStatus.Partner, partnerName) {
		return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_NotMarried})
	}

	if cost := p.Cfg.Marriage.DivorceCost; cost > 0 {
		if !p.RemoveItem(1, cost) {
			return p.Bus.SendPacket(&server.MarriageReplyServerPacket{ReplyCode: server.MarriageReply_NotEnoughGold})
		}
	}

	tx, err := p.DB.BeginTx(ctx)
	if err != nil {
		slog.Error("failed to begin divorce transaction", "id", p.ID, "err", err)
		return nil
	}
	defer tx.Rollback() //nolint:errcheck

	if err := execRelationshipUpdate(ctx, tx, `UPDATE characters SET fiance = '', partner = '' WHERE id = ?`, *p.CharacterID); err != nil {
		slog.Error("failed to clear player partner", "id", p.ID, "err", err)
		return nil
	}
	if err := execRelationshipUpdate(ctx, tx, `UPDATE characters SET fiance = '', partner = '' WHERE LOWER(name) = ?`, partnerName); err != nil {
		slog.Error("failed to clear partner relationship", "id", p.ID, "err", err)
		return nil
	}
	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit divorce transaction", "id", p.ID, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.MarriageReplyServerPacket{
		ReplyCode: server.MarriageReply_Success,
		ReplyCodeData: &server.MarriageReplyReplyCodeDataSuccess{
			GoldAmount: p.Inventory[1],
		},
	})
}

type characterMarriageStatus struct {
	Name    string
	Level   int
	Fiance  string
	Partner string
}

func loadCharacterMarriageStatusByID(ctx context.Context, p *player.Player, characterID int) (characterMarriageStatus, error) {
	return loadCharacterMarriageStatus(ctx, p, `WHERE id = ?`, characterID)
}

func loadCharacterMarriageStatusByName(ctx context.Context, p *player.Player, name string) (characterMarriageStatus, error) {
	return loadCharacterMarriageStatus(ctx, p, `WHERE LOWER(name) = ?`, strings.ToLower(name))
}

func loadCharacterMarriageStatus(ctx context.Context, p *player.Player, whereClause string, arg any) (characterMarriageStatus, error) {
	var status characterMarriageStatus
	err := p.DB.QueryRow(ctx,
		`SELECT COALESCE(name, ''), COALESCE(level, 0), COALESCE(fiance, ''), COALESCE(partner, '') FROM characters `+whereClause,
		arg,
	).Scan(&status.Name, &status.Level, &status.Fiance, &status.Partner)
	return status, err
}

func execRelationshipUpdate(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

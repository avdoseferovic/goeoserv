package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	"github.com/avdo/goeoserv/internal/deep"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/player/handlers/account"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Account, eonet.PacketAction_Request, handleAccountRequest)
	player.Register(eonet.PacketFamily_Account, eonet.PacketAction_Accept, handleAccountAccept)
	player.Register(eonet.PacketFamily_Account, eonet.PacketAction_Create, handleAccountCreate)
	player.Register(eonet.PacketFamily_Account, eonet.PacketAction_Agree, handleAccountAgree)
}

// handleAccountRequest handles the pre-creation check (does account exist?).
func handleAccountRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.AccountRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize account request", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateAccepted {
		return nil
	}

	exists, err := account.Exists(ctx, p.DB, pkt.Username)
	if err != nil {
		slog.Error("error checking account exists", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if exists {
		return p.Bus.SendPacket(&server.AccountReplyServerPacket{
			ReplyCode:     server.AccountReply_Exists,
			ReplyCodeData: &server.AccountReplyReplyCodeDataExists{},
		})
	}

	if p.IsDeep() {
		payload, err := deep.SerializeAccountConfig(p.Cfg.Account.DelayMinutes(), p.Cfg.Account.EmailValidation)
		if err == nil {
			_ = p.Bus.Send(eonet.PacketAction(deep.ActionConfig), eonet.PacketFamily_Account, payload)
		}
	}

	sessionID := p.GenerateSessionID()

	return p.Bus.SendPacket(&server.AccountReplyServerPacket{
		ReplyCode: server.AccountReply(sessionID),
		ReplyCodeData: &server.AccountReplyReplyCodeDataDefault{
			SequenceStart: p.Bus.CurrentSequenceStart(),
		},
	})
}

func handleAccountAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if !p.IsDeep() || p.State != player.StateAccepted {
		return nil
	}
	req, err := deep.DeserializeAccountAccept(reader)
	if err != nil {
		return nil
	}
	_ = req.SequenceNumber
	payload, err := deep.SerializeAccountAcceptReply(1)
	if err != nil {
		return nil
	}
	if p.Cfg.Account.EmailValidation {
		sender := account.NewSender(p.Cfg)
		status := sender.Status()
		if err := sender.SendAccountValidation(ctx, account.ValidationEmail{
			AccountName: req.AccountName,
			Email:       req.EmailAddress,
		}); err != nil {
			slog.Warn("email validation requested but no email was sent",
				"id", p.ID,
				"account", req.AccountName,
				"configured", status.Configured,
				"reason", status.Reason,
			)
		}
	}
	return p.Bus.Send(eonet.PacketAction_Accept, eonet.PacketFamily_Account, payload)
}

// handleAccountCreate creates a new account.
func handleAccountCreate(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.AccountCreateClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize account create", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateAccepted {
		return nil
	}

	if !p.TakeAndValidateSessionID(pkt.SessionId) {
		slog.Warn("wrong session id in account create", "id", p.ID)
		p.Close()
		return nil
	}

	exists, err := account.Exists(ctx, p.DB, pkt.Username)
	if err != nil {
		slog.Error("error checking account exists", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if exists {
		return p.Bus.SendPacket(&server.AccountReplyServerPacket{
			ReplyCode:     server.AccountReply_Exists,
			ReplyCodeData: &server.AccountReplyReplyCodeDataExists{},
		})
	}

	passwordHash := account.HashPassword(pkt.Username, pkt.Password)

	result, err := p.DB.DB().ExecContext(ctx,
		`INSERT INTO accounts (name, password_hash, real_name, location, email, computer, hdid)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		strings.ToLower(pkt.Username), passwordHash, pkt.FullName, pkt.Location,
		pkt.Email, pkt.Computer, pkt.Hdid)
	if err != nil {
		slog.Error("error creating account", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	accountID, _ := result.LastInsertId()
	p.AccountID = int(accountID)

	slog.Info("new account created", "name", pkt.Username, "id", p.ID)

	return p.Bus.SendPacket(&server.AccountReplyServerPacket{
		ReplyCode:     server.AccountReply_Created,
		ReplyCodeData: &server.AccountReplyReplyCodeDataCreated{},
	})
}

// handleAccountAgree handles password change.
func handleAccountAgree(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.AccountAgreeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize account agree", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateLoggedIn {
		return nil
	}

	p.LoginAttempts++
	if p.AccountID <= 0 {
		return p.Bus.SendPacket(&server.AccountReplyServerPacket{
			ReplyCode:     server.AccountReply_ChangeFailed,
			ReplyCodeData: &server.AccountReplyReplyCodeDataChangeFailed{},
		})
	}

	var username, passwordHash string
	err := p.DB.QueryRow(ctx,
		`SELECT name, password_hash FROM accounts WHERE id = ?`,
		p.AccountID).Scan(&username, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		if p.LoginAttempts >= p.Cfg.Server.MaxLoginAttempts {
			p.Close()
			return nil
		}
		return p.Bus.SendPacket(&server.AccountReplyServerPacket{
			ReplyCode:     server.AccountReply_ChangeFailed,
			ReplyCodeData: &server.AccountReplyReplyCodeDataChangeFailed{},
		})
	}
	if err != nil {
		slog.Error("error getting password hash", "id", p.ID, "err", err)
		return p.Bus.SendPacket(&server.AccountReplyServerPacket{
			ReplyCode:     server.AccountReply_ChangeFailed,
			ReplyCodeData: &server.AccountReplyReplyCodeDataChangeFailed{},
		})
	}

	if !strings.EqualFold(pkt.Username, username) {
		return p.Bus.SendPacket(&server.AccountReplyServerPacket{
			ReplyCode:     server.AccountReply_ChangeFailed,
			ReplyCodeData: &server.AccountReplyReplyCodeDataChangeFailed{},
		})
	}

	if !account.ValidatePassword(username, pkt.OldPassword, passwordHash) {
		if p.LoginAttempts >= p.Cfg.Server.MaxLoginAttempts {
			p.Close()
			return nil
		}
		return p.Bus.SendPacket(&server.AccountReplyServerPacket{
			ReplyCode:     server.AccountReply_ChangeFailed,
			ReplyCodeData: &server.AccountReplyReplyCodeDataChangeFailed{},
		})
	}

	p.LoginAttempts = 0
	newHash := account.HashPassword(username, pkt.NewPassword)

	if err := p.DB.Execute(ctx,
		`UPDATE accounts SET password_hash = ? WHERE id = ?`, newHash, p.AccountID); err != nil {
		slog.Error("error updating password", "id", p.ID, "err", err)
		p.Close()
		return nil
	}
	if err := p.DB.Execute(ctx,
		`DELETE FROM account_sessions WHERE account_id = ?`, p.AccountID); err != nil {
		slog.Warn("failed to revoke remembered sessions after password change", "id", p.ID, "account_id", p.AccountID, "err", err)
	}

	return p.Bus.SendPacket(&server.AccountReplyServerPacket{
		ReplyCode:     server.AccountReply_Changed,
		ReplyCodeData: &server.AccountReplyReplyCodeDataChanged{},
	})
}

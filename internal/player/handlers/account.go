package handlers

import (
	"errors"
	"context"
	"database/sql"
	"log/slog"
	"strings"

	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/player/handlers/account"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Account, eonet.PacketAction_Request, handleAccountRequest)
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

	sessionID := p.GenerateSessionID()

	return p.Bus.SendPacket(&server.AccountReplyServerPacket{
		ReplyCode: server.AccountReply(sessionID),
		ReplyCodeData: &server.AccountReplyReplyCodeDataDefault{
			SequenceStart: p.Bus.Sequencer.Start(),
		},
	})
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

	sessionID, ok := p.TakeSessionID()
	if !ok || sessionID != pkt.SessionId {
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

	// Verify account exists
	var accountID int
	var username, passwordHash string
	err := p.DB.QueryRow(ctx,
		`SELECT id, name, password_hash FROM accounts WHERE name = ?`,
		strings.ToLower(pkt.Username)).Scan(&accountID, &username, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		if p.LoginAttempts >= p.Cfg.Server.MaxLoginAttempts {
			p.Close()
			return nil
		}
		return p.Bus.SendPacket(&server.AccountReplyServerPacket{
			ReplyCode:     server.AccountReply_Exists,
			ReplyCodeData: &server.AccountReplyReplyCodeDataExists{},
		})
	}
	if err != nil {
		slog.Error("error getting password hash", "id", p.ID, "err", err)
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
		`UPDATE accounts SET password_hash = ? WHERE id = ?`, newHash, accountID); err != nil {
		slog.Error("error updating password", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	return p.Bus.SendPacket(&server.AccountReplyServerPacket{
		ReplyCode:     server.AccountReply_Changed,
		ReplyCodeData: &server.AccountReplyReplyCodeDataChanged{},
	})
}

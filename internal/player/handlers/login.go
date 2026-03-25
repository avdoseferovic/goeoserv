package handlers

import (
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
	player.Register(eonet.PacketFamily_Login, eonet.PacketAction_Request, handleLoginRequest)
}

func handleLoginRequest(p *player.Player, reader *player.EoReader) error {
	var pkt client.LoginRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize login request", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateAccepted {
		slog.Warn("login before connection accepted", "id", p.ID)
		p.Close()
		return nil
	}

	// TODO: Check player count vs max_players (needs world integration)

	p.LoginAttempts++

	// Check account exists
	exists, err := account.Exists(p.DB, pkt.Username)
	if err != nil {
		slog.Error("error checking account", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if !exists {
		if p.LoginAttempts >= p.Cfg.Server.MaxLoginAttempts {
			p.Close()
			return nil
		}
		return p.Bus.SendPacket(&server.LoginReplyServerPacket{
			ReplyCode:     server.LoginReply_WrongUser,
			ReplyCodeData: &server.LoginReplyReplyCodeDataWrongUser{},
		})
	}

	// Check ban (duration=0 means permanent, otherwise duration in minutes from created_at)
	var banned bool
	var banCount int
	err = p.DB.QueryRow(context.Background(),
		`SELECT COUNT(1) FROM bans WHERE account_id = (SELECT id FROM accounts WHERE name = ?)
		 AND (duration = 0 OR datetime(created_at, '+' || duration || ' minutes') > datetime('now'))`,
		strings.ToLower(pkt.Username)).Scan(&banCount)
	if err != nil && err != sql.ErrNoRows {
		slog.Error("error checking ban", "id", p.ID, "err", err)
	}
	banned = banCount > 0

	if banned {
		_ = p.Bus.SendPacket(&server.LoginReplyServerPacket{
			ReplyCode:     server.LoginReply_Banned,
			ReplyCodeData: &server.LoginReplyReplyCodeDataBanned{},
		})
		p.Close()
		return nil
	}

	// Get password hash
	var accountID int
	var username, passwordHash string
	err = p.DB.QueryRow(context.Background(),
		`SELECT id, name, password_hash FROM accounts WHERE name = ?`,
		strings.ToLower(pkt.Username)).Scan(&accountID, &username, &passwordHash)
	if err != nil {
		slog.Error("error getting password hash", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if !account.ValidatePassword(username, pkt.Password, passwordHash) {
		if p.LoginAttempts >= p.Cfg.Server.MaxLoginAttempts {
			p.Close()
			return nil
		}
		return p.Bus.SendPacket(&server.LoginReplyServerPacket{
			ReplyCode:     server.LoginReply_WrongUserPassword,
			ReplyCodeData: &server.LoginReplyReplyCodeDataWrongUserPassword{},
		})
	}

	// TODO: Check if already logged in (needs world integration)

	// Get character list
	characters, err := account.GetCharacterList(p.DB, accountID)
	if err != nil {
		slog.Error("error getting character list", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	p.AccountID = accountID
	p.State = player.StateLoggedIn
	p.LoginAttempts = 0

	slog.Info("player logged in", "id", p.ID, "account", username)

	return p.Bus.SendPacket(&server.LoginReplyServerPacket{
		ReplyCode: server.LoginReply_Ok,
		ReplyCodeData: &server.LoginReplyReplyCodeDataOk{
			Characters: characters,
		},
	})
}

package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/avdoseferovic/geoserv/internal/deep"
	"github.com/avdoseferovic/geoserv/internal/player"
	"github.com/avdoseferovic/geoserv/internal/player/handlers/account"
	"github.com/ethanmoffat/eolib-go/v3/data"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Login, eonet.PacketAction_Request, handleLoginRequest)
	player.Register(eonet.PacketFamily_Login, eonet.PacketAction_Use, handleLoginUse)
	player.Register(eonet.PacketFamily_Login, eonet.PacketAction_Take, handleLoginTake)
	player.Register(eonet.PacketFamily_Login, eonet.PacketAction_Create, handleLoginCreate)
	player.Register(eonet.PacketFamily_Login, eonet.PacketAction_Accept, handleLoginAccept)
	player.Register(eonet.PacketFamily_Login, eonet.PacketAction_Agree, handleLoginAgree)
}

func handleLoginRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
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

	if err := sendLoginBusyIfFull(p); err != nil || p.State != player.StateAccepted {
		return err
	}

	p.LoginAttempts++
	rememberMe := false
	if reader.Remaining() > 0 {
		rememberMe = reader.GetChar() == 1
	}

	// Check account exists
	exists, err := account.Exists(ctx, p.DB, pkt.Username)
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

	// Get password hash
	var accountID int
	var username, passwordHash string
	err = p.DB.QueryRow(ctx,
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

	if banned, err := isAccountBanned(ctx, p, accountID); err != nil {
		slog.Error("error checking ban", "id", p.ID, "err", err)
		p.Close()
		return nil
	} else if banned {
		return sendBannedLoginReply(p)
	}

	// Check if already logged in
	if p.World != nil && p.World.IsLoggedIn(accountID) {
		return p.Bus.SendPacket(&server.LoginReplyServerPacket{
			ReplyCode:     server.LoginReply_LoggedIn,
			ReplyCodeData: &server.LoginReplyReplyCodeDataLoggedIn{},
		})
	}

	return completeLogin(ctx, p, accountID, username, rememberMe)
}

func handleLoginUse(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateAccepted {
		slog.Warn("session resume before connection accepted", "id", p.ID)
		p.Close()
		return nil
	}

	if err := sendLoginBusyIfFull(p); err != nil || p.State != player.StateAccepted {
		return err
	}

	p.LoginAttempts++
	token, err := reader.GetString()
	if err != nil || !account.IsSessionTokenFormatValid(token) {
		return sendInvalidLoginToken(p)
	}

	session, err := account.GetSessionAccount(ctx, p.DB, token)
	if errors.Is(err, sql.ErrNoRows) {
		return sendInvalidLoginToken(p)
	}
	if err != nil {
		slog.Error("error loading account session", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if banned, err := isAccountBanned(ctx, p, session.AccountID); err != nil {
		slog.Error("error checking ban", "id", p.ID, "err", err)
		p.Close()
		return nil
	} else if banned {
		return sendBannedLoginReply(p)
	}

	if p.World != nil && p.World.IsLoggedIn(session.AccountID) {
		return p.Bus.SendPacket(&server.LoginReplyServerPacket{
			ReplyCode:     server.LoginReply_LoggedIn,
			ReplyCodeData: &server.LoginReplyReplyCodeDataLoggedIn{},
		})
	}

	return completeLogin(ctx, p, session.AccountID, session.Username, true)
}

func handleLoginTake(_ context.Context, p *player.Player, _ *player.EoReader) error {
	if !p.IsDeep() {
		return nil
	}
	code := 4
	if p.Cfg.Account.Recovery {
		code = 1
	}
	payload, err := deep.SerializeShortCode(code)
	if err != nil {
		return nil
	}
	return p.Bus.Send(eonet.PacketAction_Take, eonet.PacketFamily_Login, payload)
}

func handleLoginCreate(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if !p.IsDeep() {
		return nil
	}
	req, err := deep.DeserializeLoginCreate(reader)
	if err != nil {
		return nil
	}
	p.ClearRecoveryState()
	var accountID int
	var email string
	err = p.DB.QueryRow(ctx, `SELECT id, email FROM accounts WHERE name = ?`, strings.ToLower(req.AccountName)).Scan(&accountID, &email)
	if errors.Is(err, sql.ErrNoRows) {
		payload, _ := deep.SerializeLoginCreateReply(0, "")
		return p.Bus.Send(eonet.PacketAction_Create, eonet.PacketFamily_Login, payload)
	}
	if err != nil {
		return nil
	}
	p.AccountID = accountID
	p.StartRecovery(req.AccountName, generateEmailPin(), time.Now())
	sender := account.NewSender(p.Cfg)
	status := sender.Status()
	sendErr := sender.SendRecoveryPIN(ctx, account.RecoveryEmail{
		AccountName: req.AccountName,
		Email:       email,
		PIN:         p.EmailPin,
		ExpiresIn:   p.Cfg.Account.DelayDuration(),
	})
	if sendErr != nil {
		slog.Warn("recovery email not sent",
			"id", p.ID,
			"account", req.AccountName,
			"configured", status.Configured,
			"reason", status.Reason,
		)
		slog.Debug("recovery pin generated for local troubleshooting",
			"id", p.ID,
			"account", req.AccountName,
			"pin", p.EmailPin,
			"expires_at", p.RecoveryPinExpiresAt,
		)
	}
	masked := ""
	if sendErr == nil && p.Cfg.Account.RecoveryShowEmail {
		masked = email
		if p.Cfg.Account.RecoveryMaskEmail {
			masked = account.MaskEmail(email)
		}
	}
	code := 1
	if masked != "" {
		code = 2
	}
	payload, _ := deep.SerializeLoginCreateReply(code, masked)
	return p.Bus.Send(eonet.PacketAction_Create, eonet.PacketFamily_Login, payload)
}

func handleLoginAccept(_ context.Context, p *player.Player, reader *player.EoReader) error {
	if !p.IsDeep() {
		return nil
	}
	pin, err := deep.DeserializeLoginAccept(reader)
	if err != nil {
		return nil
	}
	code := 0
	if p.HasActiveRecoveryPIN(time.Now()) && pin == p.EmailPin {
		code = 1
	}
	payload, _ := deep.SerializeShortCode(code)
	return p.Bus.Send(eonet.PacketAction_Accept, eonet.PacketFamily_Login, payload)
}

func handleLoginAgree(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if !p.IsDeep() {
		return nil
	}
	req, err := deep.DeserializeLoginAgree(reader)
	if err != nil {
		return nil
	}
	code := 0
	if p.HasActiveRecoveryPIN(time.Now()) && req.Pin == p.EmailPin && strings.EqualFold(req.AccountName, p.RecoveryAccountName) {
		var accountID int
		err = p.DB.QueryRow(ctx, `SELECT id FROM accounts WHERE name = ?`, strings.ToLower(req.AccountName)).Scan(&accountID)
		if err == nil {
			newHash := account.HashPassword(req.AccountName, req.Password)
			if p.DB.Execute(ctx, `UPDATE accounts SET password_hash = ? WHERE id = ?`, newHash, accountID) == nil {
				_ = p.DB.Execute(ctx, `DELETE FROM account_sessions WHERE account_id = ?`, accountID)
				code = 1
				p.ClearRecoveryState()
			}
		}
	}
	payload, _ := deep.SerializeShortCode(code)
	return p.Bus.Send(eonet.PacketAction_Agree, eonet.PacketFamily_Login, payload)
}

func generateEmailPin() string {
	const digits = "0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = digits[rand.IntN(len(digits))]
	}
	return string(b)
}

func sendLoginBusyIfFull(p *player.Player) error {
	if p.World == nil || p.Cfg.Server.MaxPlayers <= 0 || p.World.OnlinePlayerCount() < p.Cfg.Server.MaxPlayers {
		return nil
	}

	return p.Bus.SendPacket(&server.LoginReplyServerPacket{
		ReplyCode:     server.LoginReply_Busy,
		ReplyCodeData: &server.LoginReplyReplyCodeDataBusy{},
	})
}

func isAccountBanned(ctx context.Context, p *player.Player, accountID int) (bool, error) {
	var banCount int
	expiryExpr := p.DB.AddMinutesExpr("created_at", "duration")
	nowExpr := p.DB.CurrentTimestampExpr()
	err := p.DB.QueryRow(ctx,
		`SELECT COUNT(1) FROM bans
		 WHERE account_id = ?
		 AND (duration = 0 OR `+expiryExpr+` > `+nowExpr+`)`,
		accountID,
	).Scan(&banCount)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return banCount > 0, nil
}

func sendBannedLoginReply(p *player.Player) error {
	_ = p.Bus.SendPacket(&server.LoginReplyServerPacket{
		ReplyCode:     server.LoginReply_Banned,
		ReplyCodeData: &server.LoginReplyReplyCodeDataBanned{},
	})
	p.Close()
	return nil
}

func sendInvalidLoginToken(p *player.Player) error {
	if p.LoginAttempts >= p.Cfg.Server.MaxLoginAttempts {
		p.Close()
		return nil
	}

	return p.Bus.SendPacket(&server.LoginReplyServerPacket{
		ReplyCode:     server.LoginReply_WrongUser,
		ReplyCodeData: &server.LoginReplyReplyCodeDataWrongUser{},
	})
}

func completeLogin(ctx context.Context, p *player.Player, accountID int, username string, rememberMe bool) error {
	characters, err := account.GetCharacterList(ctx, p.DB, accountID)
	if err != nil {
		slog.Error("error getting character list", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	p.AccountID = accountID
	p.State = player.StateLoggedIn
	p.LoginAttempts = 0
	p.AccountSessionToken = ""
	if rememberMe {
		if token, err := account.CreateSession(ctx, p.DB, accountID); err == nil {
			p.AccountSessionToken = token
		} else {
			slog.Warn("failed to create account session", "id", p.ID, "err", err)
		}
	}
	if p.IsDeep() {
		payload, err := deep.SerializeLoginConfig(p.Cfg.Character.MaxSkin+1, p.Cfg.Character.MaxHairStyle, p.Cfg.Character.MaxNameLength)
		if err == nil {
			_ = p.Bus.Send(eonet.PacketAction(deep.ActionConfig), eonet.PacketFamily_Login, payload)
		}
	}
	if p.AccountSessionToken != "" {
		meta, _ := json.Marshal(map[string]string{"token": p.AccountSessionToken})
		slog.Debug("remember me token created", "id", p.ID, "meta", string(meta))
	}
	if p.World != nil {
		p.World.AddLoggedInAccount(accountID)
	}

	slog.Info("player logged in", "id", p.ID, "account", username)

	packet := &server.LoginReplyServerPacket{
		ReplyCode: server.LoginReply_Ok,
		ReplyCodeData: &server.LoginReplyReplyCodeDataOk{
			Characters: characters,
		},
	}
	writer := data.NewEoWriter()
	if err := packet.Serialize(writer); err != nil {
		p.Close()
		return nil
	}
	if p.AccountSessionToken != "" {
		_ = writer.AddString(p.AccountSessionToken)
	}
	return p.Bus.Send(eonet.PacketAction_Reply, eonet.PacketFamily_Login, writer.Array())
}

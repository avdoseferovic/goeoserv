package handlers

import (
	"context"
	"log/slog"
	"strings"

	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/player/handlers/account"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Character, eonet.PacketAction_Request, handleCharacterRequest)
	player.Register(eonet.PacketFamily_Character, eonet.PacketAction_Create, handleCharacterCreate)
	player.Register(eonet.PacketFamily_Character, eonet.PacketAction_Take, handleCharacterTake)
	player.Register(eonet.PacketFamily_Character, eonet.PacketAction_Remove, handleCharacterRemove)
}

// handleCharacterRequest handles the "NEW" character request (pre-creation check).
func handleCharacterRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.CharacterRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize character request", "id", p.ID, "err", err)
		return nil
	}

	if pkt.RequestString != "NEW" {
		return nil
	}

	if p.State != player.StateLoggedIn {
		return nil
	}

	count, err := account.GetCharacterCount(ctx, p.DB, p.AccountID)
	if err != nil {
		slog.Error("error getting character count", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if count >= p.Cfg.Account.MaxCharacters {
		return p.Bus.SendPacket(&server.CharacterReplyServerPacket{
			ReplyCode:     server.CharacterReply_Full,
			ReplyCodeData: &server.CharacterReplyReplyCodeDataFull{},
		})
	}

	sessionID := p.GenerateSessionID()

	return p.Bus.SendPacket(&server.CharacterReplyServerPacket{
		ReplyCode:     server.CharacterReply(sessionID),
		ReplyCodeData: &server.CharacterReplyReplyCodeDataDefault{},
	})
}

// handleCharacterCreate creates a new character.
func handleCharacterCreate(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.CharacterCreateClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize character create", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateLoggedIn {
		return nil
	}

	// Validate session
	sessionID, ok := p.TakeSessionID()
	if !ok || sessionID != pkt.SessionId {
		slog.Warn("wrong session id in character create", "id", p.ID)
		p.Close()
		return nil
	}

	// Validate character name: 3-16 lowercase alphanumeric only
	if len(pkt.Name) < 3 || len(pkt.Name) > p.Cfg.Character.MaxNameLength {
		return nil
	}
	for _, r := range pkt.Name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return nil
		}
	}

	// Validate appearance
	if pkt.HairColor < 0 || pkt.HairColor > p.Cfg.Character.MaxHairColor ||
		pkt.HairStyle < 0 || pkt.HairStyle > p.Cfg.Character.MaxHairStyle ||
		pkt.Skin < 0 || pkt.Skin > p.Cfg.Character.MaxSkin {
		return nil
	}

	// Check name taken
	exists, err := account.CharacterExists(ctx, p.DB, pkt.Name)
	if err != nil {
		slog.Error("error checking character exists", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if exists {
		return p.Bus.SendPacket(&server.CharacterReplyServerPacket{
			ReplyCode:     server.CharacterReply_Exists,
			ReplyCodeData: &server.CharacterReplyReplyCodeDataExists{},
		})
	}

	// Determine admin level for first character
	adminLevel := 0
	if p.Cfg.Server.AutoAdmin {
		var totalChars int
		_ = p.DB.QueryRow(ctx,
			`SELECT COUNT(1) FROM characters`).Scan(&totalChars)
		if totalChars == 0 {
			adminLevel = 5
		}
	}

	// Create character
	_, err = p.DB.DB().ExecContext(ctx,
		`INSERT INTO characters (account_id, name, home, gender, race, hair_style, hair_color,
		 map, x, y, direction, admin_level)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.AccountID, strings.ToLower(pkt.Name), p.Cfg.NewCharacter.Home,
		int(pkt.Gender), int(pkt.Skin), pkt.HairStyle, pkt.HairColor,
		p.Cfg.NewCharacter.SpawnMap, p.Cfg.NewCharacter.SpawnX, p.Cfg.NewCharacter.SpawnY,
		p.Cfg.NewCharacter.SpawnDirection, adminLevel)
	if err != nil {
		slog.Error("error creating character", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	slog.Info("new character created", "name", pkt.Name, "account_id", p.AccountID)

	characters, err := account.GetCharacterList(ctx, p.DB, p.AccountID)
	if err != nil {
		slog.Error("error getting character list", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	return p.Bus.SendPacket(&server.CharacterReplyServerPacket{
		ReplyCode: server.CharacterReply_Ok,
		ReplyCodeData: &server.CharacterReplyReplyCodeDataOk{
			Characters: characters,
		},
	})
}

// handleCharacterTake initiates character deletion (sends session ID).
func handleCharacterTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.CharacterTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize character take", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateLoggedIn {
		return nil
	}

	// Verify character belongs to this account
	var ownerAccountID int
	err := p.DB.QueryRow(ctx,
		`SELECT account_id FROM characters WHERE id = ?`, pkt.CharacterId).Scan(&ownerAccountID)
	if err != nil {
		slog.Error("error loading character for deletion", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if ownerAccountID != p.AccountID {
		slog.Warn("attempt to delete another account's character", "id", p.ID)
		p.Close()
		return nil
	}

	sessionID := p.GenerateSessionID()

	return p.Bus.SendPacket(&server.CharacterPlayerServerPacket{
		SessionId:   sessionID,
		CharacterId: pkt.CharacterId,
	})
}

// handleCharacterRemove completes character deletion.
func handleCharacterRemove(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.CharacterRemoveClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize character remove", "id", p.ID, "err", err)
		return nil
	}

	if p.State != player.StateLoggedIn {
		return nil
	}

	sessionID, ok := p.TakeSessionID()
	if !ok || sessionID != pkt.SessionId {
		slog.Warn("wrong session id in character remove", "id", p.ID)
		p.Close()
		return nil
	}

	// Verify ownership
	var ownerAccountID int
	err := p.DB.QueryRow(ctx,
		`SELECT account_id FROM characters WHERE id = ?`, pkt.CharacterId).Scan(&ownerAccountID)
	if err != nil {
		slog.Error("error loading character for deletion", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	if ownerAccountID != p.AccountID {
		slog.Warn("attempt to delete another account's character", "id", p.ID)
		p.Close()
		return nil
	}

	// Delete character and related data
	tx, err := p.DB.BeginTx(ctx)
	if err != nil {
		slog.Error("error starting transaction", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	// security: table names are from a hardcoded slice, not user input — safe from SQL injection.
	for _, table := range []string{
		"character_inventory", "character_bank", "character_spells",
		"character_quest_progress",
	} {
		_, _ = tx.ExecContext(ctx, "DELETE FROM `"+table+"` WHERE character_id = ?", pkt.CharacterId)
	}
	_, _ = tx.ExecContext(ctx, "DELETE FROM characters WHERE id = ?", pkt.CharacterId)

	if err := tx.Commit(); err != nil {
		slog.Error("error committing character deletion", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	slog.Info("character deleted", "character_id", pkt.CharacterId, "account_id", p.AccountID)

	characters, err := account.GetCharacterList(ctx, p.DB, p.AccountID)
	if err != nil {
		slog.Error("error getting character list", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	return p.Bus.SendPacket(&server.CharacterReplyServerPacket{
		ReplyCode: server.CharacterReply_Deleted,
		ReplyCodeData: &server.CharacterReplyReplyCodeDataDeleted{
			Characters: characters,
		},
	})
}

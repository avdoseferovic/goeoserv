package account

import (
	"errors"
	"context"
	"database/sql"
	"log/slog"
	"strings"

	"github.com/avdo/goeoserv/internal/db"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	"github.com/ethanmoffat/eolib-go/v3/protocol"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// Exists checks if an account with the given username exists.
func Exists(ctx context.Context, database *db.Database, username string) (bool, error) {
	var id int
	err := database.QueryRow(ctx,
		`SELECT id FROM accounts WHERE name = ?`,
		strings.ToLower(username)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CharacterExists checks if a character with the given name exists.
func CharacterExists(ctx context.Context, database *db.Database, name string) (bool, error) {
	var id int
	err := database.QueryRow(ctx,
		`SELECT id FROM characters WHERE name = ?`,
		strings.ToLower(name)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetCharacterCount returns the number of characters for an account.
func GetCharacterCount(ctx context.Context, database *db.Database, accountID int) (int, error) {
	var count int
	err := database.QueryRow(ctx,
		`SELECT COUNT(1) FROM characters WHERE account_id = ?`, accountID).Scan(&count)
	return count, err
}

// GetCharacterList returns the character selection list entries for an account.
func GetCharacterList(ctx context.Context, database *db.Database, accountID int) ([]server.CharacterSelectionListEntry, error) {
	rows, err := database.Query(ctx,
		`SELECT id, name, level, gender, hair_style, hair_color, race, admin_level,
		        boots, armor, hat, shield, weapon
		 FROM characters WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "err", err)
		}
	}()

	var characters []server.CharacterSelectionListEntry
	for rows.Next() {
		var (
			id, level, gender, hairStyle, hairColor, skin, admin int
			boots, armor, hat, shield, weapon                    int
			name                                                 string
		)
		if err := rows.Scan(&id, &name, &level, &gender, &hairStyle, &hairColor, &skin, &admin,
			&boots, &armor, &hat, &shield, &weapon); err != nil {
			return nil, err
		}

		characters = append(characters, server.CharacterSelectionListEntry{
			Id:        id,
			Name:      name,
			Level:     level,
			Gender:    protocol.Gender(gender),
			HairStyle: hairStyle,
			HairColor: hairColor,
			Skin:      skin,
			Admin:     protocol.AdminLevel(admin),
			Equipment: server.EquipmentCharacterSelect{
				Boots:  pubdata.ItemGraphicID(boots),
				Armor:  pubdata.ItemGraphicID(armor),
				Hat:    pubdata.ItemGraphicID(hat),
				Shield: pubdata.ItemGraphicID(shield),
				Weapon: pubdata.ItemGraphicID(weapon),
			},
		})
	}

	return characters, rows.Err()
}

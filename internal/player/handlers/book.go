package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	"github.com/ethanmoffat/eolib-go/v3/protocol"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func handleBookRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.BookRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize book request", "id", p.ID, "err", err)
		return nil
	}

	targetName := p.World.GetPlayerName(pkt.PlayerId)
	if targetName == "" {
		return nil
	}

	var (
		name, home, partner, title, guild, guildRank string
		classID, gender, admin                       int
	)

	err := p.DB.QueryRow(ctx,
		`SELECT c.name, COALESCE(c.home, ''), COALESCE(c.partner, ''), COALESCE(c.title, ''),
		        COALESCE(g.name, ''), COALESCE(c.guild_rank_string, ''),
		        c.class, c.gender, c.admin_level
		 FROM characters c
		 LEFT JOIN guilds g ON c.guild_id = g.id
		 WHERE c.name = ?`,
		targetName,
	).Scan(&name, &home, &partner, &title, &guild, &guildRank, &classID, &gender, &admin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		slog.Debug("book query error", "id", p.ID, "target", pkt.PlayerId, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.BookReplyServerPacket{
		Details: server.CharacterDetails{
			Name:      name,
			Home:      home,
			Partner:   partner,
			Title:     title,
			Guild:     guild,
			GuildRank: guildRank,
			PlayerId:  pkt.PlayerId,
			ClassId:   classID,
			Gender:    protocol.Gender(gender),
			Admin:     protocol.AdminLevel(admin),
		},
		Icon:       adminIcon(admin),
		QuestNames: []string{},
	})
}

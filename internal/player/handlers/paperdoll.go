package handlers

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	"github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Paperdoll, eonet.PacketAction_Request, handlePaperdollRequest)
}

func handlePaperdollRequest(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.PaperdollRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize paperdoll request", "id", p.ID, "err", err)
		return nil
	}

	// Look up character data for the requested player
	// For now query the DB directly (in later phases this would come from in-memory state)
	var (
		name, home, partner, title, guild, guildRank string
		classID, gender, admin                       int
		boots, armor, hat, shield, weapon            int
		bracer, gloves, belt, necklace               int
		ring1, ring2, armlet1, armlet2               int
	)

	err := p.DB.QueryRow(context.Background(),
		`SELECT c.name, c.home, COALESCE(c.partner, ''), COALESCE(c.title, ''),
		        COALESCE(g.name, ''), COALESCE(gr.name, ''),
		        c.class, c.gender, c.admin_level,
		        c.boots, c.armor, c.hat, c.shield, c.weapon,
		        c.bracer, c.gloves, c.belt, c.necklace,
		        c.ring1, c.ring2, c.armlet1, c.armlet2
		 FROM characters c
		 LEFT JOIN guilds g ON c.guild_id = g.id
		 LEFT JOIN guild_ranks gr ON c.guild_rank_id = gr.id
		 WHERE c.player_id = ?`, pkt.PlayerId,
	).Scan(&name, &home, &partner, &title, &guild, &guildRank,
		&classID, &gender, &admin,
		&boots, &armor, &hat, &shield, &weapon,
		&bracer, &gloves, &belt, &necklace,
		&ring1, &ring2, &armlet1, &armlet2)

	if err == sql.ErrNoRows {
		// Player not found — silently ignore
		return nil
	}
	if err != nil {
		slog.Debug("paperdoll query error", "id", p.ID, "target", pkt.PlayerId, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.PaperdollReplyServerPacket{
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
		Equipment: server.EquipmentPaperdoll{
			Boots:    boots,
			Armor:    armor,
			Hat:      hat,
			Shield:   shield,
			Weapon:   weapon,
			Gloves:   gloves,
			Belt:     belt,
			Necklace: necklace,
			Ring:     []int{ring1, ring2},
			Armlet:   []int{armlet1, armlet2},
			Bracer:   []int{bracer, 0},
		},
		Icon: server.CharacterIcon(admin),
	})
}

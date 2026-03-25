package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Request, handleGuildRequest)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Accept, handleGuildAccept)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Create, handleGuildCreate)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Player, handleGuildPlayer)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Take, handleGuildTake)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Use, handleGuildUse)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Buy, handleGuildBuy)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Open, handleGuildOpen)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Tell, handleGuildTell)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Report, handleGuildReport)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Junk, handleGuildJunk)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Kick, handleGuildKick)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Rank, handleGuildRank)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Remove, handleGuildRemove)
	player.Register(eonet.PacketFamily_Guild, eonet.PacketAction_Agree, handleGuildAgree)
}

func handleGuildRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.GuildRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize guild request", "id", p.ID, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.GuildReplyServerPacket{
		ReplyCode: server.GuildReply_CreateAdd,
		ReplyCodeData: &server.GuildReplyReplyCodeDataCreateAdd{
			Name: pkt.GuildName,
		},
	})
}

func handleGuildCreate(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.GuildCreateClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize guild create", "id", p.ID, "err", err)
		return nil
	}

	result, err := p.DB.DB().ExecContext(ctx,
		`INSERT INTO guilds (tag, name, description) VALUES (?, ?, ?)`,
		pkt.GuildTag, pkt.GuildName, pkt.Description)
	if err != nil {
		slog.Error("error creating guild", "id", p.ID, "err", err)
		return nil
	}

	guildID, _ := result.LastInsertId()

	_ = p.DB.Execute(ctx,
		`UPDATE characters SET guild_id = ? WHERE id = ?`, guildID, *p.CharacterID)

	slog.Info("guild created", "tag", pkt.GuildTag, "name", pkt.GuildName, "player", p.ID)

	return p.Bus.SendPacket(&server.GuildCreateServerPacket{
		LeaderPlayerId: p.ID,
		GuildTag:       pkt.GuildTag,
		GuildName:      pkt.GuildName,
		RankName:       p.Cfg.Guild.DefaultLeaderRankName,
	})
}

func handleGuildOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.GuildOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize guild open", "id", p.ID, "err", err)
		return nil
	}

	sessionID := p.GenerateSessionID()

	return p.Bus.SendPacket(&server.GuildOpenServerPacket{
		SessionId: sessionID,
	})
}

func handleGuildTell(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.GuildTellClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	// Look up guild members from DB
	rows, err := p.DB.Query(ctx,
		`SELECT c.name, COALESCE(c.level, 0)
		 FROM characters c
		 JOIN guilds g ON c.guild_id = g.id
		 WHERE g.tag = ?
		 ORDER BY c.name`,
		pkt.GuildIdentity)
	if err != nil {
		slog.Error("error querying guild members", "id", p.ID, "err", err)
		return p.Bus.SendPacket(&server.GuildTellServerPacket{
			Members: []server.GuildMember{},
		})
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "err", err)
		}
	}()

	var members []server.GuildMember
	rank := 1
	for rows.Next() {
		var name string
		var level int
		if err := rows.Scan(&name, &level); err != nil {
			continue
		}
		members = append(members, server.GuildMember{
			Rank:     rank,
			Name:     name,
			RankName: "Member",
		})
		rank++
	}

	return p.Bus.SendPacket(&server.GuildTellServerPacket{
		Members: members,
	})
}

// Remaining guild handlers — stubs
func handleGuildAccept(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handleGuildPlayer(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handleGuildTake(_ context.Context, _ *player.Player, _ *player.EoReader) error   { return nil }
func handleGuildUse(_ context.Context, _ *player.Player, _ *player.EoReader) error    { return nil }
func handleGuildBuy(_ context.Context, _ *player.Player, _ *player.EoReader) error    { return nil }
func handleGuildReport(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handleGuildJunk(_ context.Context, _ *player.Player, _ *player.EoReader) error   { return nil }
func handleGuildKick(_ context.Context, _ *player.Player, _ *player.EoReader) error   { return nil }
func handleGuildRank(_ context.Context, _ *player.Player, _ *player.EoReader) error   { return nil }
func handleGuildRemove(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handleGuildAgree(_ context.Context, _ *player.Player, _ *player.EoReader) error  { return nil }

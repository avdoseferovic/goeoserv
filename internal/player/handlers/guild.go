package handlers

import (
	"context"
	"log/slog"
	"strings"

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
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}

	var pkt client.GuildCreateClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize guild create", "id", p.ID, "err", err)
		return nil
	}
	if !p.TakeAndValidateSessionID(pkt.SessionId) {
		return nil
	}
	if p.GuildTag != "" {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_AlreadyMember})
	}

	trimmedTag := strings.ToUpper(strings.TrimSpace(pkt.GuildTag))
	trimmedName := strings.TrimSpace(pkt.GuildName)
	trimmedDescription := strings.TrimSpace(pkt.Description)

	if len(trimmedTag) < p.Cfg.Guild.MinTagLength || len(trimmedTag) > p.Cfg.Guild.MaxTagLength {
		return nil
	}
	if len(trimmedName) < 3 || len(trimmedName) > p.Cfg.Guild.MaxNameLength {
		return nil
	}
	if len(trimmedDescription) > p.Cfg.Guild.MaxDescriptionLength {
		return nil
	}

	exists, err := guildExists(ctx, p, trimmedTag, trimmedName)
	if err != nil || exists {
		return nil
	}

	if cost := p.Cfg.Guild.CreateCost; cost > 0 {
		if !p.RemoveItem(1, cost) {
			return nil
		}
	}

	result, err := p.DB.DB().ExecContext(ctx,
		`INSERT INTO guilds (tag, name, description) VALUES (?, ?, ?)`,
		trimmedTag, trimmedName, trimmedDescription)
	if err != nil {
		slog.Error("error creating guild", "id", p.ID, "err", err)
		return nil
	}

	guildID, _ := result.LastInsertId()

	_ = p.DB.Execute(ctx,
		`UPDATE characters SET guild_id = ?, guild_rank = 1, guild_rank_string = ? WHERE id = ?`, guildID, p.Cfg.Guild.DefaultLeaderRankName, *p.CharacterID)
	p.GuildTag = trimmedTag

	slog.Info("guild created", "tag", trimmedTag, "name", trimmedName, "player", p.ID)

	return p.Bus.SendPacket(&server.GuildCreateServerPacket{
		LeaderPlayerId: p.ID,
		GuildTag:       trimmedTag,
		GuildName:      trimmedName,
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

func handleGuildTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.GuildTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}

	guild, ranks, err := loadGuildByTag(ctx, p, strings.TrimSpace(pkt.GuildTag))
	if err != nil {
		return nil
	}

	switch pkt.InfoType {
	case client.GuildInfo_Description:
		return p.Bus.SendPacket(&server.GuildTakeServerPacket{Description: guild.Description})
	case client.GuildInfo_Ranks:
		return p.Bus.SendPacket(&server.GuildRankServerPacket{Ranks: normalizeRanks(ranks)})
	case client.GuildInfo_Bank:
		return p.Bus.SendPacket(&server.GuildSellServerPacket{GoldAmount: guild.Bank})
	default:
		return nil
	}
}

func handleGuildBuy(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}

	var pkt client.GuildBuyClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	if pkt.GoldAmount < p.Cfg.Guild.MinDeposit {
		return nil
	}

	info, err := loadOwnGuildInfo(ctx, p)
	if err != nil {
		return nil
	}
	if pkt.GoldAmount <= 0 || !p.RemoveItem(1, pkt.GoldAmount) {
		return nil
	}

	if info.Bank+pkt.GoldAmount > p.Cfg.Guild.BankMaxGold {
		p.AddItem(1, pkt.GoldAmount)
		return nil
	}
	if err := p.DB.Execute(ctx, `UPDATE guilds SET bank = bank + ? WHERE id = ?`, pkt.GoldAmount, info.ID); err != nil {
		p.AddItem(1, pkt.GoldAmount)
		return nil
	}
	return p.Bus.SendPacket(&server.GuildBuyServerPacket{GoldAmount: info.Bank + pkt.GoldAmount})
}

func handleGuildUse(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil || p.World == nil {
		return nil
	}

	var pkt client.GuildUseClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	info, err := loadOwnGuildInfo(ctx, p)
	if err != nil || pkt.PlayerId == 0 {
		return nil
	}
	if info.Rank > 2 {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_NotRecruiter})
	}

	targetName := p.World.GetPlayerName(pkt.PlayerId)
	if targetName == "" {
		return nil
	}
	targetInfo, err := loadGuildMemberByPlayerID(ctx, p, pkt.PlayerId)
	if err != nil || targetInfo.GuildID != 0 {
		return nil
	}
	if p.Cfg.Guild.RecruitCost > 0 {
		if info.Bank < p.Cfg.Guild.RecruitCost {
			return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_AccountLow})
		}
		if err := p.DB.Execute(ctx, `UPDATE guilds SET bank = bank - ? WHERE id = ?`, p.Cfg.Guild.RecruitCost, info.ID); err != nil {
			return nil
		}
	}
	rankName := normalizeRanks(mustLoadGuildRanks(ctx, p, info.ID))[8]
	if err := p.DB.Execute(ctx, `UPDATE characters SET guild_id = ?, guild_rank = 9, guild_rank_string = ? WHERE LOWER(name) = ?`, info.ID, rankName, strings.ToLower(targetName)); err != nil {
		return nil
	}
	p.World.SendToPlayer(pkt.PlayerId, &server.GuildAgreeServerPacket{
		RecruiterId: p.ID,
		GuildTag:    info.Tag,
		GuildName:   info.Name,
		RankName:    rankName,
	})
	return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_Accepted})
}

func handleGuildPlayer(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil || p.World == nil {
		return nil
	}

	var pkt client.GuildPlayerClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	if p.GuildTag != "" {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_AlreadyMember})
	}
	recruiterID, found := p.World.FindPlayerByName(strings.ToLower(pkt.RecruiterName))
	if !found {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RecruiterOffline})
	}
	recruiterInfo, err := loadGuildMemberByPlayerID(ctx, p, recruiterID)
	if err != nil || !strings.EqualFold(recruiterInfo.GuildTag, pkt.GuildTag) {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RecruiterWrongGuild})
	}
	if recruiterInfo.Rank == 0 || recruiterInfo.Rank > 2 {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_NotRecruiter})
	}
	p.World.SendToPlayer(recruiterID, &server.GuildReplyServerPacket{
		ReplyCode:     server.GuildReply_JoinRequest,
		ReplyCodeData: &server.GuildReplyReplyCodeDataJoinRequest{PlayerId: p.ID, Name: p.CharName},
	})
	return nil
}

func handleGuildAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	var pkt client.GuildAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	p.World.SendToPlayer(pkt.InviterPlayerId, &server.GuildReplyServerPacket{ReplyCode: server.GuildReply_Accepted})
	return nil
}

func handleGuildReport(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	var pkt client.GuildReportClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	guild, ranks, err := loadGuildByTag(ctx, p, strings.TrimSpace(pkt.GuildIdentity))
	if err != nil {
		return nil
	}
	staff, _ := loadGuildStaff(ctx, p, guild.ID)
	return p.Bus.SendPacket(&server.GuildReportServerPacket{
		Name:        guild.Name,
		Tag:         guild.Tag,
		CreateDate:  guild.CreatedAt,
		Description: guild.Description,
		Wealth:      guildBankWealth(guild.Bank),
		Ranks:       normalizeRanks(ranks),
		Staff:       staff,
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
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	rows, err := p.DB.Query(ctx,
		`SELECT c.name, COALESCE(c.guild_rank, 9), COALESCE(c.guild_rank_string, '')
		 FROM characters c
		 JOIN guilds g ON c.guild_id = g.id
		 WHERE g.tag = ?
		 ORDER BY COALESCE(c.guild_rank, 9), c.name`,
		strings.TrimSpace(pkt.GuildIdentity))
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
	for rows.Next() {
		var name string
		var rank int
		var rankName string
		if err := rows.Scan(&name, &rank, &rankName); err != nil {
			continue
		}
		members = append(members, server.GuildMember{
			Rank:     rank,
			Name:     name,
			RankName: rankName,
		})
	}

	return p.Bus.SendPacket(&server.GuildTellServerPacket{
		Members: members,
	})
}

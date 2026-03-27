package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/avdo/goeoserv/internal/config"
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

	// Deduct guild creation cost
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

func handleGuildKick(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.GuildKickClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	info, err := loadOwnGuildInfo(ctx, p)
	if err != nil {
		return nil
	}
	if info.Rank > 2 {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_NotRecruiter})
	}
	member, err := loadGuildMemberByName(ctx, p, strings.ToLower(pkt.MemberName))
	if err != nil || member.GuildID != info.ID {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RemoveNotMember})
	}
	if member.Rank == 1 {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RemoveLeader})
	}
	if member.Rank <= info.Rank {
		return nil
	}
	if err := p.DB.Execute(ctx, `UPDATE characters SET guild_id = NULL, guild_rank = NULL, guild_rank_string = NULL WHERE id = ?`, member.ID); err != nil {
		return nil
	}
	if id, found := p.World.FindPlayerByName(strings.ToLower(pkt.MemberName)); found {
		p.World.SendToPlayer(id, &server.GuildKickServerPacket{})
	}
	return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_Removed})
}

func handleGuildRank(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.GuildRankClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	info, err := loadOwnGuildInfo(ctx, p)
	if err != nil {
		return nil
	}
	if info.Rank != 1 {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RankingLeader})
	}
	if pkt.Rank < 1 || pkt.Rank > 9 {
		return nil
	}
	member, err := loadGuildMemberByName(ctx, p, strings.ToLower(pkt.MemberName))
	if err != nil || member.GuildID != info.ID {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RankingNotMember})
	}
	if member.Rank == 1 {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RankingLeader})
	}
	ranks, _ := loadGuildRanks(ctx, p, info.ID)
	norm := normalizeRanks(ranks)
	idx := pkt.Rank - 1
	if idx < 0 || idx >= len(norm) {
		return nil
	}
	if err := p.DB.Execute(ctx, `UPDATE characters SET guild_rank = ?, guild_rank_string = ? WHERE id = ?`, pkt.Rank, norm[idx], member.ID); err != nil {
		return nil
	}
	if id, found := p.World.FindPlayerByName(strings.ToLower(pkt.MemberName)); found {
		p.World.SendToPlayer(id, &server.GuildAcceptServerPacket{Rank: pkt.Rank})
	}
	return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_Updated})
}

func handleGuildRemove(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.GuildRemoveClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	info, err := loadOwnGuildInfo(ctx, p)
	if err != nil {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RemoveNotMember})
	}
	if info.Rank == 1 {
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RemoveLeader})
	}
	if err := p.DB.Execute(ctx, `UPDATE characters SET guild_id = NULL, guild_rank = NULL, guild_rank_string = NULL WHERE id = ?`, *p.CharacterID); err != nil {
		return nil
	}
	p.GuildTag = ""
	return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_Removed})
}

func handleGuildJunk(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.GuildJunkClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	info, err := loadOwnGuildInfo(ctx, p)
	if err != nil || info.Rank != 1 {
		return nil
	}
	if err := p.DB.Execute(ctx, `UPDATE characters SET guild_id = NULL, guild_rank = NULL, guild_rank_string = NULL WHERE guild_id = ?`, info.ID); err != nil {
		return nil
	}
	if err := p.DB.Execute(ctx, `DELETE FROM guild_ranks WHERE guild_id = ?`, info.ID); err != nil {
		return nil
	}
	if err := p.DB.Execute(ctx, `DELETE FROM guilds WHERE id = ?`, info.ID); err != nil {
		return nil
	}
	p.GuildTag = ""
	return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_Removed})
}

func handleGuildAgree(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.GuildAgreeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	info, err := loadOwnGuildInfo(ctx, p)
	if err != nil || info.Rank != 1 {
		return nil
	}
	switch pkt.InfoType {
	case client.GuildInfo_Description:
		data, _ := pkt.InfoTypeData.(*client.GuildAgreeInfoTypeDataDescription)
		if data == nil {
			return nil
		}
		description := strings.TrimSpace(data.Description)
		if len(description) > p.Cfg.Guild.MaxDescriptionLength {
			return nil
		}
		if err := p.DB.Execute(ctx, `UPDATE guilds SET description = ? WHERE id = ?`, description, info.ID); err != nil {
			return nil
		}
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_Updated})
	case client.GuildInfo_Ranks:
		data, _ := pkt.InfoTypeData.(*client.GuildAgreeInfoTypeDataRanks)
		if data == nil {
			return nil
		}
		ranks := normalizeRanks(data.Ranks)
		if !validateGuildRanks(p.Cfg, ranks) {
			return nil
		}
		if err := p.DB.Execute(ctx, `DELETE FROM guild_ranks WHERE guild_id = ?`, info.ID); err != nil {
			return nil
		}
		for i, rank := range ranks {
			_ = p.DB.Execute(ctx, `INSERT INTO guild_ranks (guild_id, index, rank) VALUES (?, ?, ?)`, info.ID, i+1, rank)
		}
		return p.Bus.SendPacket(&server.GuildReplyServerPacket{ReplyCode: server.GuildReply_RanksUpdated})
	default:
		return nil
	}
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
	// Look up guild members from DB
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

type guildInfo struct {
	ID          int
	Tag         string
	Name        string
	Description string
	Bank        int
	CreatedAt   string
	Rank        int
}

type guildMemberInfo struct {
	ID       int
	GuildID  int
	GuildTag string
	Rank     int
}

func loadOwnGuildInfo(ctx context.Context, p *player.Player) (*guildInfo, error) {
	var info guildInfo
	err := p.DB.QueryRow(ctx, `SELECT g.id, g.tag, g.name, COALESCE(g.description,''), g.bank, COALESCE(c.guild_rank, 0)
		FROM characters c JOIN guilds g ON c.guild_id = g.id WHERE c.id = ?`, *p.CharacterID).Scan(&info.ID, &info.Tag, &info.Name, &info.Description, &info.Bank, &info.Rank)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func loadGuildByTag(ctx context.Context, p *player.Player, tag string) (*guildInfo, []string, error) {
	var info guildInfo
	err := p.DB.QueryRow(ctx, `SELECT id, tag, name, COALESCE(description,''), bank, COALESCE(created_at,'') FROM guilds WHERE tag = ?`, strings.TrimSpace(tag)).Scan(&info.ID, &info.Tag, &info.Name, &info.Description, &info.Bank, &info.CreatedAt)
	if err != nil {
		return nil, nil, err
	}
	ranks, _ := loadGuildRanks(ctx, p, info.ID)
	return &info, ranks, nil
}

func loadGuildRanks(ctx context.Context, p *player.Player, guildID int) ([]string, error) {
	rows, err := p.DB.Query(ctx, `SELECT rank FROM guild_ranks WHERE guild_id = ? ORDER BY index`, guildID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ranks []string
	for rows.Next() {
		var rank string
		if err := rows.Scan(&rank); err == nil {
			ranks = append(ranks, rank)
		}
	}
	return ranks, nil
}

func normalizeRanks(ranks []string) []string {
	base := []string{"Leader", "Recruiter", "Officer", "Veteran", "Member", "Member", "Member", "Member", "New Member"}
	copy(base, base)
	for i := 0; i < len(ranks) && i < 9; i++ {
		if strings.TrimSpace(ranks[i]) != "" {
			base[i] = ranks[i]
		}
	}
	return base
}

func loadGuildStaff(ctx context.Context, p *player.Player, guildID int) ([]server.GuildStaff, error) {
	rows, err := p.DB.Query(ctx, `SELECT COALESCE(guild_rank,0), name FROM characters WHERE guild_id = ? AND COALESCE(guild_rank,0) <= 2 ORDER BY guild_rank, name`, guildID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var staff []server.GuildStaff
	for rows.Next() {
		var rank int
		var name string
		if err := rows.Scan(&rank, &name); err == nil {
			staff = append(staff, server.GuildStaff{Rank: rank, Name: name})
		}
	}
	return staff, nil
}

func guildBankWealth(bank int) string {
	return fmt.Sprintf("%d gold", bank)
}

func loadGuildMemberByName(ctx context.Context, p *player.Player, name string) (*guildMemberInfo, error) {
	var info guildMemberInfo
	err := p.DB.QueryRow(ctx, `SELECT id, COALESCE(guild_id,0), COALESCE(guild_rank,0) FROM characters WHERE LOWER(name) = ?`, strings.ToLower(name)).Scan(&info.ID, &info.GuildID, &info.Rank)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func loadGuildMemberByPlayerID(ctx context.Context, p *player.Player, playerID int) (*guildMemberInfo, error) {
	name := p.World.GetPlayerName(playerID)
	if name == "" {
		return nil, sql.ErrNoRows
	}
	var info guildMemberInfo
	err := p.DB.QueryRow(ctx, `SELECT c.id, COALESCE(c.guild_id,0), COALESCE(c.guild_rank,0), COALESCE(g.tag,'') FROM characters c LEFT JOIN guilds g ON c.guild_id = g.id WHERE LOWER(c.name) = ?`, strings.ToLower(name)).Scan(&info.ID, &info.GuildID, &info.Rank, &info.GuildTag)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func guildExists(ctx context.Context, p *player.Player, tag, name string) (bool, error) {
	var exists int
	err := p.DB.QueryRow(ctx, `SELECT 1 FROM guilds WHERE UPPER(tag) = ? OR LOWER(name) = ? LIMIT 1`, strings.ToUpper(strings.TrimSpace(tag)), strings.ToLower(strings.TrimSpace(name))).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func validateGuildRanks(cfg *config.Config, ranks []string) bool {
	for _, rank := range ranks {
		if len(strings.TrimSpace(rank)) > cfg.Guild.MaxRankLength {
			return false
		}
	}
	return true
}

func mustLoadGuildRanks(ctx context.Context, p *player.Player, guildID int) []string {
	ranks, err := loadGuildRanks(ctx, p, guildID)
	if err != nil {
		return nil
	}
	return ranks
}

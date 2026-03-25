package handlers

import (
	"log/slog"
	"strings"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Talk, eonet.PacketAction_Report, handleTalkLocal)
	player.Register(eonet.PacketFamily_Talk, eonet.PacketAction_Msg, handleTalkGlobal)
	player.Register(eonet.PacketFamily_Talk, eonet.PacketAction_Tell, handleTalkPrivate)
	player.Register(eonet.PacketFamily_Talk, eonet.PacketAction_Admin, handleTalkAdmin)
	player.Register(eonet.PacketFamily_Talk, eonet.PacketAction_Announce, handleTalkAnnounce)
}

// handleTalkLocal — local chat visible to same map
func handleTalkLocal(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.TalkReportClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize talk report", "id", p.ID, "err", err)
		return nil
	}

	p.World.BroadcastMap(p.MapID, p.ID, &server.TalkPlayerServerPacket{
		PlayerId: p.ID,
		Message:  pkt.Message,
	})
	return nil
}

// handleTalkGlobal — global chat visible to all maps
func handleTalkGlobal(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.TalkMsgClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize talk msg", "id", p.ID, "err", err)
		return nil
	}

	p.World.BroadcastGlobal(p.ID, &server.TalkMsgServerPacket{
		PlayerName: p.CharName,
		Message:    pkt.Message,
	})
	return nil
}

// handleTalkPrivate — private message to a specific player
func handleTalkPrivate(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.TalkTellClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize talk tell", "id", p.ID, "err", err)
		return nil
	}

	targetName := strings.ToLower(pkt.Name)
	targetID, found := p.World.FindPlayerByName(targetName)
	if !found {
		return p.Bus.SendPacket(&server.TalkReplyServerPacket{
			ReplyCode: server.TalkReply_NotFound,
			Name:      pkt.Name,
		})
	}

	p.World.SendToPlayer(targetID, &server.TalkTellServerPacket{
		PlayerName: p.CharName,
		Message:    pkt.Message,
	})
	return nil
}

// handleTalkAdmin — admin chat (visible to all admins)
func handleTalkAdmin(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.TalkAdminClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize talk admin", "id", p.ID, "err", err)
		return nil
	}

	// TODO: Check admin level, broadcast only to admins
	// For now, broadcast globally
	p.World.BroadcastGlobal(p.ID, &server.TalkAdminServerPacket{
		PlayerName: p.CharName,
		Message:    pkt.Message,
	})
	return nil
}

// handleTalkAnnounce — server announcement (admin only)
func handleTalkAnnounce(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.TalkAnnounceClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize talk announce", "id", p.ID, "err", err)
		return nil
	}

	// TODO: Check admin level
	p.World.BroadcastGlobal(-1, &server.TalkAnnounceServerPacket{
		PlayerName: p.CharName,
		Message:    pkt.Message,
	})
	return nil
}

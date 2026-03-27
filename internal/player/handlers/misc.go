package handlers

import (
	"context"

	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	// Jukebox
	player.Register(eonet.PacketFamily_Jukebox, eonet.PacketAction_Open, handleJukeboxOpen)
	player.Register(eonet.PacketFamily_Jukebox, eonet.PacketAction_Msg, handleJukeboxMsg)

	// AdminInteract
	player.Register(eonet.PacketFamily_AdminInteract, eonet.PacketAction_Tell, handleAdminInteractTell)
	player.Register(eonet.PacketFamily_AdminInteract, eonet.PacketAction_Report, handleAdminInteractReport)

	// Book (character info)
	player.Register(eonet.PacketFamily_Book, eonet.PacketAction_Request, handleBookRequest)

	// Message (server message)
	player.Register(eonet.PacketFamily_Message, eonet.PacketAction_Ping, handleMessagePing)

	// Players list
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_Request, handlePlayersRequest)
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_Accept, handlePlayersAccept)
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_List, handlePlayersList)
}

// Jukebox
func handleJukeboxOpen(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	return p.Bus.SendPacket(&server.JukeboxOpenServerPacket{MapId: p.MapID})
}

func handleJukeboxMsg(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	var pkt client.JukeboxMsgClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	// Validate track ID
	if pkt.TrackId < 1 || pkt.TrackId > p.Cfg.Jukebox.MaxTrackID {
		return nil
	}

	// Deduct gold cost
	if !p.RemoveItem(1, p.Cfg.Jukebox.Cost) {
		return nil
	}

	if p.World == nil {
		p.AddItem(1, p.Cfg.Jukebox.Cost)
		return nil
	}

	if !p.World.TryStartJukebox(p.MapID, pkt.TrackId) {
		p.AddItem(1, p.Cfg.Jukebox.Cost)
		return nil
	}

	// Broadcast track to all players on map (1-indexed for server packet)
	p.World.BroadcastMap(p.MapID, -1, &server.JukeboxUseServerPacket{
		TrackId: pkt.TrackId + 1,
	})

	return p.Bus.SendPacket(&server.JukeboxAgreeServerPacket{
		GoldAmount: p.Inventory[1],
	})
}

// Players list (online users)
func handlePlayersRequest(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	if p.World == nil {
		return p.Bus.SendPacket(&server.PlayersListServerPacket{})
	}
	raw := p.World.GetOnlinePlayers()
	infos, _ := raw.([]gamemap.OnlinePlayerInfo)

	var records []server.OnlinePlayer
	for _, info := range infos {
		records = append(records, server.OnlinePlayer{
			Name:     info.Name,
			Title:    info.Title,
			Level:    info.Level,
			Icon:     adminIcon(info.Admin),
			ClassId:  info.ClassID,
			GuildTag: info.GuildTag,
		})
	}
	return p.Bus.SendPacket(&server.PlayersListServerPacket{
		PlayersList: server.PlayersList{
			Players: records,
		},
	})
}

func handleMessagePing(_ context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	return p.Bus.SendPacket(&server.MessagePongServerPacket{})
}

func adminIcon(admin int) server.CharacterIcon {
	switch {
	case admin >= 4:
		return server.CharacterIcon_Hgm
	case admin >= 1:
		return server.CharacterIcon_Gm
	default:
		return server.CharacterIcon_Player
	}
}

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


	// Citizen (inn)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Open, handleCitizenNoop)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Request, handleCitizenNoop)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Accept, handleCitizenNoop)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Reply, handleCitizenNoop)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Remove, handleCitizenNoop)

	// Priest / Marriage
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Open, handlePriestNoop)
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Request, handlePriestNoop)
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Accept, handlePriestNoop)
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Use, handlePriestNoop)
	player.Register(eonet.PacketFamily_Marriage, eonet.PacketAction_Open, handleMarriageNoop)
	player.Register(eonet.PacketFamily_Marriage, eonet.PacketAction_Request, handleMarriageNoop)

	// AdminInteract
	player.Register(eonet.PacketFamily_AdminInteract, eonet.PacketAction_Tell, handleAdminInteractNoop)
	player.Register(eonet.PacketFamily_AdminInteract, eonet.PacketAction_Report, handleAdminInteractNoop)

	// Book (character info)
	player.Register(eonet.PacketFamily_Book, eonet.PacketAction_Request, handleBookNoop)

	// Message (server message)
	player.Register(eonet.PacketFamily_Message, eonet.PacketAction_Ping, handleMessageNoop)

	// Players list
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_Request, handlePlayersRequest)
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_Accept, handlePlayersNoop)
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_List, handlePlayersNoop)
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

	// Broadcast track to all players on map (1-indexed for server packet)
	if p.World != nil {
		p.World.BroadcastMap(p.MapID, -1, &server.JukeboxUseServerPacket{
			TrackId: pkt.TrackId + 1,
		})
	}

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

// No-op stubs for unimplemented features
func handleCitizenNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error  { return nil }
func handlePriestNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error   { return nil }
func handleMarriageNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handleAdminInteractNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error {
	return nil
}
func handleBookNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error    { return nil }
func handleMessageNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handlePlayersNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }

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

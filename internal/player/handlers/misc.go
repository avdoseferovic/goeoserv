package handlers

import (
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	// Barber
	player.Register(eonet.PacketFamily_Barber, eonet.PacketAction_Open, handleBarberOpen)
	player.Register(eonet.PacketFamily_Barber, eonet.PacketAction_Buy, handleBarberBuy)

	// Jukebox
	player.Register(eonet.PacketFamily_Jukebox, eonet.PacketAction_Open, handleJukeboxOpen)
	player.Register(eonet.PacketFamily_Jukebox, eonet.PacketAction_Msg, handleJukeboxMsg)
	player.Register(eonet.PacketFamily_Jukebox, eonet.PacketAction_Use, handleJukeboxUse)

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

	// Board
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Open, handleBoardOpen)
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Take, handleBoardNoop)
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Create, handleBoardNoop)
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Remove, handleBoardNoop)

	// Book (character info)
	player.Register(eonet.PacketFamily_Book, eonet.PacketAction_Request, handleBookNoop)

	// Locker
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Open, handleLockerNoop)
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Add, handleLockerNoop)
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Take, handleLockerNoop)
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Buy, handleLockerNoop)

	// Chest
	player.Register(eonet.PacketFamily_Chest, eonet.PacketAction_Open, handleChestNoop)
	player.Register(eonet.PacketFamily_Chest, eonet.PacketAction_Add, handleChestNoop)
	player.Register(eonet.PacketFamily_Chest, eonet.PacketAction_Take, handleChestNoop)

	// Message (server message)
	player.Register(eonet.PacketFamily_Message, eonet.PacketAction_Ping, handleMessageNoop)

	// Players list
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_Request, handlePlayersRequest)
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_Accept, handlePlayersNoop)
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_List, handlePlayersNoop)

	// Warp accept
	player.Register(eonet.PacketFamily_Warp, eonet.PacketAction_Accept, handleWarpAccept)
	player.Register(eonet.PacketFamily_Warp, eonet.PacketAction_Take, handleWarpNoop)
}

// Barber
func handleBarberOpen(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	var pkt client.BarberOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	sessionID := p.GenerateSessionID()
	return p.Bus.SendPacket(&server.BarberOpenServerPacket{SessionId: sessionID})
}

func handleBarberBuy(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	var pkt client.BarberBuyClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	// TODO: Change hair, deduct gold
	return p.Bus.SendPacket(&server.BarberAgreeServerPacket{
		GoldAmount: p.Inventory[1],
		Change: server.AvatarChange{
			PlayerId:   p.ID,
			ChangeType: server.AvatarChange_Hair,
			Sound:      false,
			ChangeTypeData: &server.ChangeTypeDataHair{
				HairStyle: pkt.HairStyle,
				HairColor: pkt.HairColor,
			},
		},
	})
}

// Jukebox
func handleJukeboxOpen(p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	return p.Bus.SendPacket(&server.JukeboxOpenServerPacket{MapId: p.MapID})
}

func handleJukeboxMsg(_ *player.Player, _ *player.EoReader) error { return nil }
func handleJukeboxUse(_ *player.Player, _ *player.EoReader) error { return nil }

// Board
func handleBoardOpen(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	var pkt client.BoardOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	return p.Bus.SendPacket(&server.BoardOpenServerPacket{
		BoardId: pkt.BoardId,
	})
}

// Players list (online users)
func handlePlayersRequest(p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	// TODO: Get actual online player list from world
	return p.Bus.SendPacket(&server.PlayersListServerPacket{})
}

// Warp accept — client acknowledges a map change
func handleWarpAccept(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.WarpAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize warp accept", "id", p.ID, "err", err)
		return nil
	}

	// TODO: Complete the warp — move player to new map, send WarpAgreeServerPacket
	return nil
}

// No-op stubs
func handleCitizenNoop(_ *player.Player, _ *player.EoReader) error       { return nil }
func handlePriestNoop(_ *player.Player, _ *player.EoReader) error        { return nil }
func handleMarriageNoop(_ *player.Player, _ *player.EoReader) error      { return nil }
func handleAdminInteractNoop(_ *player.Player, _ *player.EoReader) error { return nil }
func handleBoardNoop(_ *player.Player, _ *player.EoReader) error         { return nil }
func handleBookNoop(_ *player.Player, _ *player.EoReader) error          { return nil }
func handleLockerNoop(_ *player.Player, _ *player.EoReader) error        { return nil }
func handleChestNoop(_ *player.Player, _ *player.EoReader) error         { return nil }
func handleMessageNoop(_ *player.Player, _ *player.EoReader) error       { return nil }
func handlePlayersNoop(_ *player.Player, _ *player.EoReader) error       { return nil }
func handleWarpNoop(_ *player.Player, _ *player.EoReader) error          { return nil }

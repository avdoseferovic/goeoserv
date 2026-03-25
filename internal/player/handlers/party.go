package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/protocol"
	worldpkg "github.com/avdo/goeoserv/internal/world"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Party, eonet.PacketAction_Request, handlePartyRequest)
	player.Register(eonet.PacketFamily_Party, eonet.PacketAction_Accept, handlePartyAccept)
	player.Register(eonet.PacketFamily_Party, eonet.PacketAction_Remove, handlePartyRemove)
	player.Register(eonet.PacketFamily_Party, eonet.PacketAction_Take, handlePartyTake)
}

// pendingInvite stores info about the inviter so we can create a proper party.
type pendingInvite struct {
	PlayerID int
	Name     string
	Level    int
	HP       int
	MaxHP    int
	MapID    int
	Bus      *protocol.PacketBus
	Player   *player.Player
}

// Pending invites: invitedPlayerID -> inviter info
var pendingInvites = make(map[int]pendingInvite)

func handlePartyRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.PartyRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize party request", "id", p.ID, "err", err)
		return nil
	}

	targetID := pkt.PlayerId

	// Send invite to the target player
	pendingInvites[targetID] = pendingInvite{
		PlayerID: p.ID,
		Name:     p.CharName,
		Level:    p.CharLevel,
		HP:       p.CharHP,
		MaxHP:    p.CharMaxHP,
		MapID:    p.MapID,
		Bus:      p.Bus,
		Player:   p,
	}

	p.World.SendToPlayer(targetID, &server.PartyRequestServerPacket{
		RequestType:     pkt.RequestType,
		InviterPlayerId: p.ID,
		PlayerName:      p.CharName,
	})

	return nil
}

func handlePartyAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.PartyAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize party accept", "id", p.ID, "err", err)
		return nil
	}

	invite, ok := pendingInvites[p.ID]
	if !ok || invite.PlayerID != pkt.InviterPlayerId {
		return nil
	}
	delete(pendingInvites, p.ID)

	newMember := worldpkg.PartyMemberInfo{
		PlayerID: p.ID,
		Name:     p.CharName,
		Level:    p.CharLevel,
		HP:       p.CharHP,
		MaxHP:    p.CharMaxHP,
		MapID:    p.MapID,
		Bus:      p.Bus,
		Player:   p,
	}

	// Check if inviter already has a party
	party := worldpkg.GetParty(invite.PlayerID)
	if party != nil {
		party.AddMember(newMember, p.Cfg.Limits.MaxPartySize)
	} else {
		// Create new party with inviter as leader using stored info
		inviterMember := worldpkg.PartyMemberInfo{
			PlayerID: invite.PlayerID,
			Name:     invite.Name,
			Level:    invite.Level,
			HP:       invite.HP,
			MaxHP:    invite.MaxHP,
			MapID:    invite.MapID,
			Bus:      invite.Bus,
			Player:   invite.Player,
		}

		party = worldpkg.CreateParty(inviterMember)
		party.AddMember(newMember, p.Cfg.Limits.MaxPartySize)
	}

	return nil
}

func handlePartyRemove(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.PartyRemoveClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize party remove", "id", p.ID, "err", err)
		return nil
	}

	party := worldpkg.GetParty(p.ID)
	if party == nil {
		return nil
	}

	party.RemoveMember(pkt.PlayerId)
	return nil
}

// handlePartyTake — request party member list update
func handlePartyTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.PartyTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize party take", "id", p.ID, "err", err)
		return nil
	}
	_ = pkt

	party := worldpkg.GetParty(p.ID)
	if party == nil {
		return nil
	}

	// Send updated member list
	party.BroadcastToParty(&server.PartyListServerPacket{
		Members: party.BuildMemberListPublic(),
	})
	return nil
}

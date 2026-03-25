package handlers

import (
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

// Pending invites: invitedPlayerID -> inviterPlayerID
var pendingInvites = make(map[int]int)

func handlePartyRequest(p *player.Player, reader *player.EoReader) error {
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
	pendingInvites[targetID] = p.ID

	p.World.SendToPlayer(targetID, &server.PartyRequestServerPacket{
		RequestType:     pkt.RequestType,
		InviterPlayerId: p.ID,
		PlayerName:      p.CharName,
	})

	return nil
}

func handlePartyAccept(p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.PartyAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize party accept", "id", p.ID, "err", err)
		return nil
	}

	inviterID, ok := pendingInvites[p.ID]
	if !ok || inviterID != pkt.InviterPlayerId {
		return nil
	}
	delete(pendingInvites, p.ID)

	newMember := worldpkg.PartyMemberInfo{
		PlayerID: p.ID,
		Name:     p.CharName,
		Level:    p.CharLevel,
		HP:       p.CharHP,
		MaxHP:    p.CharMaxHP,
		Bus:      p.Bus,
	}

	// Check if inviter already has a party
	party := worldpkg.GetParty(inviterID)
	if party != nil {
		party.AddMember(newMember, p.Cfg.Limits.MaxPartySize)
	} else {
		// Create new party with inviter as leader
		// We need the inviter's info — get it from world
		inviterMember := worldpkg.PartyMemberInfo{
			PlayerID: inviterID,
			Name:     "", // simplified
			Level:    0,
			HP:       0,
			MaxHP:    0,
			Bus:      nil,
		}

		// Try to get inviter's bus from world
		if busRaw := p.World.GetPlayerBus(inviterID); busRaw != nil {
			if bus, ok := busRaw.(*protocol.PacketBus); ok {
				inviterMember.Bus = bus
			}
		}

		party = worldpkg.CreateParty(inviterMember)
		party.AddMember(newMember, p.Cfg.Limits.MaxPartySize)
	}

	return nil
}

func handlePartyRemove(p *player.Player, reader *player.EoReader) error {
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
func handlePartyTake(p *player.Player, reader *player.EoReader) error {
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

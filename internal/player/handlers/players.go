package handlers

import (
	"context"
	"strings"

	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func handlePlayersAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.PlayersAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	lookup := strings.TrimSpace(strings.ToLower(pkt.Name))
	if lookup == "" {
		return nil
	}

	targetID, found := p.World.FindPlayerByName(lookup)
	if !found {
		return p.Bus.SendPacket(&server.PlayersPingServerPacket{Name: pkt.Name})
	}

	targetName := p.World.GetPlayerName(targetID)
	if targetName == "" {
		targetName = pkt.Name
	}

	pos, _ := p.World.GetPlayerPosition(targetID).(*gamemap.PlayerPosition)
	if pos == nil {
		return p.Bus.SendPacket(&server.PlayersPingServerPacket{Name: targetName})
	}

	if pos.MapID == p.MapID {
		return p.Bus.SendPacket(&server.PlayersPongServerPacket{Name: targetName})
	}

	return p.Bus.SendPacket(&server.PlayersNet242ServerPacket{Name: targetName})
}

func handlePlayersList(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	raw := p.World.GetOnlinePlayers()
	infos, _ := raw.([]gamemap.OnlinePlayerInfo)
	players := make([]string, 0, len(infos))
	for _, info := range infos {
		players = append(players, info.Name)
	}

	return p.Bus.SendPacket(&server.PlayersReplyServerPacket{
		PlayersList: server.PlayersListFriends{Players: players},
	})
}

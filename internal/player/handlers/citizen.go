package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

const citizenBehaviorInn = 1

var citizenQuestions = []string{
	"Welcome to the inn.",
	"Would you like to rest here?",
	"Your health and mana will be restored.",
}

func init() {
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Open, handleCitizenOpen)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Request, handleCitizenRequest)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Accept, handleCitizenAccept)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Reply, handleCitizenReply)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Remove, handleCitizenRemove)
}

func handleCitizenOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.CitizenOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize citizen open", "id", p.ID, "err", err)
		return nil
	}

	sessionID := p.GenerateSessionID()
	_ = pkt

	return p.Bus.SendPacket(&server.CitizenOpenServerPacket{
		BehaviorId:    citizenBehaviorInn,
		CurrentHomeId: 0,
		SessionId:     sessionID,
		Questions:     citizenQuestions,
	})
}

func handleCitizenRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.CitizenRequestClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize citizen request", "id", p.ID, "err", err)
		return nil
	}

	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}

	return p.Bus.SendPacket(&server.CitizenRequestServerPacket{Cost: 0})
}

func handleCitizenAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.CitizenAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize citizen accept", "id", p.ID, "err", err)
		return nil
	}

	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	p.ClearSessionID()

	p.CharHP = p.CharMaxHP
	p.CharTP = p.CharMaxTP

	if err := p.Bus.SendPacket(&server.CitizenAcceptServerPacket{GoldAmount: p.Inventory[1]}); err != nil {
		return err
	}
	return p.Bus.SendPacket(&server.RecoverPlayerServerPacket{Hp: p.CharHP, Tp: p.CharTP})
}

func handleCitizenReply(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.CitizenReplyClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize citizen reply", "id", p.ID, "err", err)
		return nil
	}

	if !p.ValidateSessionID(pkt.SessionId) {
		return nil
	}
	p.ClearSessionID()

	return p.Bus.SendPacket(&server.CitizenReplyServerPacket{QuestionsWrong: 0})
}

func handleCitizenRemove(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.CitizenRemoveClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize citizen remove", "id", p.ID, "err", err)
		return nil
	}
	_ = pkt

	return p.Bus.SendPacket(&server.CitizenRemoveServerPacket{ReplyCode: server.InnUnsubscribeReply_Unsubscribed})
}

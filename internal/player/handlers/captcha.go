package handlers

import (
	"context"

	"github.com/avdo/goeoserv/internal/deep"
	"github.com/avdo/goeoserv/internal/formula"
	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(deep.FamilyCaptcha, eonet.PacketAction_Request, handleCaptchaRequest)
	player.Register(deep.FamilyCaptcha, eonet.PacketAction_Reply, handleCaptchaReply)
}

func handleCaptchaRequest(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	_ = ctx
	_ = deep.DeserializeCaptchaRequest(reader)
	if !p.World.HasCaptcha(p.ID) {
		return nil
	}
	p.World.RefreshCaptcha(p.ID)
	return nil
}

func handleCaptchaReply(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	_, value, err := deep.DeserializeCaptchaReply(reader)
	if err != nil {
		return nil
	}
	reward, solved := p.World.VerifyCaptcha(p.ID, value)
	if !solved {
		return nil
	}
	p.CharExp += reward
	newLevel := formula.LevelForExp(p.CharExp)
	if newLevel > p.CharLevel {
		p.CharLevel = newLevel
	}
	p.CalculateStats()
	payload, err := deep.SerializeCaptchaClose(p.CharExp)
	if err != nil {
		return nil
	}
	if err := p.Bus.Send(eonet.PacketAction_Close, deep.FamilyCaptcha, payload); err != nil {
		return err
	}
	_ = p.Bus.SendPacket(&server.RecoverReplyServerPacket{Experience: reward, Karma: 0})
	p.SaveCharacterAsync()
	return nil
}

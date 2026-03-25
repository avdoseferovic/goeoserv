package handlers

import (
	"context"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Barber, eonet.PacketAction_Open, handleBarberOpen)
	player.Register(eonet.PacketFamily_Barber, eonet.PacketAction_Buy, handleBarberBuy)
}

func handleBarberOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
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

func handleBarberBuy(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	var pkt client.BarberBuyClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	cost := p.Cfg.Barber.BaseCost + (p.CharLevel * p.Cfg.Barber.CostPerLevel)
	if !p.RemoveItem(1, cost) {
		return nil
	}
	p.CharHairStyle = pkt.HairStyle
	p.CharHairColor = pkt.HairColor

	if p.World != nil {
		p.World.BroadcastMap(p.MapID, p.ID, &server.AvatarAgreeServerPacket{
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

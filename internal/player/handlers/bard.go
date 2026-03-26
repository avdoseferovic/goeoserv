package handlers

import (
	"context"
	"slices"

	"github.com/avdo/goeoserv/internal/player"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	eoproto "github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Jukebox, eonet.PacketAction_Use, handlePlayInstrument)
}

func handlePlayInstrument(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}

	var pkt client.JukeboxUseClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	// Validate note ID
	if pkt.NoteId < 1 || pkt.NoteId > p.Cfg.Bard.MaxNoteID {
		return nil
	}

	// Validate instrument ID is in the allowed list
	if !slices.Contains(p.Cfg.Bard.InstrumentItems, pkt.InstrumentId) {
		return nil
	}

	// Check player has a weapon equipped and its graphic matches the instrument
	if p.Equipment.Weapon == 0 {
		return nil
	}
	item := pubdata.GetItem(p.Equipment.Weapon)
	if item == nil || item.Spec1 != pkt.InstrumentId {
		return nil
	}

	// Broadcast instrument note to nearby players
	p.World.BroadcastMap(p.MapID, p.ID, &server.JukeboxMsgServerPacket{
		PlayerId:     p.ID,
		Direction:    eoproto.Direction(p.CharDirection),
		InstrumentId: pkt.InstrumentId,
		NoteId:       pkt.NoteId,
	})

	return nil
}

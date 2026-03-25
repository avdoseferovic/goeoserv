package handlers

import (
	"log/slog"

	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/protocol"
	"github.com/ethanmoffat/eolib-go/v3/encrypt"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Init, eonet.PacketAction_Init, handleInitInit)
}

func handleInitInit(p *player.Player, reader *player.EoReader) error {
	var pkt client.InitInitClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize init packet", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	// TODO: Check bans
	// TODO: Check version min/max

	// Generate encryption parameters
	seq1, seq2, seqStart := protocol.GenerateInitSequenceBytes()
	p.Bus.Sequencer.SetStart(seqStart)
	// The client consumes the first sequence slot during init, so advance the server's counter to match
	p.Bus.Sequencer.NextSequence()

	challengeResponse := encrypt.ServerVerificationHash(pkt.Challenge)

	p.Bus.ClientEncryptionMultiple = protocol.GenerateSwapMultipleValue()
	p.Bus.ServerEncryptionMultiple = protocol.GenerateSwapMultipleValue()
	p.State = player.StateInitialized

	reply := &server.InitInitServerPacket{
		ReplyCode: server.InitReply_Ok,
		ReplyCodeData: &server.InitInitReplyCodeDataOk{
			Seq1:                     seq1,
			Seq2:                     seq2,
			ServerEncryptionMultiple: p.Bus.ServerEncryptionMultiple,
			ClientEncryptionMultiple: p.Bus.ClientEncryptionMultiple,
			ChallengeResponse:        challengeResponse,
			PlayerId:                 p.ID,
		},
	}

	if err := p.Bus.SendPacket(reply); err != nil {
		slog.Error("failed to send init reply", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	slog.Info("player initialized",
		"id", p.ID,
		"ip", p.IP,
		"version", pkt.Version,
	)

	return nil
}

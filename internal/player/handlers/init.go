package handlers

import (
	"errors"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

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

func handleInitInit(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	var pkt client.InitInitClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize init packet", "id", p.ID, "err", err)
		p.Close()
		return nil
	}

	// Check IP bans
	var banCount int
	err := p.DB.QueryRow(ctx,
		`SELECT COUNT(1) FROM bans WHERE ip = ?
		 AND (duration = 0 OR datetime(created_at, '+' || duration || ' minutes') > datetime('now'))`,
		p.IP).Scan(&banCount)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("error checking IP ban", "id", p.ID, "err", err)
	}
	if banCount > 0 {
		_ = p.Bus.SendPacket(&server.InitInitServerPacket{
			ReplyCode:     server.InitReply_Banned,
			ReplyCodeData: &server.InitInitReplyCodeDataBanned{},
		})
		p.Close()
		return nil
	}

	// Check client version
	if p.Cfg.Server.MinVersion != "" || p.Cfg.Server.MaxVersion != "" {
		clientVer := fmt.Sprintf("%d.%d.%d", pkt.Version.Major, pkt.Version.Minor, pkt.Version.Patch)
		if p.Cfg.Server.MinVersion != "" && compareVersions(clientVer, p.Cfg.Server.MinVersion) < 0 {
			_ = p.Bus.SendPacket(&server.InitInitServerPacket{
				ReplyCode:     server.InitReply_OutOfDate,
				ReplyCodeData: &server.InitInitReplyCodeDataOutOfDate{},
			})
			p.Close()
			return nil
		}
		if p.Cfg.Server.MaxVersion != "" && compareVersions(clientVer, p.Cfg.Server.MaxVersion) > 0 {
			_ = p.Bus.SendPacket(&server.InitInitServerPacket{
				ReplyCode:     server.InitReply_OutOfDate,
				ReplyCodeData: &server.InitInitReplyCodeDataOutOfDate{},
			})
			p.Close()
			return nil
		}
	}

	// Generate encryption parameters
	seq1, seq2, seqStart := protocol.GenerateInitSequenceBytes()
	p.Bus.Sequencer.SetStart(seqStart)
	p.Bus.Sequencer.NextSequence() // client consumes sequence slot 0 during init

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

	// INIT reply is automatically sent unencrypted (validForEncryption skips 0xFF 0xFF packets)
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

// compareVersions compares two semver strings ("major.minor.patch").
// Returns -1, 0, or 1.
func compareVersions(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := range 3 {
		va, vb := 0, 0
		if i < len(pa) {
			va, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			vb, _ = strconv.Atoi(pb[i])
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

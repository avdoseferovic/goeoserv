package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/avdo/goeoserv/internal/player"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Open, handleBoardOpen)
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Take, handleBoardTake)
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Create, handleBoardCreate)
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Remove, handleBoardRemove)
}

func handleBoardOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	var pkt client.BoardOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	// Admin board access control
	if pkt.BoardId == p.Cfg.Board.AdminBoard && p.CharAdmin < 1 {
		return nil
	}

	// Use higher limit for admin board
	maxPosts := p.Cfg.Board.MaxPosts
	if pkt.BoardId == p.Cfg.Board.AdminBoard && p.Cfg.Board.AdminMaxPosts > 0 {
		maxPosts = p.Cfg.Board.AdminMaxPosts
	}

	posts, err := queryBoardPostsWithLimit(ctx, p, pkt.BoardId, maxPosts)
	if err != nil {
		slog.Error("board open query failed", "board_id", pkt.BoardId, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.BoardOpenServerPacket{
		BoardId: pkt.BoardId,
		Posts:   posts,
	})
}

func handleBoardTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	var pkt client.BoardTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	var author, subject, body string
	err := p.DB.QueryRow(ctx,
		`SELECT c.name, bp.subject, bp.body FROM board_posts bp JOIN characters c ON bp.character_id = c.id WHERE bp.id = ?`,
		pkt.PostId).Scan(&author, &subject, &body)
	if err != nil {
		slog.Error("board take query failed", "post_id", pkt.PostId, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.BoardPlayerServerPacket{
		PostId:   pkt.PostId,
		PostBody: author + "\n" + subject + "\n" + body,
	})
}

func handleBoardCreate(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	if p.CharacterID == nil {
		return nil
	}
	var pkt client.BoardCreateClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	// Check per-user post limit
	if p.Cfg.Board.MaxUserPosts > 0 {
		var userPostCount int
		_ = p.DB.QueryRow(ctx,
			`SELECT COUNT(1) FROM board_posts WHERE board_id = ? AND character_id = ?`,
			pkt.BoardId, *p.CharacterID).Scan(&userPostCount)
		if userPostCount >= p.Cfg.Board.MaxUserPosts {
			return nil
		}
	}

	// Truncate subject and body to configured limits
	subject := pkt.PostSubject
	if maxLen := p.Cfg.Board.MaxSubjectLength; maxLen > 0 && len(subject) > maxLen {
		subject = subject[:maxLen]
	}
	body := pkt.PostBody
	if maxLen := p.Cfg.Board.MaxPostLength; maxLen > 0 && len(body) > maxLen {
		body = body[:maxLen]
	}

	err := p.DB.Execute(ctx,
		`INSERT INTO board_posts (board_id, character_id, subject, body) VALUES (?, ?, ?, ?)`,
		pkt.BoardId, *p.CharacterID, subject, body)
	if err != nil {
		slog.Error("board create insert failed", "board_id", pkt.BoardId, "err", err)
		return nil
	}

	posts, err := queryBoardPosts(ctx, p, pkt.BoardId)
	if err != nil {
		slog.Error("board create re-query failed", "board_id", pkt.BoardId, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.BoardOpenServerPacket{
		BoardId: pkt.BoardId,
		Posts:   posts,
	})
}

func handleBoardRemove(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	if p.CharacterID == nil {
		return nil
	}
	var pkt client.BoardRemoveClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	var ownerID int
	err := p.DB.QueryRow(ctx,
		`SELECT character_id FROM board_posts WHERE id = ?`, pkt.PostId).Scan(&ownerID)
	if err != nil {
		return nil
	}
	if ownerID != *p.CharacterID && p.CharAdmin < 1 {
		return nil
	}

	_ = p.DB.Execute(ctx,
		`DELETE FROM board_posts WHERE id = ?`, pkt.PostId)
	return nil
}

func queryBoardPosts(ctx context.Context, p *player.Player, boardID int) ([]server.BoardPostListing, error) {
	return queryBoardPostsWithLimit(ctx, p, boardID, p.Cfg.Board.MaxPosts)
}

func queryBoardPostsWithLimit(ctx context.Context, p *player.Player, boardID, limit int) ([]server.BoardPostListing, error) {
	rows, err := p.DB.Query(ctx,
		`SELECT bp.id, c.name, bp.subject, bp.created_at FROM board_posts bp JOIN characters c ON bp.character_id = c.id WHERE bp.board_id = ? ORDER BY bp.created_at DESC LIMIT ?`,
		boardID, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "err", err)
		}
	}()

	var posts []server.BoardPostListing
	for rows.Next() {
		var post server.BoardPostListing
		var createdAt time.Time
		if err := rows.Scan(&post.PostId, &post.Author, &post.Subject, &createdAt); err != nil {
			return nil, err
		}
		// Append time-ago to subject if date_posts is enabled
		if p.Cfg.Board.DatePosts && !createdAt.IsZero() {
			mins := int(time.Since(createdAt).Minutes())
			if mins < 1 {
				post.Subject += " (just now)"
			} else {
				post.Subject += fmt.Sprintf(" (%d min ago)", mins)
			}
		}
		posts = append(posts, post)
	}
	return posts, rows.Err()
}

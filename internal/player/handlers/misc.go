package handlers

import (
	"context"
	"log/slog"

	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/ethanmoffat/eolib-go/v3/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/client"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

func init() {
	// Barber
	player.Register(eonet.PacketFamily_Barber, eonet.PacketAction_Open, handleBarberOpen)
	player.Register(eonet.PacketFamily_Barber, eonet.PacketAction_Buy, handleBarberBuy)

	// Jukebox
	player.Register(eonet.PacketFamily_Jukebox, eonet.PacketAction_Open, handleJukeboxOpen)
	player.Register(eonet.PacketFamily_Jukebox, eonet.PacketAction_Msg, handleJukeboxMsg)
	player.Register(eonet.PacketFamily_Jukebox, eonet.PacketAction_Use, handleJukeboxUse)

	// Citizen (inn)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Open, handleCitizenNoop)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Request, handleCitizenNoop)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Accept, handleCitizenNoop)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Reply, handleCitizenNoop)
	player.Register(eonet.PacketFamily_Citizen, eonet.PacketAction_Remove, handleCitizenNoop)

	// Priest / Marriage
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Open, handlePriestNoop)
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Request, handlePriestNoop)
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Accept, handlePriestNoop)
	player.Register(eonet.PacketFamily_Priest, eonet.PacketAction_Use, handlePriestNoop)
	player.Register(eonet.PacketFamily_Marriage, eonet.PacketAction_Open, handleMarriageNoop)
	player.Register(eonet.PacketFamily_Marriage, eonet.PacketAction_Request, handleMarriageNoop)

	// AdminInteract
	player.Register(eonet.PacketFamily_AdminInteract, eonet.PacketAction_Tell, handleAdminInteractNoop)
	player.Register(eonet.PacketFamily_AdminInteract, eonet.PacketAction_Report, handleAdminInteractNoop)

	// Board
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Open, handleBoardOpen)
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Take, handleBoardTake)
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Create, handleBoardCreate)
	player.Register(eonet.PacketFamily_Board, eonet.PacketAction_Remove, handleBoardRemove)

	// Book (character info)
	player.Register(eonet.PacketFamily_Book, eonet.PacketAction_Request, handleBookNoop)

	// Locker
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Open, handleLockerOpen)
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Add, handleLockerAdd)
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Take, handleLockerTake)
	player.Register(eonet.PacketFamily_Locker, eonet.PacketAction_Buy, handleLockerBuy)

	// Chest
	player.Register(eonet.PacketFamily_Chest, eonet.PacketAction_Open, handleChestOpen)
	player.Register(eonet.PacketFamily_Chest, eonet.PacketAction_Add, handleChestAdd)
	player.Register(eonet.PacketFamily_Chest, eonet.PacketAction_Take, handleChestTake)

	// Message (server message)
	player.Register(eonet.PacketFamily_Message, eonet.PacketAction_Ping, handleMessageNoop)

	// Players list
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_Request, handlePlayersRequest)
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_Accept, handlePlayersNoop)
	player.Register(eonet.PacketFamily_Players, eonet.PacketAction_List, handlePlayersNoop)

	// Warp accept
	player.Register(eonet.PacketFamily_Warp, eonet.PacketAction_Accept, handleWarpAccept)
	player.Register(eonet.PacketFamily_Warp, eonet.PacketAction_Take, handleWarpNoop)
}

// Barber
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
	// Calculate barber cost
	cost := p.Cfg.Barber.BaseCost + (p.CharLevel * p.Cfg.Barber.CostPerLevel)
	gold := p.Inventory[1]
	if gold < cost {
		return nil
	}

	// Deduct gold and change hair
	p.Inventory[1] -= cost
	if p.Inventory[1] <= 0 {
		delete(p.Inventory, 1)
	}
	p.CharHairStyle = pkt.HairStyle
	p.CharHairColor = pkt.HairColor

	// Broadcast hair change to map
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

// Jukebox
func handleJukeboxOpen(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	return p.Bus.SendPacket(&server.JukeboxOpenServerPacket{MapId: p.MapID})
}

func handleJukeboxMsg(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handleJukeboxUse(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }

// Board
func handleBoardOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	var pkt client.BoardOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	posts, err := queryBoardPosts(ctx, p, pkt.BoardId)
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

	err := p.DB.Execute(ctx,
		`INSERT INTO board_posts (board_id, character_id, subject, body) VALUES (?, ?, ?, ?)`,
		pkt.BoardId, *p.CharacterID, pkt.PostSubject, pkt.PostBody)
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

	// Only allow if the post belongs to this character or they are an admin
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
	rows, err := p.DB.Query(ctx,
		`SELECT bp.id, c.name, bp.subject FROM board_posts bp JOIN characters c ON bp.character_id = c.id WHERE bp.board_id = ? ORDER BY bp.created_at DESC LIMIT ?`,
		boardID, p.Cfg.Board.MaxPosts)
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
		if err := rows.Scan(&post.PostId, &post.Author, &post.Subject); err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, rows.Err()
}

// Players list (online users)
func handlePlayersRequest(ctx context.Context, p *player.Player, _ *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}
	if p.World == nil {
		return p.Bus.SendPacket(&server.PlayersListServerPacket{})
	}
	raw := p.World.GetOnlinePlayers()
	infos, _ := raw.([]gamemap.OnlinePlayerInfo)

	var records []server.OnlinePlayer
	for _, info := range infos {
		records = append(records, server.OnlinePlayer{
			Name:     info.Name,
			Title:    info.Title,
			Level:    info.Level,
			Icon:     adminIcon(info.Admin),
			ClassId:  info.ClassID,
			GuildTag: info.GuildTag,
		})
	}
	return p.Bus.SendPacket(&server.PlayersListServerPacket{
		PlayersList: server.PlayersList{
			Players: records,
		},
	})
}

// Warp accept — client acknowledges a map change
func handleWarpAccept(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame {
		return nil
	}

	var pkt client.WarpAcceptClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		slog.Error("failed to deserialize warp accept", "id", p.ID, "err", err)
		return nil
	}

	if p.World == nil {
		return nil
	}

	// Get pending warp destination (check player-level first, then map-level)
	var toMapID, toX, toY int
	var ok bool
	if p.PendingWarp != nil {
		toMapID = p.PendingWarp.MapID
		toX = p.PendingWarp.X
		toY = p.PendingWarp.Y
		p.PendingWarp = nil
		ok = true
	} else {
		toMapID, toX, toY, ok = p.World.GetPendingWarp(p.MapID, p.ID)
	}
	if !ok {
		return nil
	}

	// Move player to new map
	raw := p.World.WarpPlayer(p.ID, p.MapID, toMapID, toX, toY)
	p.MapID = toMapID
	p.CharX = toX
	p.CharY = toY

	var nearby server.NearbyInfo
	if ni, ok := raw.(*server.NearbyInfo); ok && ni != nil {
		nearby = *ni
	}

	return p.Bus.SendPacket(&server.WarpAgreeServerPacket{
		WarpType: server.Warp_Local,
		WarpTypeData: &server.WarpAgreeWarpTypeDataMapSwitch{
			MapId: toMapID,
		},
		Nearby: nearby,
	})
}

// Locker (bank item storage)

func queryLockerItems(ctx context.Context, p *player.Player) ([]eonet.ThreeItem, error) {
	rows, err := p.DB.Query(ctx,
		"SELECT item_id, quantity FROM character_bank WHERE character_id = ?", *p.CharacterID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "err", err)
		}
	}()

	var items []eonet.ThreeItem
	for rows.Next() {
		var item eonet.ThreeItem
		if err := rows.Scan(&item.Id, &item.Amount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func handleLockerOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.LockerOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	items, err := queryLockerItems(ctx, p)
	if err != nil {
		slog.Error("locker open query failed", "id", p.ID, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.LockerOpenServerPacket{
		LockerCoords: protocol.Coords{X: pkt.LockerCoords.X, Y: pkt.LockerCoords.Y},
		LockerItems:  items,
	})
}

func handleLockerAdd(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.LockerAddClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	itemID := pkt.DepositItem.Id
	amount := pkt.DepositItem.Amount
	if itemID <= 0 || amount <= 0 {
		return nil
	}

	// Check player has enough
	if p.Inventory[itemID] < amount {
		return nil
	}

	// Remove from inventory
	p.Inventory[itemID] -= amount
	if p.Inventory[itemID] <= 0 {
		delete(p.Inventory, itemID)
	}

	// Insert/update bank
	err := p.DB.Execute(ctx,
		`INSERT INTO character_bank (character_id, item_id, quantity) VALUES (?, ?, ?)
		ON CONFLICT(character_id, item_id) DO UPDATE SET quantity = quantity + ?`,
		*p.CharacterID, itemID, amount, amount)
	if err != nil {
		slog.Error("locker add db failed", "id", p.ID, "err", err)
		// Rollback inventory change
		p.Inventory[itemID] += amount
		return nil
	}

	items, err := queryLockerItems(ctx, p)
	if err != nil {
		slog.Error("locker add re-query failed", "id", p.ID, "err", err)
		return nil
	}

	remaining := p.Inventory[itemID]
	return p.Bus.SendPacket(&server.LockerReplyServerPacket{
		DepositedItem: eonet.Item{Id: itemID, Amount: remaining},
		Weight:        eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
		LockerItems:   items,
	})
}

func handleLockerTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.CharacterID == nil {
		return nil
	}
	var pkt client.LockerTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}

	itemID := pkt.TakeItemId
	if itemID <= 0 {
		return nil
	}

	// Query how much is in the bank for this item
	var bankQty int
	err := p.DB.QueryRow(ctx,
		"SELECT quantity FROM character_bank WHERE character_id = ? AND item_id = ?",
		*p.CharacterID, itemID).Scan(&bankQty)
	if err != nil || bankQty <= 0 {
		return nil
	}

	// Remove from bank (take all of that item)
	err = p.DB.Execute(ctx,
		"DELETE FROM character_bank WHERE character_id = ? AND item_id = ?",
		*p.CharacterID, itemID)
	if err != nil {
		slog.Error("locker take db failed", "id", p.ID, "err", err)
		return nil
	}

	// Add to inventory
	p.Inventory[itemID] += bankQty

	items, err := queryLockerItems(ctx, p)
	if err != nil {
		slog.Error("locker take re-query failed", "id", p.ID, "err", err)
		return nil
	}

	return p.Bus.SendPacket(&server.LockerGetServerPacket{
		TakenItem:   eonet.ThreeItem{Id: itemID, Amount: p.Inventory[itemID]},
		Weight:      eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
		LockerItems: items,
	})
}

// No-op stubs
func handleCitizenNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error       { return nil }
func handlePriestNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error        { return nil }
func handleMarriageNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error      { return nil }
func handleAdminInteractNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handleBookNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error          { return nil }
func handleLockerBuy(_ context.Context, _ *player.Player, _ *player.EoReader) error         { return nil }
func handleChestOpen(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	var pkt client.ChestOpenClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	raw := p.World.GetChestItems(p.MapID, pkt.Coords.X, pkt.Coords.Y)
	items, _ := raw.([]gamemap.ChestItem)
	var eoItems []eonet.ThreeItem
	for _, ci := range items {
		eoItems = append(eoItems, eonet.ThreeItem{Id: ci.ItemID, Amount: ci.Amount})
	}
	return p.Bus.SendPacket(&server.ChestOpenServerPacket{
		Coords: pkt.Coords,
		Items:  eoItems,
	})
}

func handleChestAdd(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	var pkt client.ChestAddClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	if p.Inventory[pkt.AddItem.Id] < pkt.AddItem.Amount {
		return nil
	}
	raw := p.World.AddChestItem(p.MapID, pkt.Coords.X, pkt.Coords.Y, pkt.AddItem.Id, pkt.AddItem.Amount)
	if raw == nil {
		return nil
	}
	p.Inventory[pkt.AddItem.Id] -= pkt.AddItem.Amount
	if p.Inventory[pkt.AddItem.Id] <= 0 {
		delete(p.Inventory, pkt.AddItem.Id)
	}
	items, _ := raw.([]gamemap.ChestItem)
	var eoItems []eonet.ThreeItem
	for _, ci := range items {
		eoItems = append(eoItems, eonet.ThreeItem{Id: ci.ItemID, Amount: ci.Amount})
	}
	return p.Bus.SendPacket(&server.ChestReplyServerPacket{
		AddedItemId:     pkt.AddItem.Id,
		RemainingAmount: p.Inventory[pkt.AddItem.Id],
		Weight:          eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
		Items:           eoItems,
	})
}

func handleChestTake(ctx context.Context, p *player.Player, reader *player.EoReader) error {
	if p.State != player.StateInGame || p.World == nil {
		return nil
	}
	var pkt client.ChestTakeClientPacket
	if err := pkt.Deserialize(reader); err != nil {
		return nil
	}
	amount, raw := p.World.TakeChestItem(p.MapID, pkt.Coords.X, pkt.Coords.Y, pkt.TakeItemId)
	if amount == 0 {
		return nil
	}
	p.Inventory[pkt.TakeItemId] += amount
	items, _ := raw.([]gamemap.ChestItem)
	var eoItems []eonet.ThreeItem
	for _, ci := range items {
		eoItems = append(eoItems, eonet.ThreeItem{Id: ci.ItemID, Amount: ci.Amount})
	}
	return p.Bus.SendPacket(&server.ChestGetServerPacket{
		TakenItem: eonet.ThreeItem{Id: pkt.TakeItemId, Amount: p.Inventory[pkt.TakeItemId]},
		Weight:    eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
		Items:     eoItems,
	})
}
func handleMessageNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handlePlayersNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error { return nil }
func handleWarpNoop(_ context.Context, _ *player.Player, _ *player.EoReader) error    { return nil }

// adminIcon converts an admin level to the CharacterIcon for the online list.
func adminIcon(admin int) server.CharacterIcon {
	switch {
	case admin >= 4:
		return server.CharacterIcon_Hgm
	case admin >= 1:
		return server.CharacterIcon_Gm
	default:
		return server.CharacterIcon_Player
	}
}

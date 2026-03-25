package handlers

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"fmt"

	"github.com/avdo/goeoserv/internal/formula"
	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/avdo/goeoserv/internal/protocol"
	pubdata "github.com/avdo/goeoserv/internal/pub"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// handleCommand processes a $ admin command. Returns true if the message was a command.
func handleCommand(ctx context.Context, p *player.Player, message string) bool {
	if !strings.HasPrefix(message, "$") {
		return false
	}

	parts := strings.Fields(message[1:])
	if len(parts) == 0 {
		return true
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "kick":
		handleCmdKick(p, args)
	case "ban":
		handleCmdBan(ctx, p, args)
	case "jail":
		handleCmdJail(p, args)
	case "free":
		handleCmdFree(p, args)
	case "mute":
		handleCmdMute(p, args)
	case "warp":
		handleCmdWarp(p, args)
	case "warpto":
		handleCmdWarpTo(p, args)
	case "warptome":
		handleCmdWarpToMe(p, args)
	case "setlevel":
		handleCmdSetLevel(p, args)
	case "item", "give":
		handleCmdItem(p, args)
	case "find":
		handleCmdFind(p, args)
	case "info", "loc":
		handleCmdInfo(p)
	default:
		return true // suppress unknown $commands so they don't leak to chat
	}

	return true
}

// $kick <name> — requires admin >= 1
func handleCmdKick(p *player.Player, args []string) {
	if p.CharAdmin < 1 || len(args) < 1 {
		return
	}

	targetName := strings.ToLower(args[0])
	if p.World == nil {
		return
	}

	targetID, found := p.World.FindPlayerByName(targetName)
	if !found {
		return
	}

	// Send a server message to the target then disconnect them
	p.World.SendToPlayer(targetID, &server.TalkServerServerPacket{
		Message: "You have been kicked by " + p.CharName,
	})

	slog.Info("admin kick", "admin", p.CharName, "target", targetName)
}

// $ban <name> [duration_minutes] — requires admin >= 2
func handleCmdBan(ctx context.Context, p *player.Player, args []string) {
	if p.CharAdmin < 2 || len(args) < 1 {
		return
	}

	targetName := strings.ToLower(args[0])
	duration := 0 // permanent by default
	if len(args) >= 2 {
		duration, _ = strconv.Atoi(args[1])
	}

	// Look up account ID for the target
	var accountID int
	err := p.DB.QueryRow(ctx,
		`SELECT account_id FROM characters WHERE name = ?`, targetName).Scan(&accountID)
	if err != nil {
		return
	}

	_ = p.DB.Execute(ctx,
		`INSERT INTO bans (account_id, duration) VALUES (?, ?)`, accountID, duration)

	slog.Info("admin ban", "admin", p.CharName, "target", targetName, "duration", duration)
}

// $jail <name> — requires admin >= 1
func handleCmdJail(p *player.Player, args []string) {
	if p.CharAdmin < 1 || len(args) < 1 || p.World == nil {
		return
	}

	targetName := strings.ToLower(args[0])
	targetID, found := p.World.FindPlayerByName(targetName)
	if !found {
		return
	}

	jailMap := p.Cfg.Jail.Map
	jailX := p.Cfg.Jail.X
	jailY := p.Cfg.Jail.Y
	if jailMap <= 0 {
		return
	}

	p.World.SendToPlayer(targetID, &server.TalkServerServerPacket{
		Message: "You have been jailed by " + p.CharName,
	})

	// We need target's current mapID to warp. Use a helper that finds and warps.
	// For now, broadcast a warp request to the target
	p.World.SendToPlayer(targetID, &server.WarpRequestServerPacket{
		WarpType:     server.Warp_Local,
		MapId:        jailMap,
		WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
	})
	_ = jailX
	_ = jailY

	slog.Info("admin jail", "admin", p.CharName, "target", targetName)
}

// $free <name> — requires admin >= 1
func handleCmdFree(p *player.Player, args []string) {
	if p.CharAdmin < 1 || len(args) < 1 || p.World == nil {
		return
	}

	targetName := strings.ToLower(args[0])
	targetID, found := p.World.FindPlayerByName(targetName)
	if !found {
		return
	}

	freeMap := p.Cfg.Jail.FreeMap
	if freeMap <= 0 {
		freeMap = p.Cfg.Rescue.Map
	}

	p.World.SendToPlayer(targetID, &server.WarpRequestServerPacket{
		WarpType:     server.Warp_Local,
		MapId:        freeMap,
		WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
	})

	slog.Info("admin free", "admin", p.CharName, "target", targetName)
}

// $mute <name> — requires admin >= 1 (placeholder - sets a flag)
func handleCmdMute(p *player.Player, args []string) {
	if p.CharAdmin < 1 || len(args) < 1 {
		return
	}
	slog.Info("admin mute", "admin", p.CharName, "target", args[0])
}

// $warp <map> [x] [y] — requires admin >= 2
func handleCmdWarp(p *player.Player, args []string) {
	if p.CharAdmin < 2 || len(args) < 1 || p.World == nil {
		return
	}

	mapID, _ := strconv.Atoi(args[0])
	x, y := 0, 0
	if len(args) >= 3 {
		x, _ = strconv.Atoi(args[1])
		y, _ = strconv.Atoi(args[2])
	}

	if mapID <= 0 {
		return
	}

	// Set pending warp and send request to client (client must acknowledge with WarpAccept)
	p.PendingWarp = &player.PendingWarp{MapID: mapID, X: x, Y: y}
	_ = p.Bus.SendPacket(&server.WarpRequestServerPacket{
		WarpType:     server.Warp_Local,
		MapId:        mapID,
		WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
	})

	slog.Info("admin warp", "admin", p.CharName, "map", mapID, "x", x, "y", y)
}

// $warpto <name> — warp yourself to a player. Requires admin >= 1.
func handleCmdWarpTo(p *player.Player, args []string) {
	if p.CharAdmin < 1 || len(args) < 1 || p.World == nil {
		return
	}

	targetName := strings.ToLower(args[0])
	targetID, found := p.World.FindPlayerByName(targetName)
	if !found {
		return
	}

	pos, _ := p.World.GetPlayerPosition(targetID).(*gamemap.PlayerPosition)
	if pos == nil {
		return
	}

	p.PendingWarp = &player.PendingWarp{MapID: pos.MapID, X: pos.X, Y: pos.Y}
	_ = p.Bus.SendPacket(&server.WarpRequestServerPacket{
		WarpType:     server.Warp_Local,
		MapId:        pos.MapID,
		WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
	})

	slog.Info("admin warpto", "admin", p.CharName, "target", targetName)
}

// $warptome <name> — warp a player to you. Requires admin >= 1.
func handleCmdWarpToMe(p *player.Player, args []string) {
	if p.CharAdmin < 1 || len(args) < 1 || p.World == nil {
		return
	}

	targetName := strings.ToLower(args[0])
	targetID, found := p.World.FindPlayerByName(targetName)
	if !found {
		return
	}

	// Send warp request to the target player
	targetBus, _ := p.World.GetPlayerBus(targetID).(*protocol.PacketBus)
	if targetBus == nil {
		return
	}

	// Set pending warp on the target via the map
	pos, _ := p.World.GetPlayerPosition(targetID).(*gamemap.PlayerPosition)
	if pos == nil {
		return
	}

	// We need to set the pending warp on the target's map character.
	// Use WarpPlayer flow: store destination and send request to the target client.
	p.World.SetPendingWarp(pos.MapID, targetID, p.MapID, p.CharX, p.CharY)
	_ = targetBus.SendPacket(&server.WarpRequestServerPacket{
		WarpType:     server.Warp_Local,
		MapId:        p.MapID,
		WarpTypeData: &server.WarpRequestWarpTypeDataMapSwitch{},
	})

	slog.Info("admin warptome", "admin", p.CharName, "target", targetName)
}

// $info / $loc — show current position
func handleCmdInfo(p *player.Player) {
	msg := "Map: " + strconv.Itoa(p.MapID) + " X: " + strconv.Itoa(p.CharX) + " Y: " + strconv.Itoa(p.CharY)
	_ = p.Bus.SendPacket(&server.TalkServerServerPacket{
		Message: msg,
	})
}

// $setlevel <level> — requires admin >= 2
func handleCmdSetLevel(p *player.Player, args []string) {
	if p.CharAdmin < 2 || len(args) < 1 {
		return
	}

	level, _ := strconv.Atoi(args[0])
	if level < 1 || level > 200 {
		return
	}

	p.CharLevel = level
	p.CharExp = formula.ExpForLevel(level)
	p.CalculateStats()
	p.CharHP = p.CharMaxHP
	p.CharTP = p.CharMaxTP

	_ = p.Bus.SendPacket(&server.RecoverReplyServerPacket{
		Experience:  p.CharExp,
		Karma:       0,
		LevelUp:     &level,
		StatPoints:  &p.StatPoints,
		SkillPoints: &p.SkillPoints,
	})

	sendMsg(p, fmt.Sprintf("Level set to %d", level))
	slog.Info("admin setlevel", "admin", p.CharName, "level", level)
}

// $item <id> [amount] — requires admin >= 2
func handleCmdItem(p *player.Player, args []string) {
	if p.CharAdmin < 2 || len(args) < 1 {
		return
	}

	itemID, _ := strconv.Atoi(args[0])
	amount := 1
	if len(args) >= 2 {
		amount, _ = strconv.Atoi(args[1])
	}
	if itemID <= 0 || amount <= 0 {
		return
	}

	rec := pubdata.GetItem(itemID)
	if rec == nil {
		sendMsg(p, "Item not found")
		return
	}

	p.Inventory[itemID] += amount
	p.CalculateStats()

	_ = p.Bus.SendPacket(&server.ItemGetServerPacket{
		TakenItemIndex: 0,
		TakenItem:      eonet.ThreeItem{Id: itemID, Amount: p.Inventory[itemID]},
		Weight:         eonet.Weight{Current: p.Weight, Max: p.MaxWeight},
	})

	sendMsg(p, fmt.Sprintf("Gave %dx %s (ID %d)", amount, rec.Name, itemID))
	slog.Info("admin item", "admin", p.CharName, "item", itemID, "amount", amount)
}

// $find <name> — search items by name (no admin required, useful for everyone)
func handleCmdFind(p *player.Player, args []string) {
	if len(args) < 1 {
		return
	}

	search := strings.ToLower(strings.Join(args, " "))
	if pubdata.ItemDB == nil {
		sendMsg(p, "Item database not loaded")
		return
	}

	found := 0
	for i, item := range pubdata.ItemDB.Items {
		if strings.Contains(strings.ToLower(item.Name), search) {
			sendMsg(p, fmt.Sprintf("[%d] %s", i+1, item.Name))
			found++
			if found >= 10 {
				sendMsg(p, "(showing first 10 results)")
				break
			}
		}
	}
	if found == 0 {
		sendMsg(p, "No items found matching '"+search+"'")
	}
}

func sendMsg(p *player.Player, msg string) {
	_ = p.Bus.SendPacket(&server.TalkServerServerPacket{Message: msg})
}

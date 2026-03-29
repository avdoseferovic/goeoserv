package handlers

import (
	"context"
	"strings"

	"github.com/avdo/goeoserv/internal/gamemap"
	"github.com/avdo/goeoserv/internal/player"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// handleCommand processes a $ admin command. Returns true if the message was a command.
func handleCommand(ctx context.Context, p *player.Player, message string) bool {
	cmdStr, ok := strings.CutPrefix(message, "$")
	if !ok {
		return false
	}

	parts := strings.Fields(cmdStr)
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
	case "unmute":
		handleCmdUnmute(p, args)
	case "lookup", "where":
		handleCmdLookup(p, args)
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
	case "evacuate":
		handleCmdEvacuate(p)
	case "captcha":
		handleCmdCaptcha(p, args)
	default:
		return true // suppress unknown $commands so they don't leak to chat
	}

	return true
}

func sendMsg(p *player.Player, msg string) {
	_ = p.Bus.SendPacket(&server.TalkServerServerPacket{Message: msg})
}

func onlinePlayers(world player.WorldInterface) []gamemap.OnlinePlayerInfo {
	raw := world.GetOnlinePlayers()
	infos, _ := raw.([]gamemap.OnlinePlayerInfo)
	return infos
}

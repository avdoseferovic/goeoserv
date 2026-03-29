package handlers

import (
	"fmt"
	"strings"

	"github.com/avdo/goeoserv/internal/player"
	pubdata "github.com/avdo/goeoserv/internal/pub"
)

// $info / $loc — show current position (no admin required)
func handleCmdInfo(p *player.Player) {
	sendMsg(p, fmt.Sprintf("Map: %d X: %d Y: %d", p.MapID, p.CharX, p.CharY))
}

// $find <name> — search items by name (no admin required)
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

package world

import (
	"github.com/avdo/goeoserv/internal/protocol"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// WeddingState tracks the ceremony progress.
type WeddingState int

const (
	WeddingRequested WeddingState = iota
	WeddingAccepted
	WeddingPriestDialog1
	WeddingPriestDoYouPartner
	WeddingWaitingForPartner
	WeddingPartnerAgrees
	WeddingPriestDoYouPlayer
	WeddingWaitingForPlayer
	WeddingPlayerAgrees
	WeddingFinalizing
	WeddingPriestAnnounce
	WeddingDone
)

// Wedding tracks an active marriage ceremony on a map.
type Wedding struct {
	PlayerID   int
	PartnerID  int
	NpcIndex   int
	State      WeddingState
	Ticks      int // countdown for current state
	MapID      int
	PlayerBus  *protocol.PacketBus
	PartnerBus *protocol.PacketBus
}

// MapWeddings tracks active weddings per map.
// Key: mapID, only one wedding per map at a time.
var activeWeddings = make(map[int]*Wedding)

// StartWedding begins a wedding ceremony on a map.
func StartWedding(mapID, playerID, partnerID, npcIndex int, playerBus, partnerBus *protocol.PacketBus) bool {
	if _, exists := activeWeddings[mapID]; exists {
		return false // already a wedding in progress
	}
	activeWeddings[mapID] = &Wedding{
		PlayerID:   playerID,
		PartnerID:  partnerID,
		NpcIndex:   npcIndex,
		State:      WeddingRequested,
		MapID:      mapID,
		PlayerBus:  playerBus,
		PartnerBus: partnerBus,
	}
	return true
}

// GetWedding returns the active wedding on a map, or nil.
func GetWedding(mapID int) *Wedding {
	return activeWeddings[mapID]
}

// EndWedding removes the active wedding on a map.
func EndWedding(mapID int) {
	delete(activeWeddings, mapID)
}

// TickWeddings advances all active wedding state machines by one tick.
func TickWeddings(delayTicks int) {
	for mapID, w := range activeWeddings {
		w.Ticks--
		if w.Ticks > 0 {
			continue
		}

		switch w.State {
		case WeddingRequested:
			// Wait for partner acceptance — handled by handler
			// Timeout after 30 seconds
			if w.Ticks <= -30*8 {
				EndWedding(mapID)
			}

		case WeddingAccepted:
			// Start ceremony after delay
			w.State = WeddingPriestDialog1
			w.Ticks = delayTicks

		case WeddingPriestDialog1:
			// Ask partner "do you?"
			w.State = WeddingPriestDoYouPartner
			w.Ticks = 0
			// Send dialog to partner
			_ = w.PartnerBus.SendPacket(&server.PriestReplyServerPacket{
				ReplyCode: server.PriestReply_DoYou,
			})

		case WeddingPriestDoYouPartner:
			w.State = WeddingWaitingForPartner
			w.Ticks = 20 * 8 // 20 second timeout

		case WeddingWaitingForPartner:
			// Timeout — cancel
			_ = w.PlayerBus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
			_ = w.PartnerBus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
			EndWedding(mapID)

		case WeddingPartnerAgrees:
			// Ask player "do you?"
			w.State = WeddingPriestDoYouPlayer
			w.Ticks = 8 // brief delay
			_ = w.PlayerBus.SendPacket(&server.PriestReplyServerPacket{
				ReplyCode: server.PriestReply_DoYou,
			})

		case WeddingPriestDoYouPlayer:
			w.State = WeddingWaitingForPlayer
			w.Ticks = 20 * 8

		case WeddingWaitingForPlayer:
			_ = w.PlayerBus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
			_ = w.PartnerBus.SendPacket(&server.TalkServerServerPacket{Message: "Wedding ceremony cancelled."})
			EndWedding(mapID)

		case WeddingPlayerAgrees:
			// Announce and complete
			w.State = WeddingPriestAnnounce
			w.Ticks = 16 // 2 second celebration delay

		case WeddingFinalizing:
			// Wait for the priest use handler to finish DB persistence.
			continue

		case WeddingPriestAnnounce:
			w.State = WeddingDone
			_ = w.PlayerBus.SendPacket(&server.TalkServerServerPacket{Message: "The wedding ceremony is complete."})
			_ = w.PartnerBus.SendPacket(&server.TalkServerServerPacket{Message: "The wedding ceremony is complete."})
			// Ceremony complete — caller should give rings and update DB

		case WeddingDone:
			EndWedding(mapID)
		}
	}
}

// AcceptWedding marks that the partner has accepted the ceremony request.
func AcceptWedding(mapID, partnerID int) bool {
	w := activeWeddings[mapID]
	if w == nil || w.PartnerID != partnerID || w.State != WeddingRequested {
		return false
	}
	w.State = WeddingAccepted
	w.Ticks = 0
	return true
}

// RespondIDo handles the "I do" response from either player or partner.
func RespondIDo(mapID, playerID int) bool {
	w := activeWeddings[mapID]
	if w == nil {
		return false
	}
	if playerID == w.PartnerID && w.State == WeddingWaitingForPartner {
		w.State = WeddingPartnerAgrees
		w.Ticks = 0
		return true
	}
	if playerID == w.PlayerID && w.State == WeddingWaitingForPlayer {
		w.State = WeddingPlayerAgrees
		w.Ticks = 0
		return true
	}
	return false
}

// ReadyToFinalize reports whether both participants have accepted the wedding.
func ReadyToFinalize(mapID int) bool {
	w := activeWeddings[mapID]
	return w != nil && w.State == WeddingPlayerAgrees
}

// BeginWeddingFinalization atomically marks a ready wedding as finalizing.
func BeginWeddingFinalization(mapID int) (int, int, bool) {
	w := activeWeddings[mapID]
	if w == nil || w.State != WeddingPlayerAgrees {
		return 0, 0, false
	}
	w.State = WeddingFinalizing
	w.Ticks = 0
	return w.PlayerID, w.PartnerID, true
}

// Participants returns the player and partner ids for an active wedding.
func Participants(mapID int) (int, int, bool) {
	w := activeWeddings[mapID]
	if w == nil {
		return 0, 0, false
	}
	return w.PlayerID, w.PartnerID, true
}

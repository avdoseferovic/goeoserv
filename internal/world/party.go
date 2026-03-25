package world

import (
	"sync"

	"github.com/avdo/goeoserv/internal/protocol"
	eonet "github.com/ethanmoffat/eolib-go/v3/protocol/net"
	"github.com/ethanmoffat/eolib-go/v3/protocol/net/server"
)

// Party represents a group of players.
type Party struct {
	mu       sync.RWMutex
	ID       int
	LeaderID int
	Members  []PartyMemberInfo
}

type PartyMemberInfo struct {
	PlayerID int
	Name     string
	Level    int
	HP       int
	MaxHP    int
	Bus      *protocol.PacketBus
}

var (
	partyMu     sync.Mutex
	parties     = make(map[int]*Party) // partyID -> party
	playerParty = make(map[int]int)    // playerID -> partyID
	nextPartyID = 1
)

// CreateParty creates a new party with the given leader.
func CreateParty(leader PartyMemberInfo) *Party {
	partyMu.Lock()
	defer partyMu.Unlock()

	id := nextPartyID
	nextPartyID++

	p := &Party{
		ID:       id,
		LeaderID: leader.PlayerID,
		Members:  []PartyMemberInfo{leader},
	}
	parties[id] = p
	playerParty[leader.PlayerID] = id
	return p
}

// GetParty returns the party a player belongs to, or nil.
func GetParty(playerID int) *Party {
	partyMu.Lock()
	defer partyMu.Unlock()

	pid, ok := playerParty[playerID]
	if !ok {
		return nil
	}
	return parties[pid]
}

// AddMember adds a player to a party.
func (p *Party) AddMember(member PartyMemberInfo, maxSize int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.Members) >= maxSize {
		return false
	}

	p.Members = append(p.Members, member)

	partyMu.Lock()
	playerParty[member.PlayerID] = p.ID
	partyMu.Unlock()

	// Notify all members
	addPkt := &server.PartyAddServerPacket{
		Member: server.PartyMember{
			PlayerId:     member.PlayerID,
			Leader:       false,
			Level:        member.Level,
			HpPercentage: hpPct(member.HP, member.MaxHP),
			Name:         member.Name,
		},
	}
	for _, m := range p.Members {
		if m.PlayerID != member.PlayerID {
			_ = m.Bus.SendPacket(addPkt)
		}
	}

	// Send full party list to new member
	_ = member.Bus.SendPacket(&server.PartyCreateServerPacket{
		Members: p.buildMemberList(),
	})

	return true
}

// RemoveMember removes a player from the party.
func (p *Party) RemoveMember(playerID int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	partyMu.Lock()
	delete(playerParty, playerID)
	partyMu.Unlock()

	for i, m := range p.Members {
		if m.PlayerID == playerID {
			p.Members = append(p.Members[:i], p.Members[i+1:]...)
			break
		}
	}

	// If party is empty or only 1 member left, disband
	if len(p.Members) <= 1 {
		p.disband()
		return
	}

	// If leader left, promote next member
	if p.LeaderID == playerID {
		p.LeaderID = p.Members[0].PlayerID
	}

	// Notify remaining members
	pkt := &server.PartyRemoveServerPacket{PlayerId: playerID}
	for _, m := range p.Members {
		_ = m.Bus.SendPacket(pkt)
	}
}

func (p *Party) disband() {
	partyMu.Lock()
	defer partyMu.Unlock()

	closePkt := &server.PartyCloseServerPacket{}
	for _, m := range p.Members {
		delete(playerParty, m.PlayerID)
		_ = m.Bus.SendPacket(closePkt)
	}
	delete(parties, p.ID)
}

// BuildMemberListPublic returns the party member list for sending to clients.
func (p *Party) BuildMemberListPublic() []server.PartyMember {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.buildMemberList()
}

func (p *Party) buildMemberList() []server.PartyMember {
	var members []server.PartyMember
	for _, m := range p.Members {
		members = append(members, server.PartyMember{
			PlayerId:     m.PlayerID,
			Leader:       m.PlayerID == p.LeaderID,
			Level:        m.Level,
			HpPercentage: hpPct(m.HP, m.MaxHP),
			Name:         m.Name,
		})
	}
	return members
}

// BroadcastToParty sends a packet to all party members.
func (p *Party) BroadcastToParty(pkt eonet.Packet) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, m := range p.Members {
		_ = m.Bus.SendPacket(pkt)
	}
}

func hpPct(hp, maxHP int) int {
	if maxHP <= 0 {
		return 100
	}
	return int(float64(hp) / float64(maxHP) * 100)
}

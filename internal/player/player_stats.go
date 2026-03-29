package player

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

// GenerateSessionID creates and stores a cryptographically random session ID.
// Must fit in an EO short (max 64008).
func (p *Player) GenerateSessionID() int {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	id := int(binary.LittleEndian.Uint32(buf[:]))%64000 + 1
	p.SessionID = &id
	return id
}

// TakeSessionID returns and clears the stored session ID.
func (p *Player) TakeSessionID() (int, bool) {
	if p.SessionID == nil {
		return 0, false
	}
	id := *p.SessionID
	p.SessionID = nil
	return id, true
}

// TakeAndValidateSessionID returns true when the stored session id exists and
// matches the expected value. The session is cleared in both cases to preserve
// current single-use behavior.
func (p *Player) TakeAndValidateSessionID(expected int) bool {
	id, ok := p.TakeSessionID()
	if !ok {
		return false
	}
	return id == expected
}

// ValidateSessionID reports whether the current session id matches the expected
// value without clearing it.
func (p *Player) ValidateSessionID(expected int) bool {
	if p.SessionID == nil {
		return false
	}
	return *p.SessionID == expected
}

// ClearSessionID clears any stored handler session id.
func (p *Player) ClearSessionID() {
	p.SessionID = nil
}

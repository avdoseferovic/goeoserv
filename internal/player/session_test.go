package player

import "testing"

func TestTakeAndValidateSessionID(t *testing.T) {
	p := &Player{}
	id := 12345
	p.SessionID = &id

	if !p.TakeAndValidateSessionID(12345) {
		t.Fatal("expected session id to validate")
	}

	if p.SessionID != nil {
		t.Fatal("expected session id to be cleared after validation")
	}
}

func TestTakeAndValidateSessionIDWrongValueClearsSession(t *testing.T) {
	p := &Player{}
	id := 12345
	p.SessionID = &id

	if p.TakeAndValidateSessionID(54321) {
		t.Fatal("expected session id validation to fail")
	}

	if p.SessionID != nil {
		t.Fatal("expected session id to be cleared after mismatch")
	}
}

func TestValidateSessionIDDoesNotClearSession(t *testing.T) {
	p := &Player{}
	id := 12345
	p.SessionID = &id

	if !p.ValidateSessionID(12345) {
		t.Fatal("expected session id to validate")
	}

	if p.SessionID == nil {
		t.Fatal("expected session id to remain after validation")
	}

	p.ClearSessionID()
	if p.SessionID != nil {
		t.Fatal("expected session id to clear")
	}
}

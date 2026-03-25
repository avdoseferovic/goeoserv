package account

import "testing"

func TestHashAndValidatePassword(t *testing.T) {
	hash := HashPassword("testuser", "testpass")
	if hash == "" {
		t.Fatal("hash should not be empty")
	}

	if !ValidatePassword("testuser", "testpass", hash) {
		t.Fatal("password should validate")
	}

	if ValidatePassword("testuser", "wrongpass", hash) {
		t.Fatal("wrong password should not validate")
	}

	if ValidatePassword("wronguser", "testpass", hash) {
		t.Fatal("wrong username should not validate")
	}
}

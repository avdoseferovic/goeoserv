package account

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword generates an argon2id hash of username+password.
func HashPassword(username, password string) string {
	input := []byte(strings.ToLower(username) + password)
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		panic(fmt.Sprintf("failed to generate salt: %v", err))
	}

	hash := argon2.IDKey(input, salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads, saltB64, hashB64)
}

// ValidatePassword checks a password against an argon2id hash string.
func ValidatePassword(username, password, encodedHash string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false
	}

	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	input := []byte(strings.ToLower(username) + password)
	hash := argon2.IDKey(input, salt, time, memory, threads, uint32(len(expectedHash)))

	if len(hash) != len(expectedHash) {
		return false
	}

	// Constant-time comparison
	var diff byte
	for i := range hash {
		diff |= hash[i] ^ expectedHash[i]
	}
	return diff == 0
}

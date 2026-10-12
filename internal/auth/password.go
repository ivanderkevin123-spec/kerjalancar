// Package auth owns users, password hashing, and credential checks.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params are the Argon2id cost parameters for new hashes. Existing hashes
// carry their own parameters in the PHC string, so these can be raised later
// without resetting anyone's password.
type Params struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
}

// DefaultParams is the production setting (64 MiB, 3 passes, 2 lanes).
var DefaultParams = Params{MemoryKiB: 64 * 1024, Time: 3, Threads: 2}

const (
	argonSaltLen   = 16
	argonKeyLen    = 32
	maxPasswordLen = 1024
)

var errMalformedHash = errors.New("auth: malformed password hash")

// HashPassword returns a PHC-formatted Argon2id hash using DefaultParams.
func HashPassword(password string) (string, error) {
	return HashPasswordWith(DefaultParams, password)
}

// HashPasswordWith returns a PHC-formatted Argon2id hash with a fresh random salt.
func HashPasswordWith(p Params, password string) (string, error) {
	if p.MemoryKiB == 0 || p.Time == 0 || p.Threads == 0 {
		return "", errors.New("auth: argon2 parameters must be positive")
	}
	if password == "" || len(password) > maxPasswordLen {
		return "", errors.New("auth: password must be between 1 and 1024 bytes")
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Threads, argonKeyLen)
	enc := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.MemoryKiB, p.Time, p.Threads, enc(salt), enc(key)), nil
}

// VerifyPassword reports whether password matches the PHC hash. The error is
// non-nil only when the hash itself cannot be parsed.
func VerifyPassword(password, hash string) (bool, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, errMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errMalformedHash
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false, errMalformedHash
	}
	if memory == 0 || iterations == 0 || threads == 0 {
		return false, errMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errMalformedHash
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

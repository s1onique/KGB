package config

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
)

var (
	ErrEmptyPassword = errors.New("password cannot be empty")
	ErrSaltLength    = errors.New("salt must be 16 bytes")
)

// GenerateSalt creates a random 16-byte salt for password hashing.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// HashPassword creates a sha256:<salt>:<hex> hash from a plaintext password.
// The salt should be 16 bytes.
func HashPassword(password string, salt []byte) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}
	if len(salt) != 16 {
		return "", ErrSaltLength
	}

	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(password))
	hash := h.Sum(nil)

	return "sha256:" + hex.EncodeToString(salt) + ":" + hex.EncodeToString(hash), nil
}

// VerifyPassword checks a plaintext password against a sha256:<salt>:<hex> hash.
// Uses constant-time comparison to prevent timing attacks.
func VerifyPassword(password, storedHash string) (bool, error) {
	if password == "" {
		return false, ErrEmptyPassword
	}
	if storedHash == "" {
		return false, ErrEmptyPasswordFormat
	}

	parts := strings.SplitN(storedHash, ":", 4)
	if len(parts) != 3 || parts[0] != "sha256" {
		return false, ErrInvalidPasswordFormat
	}

	saltHex := parts[1]
	hashHex := parts[2]

	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false, ErrInvalidPasswordFormat
	}

	storedHashBytes, err := hex.DecodeString(hashHex)
	if err != nil {
		return false, ErrInvalidPasswordFormat
	}

	// Compute expected hash
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(password))
	expectedHash := h.Sum(nil)

	// Constant-time comparison
	return subtle.ConstantTimeCompare(expectedHash, storedHashBytes) == 1, nil
}

package config

import (
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	password := "test-password-123"
	salt := []byte("1234567890abcdef") // 16 bytes

	hash, err := HashPassword(password, salt)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// Verify format
	if hash[:7] != "sha256:" {
		t.Errorf("Hash should start with 'sha256:', got %s", hash[:7])
	}

	// Verify password
	ok, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if !ok {
		t.Error("VerifyPassword should return true for correct password")
	}

	// Verify wrong password
	ok, err = VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if ok {
		t.Error("VerifyPassword should return false for wrong password")
	}
}

func TestVerifyPassword_InvalidFormat(t *testing.T) {
	tests := []string{
		"",
		"invalid",
		"sha256:abc",
		"sha256:zzz:yyy", // invalid hex
	}

	for _, hash := range tests {
		_, err := VerifyPassword("password", hash)
		if err == nil {
			t.Errorf("Expected error for invalid hash %q", hash)
		}
	}
}

func TestVerifyPassword_EmptyPassword(t *testing.T) {
	_, err := VerifyPassword("", "sha256:abc:def123456")
	if err != ErrEmptyPassword {
		t.Errorf("Expected ErrEmptyPassword, got %v", err)
	}
}

func TestGenerateSalt(t *testing.T) {
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}
	if len(salt) != 16 {
		t.Errorf("Salt should be 16 bytes, got %d", len(salt))
	}

	// Generate another salt and ensure they're different
	salt2, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}
	if string(salt) == string(salt2) {
		t.Error("Two generated salts should be different")
	}
}

func TestHashPassword_InvalidSalt(t *testing.T) {
	_, err := HashPassword("password", []byte("short"))
	if err != ErrSaltLength {
		t.Errorf("Expected ErrSaltLength, got %v", err)
	}
}

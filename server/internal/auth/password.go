package auth

import (
	"crypto/sha256"
	"encoding/base64"

	"golang.org/x/crypto/bcrypt"
)

// hashPassword bcrypt-hashes the password. bcrypt only looks at the first 72
// bytes, so we SHA-256 + base64 first: that collapses any-length input to a
// fixed 44-byte, NUL-free string, so passwords up to the API's 256-char limit
// are fully covered (without pre-hashing, a long password would be silently
// truncated and bcrypt would reject inputs over 72 bytes).
func hashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword(prehash(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// comparePassword reports whether password matches the stored bcrypt hash.
func comparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), prehash(password))
}

func prehash(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	return []byte(base64.StdEncoding.EncodeToString(sum[:]))
}

package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// sessionTTL is the lifetime of the access token / jp_session cookie. Single
// access token for v1 (no refresh token yet — docs/API.md §2).
const sessionTTL = 7 * 24 * time.Hour

// issueToken signs an HS256 JWT whose subject is the user id.
func issueToken(secret, userID string, now time.Time) (string, time.Time, error) {
	exp := now.Add(sessionTTL)
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, exp, nil
}

// parseToken verifies an HS256 JWT and returns its subject (the user id).
func parseToken(secret, raw string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		// Pin the algorithm to HMAC: without this check an attacker could swap
		// alg to "none" or to an asymmetric scheme and bypass verification.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %q", t.Method.Alg())
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", fmt.Errorf("auth: parse token: %w", err)
	}
	if !token.Valid {
		return "", fmt.Errorf("auth: invalid token")
	}
	return claims.Subject, nil
}

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

// parseToken verifies an HS256 JWT and returns its subject (the user id) plus its
// expiry. The expiry is surfaced so the WS layer can arm a close at session end (a
// long-lived socket outlives the request that authenticated it — nothing else
// re-validates the cookie mid-connection; docs/DESIGN-PHASE3-LIVE.md §3.4). A token
// with no exp claim yields the zero time (never issued by issueToken).
func parseToken(secret, raw string) (string, time.Time, error) {
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
		return "", time.Time{}, fmt.Errorf("auth: parse token: %w", err)
	}
	if !token.Valid {
		return "", time.Time{}, fmt.Errorf("auth: invalid token")
	}
	var exp time.Time
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Time
	}
	return claims.Subject, exp, nil
}

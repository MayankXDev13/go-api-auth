package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessClaims is the JWT payload for access tokens. TokenVersion lets a
// user's outstanding tokens be invalidated by bumping the stored version.
type AccessClaims struct {
	TokenVersion int `json:"ver"`
	jwt.RegisteredClaims
}

func NewAccessToken(secret []byte, userID uuid.UUID, tokenVersion int, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := AccessClaims{
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// ParseAccessToken validates the token and returns the user ID and the
// token version it was issued with.
func ParseAccessToken(secret []byte, tok string) (uuid.UUID, int, error) {
	parsed, err := jwt.ParseWithClaims(tok, &AccessClaims{}, func(_ *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		return uuid.Nil, 0, err
	}
	claims, ok := parsed.Claims.(*AccessClaims)
	if !ok || !parsed.Valid {
		return uuid.Nil, 0, jwt.ErrTokenInvalidClaims
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, 0, err
	}
	return id, claims.TokenVersion, nil
}

func NewOpaqueToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return raw, hash, nil
}

func HashOpaque(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ctxKey string

const userIDKey ctxKey = "userID"

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}

// RequireAuth returns middleware that validates an access token signed with
// secret and stores the caller's user ID on the request context. The token is
// read from the Authorization header, or from the access_token cookie for
// browser clients. A token is rejected when its version is older than the
// user's stored version, which is how logout-all and password changes revoke
// outstanding access tokens.
func RequireAuth(db *gorm.DB, secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := accessTokenFromRequest(r)
			if tok == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			id, ver, err := ParseAccessToken(secret, tok)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			var u User
			if err := db.Select("id", "token_version").Where("id = ?", id).First(&u).Error; err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if u.TokenVersion != ver {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey, id)))
		})
	}
}

// accessTokenFromRequest prefers the bearer header and falls back to the
// access_token cookie.
func accessTokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie("access_token"); err == nil {
		return c.Value
	}
	return ""
}

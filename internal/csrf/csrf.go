// Package csrf adds double-submit CSRF protection for requests that
// authenticate with cookies. Bearer-token requests are exempt because a
// browser does not attach the Authorization header on its own.
package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"

	"go-api-auth/internal/httpx"
)

const (
	// HeaderName is the request header the client echoes the token in.
	HeaderName = "X-CSRF-Token"
	// CookieName is the readable cookie holding the token.
	CookieName = "csrf_token"
)

// Token returns a new random CSRF token.
func Token() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Cookie builds the CSRF cookie. It is deliberately not HttpOnly so the
// client can read it and send it back in the header.
func Cookie(value string, maxAge time.Duration, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   int(maxAge.Seconds()),
	}
}

// Middleware rejects unsafe requests that carry a session cookie but omit or
// mismatch the CSRF header. Safe methods and bearer-token requests pass
// through.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) || r.Header.Get("Authorization") != "" || !hasSessionCookie(r) {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			httpx.Error(w, http.StatusForbidden, "csrf token missing")
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderName)), []byte(c.Value)) != 1 {
			httpx.Error(w, http.StatusForbidden, "csrf token invalid")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// hasSessionCookie reports whether the request carries a cookie that the
// server would authenticate. Names match the auth package's cookies.
func hasSessionCookie(r *http.Request) bool {
	for _, name := range []string{"access_token", "refresh_token"} {
		if c, err := r.Cookie(name); err == nil && c.Value != "" {
			return true
		}
	}
	return false
}

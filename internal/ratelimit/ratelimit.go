// Package ratelimit provides request throttling. Limiter is small enough
// that a distributed implementation can replace the in-memory one without
// changing handlers or middleware.
package ratelimit

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-api-auth/internal/httpx"
)

// Limiter reports whether a request keyed by key is allowed right now.
type Limiter interface {
	Allow(key string) bool
}

// Memory is a per-key token bucket kept in process memory. It is intended
// for single-instance deployments.
type Memory struct {
	rate  float64 // tokens added per second
	burst float64 // bucket capacity

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// NewMemory returns a limiter allowing up to perMinute requests per minute
// per key, with a burst equal to perMinute.
func NewMemory(perMinute int) *Memory {
	return &Memory{
		rate:    float64(perMinute) / 60.0,
		burst:   float64(perMinute),
		buckets: make(map[string]*bucket),
	}
}

// Allow consumes a token for key, reporting whether the request may proceed.
func (m *Memory) Allow(key string) bool {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.buckets[key]
	if !ok {
		b = &bucket{tokens: m.burst, lastSeen: now}
		m.buckets[key] = b
	}
	b.tokens = min(m.burst, b.tokens+now.Sub(b.lastSeen).Seconds()*m.rate)
	b.lastSeen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// RetryAfter is a hint for the Retry-After header.
func (m *Memory) RetryAfter() time.Duration {
	if m.rate <= 0 {
		return time.Minute
	}
	return time.Duration(float64(time.Second) / m.rate)
}

// Middleware rate limits each request by client IP and, when the JSON body
// carries an email, by that email as well.
func Middleware(l Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keys := []string{"ip:" + clientIP(r)}
			if email := peekEmail(r); email != "" {
				keys = append(keys, "email:"+email)
			}
			for _, key := range keys {
				if !l.Allow(key) {
					w.Header().Set("Retry-After", retryAfter(l))
					httpx.Error(w, http.StatusTooManyRequests, "too many requests")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func retryAfter(l Limiter) string {
	if r, ok := l.(interface{ RetryAfter() time.Duration }); ok {
		secs := int(r.RetryAfter().Seconds())
		if secs < 1 {
			secs = 1
		}
		return strconv.Itoa(secs)
	}
	return "60"
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// peekEmail reads (and restores) the request body to find an email field.
func peekEmail(r *http.Request) string {
	if r.Body == nil || !strings.Contains(r.Header.Get("Content-Type"), "json") {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return ""
	}
	var probe struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(body, &probe) != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(probe.Email))
}

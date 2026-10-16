package server

import (
	"encoding/json"
	"net/http"
	"time"

	"go-api-auth/internal/auth"
	"go-api-auth/internal/config"
	"go-api-auth/internal/cors"
	"go-api-auth/internal/mailer"
	"go-api-auth/internal/todos"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"
)

// NewRouter assembles the full app: middleware, healthz, and the auth and
// todos route groups, all on one root router. Each package registers its own
// routes via Register so there is no Mount("/") collision.
func NewRouter(db *gorm.DB, cfg config.Config, mail mailer.Mailer) http.Handler {
	r := newBaseRouter()
	r.Use(cors.Middleware(cfg.CORSOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	auth.Register(r, db, cfg, mail)
	todos.Register(r, db, cfg)
	return r
}

// newBaseRouter applies the cross-cutting middleware: request IDs, real IP
// extraction, request logging, panic recovery and a request timeout.
func newBaseRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestIDHeader)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	return r
}

// requestIDHeader echoes the request ID assigned by chi's RequestID
// middleware back to the client.
func requestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		next.ServeHTTP(w, r)
	})
}

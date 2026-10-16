package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
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
// routes via Register so there is no Mount("/") collision. The logger carries
// through to request logging and the handler packages.
func NewRouter(db *gorm.DB, cfg config.Config, mail mailer.Mailer, log *slog.Logger) http.Handler {
	r := newBaseRouter(log)
	r.Use(cors.Middleware(cfg.CORSOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	auth.Register(r, db, cfg, mail, log)
	todos.Register(r, db, cfg, log)
	return r
}

// newBaseRouter applies the cross-cutting middleware: request IDs, real IP
// extraction, structured request logging, panic recovery and a request
// timeout.
func newBaseRouter(log *slog.Logger) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestIDHeader)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(log))
	r.Use(recoverer(log))
	r.Use(middleware.Timeout(60 * time.Second))
	return r
}

// requestLogger writes one structured record per request with the method,
// path, status, size and duration. Server errors log at Error, the rest at
// Info.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			defer func() {
				attrs := []any{
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"duration_ms", time.Since(start).Milliseconds(),
					"request_id", middleware.GetReqID(r.Context()),
					"remote_addr", r.RemoteAddr,
				}
				if ww.Status() >= http.StatusInternalServerError {
					log.Error("request", attrs...)
					return
				}
				log.Info("request", attrs...)
			}()
			next.ServeHTTP(ww, r)
		})
	}
}

// recoverer turns a panic into a 500 instead of crashing the process, logging
// the panic and stack trace through the structured logger.
func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rvr := recover()
				if rvr == nil {
					return
				}
				if rvr == http.ErrAbortHandler {
					panic(rvr)
				}
				log.Error("panic recovered",
					"panic", rvr,
					"path", r.URL.Path,
					"request_id", middleware.GetReqID(r.Context()),
					"stack", string(debug.Stack()),
				)
				w.WriteHeader(http.StatusInternalServerError)
			}()
			next.ServeHTTP(w, r)
		})
	}
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

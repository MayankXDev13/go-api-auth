package server

import (
	"encoding/json"
	"net/http"

	"go-api-auth/internal/auth"
	"go-api-auth/internal/config"
	"go-api-auth/internal/mailer"
	"go-api-auth/internal/todos"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// NewRouter assembles the full app: healthz plus the auth and todos
// route groups, all on one root router. Each package registers its own
// routes via Register so there is no Mount("/") collision.
func NewRouter(db *gorm.DB, cfg config.Config, mail mailer.Mailer) http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	auth.Register(r, db, cfg, mail)
	todos.Register(r, db, cfg)
	return r
}

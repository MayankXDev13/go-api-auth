package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-api-auth/internal/auth"
	"go-api-auth/internal/todos"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&auth.User{}, &auth.RefreshToken{}, &auth.PasswordResetToken{}, &todos.Todo{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return db
}

func TestHealthz(t *testing.T) {
	r := NewRouter(newTestDB(t))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", body)
	}
}

// The whole point of the fix: auth and todos live on one root router
// without a Mount("/") panic, and both are reachable.
func TestComposedRouter_ReachesAuthAndTodos(t *testing.T) {
	r := NewRouter(newTestDB(t))

	// Auth is public: an empty body is a 400 from the handler, not a
	// 404 (route missing) and not a 401 (todos RequireAuth leaked onto it).
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("auth register should reach handler (400), got %d", rec.Code)
	}

	// Todos are gated by RequireAuth on the composed router.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/todos", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("todos route should require auth (401), got %d", rec.Code)
	}
}

// Auth's own protected routes still require a bearer token on the
// composed router, and a tokenless request must not fall through to todos.
func TestComposedRouter_AuthProtectedRoutes(t *testing.T) {
	r := NewRouter(newTestDB(t))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth/me should require auth (401), got %d", rec.Code)
	}
}

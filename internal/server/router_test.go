package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-api-auth/internal/config"
	"go-api-auth/internal/mailer"
	"go-api-auth/internal/testutil"

	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	testutil.Main(m)
}

func testCfg() config.Config {
	c := config.Default()
	c.JWTSecret = "test-secret-please-change-32bytes!!"
	c.BcryptCost = 4
	return c
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.PostgresDB(t)
}

func TestHealthz(t *testing.T) {
	r := NewRouter(newTestDB(t), testCfg(), mailer.Console{})

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
	r := NewRouter(newTestDB(t), testCfg(), mailer.Console{})

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
	r := NewRouter(newTestDB(t), testCfg(), mailer.Console{})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth/me should require auth (401), got %d", rec.Code)
	}
}

func TestCORS(t *testing.T) {
	cfg := testCfg()
	cfg.CORSOrigins = []string{"https://app.example.com"}
	r := NewRouter(newTestDB(t), cfg, mailer.Console{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("allowed origin header = %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin must not get CORS header, got %q", got)
	}

	req = httptest.NewRequest(http.MethodOptions, "/api/v1/todos", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight should 204, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("preflight missing allowed methods")
	}
}

func TestMiddleware_RequestIDAndRecover(t *testing.T) {
	r := newBaseRouter()
	r.Get("/boom", func(http.ResponseWriter, *http.Request) { panic("boom") })
	r.Get("/ok", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	req.Header.Set("X-Request-Id", "req-123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic should be recovered as 500, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "req-123" {
		t.Fatalf("request id should be echoed, got %q", got)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("server should stay up after a panic, got %d", rec.Code)
	}
}

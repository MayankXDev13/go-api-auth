package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-api-auth/internal/config"
	"go-api-auth/internal/logger"
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
	r := NewRouter(newTestDB(t), testCfg(), mailer.Console{}, logger.Discard())

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
	r := NewRouter(newTestDB(t), testCfg(), mailer.Console{}, logger.Discard())

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
	r := NewRouter(newTestDB(t), testCfg(), mailer.Console{}, logger.Discard())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth/me should require auth (401), got %d", rec.Code)
	}
}

func TestCORS(t *testing.T) {
	cfg := testCfg()
	cfg.CORSOrigins = []string{"https://app.example.com"}
	r := NewRouter(newTestDB(t), cfg, mailer.Console{}, logger.Discard())

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
	r := newBaseRouter(logger.Discard())
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

func TestMiddleware_RequestLogged(t *testing.T) {
	var buf bytes.Buffer
	r := newBaseRouter(slog.New(slog.NewJSONHandler(&buf, nil)))
	r.Get("/logged", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/logged", nil))

	out := buf.String()
	if !strings.Contains(out, `"path":"/logged"`) || !strings.Contains(out, `"status":200`) {
		t.Fatalf("request not logged: %q", out)
	}
}

func jsonReq(method, path string, body any, cookies []*http.Cookie, headers map[string]string) *http.Request {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func cookieNamed(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// loginAs registers and logs in a user, returning the access token from the
// body and the cookies set on the login response.
func loginAs(t *testing.T, h http.Handler, email string) (string, []*http.Cookie) {
	t.Helper()
	register := httptest.NewRecorder()
	h.ServeHTTP(register, jsonReq(http.MethodPost, "/api/v1/auth/register",
		map[string]string{"name": "Ada", "email": email, "password": "Secret123!"}, nil, nil))
	if register.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", register.Code, register.Body.String())
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq(http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": email, "password": "Secret123!"}, nil, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	access, _ := resp["access_token"].(string)
	if access == "" {
		t.Fatal("login returned no access token")
	}
	return access, rec.Result().Cookies()
}

func TestLogin_SetsAccessAndRefreshCookies(t *testing.T) {
	h := NewRouter(newTestDB(t), testCfg(), mailer.Console{}, logger.Discard())
	_, cookies := loginAs(t, h, "cookie@example.com")

	access := cookieNamed(cookies, "access_token")
	if access == nil || access.Value == "" || !access.HttpOnly {
		t.Fatalf("access_token cookie missing or not HttpOnly: %+v", access)
	}
	refresh := cookieNamed(cookies, "refresh_token")
	if refresh == nil || refresh.Value == "" || !refresh.HttpOnly {
		t.Fatalf("refresh_token cookie missing or not HttpOnly: %+v", refresh)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq(http.MethodGet, "/api/v1/auth/me", nil, cookies, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie-authenticated /me = %d %s", rec.Code, rec.Body.String())
	}
}

func TestCookieAuth_TodosWork(t *testing.T) {
	h := NewRouter(newTestDB(t), testCfg(), mailer.Console{}, logger.Discard())
	_, cookies := loginAs(t, h, "todo-cookie@example.com")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq(http.MethodPost, "/api/v1/todos",
		map[string]string{"title": "cookie auth"}, cookies, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("cookie-authenticated create = %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq(http.MethodGet, "/api/v1/todos", nil, cookies, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie-authenticated list = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRefresh_FromCookie(t *testing.T) {
	h := NewRouter(newTestDB(t), testCfg(), mailer.Console{}, logger.Discard())
	_, cookies := loginAs(t, h, "refresh-cookie@example.com")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq(http.MethodPost, "/api/v1/auth/refresh", nil, cookies, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh from cookie should 200, got %d %s", rec.Code, rec.Body.String())
	}
	if cookieNamed(rec.Result().Cookies(), "access_token") == nil {
		t.Fatal("refresh should set a new access_token cookie")
	}
}

func TestBearer_StillWorks(t *testing.T) {
	h := NewRouter(newTestDB(t), testCfg(), mailer.Console{}, logger.Discard())
	access, _ := loginAs(t, h, "bearer@example.com")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, jsonReq(http.MethodPost, "/api/v1/todos",
		map[string]string{"title": "bearer"}, nil,
		map[string]string{"Authorization": "Bearer " + access}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("bearer create = %d %s", rec.Code, rec.Body.String())
	}
}

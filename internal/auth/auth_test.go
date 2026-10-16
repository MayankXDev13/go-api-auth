package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-api-auth/internal/auth"
)

func doReq(t *testing.T, h http.Handler, method, path string, payload any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func registerUser(t *testing.T, h http.Handler, name, email, pass string) {
	t.Helper()
	rec := doReq(t, h, http.MethodPost, "/api/v1/auth/register",
		map[string]string{"name": name, "email": email, "password": pass}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d %s", rec.Code, rec.Body.String())
	}
}

func loginUser(t *testing.T, h http.Handler, email, pass string) (access, refresh string) {
	t.Helper()
	rec := doReq(t, h, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": email, "password": pass}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	access, _ = resp["access_token"].(string)
	refresh, _ = resp["refresh_token"].(string)
	if access == "" || refresh == "" {
		t.Fatalf("missing tokens in %v", resp)
	}
	return access, refresh
}

func TestRegister_Duplicate409(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	registerUser(t, r, "Ada", "dup@example.com", "Secret123!")

	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/register",
		map[string]string{"name": "Ada2", "email": "dup@example.com", "password": "Secret123!"}, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestRegister_Invalid400(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	cases := []map[string]string{
		{"name": "", "email": "a@example.com", "password": "Secret123!"},
		{"name": "A", "email": "not-an-email", "password": "Secret123!"},
		{"name": "A", "email": "a@example.com", "password": "short"},
	}
	for i, c := range cases {
		rec := doReq(t, r, http.MethodPost, "/api/v1/auth/register", c, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("case %d: expected 400, got %d %s", i, rec.Code, rec.Body.String())
		}
	}
}

func TestRegister_DBError500(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	_ = sqlDB.Close()

	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/register",
		map[string]string{"name": "Ada", "email": "err@example.com", "password": "Secret123!"}, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on db failure, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_SuccessAndFailures(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	registerUser(t, r, "Ada", "login@example.com", "Secret123!")

	access, refresh := loginUser(t, r, "login@example.com", "Secret123!")
	if access == "" || refresh == "" {
		t.Fatal("expected tokens")
	}

	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "login@example.com", "password": "Wrong123!"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: expected 401, got %d", rec.Code)
	}

	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "nobody@example.com", "password": "Secret123!"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown user: expected 401, got %d", rec.Code)
	}
}

func TestMe_Auth(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	registerUser(t, r, "Ada", "me@example.com", "Secret123!")
	access, _ := loginUser(t, r, "me@example.com", "Secret123!")

	rec := doReq(t, r, http.MethodGet, "/api/v1/auth/me", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: expected 401, got %d", rec.Code)
	}

	rec = doReq(t, r, http.MethodGet, "/api/v1/auth/me", nil,
		map[string]string{"Authorization": "Bearer " + access})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["email"] != "me@example.com" {
		t.Fatalf("wrong user %v", resp)
	}
}

func TestRefresh_RotatesAndOldDies(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	registerUser(t, r, "Ada", "ref@example.com", "Secret123!")
	_, refresh := loginUser(t, r, "ref@example.com", "Secret123!")

	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refresh}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	newRefresh, _ := resp["refresh_token"].(string)
	if newRefresh == "" || newRefresh == refresh {
		t.Fatalf("expected rotated token, got %v", resp)
	}

	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refresh}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old token reuse: expected 401, got %d", rec.Code)
	}

	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": "bogus"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bogus: expected 401, got %d", rec.Code)
	}
}

func TestRefresh_ReuseRevokesFamily(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	registerUser(t, r, "Ada", "fam@example.com", "Secret123!")
	_, refresh1 := loginUser(t, r, "fam@example.com", "Secret123!")

	// Rotate once: refresh1 -> refresh2 (same family).
	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refresh1}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	refresh2, _ := resp["refresh_token"].(string)
	if refresh2 == "" || refresh2 == refresh1 {
		t.Fatalf("expected rotated token, got %v", resp)
	}

	// Replay the old token: rejected.
	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refresh1}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("replayed token should 401, got %d", rec.Code)
	}

	// The whole family is now revoked, including the newest token.
	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refresh2}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("family should be revoked, refresh2 got %d", rec.Code)
	}
}

func TestLogout_Revokes(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	registerUser(t, r, "Ada", "out@example.com", "Secret123!")
	_, refresh := loginUser(t, r, "out@example.com", "Secret123!")

	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/logout",
		map[string]string{"refresh_token": refresh}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout failed: %d %s", rec.Code, rec.Body.String())
	}

	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refresh}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("after logout refresh should 401, got %d", rec.Code)
	}
}

func TestChangePassword(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	registerUser(t, r, "Ada", "chg@example.com", "Secret123!")
	access, _ := loginUser(t, r, "chg@example.com", "Secret123!")
	h := map[string]string{"Authorization": "Bearer " + access}

	rec := doReq(t, r, http.MethodPut, "/api/v1/auth/change-password",
		map[string]string{"current_password": "Secret123!", "new_password": "NewSecret123!"}, h)
	if rec.Code != http.StatusOK {
		t.Fatalf("change failed: %d %s", rec.Code, rec.Body.String())
	}

	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "chg@example.com", "password": "NewSecret123!"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login with new should work: %d %s", rec.Code, rec.Body.String())
	}

	rec = doReq(t, r, http.MethodPut, "/api/v1/auth/change-password",
		map[string]string{"current_password": "wrong", "new_password": "Another123!"}, h)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current: expected 401, got %d", rec.Code)
	}

	// The old access token is now stale.
	rec = doReq(t, r, http.MethodGet, "/api/v1/auth/me", nil, h)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old access token should 401 after change, got %d", rec.Code)
	}
}

func TestLogout_KeepsOtherSession(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	registerUser(t, r, "Ada", "other@example.com", "Secret123!")
	access1, refresh1 := loginUser(t, r, "other@example.com", "Secret123!")
	access2, refresh2 := loginUser(t, r, "other@example.com", "Secret123!")

	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/logout",
		map[string]string{"refresh_token": refresh1}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: %d %s", rec.Code, rec.Body.String())
	}

	// Logged-out session's refresh is dead.
	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refresh1}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh1 should 401, got %d", rec.Code)
	}

	// Other session (access and refresh) still works.
	rec = doReq(t, r, http.MethodGet, "/api/v1/auth/me", nil,
		map[string]string{"Authorization": "Bearer " + access2})
	if rec.Code != http.StatusOK {
		t.Fatalf("access2 should stay valid, got %d", rec.Code)
	}
	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refresh2}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh2 should stay valid, got %d", rec.Code)
	}
	_ = access1
}

func TestLogoutAll_RevokesEverything(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	registerUser(t, r, "Ada", "all@example.com", "Secret123!")
	access1, refresh1 := loginUser(t, r, "all@example.com", "Secret123!")
	access2, refresh2 := loginUser(t, r, "all@example.com", "Secret123!")

	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/logout-all", nil,
		map[string]string{"Authorization": "Bearer " + access1})
	if rec.Code != http.StatusOK {
		t.Fatalf("logout-all: %d %s", rec.Code, rec.Body.String())
	}

	for _, access := range []string{access1, access2} {
		rec = doReq(t, r, http.MethodGet, "/api/v1/auth/me", nil,
			map[string]string{"Authorization": "Bearer " + access})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("access token should be revoked, got %d", rec.Code)
		}
	}
	for _, refresh := range []string{refresh1, refresh2} {
		rec = doReq(t, r, http.MethodPost, "/api/v1/auth/refresh",
			map[string]string{"refresh_token": refresh}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("refresh token should be revoked, got %d", rec.Code)
		}
	}
}

func TestReset_RevokesAccessTokens(t *testing.T) {
	db := setupTestDB(t)
	mail := &recordingMailer{}
	r := auth.NewRouter(db, testCfg(), mail)
	registerUser(t, r, "Ada", "rst@example.com", "Secret123!")
	access, _ := loginUser(t, r, "rst@example.com", "Secret123!")

	doReq(t, r, http.MethodPost, "/api/v1/auth/forgot-password",
		map[string]string{"email": "rst@example.com"}, nil)
	if len(mail.msgs) != 1 {
		t.Fatalf("expected reset email, got %d", len(mail.msgs))
	}
	raw := tokenFromBody(t, mail.msgs[0].Body)

	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/reset-password",
		map[string]string{"token": raw, "new_password": "NewSecret123!"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}

	rec = doReq(t, r, http.MethodGet, "/api/v1/auth/me", nil,
		map[string]string{"Authorization": "Bearer " + access})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("access token should be revoked after reset, got %d", rec.Code)
	}
}

func TestRateLimit_Returns429(t *testing.T) {
	db := setupTestDB(t)
	cfg := testCfg()
	cfg.RateLimitPerMin = 3
	r := auth.NewRouter(db, cfg, &recordingMailer{})

	var last *httptest.ResponseRecorder
	for i := 0; i < 4; i++ {
		last = doReq(t, r, http.MethodPost, "/api/v1/auth/login",
			map[string]string{"email": "flood@example.com", "password": "nope"}, nil)
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after limit, got %d %s", last.Code, last.Body.String())
	}
	if last.Header().Get("Retry-After") == "" {
		t.Fatal("expected a Retry-After header")
	}
}

func TestPasswordTooLong_Rejected(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg(), &recordingMailer{})
	long := strings.Repeat("a", 73)

	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/register",
		map[string]string{"name": "Ada", "email": "long@example.com", "password": long}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("over-long register password should 400, got %d", rec.Code)
	}

	registerUser(t, r, "Ada", "len@example.com", "Secret123!")
	access, _ := loginUser(t, r, "len@example.com", "Secret123!")
	rec = doReq(t, r, http.MethodPut, "/api/v1/auth/change-password",
		map[string]string{"current_password": "Secret123!", "new_password": long},
		map[string]string{"Authorization": "Bearer " + access})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("over-long change password should 400, got %d", rec.Code)
	}
}

func TestRefreshCookie_SecureFollowsEnv(t *testing.T) {
	db := setupTestDB(t)

	dev := testCfg()
	dev.AppEnv = "development"
	rDev := auth.NewRouter(db, dev, &recordingMailer{})
	registerUser(t, rDev, "Ada", "dev@example.com", "Secret123!")
	rec := doReq(t, rDev, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "dev@example.com", "password": "Secret123!"}, nil)
	if c := refreshCookie(t, rec); c.Secure {
		t.Fatal("development cookie must not be Secure")
	}

	prod := testCfg()
	prod.AppEnv = "production"
	rProd := auth.NewRouter(db, prod, &recordingMailer{})
	registerUser(t, rProd, "Ada", "prod@example.com", "Secret123!")
	rec = doReq(t, rProd, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "prod@example.com", "password": "Secret123!"}, nil)
	c := refreshCookie(t, rec)
	if !c.Secure {
		t.Fatal("production cookie must be Secure")
	}
	if !c.HttpOnly {
		t.Fatal("cookie must be HttpOnly")
	}
}

func refreshCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			return c
		}
	}
	t.Fatal("no refresh_token cookie set")
	return nil
}

func TestForgotReset_Flow(t *testing.T) {
	db := setupTestDB(t)
	mail := &recordingMailer{}
	r := auth.NewRouter(db, testCfg(), mail)
	registerUser(t, r, "Ada", "fp@example.com", "Secret123!")

	// Unknown email still 200 (no enumeration) and sends nothing.
	rec := doReq(t, r, http.MethodPost, "/api/v1/auth/forgot-password",
		map[string]string{"email": "ghost@example.com"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("unknown forgot should 200, got %d", rec.Code)
	}
	if len(mail.msgs) != 0 {
		t.Fatalf("expected no mail for unknown email, got %d", len(mail.msgs))
	}

	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/forgot-password",
		map[string]string{"email": "fp@example.com"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("forgot failed: %d %s", rec.Code, rec.Body.String())
	}
	var count int64
	db.Model(&auth.PasswordResetToken{}).Count(&count)
	if count == 0 {
		t.Fatal("expected a reset row in db")
	}
	if len(mail.msgs) != 1 {
		t.Fatalf("expected 1 reset email, got %d", len(mail.msgs))
	}
	if mail.msgs[0].To != "fp@example.com" {
		t.Fatalf("wrong recipient %q", mail.msgs[0].To)
	}
	raw := tokenFromBody(t, mail.msgs[0].Body)
	if raw == "" {
		t.Fatal("expected a reset token in the email body")
	}

	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/reset-password",
		map[string]string{"token": "invalid", "new_password": "NewSecret123!"}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid token should 400, got %d", rec.Code)
	}

	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/reset-password",
		map[string]string{"token": raw, "new_password": "short"}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short password should 400, got %d", rec.Code)
	}

	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/reset-password",
		map[string]string{"token": raw, "new_password": "NewSecret123!"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset failed: %d %s", rec.Code, rec.Body.String())
	}

	// Old password dead, new works.
	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "fp@example.com", "password": "Secret123!"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old password should 401, got %d", rec.Code)
	}
	loginUser(t, r, "fp@example.com", "NewSecret123!")

	// Single use.
	rec = doReq(t, r, http.MethodPost, "/api/v1/auth/reset-password",
		map[string]string{"token": raw, "new_password": "Another123!"}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("reuse should 400, got %d", rec.Code)
	}
}

func tokenFromBody(t *testing.T, body string) string {
	t.Helper()
	const marker = "token="
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	tok := body[i+len(marker):]
	if j := strings.IndexAny(tok, "\r\n \t\""); j >= 0 {
		tok = tok[:j]
	}
	return tok
}

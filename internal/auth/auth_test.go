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

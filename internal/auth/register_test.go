package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-api-auth/internal/auth"
	"go-api-auth/internal/config"
	"go-api-auth/internal/testutil"

	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	testutil.Main(m)
}

func testCfg() config.Config {
	c := config.Default()
	c.JWTSecret = "test-secret-please-change-32bytes!!"
	return c
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.PostgresDB(t)
}

// First behavior — valid register returns 201 with public user fields.
func TestRegister_Returns201(t *testing.T) {
	db := setupTestDB(t)
	r := auth.NewRouter(db, testCfg())

	payload := map[string]string{
		"name":     "Ada",
		"email":    "ada@example.com",
		"password": "Secret123!",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["email"] != "ada@example.com" {
		t.Fatalf("expected email in response, got %v", resp)
	}
	if _, hasPassword := resp["password"]; hasPassword {
		t.Fatal("response must not leak password")
	}
	if _, hasHash := resp["password_hash"]; hasHash {
		t.Fatal("response must not leak password_hash")
	}
}

package todos_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-api-auth/internal/auth"
	"go-api-auth/internal/config"
	"go-api-auth/internal/todos"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testCfg() config.Config {
	c := config.Default()
	c.JWTSecret = "test-secret-please-change-32bytes!!"
	return c
}

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&auth.User{}, &auth.RefreshToken{}, &auth.PasswordResetToken{}, &todos.Todo{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Exec("DELETE FROM todos")
	db.Exec("DELETE FROM password_reset_tokens")
	db.Exec("DELETE FROM refresh_tokens")
	db.Exec("DELETE FROM users")
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return db
}

func makeUser(t *testing.T, db *gorm.DB, name, email string) (string, string) {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("Secret123!"), bcrypt.DefaultCost)
	u := auth.User{Name: name, Email: email, PasswordHash: string(hash)}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	access, err := auth.NewAccessToken([]byte(testCfg().JWTSecret), u.ID, testCfg().AccessTTL)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return u.ID.String(), access
}

func doReq(t *testing.T, h http.Handler, method, path string, payload any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTodo_Unauthorized(t *testing.T) {
	db := setupDB(t)
	r := todos.NewRouter(db, testCfg())
	rec := doReq(t, r, http.MethodGet, "/api/v1/todos", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestTodo_CRUD(t *testing.T) {
	db := setupDB(t)
	r := todos.NewRouter(db, testCfg())
	_, tok := makeUser(t, db, "Ada", "ada@example.com")

	// Create invalid.
	rec := doReq(t, r, http.MethodPost, "/api/v1/todos", map[string]string{"title": ""}, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty title should 400, got %d", rec.Code)
	}

	// Create.
	rec = doReq(t, r, http.MethodPost, "/api/v1/todos",
		map[string]string{"title": "Buy milk", "description": "oat"}, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&created)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("missing id %v", created)
	}

	// List.
	rec = doReq(t, r, http.MethodGet, "/api/v1/todos", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var list map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if data, ok := list["data"].([]any); !ok || len(data) != 1 {
		t.Fatalf("expected 1 todo, got %v", list)
	}

	// Get one.
	rec = doReq(t, r, http.MethodGet, "/api/v1/todos/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}

	// Update.
	rec = doReq(t, r, http.MethodPut, "/api/v1/todos/"+id,
		map[string]any{"title": "Buy oat milk", "completed": true}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated["title"] != "Buy oat milk" || updated["completed"] != true {
		t.Fatalf("bad update %v", updated)
	}

	// Filter completed=true finds it.
	rec = doReq(t, r, http.MethodGet, "/api/v1/todos?completed=true", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("filter: %d", rec.Code)
	}
	var filtered map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&filtered)
	if meta, _ := filtered["meta"].(map[string]any); meta["total"] == float64(0) {
		t.Fatalf("expected 1 completed, got %v", filtered)
	}

	// Delete.
	rec = doReq(t, r, http.MethodDelete, "/api/v1/todos/"+id, nil, tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	rec = doReq(t, r, http.MethodGet, "/api/v1/todos/"+id, nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("after delete should 404, got %d", rec.Code)
	}
}

func TestTodo_OwnershipIsolation(t *testing.T) {
	db := setupDB(t)
	r := todos.NewRouter(db, testCfg())
	_, tokA := makeUser(t, db, "A", "a@example.com")
	_, tokB := makeUser(t, db, "B", "b@example.com")

	rec := doReq(t, r, http.MethodPost, "/api/v1/todos",
		map[string]string{"title": "A secret"}, tokA)
	var created map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&created)
	id, _ := created["id"].(string)

	for _, tc := range []struct {
		method string
		path   string
		code   int
	}{
		{http.MethodGet, "/api/v1/todos/" + id, http.StatusNotFound},
		{http.MethodPut, "/api/v1/todos/" + id, http.StatusNotFound},
		{http.MethodDelete, "/api/v1/todos/" + id, http.StatusNotFound},
	} {
		payload := map[string]string{"title": "hijack"}
		if tc.method == http.MethodGet || tc.method == http.MethodDelete {
			payload = nil
		}
		rec = doReq(t, r, tc.method, tc.path, payload, tokB)
		if rec.Code != tc.code {
			t.Fatalf("%s %s: expected %d, got %d %s", tc.method, tc.path, tc.code, rec.Code, rec.Body.String())
		}
	}

	// B list is empty.
	rec = doReq(t, r, http.MethodGet, "/api/v1/todos", nil, tokB)
	var list map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if meta, _ := list["meta"].(map[string]any); meta["total"] != float64(0) {
		t.Fatalf("B should see 0, got %v", list)
	}
}

func TestTodo_CompleteToggle(t *testing.T) {
	db := setupDB(t)
	r := todos.NewRouter(db, testCfg())
	_, tok := makeUser(t, db, "Ada", "c@example.com")

	rec := doReq(t, r, http.MethodPost, "/api/v1/todos", map[string]string{"title": "task"}, tok)
	var created map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&created)
	id, _ := created["id"].(string)

	rec = doReq(t, r, http.MethodPatch, "/api/v1/todos/"+id+"/complete", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: %d %s", rec.Code, rec.Body.String())
	}
	var done map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&done)
	if done["completed"] != true {
		t.Fatalf("should be completed %v", done)
	}
}

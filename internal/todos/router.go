package todos

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-api-auth/internal/auth"
	"go-api-auth/internal/config"
	"go-api-auth/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler struct {
	db  *gorm.DB
	log *slog.Logger
}

func NewRouter(db *gorm.DB, cfg config.Config, log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	Register(r, db, cfg, log)
	return r
}

// Register mounts the todo routes onto r. The routes sit in their own
// Group so RequireAuth applies here without leaking onto auth routes
// when both are registered on the same root router.
func Register(r chi.Router, db *gorm.DB, cfg config.Config, log *slog.Logger) {
	h := &Handler{db: db, log: log}
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(db, []byte(cfg.JWTSecret)))
		r.Post("/api/v1/todos", h.create)
		r.Get("/api/v1/todos", h.list)
		r.Get("/api/v1/todos/{id}", h.get)
		r.Put("/api/v1/todos/{id}", h.update)
		r.Delete("/api/v1/todos/{id}", h.remove)
		r.Patch("/api/v1/todos/{id}/complete", h.complete)
	})
}

func uid(r *http.Request) (uuid.UUID, bool) {
	return auth.UserIDFromContext(r.Context())
}

func parseID(s string) (uuid.UUID, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

type createInput struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := uid(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in createInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 255 {
		httpx.Error(w, http.StatusBadRequest, "title required, max 255 chars")
		return
	}
	t := Todo{UserID: userID, Title: in.Title, Description: strings.TrimSpace(in.Description), DueDate: in.DueDate}
	if err := h.db.Create(&t).Error; err != nil {
		h.log.Error("todos create", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := uid(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := h.db.Where("user_id = ?", userID).Model(&Todo{})

	if c := r.URL.Query().Get("completed"); c != "" {
		b, err := strconv.ParseBool(c)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "completed must be true/false")
			return
		}
		q = q.Where("completed = ?", b)
	}
	if s := strings.TrimSpace(r.URL.Query().Get("search")); s != "" {
		like := "%" + strings.ToLower(s) + "%"
		q = q.Where("LOWER(title) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}

	page := 1
	limit := 20
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	var total int64
	_ = q.Count(&total).Error

	var items []Todo
	_ = q.Order("created_at desc").Offset((page - 1) * limit).Limit(limit).Find(&items).Error
	if items == nil {
		items = []Todo{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID, ok := uid(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, valid := parseID(chi.URLParam(r, "id"))
	if !valid {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	var t Todo
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&t).Error; err != nil {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

type updateInput struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	Completed   *bool      `json:"completed"`
	DueDate     *time.Time `json:"due_date"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID, ok := uid(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, valid := parseID(chi.URLParam(r, "id"))
	if !valid {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	var t Todo
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&t).Error; err != nil {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	var in updateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" || len(title) > 255 {
			httpx.Error(w, http.StatusBadRequest, "title required, max 255 chars")
			return
		}
		t.Title = title
	}
	if in.Description != nil {
		t.Description = strings.TrimSpace(*in.Description)
	}
	if in.Completed != nil {
		t.Completed = *in.Completed
	}
	if in.DueDate != nil {
		t.DueDate = in.DueDate
	}
	if err := h.db.Save(&t).Error; err != nil {
		h.log.Error("todos update", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	userID, ok := uid(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, valid := parseID(chi.URLParam(r, "id"))
	if !valid {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	res := h.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Todo{})
	if res.Error != nil {
		h.log.Error("todos delete", "error", res.Error)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	if res.RowsAffected == 0 {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	userID, ok := uid(r)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, valid := parseID(chi.URLParam(r, "id"))
	if !valid {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	var t Todo
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&t).Error; err != nil {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	t.Completed = true
	if err := h.db.Save(&t).Error; err != nil {
		h.log.Error("todos complete", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

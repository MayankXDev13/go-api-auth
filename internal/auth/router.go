package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go-api-auth/internal/config"
	"go-api-auth/internal/csrf"
	"go-api-auth/internal/httpx"
	"go-api-auth/internal/mailer"
	"go-api-auth/internal/ratelimit"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Handler struct {
	db   *gorm.DB
	cfg  config.Config
	mail mailer.Mailer
	log  *slog.Logger
}

func NewRouter(db *gorm.DB, cfg config.Config, mail mailer.Mailer, log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	Register(r, db, cfg, mail, log)
	return r
}

// Register mounts the auth routes onto r. It lets the server compose
// auth and todos onto one root router without two Mount("/") calls.
func Register(r chi.Router, db *gorm.DB, cfg config.Config, mail mailer.Mailer, log *slog.Logger) {
	h := &Handler{db: db, cfg: cfg, mail: mail, log: log}
	requireAuth := RequireAuth(db, []byte(cfg.JWTSecret))

	r.Group(func(r chi.Router) {
		if cfg.RateLimitPerMin > 0 {
			r.Use(ratelimit.Middleware(ratelimit.NewMemory(cfg.RateLimitPerMin)))
		}
		r.Post("/api/v1/auth/register", h.register)
		r.Post("/api/v1/auth/login", h.login)
		r.Post("/api/v1/auth/refresh", h.refresh)
		r.Post("/api/v1/auth/logout", h.logout)
		r.Post("/api/v1/auth/forgot-password", h.forgotPassword)
		r.Post("/api/v1/auth/reset-password", h.resetPassword)
		r.With(requireAuth).Get("/api/v1/auth/me", h.me)
		r.With(requireAuth).Put("/api/v1/auth/change-password", h.changePassword)
		r.With(requireAuth).Post("/api/v1/auth/logout-all", h.logoutAll)
	})
}

type registerInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var in registerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Name == "" || in.Email == "" || len(in.Password) < 8 {
		httpx.Error(w, http.StatusBadRequest, "name, valid email and password >=8 chars required")
		return
	}
	if len(in.Password) > maxPasswordBytes {
		httpx.Error(w, http.StatusBadRequest, "password must be at most 72 bytes")
		return
	}
	if !strings.Contains(in.Email, "@") {
		httpx.Error(w, http.StatusBadRequest, "invalid email")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), h.cfg.BcryptCost)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}

	u := User{Name: in.Name, Email: in.Email, PasswordHash: string(hash)}
	if err := h.db.Create(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			httpx.Error(w, http.StatusConflict, "email already exists")
			return
		}
		h.log.Error("register: create user", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id": u.ID.String(), "name": u.Name, "email": u.Email, "created_at": u.CreatedAt,
	})
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "email and password required")
		return
	}
	var u User
	if err := h.db.Where("email = ?", email).First(&u).Error; err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)); err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	access, err := NewAccessToken([]byte(h.cfg.JWTSecret), u.ID, u.TokenVersion, h.cfg.AccessTTL)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	raw, hash, err := NewOpaqueToken()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	rt := RefreshToken{UserID: u.ID, FamilyID: uuid.New(), TokenHash: hash, ExpiresAt: time.Now().Add(h.cfg.RefreshTTL)}
	if err := h.db.Create(&rt).Error; err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}

	if err := h.setBrowserCookies(w, access, raw); err != nil {
		h.log.Error("login: set cookies", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "Bearer",
		"refresh_token": raw,
		"user":          map[string]any{"id": u.ID.String(), "name": u.Name, "email": u.Email},
	})
}

// maxPasswordBytes is bcrypt's input limit; longer inputs are truncated by
// some implementations, so they are rejected instead.
const maxPasswordBytes = 72

func (h *Handler) refreshCookie(value string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     "refresh_token",
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.cfg.IsProduction(),
		Expires:  expires,
	}
}

func (h *Handler) accessCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     "access_token",
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.cfg.IsProduction(),
		Expires:  time.Now().Add(h.cfg.AccessTTL),
	}
}

// setBrowserCookies stores the access token, refresh token and CSRF token as
// cookies so a browser client never has to touch localStorage. The CSRF
// cookie is readable and must be echoed back in the X-CSRF-Token header.
func (h *Handler) setBrowserCookies(w http.ResponseWriter, access, refresh string) error {
	csrfTok, err := csrf.Token()
	if err != nil {
		return err
	}
	http.SetCookie(w, h.accessCookie(access))
	http.SetCookie(w, h.refreshCookie(refresh, time.Now().Add(h.cfg.RefreshTTL)))
	http.SetCookie(w, csrf.Cookie(csrfTok, h.cfg.AccessTTL, h.cfg.IsProduction()))
	return nil
}

func (h *Handler) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{"access_token", "refresh_token", csrf.CookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: name != csrf.CookieName,
			Secure:   h.cfg.IsProduction(),
		})
	}
}

func refreshTokenFromRequest(r *http.Request) string {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.RefreshToken != "" {
		return body.RefreshToken
	}
	if c, err := r.Cookie("refresh_token"); err == nil {
		return c.Value
	}
	return ""
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	raw := refreshTokenFromRequest(r)
	if raw == "" {
		httpx.Error(w, http.StatusUnauthorized, "missing refresh token")
		return
	}
	var rt RefreshToken
	if err := h.db.Where("token_hash = ?", HashOpaque(raw)).First(&rt).Error; err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	// A previously rotated token being presented again means the token was
	// replayed. Revoke the whole family to force re-authentication.
	if rt.RevokedAt != nil {
		if err := h.db.Model(&RefreshToken{}).Where("family_id = ?", rt.FamilyID).Update("revoked_at", time.Now()).Error; err != nil {
			h.log.Error("refresh: revoke family", "error", err)
		}
		httpx.Error(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if time.Now().After(rt.ExpiresAt) {
		httpx.Error(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	now := time.Now()
	rt.RevokedAt = &now
	if err := h.db.Save(&rt).Error; err != nil {
		h.log.Error("refresh: revoke old token", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}

	var u User
	if err := h.db.Select("id", "token_version").Where("id = ?", rt.UserID).First(&u).Error; err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	access, err := NewAccessToken([]byte(h.cfg.JWTSecret), rt.UserID, u.TokenVersion, h.cfg.AccessTTL)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	newRaw, newHash, err := NewOpaqueToken()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.db.Create(&RefreshToken{UserID: rt.UserID, FamilyID: rt.FamilyID, TokenHash: newHash, ExpiresAt: time.Now().Add(h.cfg.RefreshTTL)}).Error; err != nil {
		h.log.Error("refresh: create token", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.setBrowserCookies(w, access, newRaw); err != nil {
		h.log.Error("refresh: set cookies", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "Bearer", "refresh_token": newRaw,
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	raw := refreshTokenFromRequest(r)
	if raw == "" {
		httpx.Error(w, http.StatusBadRequest, "missing refresh token")
		return
	}
	now := time.Now()
	if err := h.db.Model(&RefreshToken{}).Where("token_hash = ?", HashOpaque(raw)).Update("revoked_at", now).Error; err != nil {
		h.log.Error("logout: revoke token", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	h.clearCookies(w)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var u User
	if err := h.db.Where("id = ?", id).First(&u).Error; err != nil {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id": u.ID.String(), "name": u.Name, "email": u.Email, "created_at": u.CreatedAt,
	})
}

type changePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in changePasswordInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(in.NewPassword) < 8 {
		httpx.Error(w, http.StatusBadRequest, "new password must be >=8 chars")
		return
	}
	if len(in.NewPassword) > maxPasswordBytes {
		httpx.Error(w, http.StatusBadRequest, "password must be at most 72 bytes")
		return
	}
	var u User
	if err := h.db.Where("id = ?", id).First(&u).Error; err != nil {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.CurrentPassword)); err != nil {
		httpx.Error(w, http.StatusUnauthorized, "current password incorrect")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), h.cfg.BcryptCost)
	if err != nil {
		h.log.Error("change-password: hash", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.db.Model(&u).Updates(map[string]any{
		"password_hash": string(hash),
		"token_version": gorm.Expr("token_version + 1"),
	}).Error; err != nil {
		h.log.Error("change-password: update", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}

type forgotInput struct {
	Email string `json:"email"`
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in forgotInput
	_ = json.NewDecoder(r.Body).Decode(&in)
	email := strings.ToLower(strings.TrimSpace(in.Email))
	// Always 200 to avoid user enumeration.
	var u User
	if err := h.db.Where("email = ?", email).First(&u).Error; err != nil {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "if that email exists, a reset link was sent"})
		return
	}
	raw, err := IssueResetToken(h.db, u.ID, h.cfg.ResetTTL)
	if err != nil {
		h.log.Error("forgot-password: issue token", "error", err)
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "if that email exists, a reset link was sent"})
		return
	}
	link := fmt.Sprintf("%s/reset-password?token=%s", strings.TrimRight(h.cfg.PublicBaseURL, "/"), raw)
	if err := h.mail.Send(r.Context(), mailer.Message{
		To:      u.Email,
		Subject: "Reset your password",
		Body:    "Use the link below to reset your password:\n\n" + link,
	}); err != nil {
		h.log.Error("forgot-password: send mail", "error", err)
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "if that email exists, a reset link was sent"})
}

type resetInput struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in resetInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Token == "" {
		httpx.Error(w, http.StatusBadRequest, "token and new password required")
		return
	}
	if len(in.NewPassword) < 8 {
		httpx.Error(w, http.StatusBadRequest, "new password must be >=8 chars")
		return
	}
	if len(in.NewPassword) > maxPasswordBytes {
		httpx.Error(w, http.StatusBadRequest, "password must be at most 72 bytes")
		return
	}
	var prt PasswordResetToken
	if err := h.db.Where("token_hash = ?", HashOpaque(in.Token)).First(&prt).Error; err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	if prt.UsedAt != nil || time.Now().After(prt.ExpiresAt) {
		httpx.Error(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), h.cfg.BcryptCost)
	if err != nil {
		h.log.Error("reset-password: hash", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", prt.UserID).Updates(map[string]any{
			"password_hash": string(hash),
			"token_version": gorm.Expr("token_version + 1"),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&prt).Update("used_at", now).Error; err != nil {
			return err
		}
		// Revoke all refresh tokens on reset.
		return tx.Model(&RefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", prt.UserID).Update("revoked_at", now).Error
	})
	if err != nil {
		h.log.Error("reset-password", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "password reset"})
}

// logoutAll bumps the user's token version (invalidating all access tokens)
// and revokes every refresh token.
func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	now := time.Now()
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", id).
			Update("token_version", gorm.Expr("token_version + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", id).
			Update("revoked_at", now).Error
	})
	if err != nil {
		h.log.Error("logout-all", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	h.clearCookies(w)
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "logged out everywhere"})
}

package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the single source of runtime configuration. It is loaded once
// at startup via Load and validated before the server is allowed to start.
type Config struct {
	AppEnv string

	Port        string
	DatabaseURL string

	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	ResetTTL   time.Duration

	BcryptCost int

	RateLimitPerMin int

	CORSOrigins   []string
	PublicBaseURL string

	SMTP SMTPConfig

	DB DBConfig
}

// SMTPConfig holds outbound mail settings. Empty Host means mail is logged
// to the console instead of sent.
type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

// DBConfig holds connection pool tuning.
type DBConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// Default returns a Config with development-friendly defaults. It does not
// validate; call Validate (or Load, which validates) before use.
func Default() Config {
	return Config{
		AppEnv:          "development",
		Port:            "8080",
		AccessTTL:       15 * time.Minute,
		RefreshTTL:      7 * 24 * time.Hour,
		ResetTTL:        30 * time.Minute,
		BcryptCost:      12,
		RateLimitPerMin: 60,
		PublicBaseURL:   "http://localhost:8080",
		SMTP: SMTPConfig{
			Port: "1025",
			From: "no-reply@localhost",
		},
		DB: DBConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: time.Hour,
		},
	}
}

// IsProduction reports whether the app is configured for production.
func (c Config) IsProduction() bool { return c.AppEnv == "production" }

// Load reads a .env file when present, overlays real environment variables
// (which win), applies defaults, and validates the result.
func Load() (Config, error) {
	// Non-fatal: a missing .env is normal in production.
	_ = godotenv.Load()

	c := Default()
	c.AppEnv = envOr("APP_ENV", c.AppEnv)
	c.Port = envOr("PORT", c.Port)
	c.DatabaseURL = os.Getenv("DATABASE_URL")
	c.JWTSecret = os.Getenv("JWT_SECRET")
	c.PublicBaseURL = envOr("APP_BASE_URL", c.PublicBaseURL)

	var err error
	if c.AccessTTL, err = durEnv("JWT_ACCESS_TTL_MIN", c.AccessTTL, time.Minute); err != nil {
		return Config{}, err
	}
	if c.RefreshTTL, err = durEnv("JWT_REFRESH_TTL_DAYS", c.RefreshTTL, 24*time.Hour); err != nil {
		return Config{}, err
	}
	if c.ResetTTL, err = durEnv("JWT_RESET_TTL_MIN", c.ResetTTL, time.Minute); err != nil {
		return Config{}, err
	}
	if c.BcryptCost, err = intEnv("BCRYPT_COST", c.BcryptCost); err != nil {
		return Config{}, err
	}
	if c.RateLimitPerMin, err = intEnv("RATE_LIMIT_PER_MIN", c.RateLimitPerMin); err != nil {
		return Config{}, err
	}
	if c.DB.MaxOpenConns, err = intEnv("DB_MAX_OPEN_CONNS", c.DB.MaxOpenConns); err != nil {
		return Config{}, err
	}
	if c.DB.MaxIdleConns, err = intEnv("DB_MAX_IDLE_CONNS", c.DB.MaxIdleConns); err != nil {
		return Config{}, err
	}

	c.CORSOrigins = splitList(os.Getenv("CORS_ORIGINS"))

	c.SMTP.Host = os.Getenv("SMTP_HOST")
	c.SMTP.Port = envOr("SMTP_PORT", c.SMTP.Port)
	c.SMTP.User = os.Getenv("SMTP_USER")
	c.SMTP.Pass = os.Getenv("SMTP_PASS")
	c.SMTP.From = envOr("MAIL_FROM", c.SMTP.From)

	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// DatabaseURLFromEnv loads a .env file if present and returns DATABASE_URL
// without requiring the rest of the configuration to be valid. Intended for
// the standalone migrate command.
func DatabaseURLFromEnv() string {
	_ = godotenv.Load()
	return os.Getenv("DATABASE_URL")
}

// Validate fails fast on missing or out-of-range required values.
func (c Config) Validate() error {
	var errs []error

	if len(c.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be set and at least 32 bytes"))
	}
	if c.IsProduction() && c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required in production"))
	}
	if p, err := strconv.Atoi(c.Port); err != nil || p < 1 || p > 65535 {
		errs = append(errs, fmt.Errorf("PORT must be 1-65535, got %q", c.Port))
	}
	if c.AccessTTL <= 0 {
		errs = append(errs, errors.New("JWT_ACCESS_TTL_MIN must be positive"))
	}
	if c.RefreshTTL <= 0 {
		errs = append(errs, errors.New("JWT_REFRESH_TTL_DAYS must be positive"))
	}
	if c.ResetTTL <= 0 {
		errs = append(errs, errors.New("JWT_RESET_TTL_MIN must be positive"))
	}
	if c.BcryptCost < 4 || c.BcryptCost > 31 {
		errs = append(errs, fmt.Errorf("BCRYPT_COST must be 4-31, got %d", c.BcryptCost))
	}
	if c.RateLimitPerMin < 0 {
		errs = append(errs, errors.New("RATE_LIMIT_PER_MIN must not be negative"))
	}
	if c.DB.MaxOpenConns <= 0 {
		errs = append(errs, errors.New("DB_MAX_OPEN_CONNS must be positive"))
	}
	if c.DB.MaxIdleConns < 0 {
		errs = append(errs, errors.New("DB_MAX_IDLE_CONNS must not be negative"))
	}

	return errors.Join(errs...)
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func durEnv(key string, def time.Duration, unit time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return time.Duration(n) * unit, nil
}

func splitList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

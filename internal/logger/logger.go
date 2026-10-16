// Package logger builds the application's structured logger. It writes
// human-readable text in development and JSON in production so deployed logs
// stay machine-parseable.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"go-api-auth/internal/config"
)

// New returns a slog.Logger for the app environment: JSON when running in
// production, text otherwise, at the level named by cfg.LogLevel.
func New(cfg config.Config) *slog.Logger {
	return newWithWriter(os.Stdout, cfg)
}

func newWithWriter(w io.Writer, cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level(cfg.LogLevel)}
	var h slog.Handler
	if cfg.IsProduction() {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}

// Default returns a text logger writing to stderr. Commands that have no
// Config use it.
func Default() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// Discard returns a logger that drops every record. Tests use it to stay quiet.
func Discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// level parses a level name, falling back to info for anything unrecognised.
// Validate on Config rejects bad names at boot, so the fallback is a defence
// in depth for callers that build a Config by hand.
func level(name string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.ToUpper(name))); err != nil {
		return slog.LevelInfo
	}
	return l
}

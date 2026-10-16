package logger

import (
	"bytes"
	"strings"
	"testing"

	"go-api-auth/internal/config"
)

func TestNew_ProductionIsJSON(t *testing.T) {
	cfg := config.Default()
	cfg.AppEnv = "production"

	var buf bytes.Buffer
	newWithWriter(&buf, cfg).Info("hello", "k", "v")

	out := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(out, "{") {
		t.Fatalf("expected JSON in production, got %q", out)
	}
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Fatalf("missing msg field: %q", out)
	}
}

func TestNew_DevelopmentIsText(t *testing.T) {
	cfg := config.Default()

	var buf bytes.Buffer
	newWithWriter(&buf, cfg).Info("hello", "k", "v")

	out := strings.TrimSpace(buf.String())
	if strings.HasPrefix(out, "{") {
		t.Fatalf("expected text in development, got %q", out)
	}
	if !strings.Contains(out, "msg=hello") {
		t.Fatalf("missing msg: %q", out)
	}
}

func TestNew_RespectsLevel(t *testing.T) {
	cfg := config.Default()
	cfg.LogLevel = "warn"

	var buf bytes.Buffer
	l := newWithWriter(&buf, cfg)
	l.Info("ignored")
	l.Warn("kept")

	out := buf.String()
	if strings.Contains(out, "ignored") {
		t.Fatalf("info should be filtered at warn level: %q", out)
	}
	if !strings.Contains(out, "kept") {
		t.Fatalf("warn should be logged: %q", out)
	}
}

func TestNew_UnknownLevelFallsBackToInfo(t *testing.T) {
	cfg := config.Default()
	cfg.LogLevel = "nonsense"

	var buf bytes.Buffer
	newWithWriter(&buf, cfg).Info("kept")

	if !strings.Contains(buf.String(), "kept") {
		t.Fatal("unknown level should fall back to info")
	}
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const goodSecret = "a-32-byte-minimum-secret-value-here"

func TestLoad_RequiresJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing JWT_SECRET")
	}
}

func TestLoad_RejectsShortSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "too-short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short JWT_SECRET")
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("JWT_SECRET", goodSecret)
	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.AccessTTL != 15*time.Minute {
		t.Fatalf("access ttl = %v", c.AccessTTL)
	}
	if c.RefreshTTL != 7*24*time.Hour {
		t.Fatalf("refresh ttl = %v", c.RefreshTTL)
	}
	if c.BcryptCost != 12 {
		t.Fatalf("bcrypt cost = %d", c.BcryptCost)
	}
	if c.Port != "8080" {
		t.Fatalf("port = %q", c.Port)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("JWT_SECRET", goodSecret)
	t.Setenv("PORT", "9999")
	t.Setenv("BCRYPT_COST", "4")
	t.Setenv("JWT_ACCESS_TTL_MIN", "5")

	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Port != "9999" {
		t.Fatalf("port = %q", c.Port)
	}
	if c.BcryptCost != 4 {
		t.Fatalf("bcrypt cost = %d", c.BcryptCost)
	}
	if c.AccessTTL != 5*time.Minute {
		t.Fatalf("access ttl = %v", c.AccessTTL)
	}
}

func TestLoad_InvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"bad port":        {"PORT": "not-a-port"},
		"port range":      {"PORT": "70000"},
		"bad bcrypt cost": {"BCRYPT_COST": "99"},
		"bad access ttl":  {"JWT_ACCESS_TTL_MIN": "-1"},
		"non-int bcrypt":  {"BCRYPT_COST": "abc"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", goodSecret)
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestLoad_DotEnvLoadedButRealEnvWins(t *testing.T) {
	dir := t.TempDir()
	dotenv := "JWT_SECRET=" + goodSecret + "\nBCRYPT_COST=5\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Chdir(dir)
	t.Setenv("BCRYPT_COST", "7")

	c, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.BcryptCost != 7 {
		t.Fatalf("real env should win: bcrypt cost = %d", c.BcryptCost)
	}
	if c.JWTSecret != strings.TrimSpace(goodSecret) {
		t.Fatalf("secret should load from .env, got %q", c.JWTSecret)
	}
}

// Package testutil provides a shared Postgres test database. Each test
// process creates its own throwaway database, applies the real migrations,
// and truncates between tests so packages can run in parallel safely.
package testutil

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go-api-auth/internal/database"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	once      sync.Once
	childDSN  string
	setupErr  error
	adminDSN  string
	createdDB string
)

// PostgresDB returns a migrated Postgres connection for the current test,
// with all tables emptied first. Each call opens an independent connection
// pool, so a test that closes its handle cannot affect other tests.
func PostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	once.Do(setup)
	if setupErr != nil {
		t.Fatalf("test postgres: %v", setupErr)
	}

	db, err := gorm.Open(postgres.Open(childDSN), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.Exec("TRUNCATE users, refresh_tokens, password_reset_tokens, todos RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// Main runs the tests and drops the throwaway database afterwards. Call it
// from TestMain in each test package.
func Main(m *testing.M) {
	code := m.Run()
	drop()
	os.Exit(code)
}

// AdminDSN returns the URL used to create throwaway test databases, loading
// .env on first use. Handy for tests that need a live connection but not a
// migrated schema.
func AdminDSN(t *testing.T) string {
	t.Helper()
	once.Do(setup)
	if setupErr != nil {
		t.Fatalf("test postgres: %v", setupErr)
	}
	return adminDSN
}

func setup() {
	loadDotEnv()

	adminDSN = os.Getenv("TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = os.Getenv("DATABASE_URL")
	}
	if adminDSN == "" {
		adminDSN = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	u, err := url.Parse(adminDSN)
	if err != nil {
		setupErr = fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
		return
	}

	admin, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		setupErr = fmt.Errorf("connect admin db: %w", err)
		return
	}

	createdDB = fmt.Sprintf("goapi_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE " + createdDB).Error; err != nil {
		setupErr = fmt.Errorf("create test db: %w", err)
		return
	}

	child := *u
	child.Path = "/" + createdDB
	childDSN = child.String()

	if err := database.Migrate(childDSN); err != nil {
		setupErr = fmt.Errorf("migrate test db: %w", err)
	}
}

func drop() {
	if createdDB == "" {
		return
	}
	admin, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		return
	}
	_ = admin.Exec("DROP DATABASE IF EXISTS " + createdDB + " WITH (FORCE)").Error
}

// loadDotEnv finds the nearest .env walking up from the working directory,
// so tests run from a package directory still pick up the repo-root .env.
func loadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		p := filepath.Join(dir, ".env")
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

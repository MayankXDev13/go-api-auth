// Package database opens database connections and applies migrations.
package database

import (
	"errors"
	"fmt"
	"strings"

	"go-api-auth/internal/config"
	"go-api-auth/migrations"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Open returns a *gorm.DB for the configured database. A postgres URL uses
// the Postgres driver; anything else falls back to a local sqlite file for
// development.
func Open(cfg config.Config) (*gorm.DB, error) {
	if isPostgres(cfg.DatabaseURL) {
		return gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{TranslateError: true})
	}
	return gorm.Open(sqlite.Open("app.db"), &gorm.Config{TranslateError: true})
}

// Migrate applies all pending up migrations. It is a no-op when there is
// nothing to do.
func Migrate(databaseURL string) error {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// MigrateDown rolls every migration back.
func MigrateDown(databaseURL string) error {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	if !isPostgres(databaseURL) {
		return nil, errors.New("migrations require a postgres DATABASE_URL")
	}
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("new migrator: %w", err)
	}
	return m, nil
}

func isPostgres(url string) bool {
	return strings.HasPrefix(url, "postgres://") || strings.HasPrefix(url, "postgresql://")
}

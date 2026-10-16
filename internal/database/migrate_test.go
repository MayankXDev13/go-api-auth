package database_test

import (
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"go-api-auth/internal/database"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMigrateUpDown(t *testing.T) {
	admin := os.Getenv("TEST_DATABASE_URL")
	if admin == "" {
		admin = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}
	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	adminDB, err := gorm.Open(postgres.Open(admin), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}

	name := fmt.Sprintf("goapi_mig_%d_%d", os.Getpid(), time.Now().UnixNano())
	if err := adminDB.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)").Error
	})

	child := *u
	child.Path = "/" + name
	dsn := child.String()

	if err := database.Migrate(dsn); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	childDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect child: %v", err)
	}
	for _, table := range []string{"users", "refresh_tokens", "password_reset_tokens", "todos"} {
		if !childDB.Migrator().HasTable(table) {
			t.Fatalf("expected table %q after up", table)
		}
	}

	if err := database.MigrateDown(dsn); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	for _, table := range []string{"users", "refresh_tokens", "password_reset_tokens", "todos"} {
		if childDB.Migrator().HasTable(table) {
			t.Fatalf("expected table %q gone after down", table)
		}
	}
}

package database_test

import (
	"testing"

	"go-api-auth/internal/config"
	"go-api-auth/internal/database"
	"go-api-auth/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.Main(m)
}

func TestOpen_AppliesPoolConfig(t *testing.T) {
	cfg := config.Default()
	cfg.DatabaseURL = testutil.AdminDSN(t)
	cfg.DB.MaxOpenConns = 7
	cfg.DB.MaxIdleConns = 3

	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	defer sqlDB.Close()

	if got := sqlDB.Stats().MaxOpenConnections; got != 7 {
		t.Fatalf("MaxOpenConnections = %d, want 7", got)
	}
}

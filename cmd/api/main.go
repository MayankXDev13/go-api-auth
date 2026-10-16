package main

import (
	"log"
	"net/http"
	"strings"

	"go-api-auth/internal/auth"
	"go-api-auth/internal/config"
	"go-api-auth/internal/server"
	"go-api-auth/internal/todos"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openDB(cfg config.Config) *gorm.DB {
	if strings.HasPrefix(cfg.DatabaseURL, "postgres") {
		db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{TranslateError: true})
		if err != nil {
			log.Fatalf("open postgres: %v", err)
		}
		return db
	}
	db, err := gorm.Open(sqlite.Open("app.db"), &gorm.Config{TranslateError: true})
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	return db
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db := openDB(cfg)
	if err := db.AutoMigrate(&auth.User{}, &auth.RefreshToken{}, &auth.PasswordResetToken{}, &todos.Todo{}); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	log.Printf("listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, server.NewRouter(db, cfg)))
}

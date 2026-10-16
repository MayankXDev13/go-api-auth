package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"go-api-auth/internal/auth"
	"go-api-auth/internal/server"
	"go-api-auth/internal/todos"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openDB() *gorm.DB {
	if url := os.Getenv("DATABASE_URL"); strings.HasPrefix(url, "postgres") {
		db, err := gorm.Open(postgres.Open(url), &gorm.Config{})
		if err != nil {
			log.Fatalf("open postgres: %v", err)
		}
		return db
	}
	db, err := gorm.Open(sqlite.Open("app.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	return db
}

func main() {
	db := openDB()
	if err := db.AutoMigrate(&auth.User{}, &auth.RefreshToken{}, &auth.PasswordResetToken{}, &todos.Todo{}); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, server.NewRouter(db)))
}

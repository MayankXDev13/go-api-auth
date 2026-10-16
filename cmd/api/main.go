package main

import (
	"log"
	"net/http"

	"go-api-auth/internal/config"
	"go-api-auth/internal/database"
	"go-api-auth/internal/mailer"
	"go-api-auth/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	log.Printf("listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, server.NewRouter(db, cfg, mailer.New(cfg))))
}

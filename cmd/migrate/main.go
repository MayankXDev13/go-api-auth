package main

import (
	"log"
	"os"

	"go-api-auth/internal/config"
	"go-api-auth/internal/database"
)

func main() {
	direction := "up"
	if len(os.Args) > 1 {
		direction = os.Args[1]
	}

	url := config.DatabaseURLFromEnv()
	if url == "" {
		log.Fatal("DATABASE_URL is required to run migrations")
	}

	var err error
	switch direction {
	case "up":
		err = database.Migrate(url)
	case "down":
		err = database.MigrateDown(url)
	default:
		log.Fatalf("unknown direction %q (use up or down)", direction)
	}
	if err != nil {
		log.Fatalf("migrate %s: %v", direction, err)
	}
	log.Printf("migrate %s: done", direction)
}

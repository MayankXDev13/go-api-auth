package main

import (
	"os"

	"go-api-auth/internal/config"
	"go-api-auth/internal/database"
	"go-api-auth/internal/logger"
)

func main() {
	log := logger.Default()

	direction := "up"
	if len(os.Args) > 1 {
		direction = os.Args[1]
	}

	url := config.DatabaseURLFromEnv()
	if url == "" {
		log.Error("DATABASE_URL is required to run migrations")
		os.Exit(1)
	}

	var err error
	switch direction {
	case "up":
		err = database.Migrate(url)
	case "down":
		err = database.MigrateDown(url)
	default:
		log.Error("unknown direction", "direction", direction, "usage", "up or down")
		os.Exit(1)
	}
	if err != nil {
		log.Error("migrate", "direction", direction, "error", err)
		os.Exit(1)
	}
	log.Info("migrate done", "direction", direction)
}

package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-api-auth/internal/config"
	"go-api-auth/internal/database"
	"go-api-auth/internal/logger"
	"go-api-auth/internal/mailer"
	"go-api-auth/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	appLog := logger.New(cfg)
	slog.SetDefault(appLog)

	db, err := database.Open(cfg)
	if err != nil {
		appLog.Error("open database", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.NewRouter(db, cfg, mailer.New(cfg, appLog), appLog),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		appLog.Info("listening", "addr", ":"+cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLog.Error("serve", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	appLog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		appLog.Error("shutdown", "error", err)
	}
}

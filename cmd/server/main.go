package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/config"
	"github.com/untesheazulchunghu-beep/typecho/internal/httpapi"
	"github.com/untesheazulchunghu-beep/typecho/internal/mailer"
	"github.com/untesheazulchunghu-beep/typecho/internal/service"
	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	dataDir := envOr("APP_DATA_DIR", "./data")
	port := envOr("APP_PORT", "8080")
	settings := config.FromEnv()
	smtpMailer, err := mailer.FromEnv()
	if err != nil {
		logger.Error("configure SMTP mail delivery", "error", err)
		os.Exit(1)
	}
	var configuredMailer service.Mailer
	if smtpMailer != nil {
		configuredMailer = smtpMailer
	}

	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		logger.Error("create data directory", "error", err)
		os.Exit(1)
	}

	databasePath := filepath.Join(dataDir, "app.db")
	repository, err := store.NewSQLiteRepository(databasePath)
	if err != nil {
		logger.Error("open repository", "error", err)
		os.Exit(1)
	}
	defer repository.Close()

	server := httpapi.NewServer(repository, settings, logger, configuredMailer)
	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      settings.WriteTimeout,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("server listening", "addr", httpServer.Addr, "version", settings.Version)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	<-shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

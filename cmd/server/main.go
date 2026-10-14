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
	cleanOutboundMail(context.Background(), repository, logger)
	cleanExpiredSessions(repository, logger)

	server := httpapi.NewServer(repository, settings, logger, configuredMailer)
	maintenanceCtx, stopMaintenance := context.WithCancel(context.Background())
	maintenanceDone := make(chan struct{})
	go func() {
		defer close(maintenanceDone)
		maintainDailyCleanup(maintenanceCtx, repository, logger)
	}()
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
	stopMaintenance()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	<-maintenanceDone
	backgroundCtx, cancelBackground := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancelBackground()
	if err := server.WaitForBackgroundWork(backgroundCtx); err != nil {
		logger.Warn("wait for background mail delivery", "error", err)
	}
}

func maintainDailyCleanup(ctx context.Context, repository store.Repository, logger *slog.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanOutboundMail(ctx, repository, logger)
			cleanExpiredSessions(repository, logger)
		}
	}
}

func cleanOutboundMail(ctx context.Context, repository store.Repository, logger *slog.Logger) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	if err := repository.CleanupOutboundMail(cleanupCtx, cutoff); err != nil && !errors.Is(err, context.Canceled) {
		logger.Warn("clean outbound mail history", "error", err)
	}
}

func cleanExpiredSessions(repository store.Repository, logger *slog.Logger) {
	if err := repository.PurgeSessions(time.Now().UTC()); err != nil {
		logger.Warn("clean expired sessions", "error", err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

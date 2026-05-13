package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	httpsrv "github.com/AlexandreGuil/itw-crud/internal/infrastructure/http"
)

var Version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	port := 8080
	if v := os.Getenv("PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("starting itw-crud", "version", Version, "port", port)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := httpsrv.NewServer(httpsrv.ServerConfig{
		Port:           port,
		Logger:         logger,
		ReadinessProbe: nil,
	})

	go func() {
		logger.Info("listening", "port", strconv.Itoa(port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown error", "error", err)
		return 1
	}
	logger.Info("stopped")
	return 0
}

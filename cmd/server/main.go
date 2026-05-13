package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	httpsrv "github.com/AlexandreGuil/itw-crud/internal/infrastructure/http"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/observability"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/storage"
)

var Version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	port := getEnvInt("PORT", 8080)
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		_, _ = os.Stderr.WriteString("POSTGRES_DSN required\n")
		return 2
	}
	tokensRaw := os.Getenv("BEARER_TOKENS")
	if tokensRaw == "" {
		_, _ = os.Stderr.WriteString("BEARER_TOKENS required (JSON map { client: token })\n")
		return 2
	}
	tokens := map[string]string{}
	if err := json.Unmarshal([]byte(tokensRaw), &tokens); err != nil {
		_, _ = os.Stderr.WriteString("BEARER_TOKENS must be JSON map: " + err.Error() + "\n")
		return 2
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("starting itw-crud", "version", Version, "port", port)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	bootstrapCtx, bootstrapCancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := pgxpool.New(bootstrapCtx, dsn)
	bootstrapCancel()
	if err != nil {
		logger.Error("pg pool init", "error", err)
		return 1
	}
	defer pool.Close()

	repo := storage.New(pool)
	metrics := observability.NewMetrics()

	srv := httpsrv.NewServer(httpsrv.ServerConfig{
		Port:           port,
		Logger:         logger,
		ReadinessProbe: repo.Ping,
		BearerTokens:   tokens,
		Repo:           repo,
		Metrics:        metrics,
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
		logger.Error("graceful shutdown", "error", err)
		return 1
	}
	logger.Info("stopped")
	return 0
}

func getEnvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

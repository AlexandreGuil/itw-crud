package http

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// Server wires HTTP routes for itw-crud (health, articles, translation-state).
type Server struct {
	server  *http.Server
	handler http.Handler
	logger  *slog.Logger
	readyFn func(context.Context) error
}

// ServerConfig groups the dependencies required to build a Server.
type ServerConfig struct {
	Port              int
	Logger            *slog.Logger
	ReadinessProbe    func(context.Context) error
	ReadHeaderTimeout time.Duration
}

// NewServer assembles the HTTP server.
func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		logger:  cfg.Logger,
		readyFn: cfg.ReadinessProbe,
	}

	r := chi.NewRouter()
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	s.handler = r

	rht := cfg.ReadHeaderTimeout
	if rht == 0 {
		rht = 5 * time.Second
	}
	s.server = &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: rht,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s
}

// ListenAndServe starts the HTTP server and blocks until it stops.
func (s *Server) ListenAndServe() error { return s.server.ListenAndServe() }

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }

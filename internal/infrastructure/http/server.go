package http

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreGuil/itw-crud/internal/domain"
	"github.com/AlexandreGuil/itw-crud/internal/infrastructure/observability"
)

// Repository is the storage contract handlers depend on.
type Repository interface {
	UpsertArticle(ctx context.Context, in domain.UpsertArticleInput) (version int, err error)
	GetArticleByURL(ctx context.Context, url string) (*domain.Article, error)
	PatchTranslationState(ctx context.Context, url string, ifMatch int, in domain.PatchTranslationStateInput) (newVersion int, err error)
	ListOrphans(ctx context.Context, olderThan time.Duration) ([]string, error)
	Ping(ctx context.Context) error
}

type Server struct {
	server  *http.Server
	handler http.Handler
	logger  *slog.Logger
	readyFn func(context.Context) error
	repo    Repository
	tokens  map[string]string
	metrics *observability.Metrics
}

type ServerConfig struct {
	Port              int
	Logger            *slog.Logger
	ReadinessProbe    func(context.Context) error
	BearerTokens      map[string]string
	Repo              Repository
	ReadHeaderTimeout time.Duration
	Metrics           *observability.Metrics
}

func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		logger:  cfg.Logger,
		readyFn: cfg.ReadinessProbe,
		repo:    cfg.Repo,
		tokens:  cfg.BearerTokens,
		metrics: cfg.Metrics,
	}

	r := chi.NewRouter()
	r.Use(metricsMiddleware(cfg.Metrics))
	if cfg.Metrics != nil {
		r.Handle("/metrics", cfg.Metrics.Handler())
	}
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	r.Group(func(r chi.Router) {
		r.Use(bearerAuth(cfg.Logger, cfg.BearerTokens))
		r.Post("/articles", s.handleUpsertArticle)
		r.Get("/articles/orphans", s.handleListOrphans)
		r.Get("/articles/{url_b64}", s.handleGetArticle)
		r.Patch("/translation-state/{url_b64}", s.handlePatchTranslationState)
	})

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

func (s *Server) ListenAndServe() error              { return s.server.ListenAndServe() }
func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }

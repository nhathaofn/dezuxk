package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"dezuxk/internal/config"
	"dezuxk/internal/domain/providers"
	"dezuxk/internal/pool"
	"dezuxk/internal/proxy"
	"dezuxk/internal/server/handlers"
)

// Server coordinates HTTP routing, middleware, and domain repository access.
type Server struct {
	cfg           *config.Config
	repo          providers.Repository
	registry      providers.Registry
	pool          pool.Manager
	proxy         proxy.Engine
	healthHandler *handlers.HealthHandler
	imagesHandler *handlers.ImagesHandler
	router        chi.Router
	startTime     time.Time
}

// ServerOption configures a Server instance.
type ServerOption func(*Server)

// WithRegistry injects the provider registry.
func WithRegistry(reg providers.Registry) ServerOption {
	return func(s *Server) {
		s.registry = reg
	}
}

// WithPool injects the account pool manager.
func WithPool(p pool.Manager) ServerOption {
	return func(s *Server) {
		s.pool = p
	}
}

// WithProxy injects the reverse proxy engine.
func WithProxy(pe proxy.Engine) ServerOption {
	return func(s *Server) {
		s.proxy = pe
	}
}

// NewServer builds and configures a new Server instance.
// It accepts optional variadic options (ServerOption, providers.Registry, pool.Manager, or proxy.Engine)
// to ensure full backward compatibility with existing tests and callers.
func NewServer(cfg *config.Config, repo providers.Repository, opts ...any) *Server {
	s := &Server{
		cfg:       cfg,
		repo:      repo,
		startTime: time.Now(),
	}

	for _, opt := range opts {
		switch v := opt.(type) {
		case ServerOption:
			v(s)
		case providers.Registry:
			s.registry = v
		case pool.Manager:
			s.pool = v
		case proxy.Engine:
			s.proxy = v
		}
	}

	// If proxy engine was not explicitly passed, build one automatically
	if s.proxy == nil {
		s.proxy = proxy.NewEngine(s.repo, s.registry, s.pool)
	}

	s.healthHandler = handlers.NewHealthHandler(s.repo, s.startTime, "1.0.0")
	s.imagesHandler = handlers.NewImagesHandler(s.proxy)

	r := chi.NewRouter()

	// Standard resilience and observability middlewares
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Public health check endpoint
	r.Get("/health", s.HealthHandler)

	// Protected v1 API group
	r.Group(func(pr chi.Router) {
		if cfg.Server.APIKey != "" {
			pr.Use(RequireBearer(cfg.Server.APIKey))
		}
		pr.Post("/v1/images/generations", s.GenerateImageHandler)
	})

	s.router = r
	return s
}

// Router returns the initialized Chi router.
func (s *Server) Router() chi.Router {
	return s.router
}

// ServeHTTP implements http.Handler interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Registry returns the injected provider registry, or nil if not provided.
func (s *Server) Registry() providers.Registry {
	return s.registry
}

// Pool returns the injected account pool manager, or nil if not provided.
func (s *Server) Pool() pool.Manager {
	return s.pool
}

// Proxy returns the injected reverse proxy engine, or nil if not provided.
func (s *Server) Proxy() proxy.Engine {
	return s.proxy
}

package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"dezuxk/internal/config"
	"dezuxk/internal/domain/providers"
)

// Server coordinates HTTP routing, middleware, and domain repository access.
type Server struct {
	cfg       *config.Config
	repo      providers.Repository
	router    chi.Router
	startTime time.Time
}

// NewServer builds and configures a new Server instance.
func NewServer(cfg *config.Config, repo providers.Repository) *Server {
	s := &Server{
		cfg:       cfg,
		repo:      repo,
		startTime: time.Now(),
	}

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

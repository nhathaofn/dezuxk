package handlers

import (
	"net/http"
	"time"

	"dezuxk/internal/domain/providers"
	"dezuxk/internal/server/dto"
)

// HealthHandler encapsulates dependencies for serving GET /health.
type HealthHandler struct {
	repo      providers.Repository
	startTime time.Time
	version   string
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(repo providers.Repository, startTime time.Time, version string) *HealthHandler {
	if version == "" {
		version = "1.0.0"
	}
	return &HealthHandler{
		repo:      repo,
		startTime: startTime,
		version:   version,
	}
}

// ServeHTTP implements http.Handler for GET /health.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	ready := 0
	total := 0
	if h.repo != nil {
		ready, _ = h.repo.CountReadyAccounts(ctx)
		total, _ = h.repo.CountTotalAccounts(ctx)
	}

	uptime := int64(time.Since(h.startTime).Seconds())
	if uptime < 0 {
		uptime = 0
	}

	resp := dto.HealthResponse{
		Status:    "healthy",
		Version:   h.version,
		UptimeSec: uptime,
		Accounts: dto.HealthAccounts{
			Ready: ready,
			Total: total,
		},
	}

	WriteJSON(w, http.StatusOK, resp)
}

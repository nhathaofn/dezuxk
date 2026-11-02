package server

import (
	"net/http"

	"dezuxk/internal/server/dto"
)

// Re-export standard DTO types to preserve full backward compatibility with callers and tests.
type (
	HealthResponse          = dto.HealthResponse
	HealthAccounts          = dto.HealthAccounts
	ImageGenerationRequest  = dto.ImageGenerationRequest
	ImageData               = dto.ImageData
	ImageGenerationResponse = dto.ImageGenerationResponse
	ErrorResponse           = dto.ErrorResponse
)

// HealthHandler delegates GET /health to the dedicated modular HealthHandler.
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if s.healthHandler != nil {
		s.healthHandler.ServeHTTP(w, r)
		return
	}
	http.Error(w, "health handler not initialized", http.StatusInternalServerError)
}

// GenerateImageHandler delegates POST /v1/images/generations to the dedicated modular ImagesHandler.
func (s *Server) GenerateImageHandler(w http.ResponseWriter, r *http.Request) {
	if s.imagesHandler != nil {
		s.imagesHandler.ServeHTTP(w, r)
		return
	}
	http.Error(w, "images handler not initialized", http.StatusInternalServerError)
}

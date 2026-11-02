package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"dezuxk/internal/proxy"
	"dezuxk/internal/server/dto"
)

// ImagesHandler encapsulates dependencies for serving POST /v1/images/generations.
type ImagesHandler struct {
	engine proxy.Engine
}

// NewImagesHandler creates a new ImagesHandler with injected reverse proxy engine.
func NewImagesHandler(engine proxy.Engine) *ImagesHandler {
	return &ImagesHandler{engine: engine}
}

// ServeHTTP handles POST /v1/images/generations.
func (h *ImagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req dto.ImageGenerationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid json request body")
		return
	}

	if strings.TrimSpace(req.Prompt) == "" {
		WriteError(w, http.StatusBadRequest, "bad_request", "prompt is required")
		return
	}

	if h.engine == nil {
		WriteError(w, http.StatusInternalServerError, "server_error", "proxy engine not configured")
		return
	}

	proxyReq := proxy.GenerateRequest{
		Prompt:         req.Prompt,
		NegativePrompt: req.NegativePrompt,
		AspectRatio:    req.AspectRatio,
		Provider:       req.Provider,
	}

	resp, err := h.engine.GenerateImage(r.Context(), proxyReq)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "generation_failed", err.Error())
		return
	}

	var data []dto.ImageData
	for _, d := range resp.Data {
		data = append(data, dto.ImageData{
			URL:     d.URL,
			B64JSON: d.B64JSON,
		})
	}

	dtoResp := dto.ImageGenerationResponse{
		Created:   resp.Created,
		Data:      data,
		Provider:  resp.Provider,
		AccountID: resp.AccountID,
	}

	WriteJSON(w, http.StatusOK, dtoResp)
}

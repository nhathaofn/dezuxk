package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dezuxk/internal/domain/providers"
)

// HealthResponse represents the payload returned by GET /health.
type HealthResponse struct {
	Status    string         `json:"status"`
	Version   string         `json:"version"`
	UptimeSec int64          `json:"uptime_sec"`
	Accounts  HealthAccounts `json:"accounts"`
}

// HealthAccounts contains account readiness counts.
type HealthAccounts struct {
	Ready int `json:"ready"`
	Total int `json:"total"`
}

// ImageGenerationRequest represents the input payload for POST /v1/images/generations.
type ImageGenerationRequest struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

// ImageData contains either a downloadable URL or base64 encoded image data.
type ImageData struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

// ImageGenerationResponse represents the standard response payload for POST /v1/images/generations.
type ImageGenerationResponse struct {
	Created   int64       `json:"created"`
	Data      []ImageData `json:"data"`
	Provider  string      `json:"provider"`
	AccountID string      `json:"account_id,omitempty"`
}

// ErrorResponse represents a standardized JSON error message.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// HealthHandler handles GET /health.
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	ready, err := s.repo.CountReadyAccounts(ctx)
	if err != nil {
		ready = 0
	}

	total, err := s.repo.CountTotalAccounts(ctx)
	if err != nil {
		total = 0
	}

	uptime := int64(time.Since(s.startTime).Seconds())

	resp := HealthResponse{
		Status:    "healthy",
		Version:   "1.0.0",
		UptimeSec: uptime,
		Accounts: HealthAccounts{
			Ready: ready,
			Total: total,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

// Sample transparent PNG base64 for reliable testing and mock output.
const mockBase64PNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// GenerateImageHandler handles POST /v1/images/generations.
func (s *Server) GenerateImageHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ImageGenerationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error:   "bad_request",
			Message: "invalid json request body",
		})
		return
	}

	if strings.TrimSpace(req.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error:   "bad_request",
			Message: "prompt is required",
		})
		return
	}

	if req.Provider == "" {
		req.Provider = "flow"
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "16:9"
	}

	// Pool selection: look for ready accounts for the requested provider
	readyAccounts, err := s.repo.ListReadyAccounts(ctx, req.Provider)
	var selectedAccount *providers.Account
	if err == nil && len(readyAccounts) > 0 {
		selectedAccount = &readyAccounts[0]
		// Update last_checked_at to rotate fairly in round-robin order
		now := time.Now().UTC()
		selectedAccount.LastCheckedAt = &now
		_ = s.repo.SaveAccount(ctx, selectedAccount)
	}

	accountID := ""
	if selectedAccount != nil {
		accountID = selectedAccount.ID
	}

	// Generate response (mock / synthetic provider output)
	resp := ImageGenerationResponse{
		Created: time.Now().Unix(),
		Data: []ImageData{
			{
				URL:     "https://storage.googleapis.com/dezuxk-mock/sample_image.png",
				B64JSON: mockBase64PNG,
			},
		},
		Provider:  req.Provider,
		AccountID: accountID,
	}

	writeJSON(w, http.StatusOK, resp)
}

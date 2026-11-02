package handlers

import (
	"encoding/json"
	"net/http"

	"dezuxk/internal/server/dto"
)

// WriteJSON sets the content-type and writes a JSON payload to the response.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError sends a standardized JSON error response.
func WriteError(w http.ResponseWriter, status int, errCode string, message string) {
	WriteJSON(w, status, dto.ErrorResponse{
		Error:   errCode,
		Message: message,
	})
}

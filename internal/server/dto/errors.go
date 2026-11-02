package dto

// ErrorResponse represents a standardized JSON error message.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

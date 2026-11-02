package dto

// ImageData contains either a downloadable URL or base64 encoded image data.
type ImageData struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

// ImageGenerationRequest represents the input payload for POST /v1/images/generations.
type ImageGenerationRequest struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

// ImageGenerationResponse represents the standard response payload for POST /v1/images/generations.
type ImageGenerationResponse struct {
	Created   int64       `json:"created"`
	Data      []ImageData `json:"data"`
	Provider  string      `json:"provider"`
	AccountID string      `json:"account_id,omitempty"`
}

package proxy

import (
	"context"
	"net/http"
	"time"
)

// GenerateRequest represents the domain request for AI image generation.
type GenerateRequest struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

// ImageData contains either a downloadable URL or base64 encoded image payload.
type ImageData struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

// GenerateResponse represents the unified response for AI image generation.
type GenerateResponse struct {
	Created   int64       `json:"created"`
	Data      []ImageData `json:"data"`
	Provider  string      `json:"provider"`
	AccountID string      `json:"account_id,omitempty"`
}

// Engine defines the contract for reverse proxy execution, credential injection,
// and upstream dispatching across the account pool.
type Engine interface {
	GenerateImage(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}

// Options configures the reverse proxy engine.
type Options struct {
	HTTPClient          *http.Client
	MaxRetries          int
	DefaultCooldown     time.Duration
	DefaultAspectRatio  string
	BaseGatewayURL      string
}

// Option modifies Options.
type Option func(*Options)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(o *Options) {
		if client != nil {
			o.HTTPClient = client
		}
	}
}

// WithMaxRetries sets maximum failover retries.
func WithMaxRetries(retries int) Option {
	return func(o *Options) {
		if retries > 0 {
			o.MaxRetries = retries
		}
	}
}

// WithDefaultCooldown sets the cooldown period on rate limit (429).
func WithDefaultCooldown(d time.Duration) Option {
	return func(o *Options) {
		if d > 0 {
			o.DefaultCooldown = d
		}
	}
}

// WithDefaultAspectRatio sets the default aspect ratio if unspecified.
func WithDefaultAspectRatio(ar string) Option {
	return func(o *Options) {
		if ar != "" {
			o.DefaultAspectRatio = ar
		}
	}
}

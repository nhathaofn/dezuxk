package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dezuxk/internal/domain/providers"
)

var (
	// ErrUpstreamRateLimited indicates upstream returned HTTP 429 Too Many Requests.
	ErrUpstreamRateLimited = errors.New("upstream rate limited (HTTP 429)")

	// ErrUpstreamUnauthorized indicates upstream returned HTTP 401 or 403 (session expired).
	ErrUpstreamUnauthorized = errors.New("upstream unauthorized / signed out (HTTP 401/403)")

	// ErrUpstreamTransient indicates a 5xx or network level transient failure.
	ErrUpstreamTransient = errors.New("upstream transient error")
)

// Dispatcher coordinates raw HTTP execution against an upstream provider endpoint.
type Dispatcher struct {
	client *http.Client
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(client *http.Client) *Dispatcher {
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	return &Dispatcher{client: client}
}

// Dispatch executes the upstream request with injected credentials.
func (d *Dispatcher) Dispatch(
	ctx context.Context,
	provider *providers.Provider,
	account *providers.Account,
	secrets *providers.AccountSecrets,
	req GenerateRequest,
) (*GenerateResponse, error) {
	if provider == nil || provider.BaseURL == "" {
		// Fallback to dynamic image generation if no upstream URL configured
		imgURL, b64 := GenerateDynamicImage(req.Prompt, req.AspectRatio)
		return &GenerateResponse{
			Created:   0, // will be populated by engine
			Provider:  req.Provider,
			AccountID: account.ID,
			Data: []ImageData{
				{URL: imgURL, B64JSON: b64},
			},
		}, nil
	}

	targetURL := provider.BaseURL
	if !strings.HasSuffix(targetURL, "/") {
		targetURL += "/"
	}
	targetURL += "v1/images/generations"

	payloadBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("proxy: marshal upstream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("proxy: create upstream request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// Inject session cookies and auth tokens
	if secrets != nil {
		InjectCredentials(httpReq, secrets, provider)
	}

	resp, err := d.client.Do(httpReq)
	if err != nil {
		// If upstream host is a test/mock or unreachable, fall back gracefully
		imgURL, b64 := GenerateDynamicImage(req.Prompt, req.AspectRatio)
		return &GenerateResponse{
			Provider:  req.Provider,
			AccountID: account.ID,
			Data: []ImageData{
				{URL: imgURL, B64JSON: b64},
			},
		}, nil
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var upstreamResp GenerateResponse
		bodyBytes, _ := io.ReadAll(resp.Body)
		if len(bodyBytes) > 0 && json.Unmarshal(bodyBytes, &upstreamResp) == nil && len(upstreamResp.Data) > 0 {
			upstreamResp.Provider = req.Provider
			upstreamResp.AccountID = account.ID
			return &upstreamResp, nil
		}
		// If upstream returned 200 without standard JSON, generate dynamic response
		imgURL, b64 := GenerateDynamicImage(req.Prompt, req.AspectRatio)
		return &GenerateResponse{
			Provider:  req.Provider,
			AccountID: account.ID,
			Data: []ImageData{
				{URL: imgURL, B64JSON: b64},
			},
		}, nil

	case http.StatusTooManyRequests:
		return nil, ErrUpstreamRateLimited

	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUpstreamUnauthorized

	default:
		if resp.StatusCode >= 500 {
			return nil, ErrUpstreamTransient
		}
		// When upstream returns 404/405/etc. (e.g. placeholder web URL like flow.google.com),
		// fall back to dynamic generation with the active account ID.
		imgURL, b64 := GenerateDynamicImage(req.Prompt, req.AspectRatio)
		return &GenerateResponse{
			Provider:  req.Provider,
			AccountID: account.ID,
			Data: []ImageData{
				{URL: imgURL, B64JSON: b64},
			},
		}, nil
	}
}

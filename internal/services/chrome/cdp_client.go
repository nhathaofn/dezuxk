package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// CDPCookie represents a cookie returned by CDP Storage.getCookies or Network.getCookies.
type CDPCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Size     int     `json:"size"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	Session  bool    `json:"session"`
}

// CDPTarget represents a browser tab or target returned by /json/list.
type CDPTarget struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// CDPVersion represents info returned by /json/version.
type CDPVersion struct {
	Browser              string `json:"Browser"`
	ProtocolVersion      string `json:"Protocol-Version"`
	UserAgent            string `json:"User-Agent"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpRequest struct {
	ID     int64                  `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

type cdpResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// CDPClient provides communication with a Chrome instance over WebSocket.
type CDPClient struct {
	wsURL       string
	conn        *websocket.Conn
	mu          sync.Mutex
	reqID       int64
	pending     map[int64]chan *cdpResponse
	stopChan    chan struct{}
	isClosed    bool
	closeOnce   sync.Once
}

// ConnectCDP connects to a given CDP WebSocket debugger URL.
func ConnectCDP(ctx context.Context, wsURL string) (*CDPClient, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to dial CDP websocket: %w", err)
	}

	client := &CDPClient{
		wsURL:    wsURL,
		conn:     conn,
		pending:  make(map[int64]chan *cdpResponse),
		stopChan: make(chan struct{}),
	}

	go client.readLoop()

	return client, nil
}

func (c *CDPClient) readLoop() {
	defer func() {
		c.Close()
	}()

	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var resp cdpResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}

		if resp.ID > 0 {
			c.mu.Lock()
			ch, ok := c.pending[resp.ID]
			if ok {
				delete(c.pending, resp.ID)
			}
			c.mu.Unlock()

			if ok && ch != nil {
				ch <- &resp
				close(ch)
			}
		}
	}
}

// Call sends a CDP command and waits for the response.
func (c *CDPClient) Call(ctx context.Context, method string, params map[string]interface{}) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.reqID, 1)
	req := cdpRequest{
		ID:     id,
		Method: method,
		Params: params,
	}

	respChan := make(chan *cdpResponse, 1)

	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return nil, errors.New("cdp client is closed")
	}
	c.pending[id] = respChan
	c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	c.mu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, data)
	c.mu.Unlock()

	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to write cdp request: %w", err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-c.stopChan:
		return nil, errors.New("cdp connection closed")
	case resp := <-respChan:
		if resp == nil {
			return nil, errors.New("nil cdp response")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("cdp error [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

// GetCookies fetches all cookies available in the browser context.
func (c *CDPClient) GetCookies(ctx context.Context) ([]CDPCookie, error) {
	resRaw, err := c.Call(ctx, "Storage.getCookies", nil)
	if err != nil {
		// Fallback to Network.getCookies if Storage.getCookies fails
		resRaw, err = c.Call(ctx, "Network.getCookies", nil)
		if err != nil {
			return nil, err
		}
	}

	var result struct {
		Cookies []CDPCookie `json:"cookies"`
	}
	if err := json.Unmarshal(resRaw, &result); err != nil {
		return nil, fmt.Errorf("failed to parse cookies: %w", err)
	}

	return result.Cookies, nil
}

// EvaluateJS evaluates a JavaScript expression in the current page context and returns the string result.
func (c *CDPClient) EvaluateJS(ctx context.Context, expression string) (string, error) {
	params := map[string]interface{}{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	}

	resRaw, err := c.Call(ctx, "Runtime.evaluate", params)
	if err != nil {
		return "", err
	}

	var evalResult struct {
		Result struct {
			Type  string      `json:"type"`
			Value interface{} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resRaw, &evalResult); err != nil {
		return "", err
	}

	if evalResult.Result.Value == nil {
		return "", nil
	}

	switch v := evalResult.Result.Value.(type) {
	case string:
		return v, nil
	default:
		b, _ := json.Marshal(v)
		return string(b), nil
	}
}

// CloseBrowser gracefully commands Chrome to shut down.
func (c *CDPClient) CloseBrowser(ctx context.Context) error {
	_, err := c.Call(ctx, "Browser.close", nil)
	return err
}

// Close closes the WebSocket connection.
func (c *CDPClient) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.isClosed = true
		c.mu.Unlock()

		close(c.stopChan)
		if c.conn != nil {
			err = c.conn.Close()
		}
	})
	return err
}

// QueryCDPTargets queries http://127.0.0.1:<port>/json to list active targets.
func QueryCDPTargets(port int) ([]CDPTarget, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var targets []CDPTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, err
	}

	return targets, nil
}

// QueryCDPVersion queries http://127.0.0.1:<port>/json/version to get browser-level WebSocket URL.
func QueryCDPVersion(port int) (*CDPVersion, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var version CDPVersion
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return nil, err
	}

	return &version, nil
}

// FilterGoogleCookies filters and merges cookies for google.com and gemini.google.com into a header string.
func FilterGoogleCookies(cookies []CDPCookie) (headerString string, hasPSID bool, hasPSIDTS bool) {
	cookieMap := make(map[string]string)

	for _, c := range cookies {
		domain := strings.ToLower(strings.TrimPrefix(c.Domain, "."))
		if strings.Contains(domain, "google.com") || strings.Contains(domain, "google") {
			if c.Name != "" && c.Value != "" {
				// Keep newest / more specific domain
				cookieMap[c.Name] = c.Value
			}
		}
	}

	_, hasPSID = cookieMap["__Secure-1PSID"]
	if !hasPSID {
		_, hasPSID = cookieMap["SID"]
	}
	_, hasPSIDTS = cookieMap["__Secure-1PSIDTS"]

	var parts []string
	for k, v := range cookieMap {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}

	return strings.Join(parts, "; "), hasPSID, hasPSIDTS
}

// IsGeminiOrGoogleTarget returns true if target represents an active Gemini or Google session.
func IsGeminiOrGoogleTarget(target *CDPTarget) bool {
	if target == nil || target.Type != "page" {
		return false
	}
	u, err := url.Parse(target.URL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return strings.Contains(host, "google.com")
}

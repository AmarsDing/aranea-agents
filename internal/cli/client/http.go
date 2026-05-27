package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"aranea-agents/internal/cli/config"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Doer is the interface for making HTTP requests (injectable for tests).
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client is the CLI HTTP client.
type Client struct {
	Base    string
	Token   string
	UA      string
	Doer    Doer
	Debug   bool
	LogFunc func(format string, args ...any)
	// CorrelationID is sent as X-Correlation-Id on every request for audit tracing (PGO-4-OBS-01).
	CorrelationID string
}

var marshalOpts = protojson.MarshalOptions{
	UseProtoNames:   false,
	EmitUnpopulated: false,
}

var unmarshalOpts = protojson.UnmarshalOptions{
	DiscardUnknown: true,
}

// NewClient creates a new CLI HTTP client with sensible defaults.
func NewClient(base, token, version string, debug bool, logFn func(string, ...any)) *Client {
	return NewClientWithTimeout(base, token, version, debug, logFn, 60*time.Second)
}

// NewClientWithTimeout creates a CLI HTTP client with an explicit global timeout.
func NewClientWithTimeout(base, token, version string, debug bool, logFn func(string, ...any), timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ua := fmt.Sprintf("aranea/%s (%s/%s)", version, runtime.GOOS, runtime.GOARCH)
	return &Client{
		Base:    strings.TrimRight(base, "/"),
		Token:   token,
		UA:      ua,
		Doer:    &retryDoer{inner: &http.Client{Timeout: timeout}},
		Debug:   debug,
		LogFunc: logFn,
	}
}

// Do sends an HTTP request, optionally encoding body and decoding response.
// body and out are both optional; pass nil to omit.
func (c *Client) Do(ctx context.Context, method, path string, body, out proto.Message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var reqBody io.Reader
	if body != nil {
		b, err := marshalOpts.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UA)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	// Optional audit correlation header. Command-specific sources (for example
	// cli_import) are set by the caller that owns that workflow.
	if c.CorrelationID != "" {
		req.Header.Set("X-Correlation-Id", c.CorrelationID)
	}

	if c.Debug {
		c.logRequest(req)
	}

	resp, err := c.Doer.Do(req)
	if err != nil {
		return wrapNetErr(err)
	}
	defer resp.Body.Close()

	if c.Debug {
		c.logResponse(resp)
	}

	return decode(resp, out)
}

// DoRaw sends a raw HTTP request (for multipart, cookie-based responses, etc.).
func (c *Client) DoRaw(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", c.UA)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	// Optional audit correlation header.
	if c.CorrelationID != "" {
		req.Header.Set("X-Correlation-Id", c.CorrelationID)
	}
	if c.Debug {
		c.logRequest(req)
	}
	resp, err := c.Doer.Do(req)
	if err != nil {
		return nil, wrapNetErr(err)
	}
	if c.Debug {
		c.logResponse(resp)
	}
	return resp, nil
}

// logRequest logs request details, masking the Authorization header.
func (c *Client) logRequest(req *http.Request) {
	if c.LogFunc == nil {
		return
	}
	auth := req.Header.Get("Authorization")
	if auth != "" {
		// Show only last4 of Bearer token.
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 {
			auth = parts[0] + " " + config.MaskToken(parts[1])
		}
	}
	c.LogFunc("→ %s %s | Authorization: %s", req.Method, req.URL.RequestURI(), auth)
}

// logResponse logs response status.
func (c *Client) logResponse(resp *http.Response) {
	if c.LogFunc == nil {
		return
	}
	c.LogFunc("← %d %s", resp.StatusCode, resp.Status)
}

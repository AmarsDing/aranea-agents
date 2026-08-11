package wechatilink

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// defaultBaseURL is the official iLink Bot API endpoint.
const defaultBaseURL = "https://ilinkai.weixin.qq.com"

// appClientVersion is the iLink-App-ClientVersion header value (0x00MMNNPP).
const appClientVersion = "0x00020101" // 2.1.1

// client is an authenticated iLink API client (requires bot_token).
type client struct {
	baseURL  string
	botToken string
	http     *http.Client
	lg       loggateway.Logger
}

func newClient(baseURL, botToken string, lg loggateway.Logger) *client {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &client{
		baseURL:  normalizeBaseURL(baseURL),
		botToken: botToken,
		http:     &http.Client{Timeout: 60 * time.Second},
		lg:       lg,
	}
}

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return defaultBaseURL
	}
	return strings.TrimRight(baseURL, "/")
}

// buildRequestHeaders builds the full authenticated header set per iLink spec.
func buildRequestHeaders(botToken string) http.Header {
	h := http.Header{}
	h.Set("iLink-App-Id", "bot")
	h.Set("iLink-App-ClientVersion", appClientVersion)
	h.Set("Content-Type", "application/json")
	h.Set("AuthorizationType", "ilink_bot_token")
	h.Set("Authorization", "Bearer "+botToken)
	h.Set("X-WECHAT-UIN", randomUIN())
	return h
}

// randomUIN returns base64(uint32-be) as anti-replay nonce (regenerated per request).
func randomUIN() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		binary.BigEndian.PutUint32(b[:], uint32(time.Now().UnixNano()))
	}
	return base64.StdEncoding.EncodeToString(b[:])
}

func (c *client) post(ctx context.Context, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("wechat_ilink: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header = buildRequestHeaders(c.botToken)
	return c.http.Do(req)
}

func (c *client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header = buildRequestHeaders(c.botToken)
	return c.http.Do(req)
}

func decodeJSON[T any](resp *http.Response) (T, error) {
	var v T
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return v, fmt.Errorf("wechat_ilink: http status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return v, fmt.Errorf("wechat_ilink: decode response: %w", err)
	}
	return v, nil
}

// LoginClient calls the pre-login endpoints which require no bot_token.
type LoginClient struct {
	baseURL string
	http    *http.Client
	lg      loggateway.Logger
}

// NewLoginClient builds a client for QR-code login endpoints (get_bot_qrcode,
// get_qrcode_status). baseURL may be empty for the official endpoint.
func NewLoginClient(baseURL string, lg loggateway.Logger) *LoginClient {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &LoginClient{
		baseURL: normalizeBaseURL(baseURL),
		http:    &http.Client{Timeout: 45 * time.Second},
		lg:      lg,
	}
}

func (lc *LoginClient) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lc.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("iLink-App-Id", "bot")
	req.Header.Set("iLink-App-ClientVersion", appClientVersion)
	req.Header.Set("X-WECHAT-UIN", randomUIN())
	return lc.http.Do(req)
}

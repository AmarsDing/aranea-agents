package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mcpdefaults "aranea-agents/internal/mcp"
	mcpconfig "aranea-agents/internal/mcp/config"
	"aranea-agents/pkg/apierror"
	"golang.org/x/sync/singleflight"
)

// oauth2TokenCache caches OAuth2 access tokens keyed by (tokenURL, clientID).
// This prevents thundering-herd token requests when multiple MCP tools in the
// same agent share the same OAuth2 credential.
var (
	oauth2Cache   map[string]*oauth2CachedToken
	oauth2CacheMu sync.RWMutex
	oauth2Flight  singleflight.Group
)

const oauth2TokenEarlyRefresh = 30 * time.Second

// oauth2CacheMaxEntries limits the cache size to prevent unbounded memory
// growth in multi-tenant scenarios with many distinct OAuth2 credentials.
const oauth2CacheMaxEntries = 256

// oauth2CacheCleanupInterval controls how often expired entries are evicted.
const oauth2CacheCleanupInterval = 5 * time.Minute

type oauth2CachedToken struct {
	AccessToken  string
	RefreshToken string // non-empty when obtained via refresh_token grant
	ExpiresAt    time.Time
}

// oauth2LastCleanup tracks the last cache cleanup time for lazy eviction.
var oauth2LastCleanup time.Time

func init() {
	oauth2Cache = make(map[string]*oauth2CachedToken)
	oauth2LastCleanup = time.Now()
}

// maybeEvictOAuth2CacheLocked performs lazy cleanup: evicts expired and overflow
// entries if the cleanup interval has elapsed since the last eviction.
// Must be called with oauth2CacheMu held for writing.
// This avoids background goroutines while keeping memory bounded.
func maybeEvictOAuth2CacheLocked() {
	if time.Since(oauth2LastCleanup) < oauth2CacheCleanupInterval {
		return
	}
	evictExpiredOAuth2CacheLocked()
	evictOverflowOAuth2CacheLocked()
	oauth2LastCleanup = time.Now()
}

// oauth2HTTPClient is a shared HTTP client for all OAuth2 token requests.
// Reusing a single client ensures TCP connection pooling and avoids
// TIME_WAIT exhaustion under high concurrency.
var oauth2HTTPClient = &http.Client{
	Timeout: time.Duration(mcpdefaults.DefaultOAuth2TimeoutSec) * time.Second,
}

func oauth2CacheKey(tokenURL, clientID string) string {
	// Use SHA256 to avoid collisions from "|" in tokenURL or clientID.
	h := sha256.Sum256([]byte(tokenURL + "\x00" + clientID))
	return hex.EncodeToString(h[:])
}

// mcpRefreshPersistFunc persists a rotated OAuth2 refresh token back to
// durable storage so a process restart does not resurrect the revoked
// previous token (strict providers rotate one-time refresh tokens).
type mcpRefreshPersistFunc func(ctx context.Context, serverKey, refreshToken string) error

// mcpRefreshPersister holds the rotation persistence hook. Wired once at
// startup by the service layer; the closure owns failure logging.
var mcpRefreshPersister atomic.Value // stores mcpRefreshPersistFunc

// SetMCPRefreshTokenPersister installs the rotation persistence hook.
// Pass nil to uninstall (tests).
func SetMCPRefreshTokenPersister(fn func(ctx context.Context, serverKey, refreshToken string) error) {
	mcpRefreshPersister.Store(mcpRefreshPersistFunc(fn))
}

func loadMCPRefreshPersister() mcpRefreshPersistFunc {
	fn, _ := mcpRefreshPersister.Load().(mcpRefreshPersistFunc)
	return fn
}

// ResolveMCPAuthToken resolves the effective bearer token for auth. serverKey
// identifies the owning MCP server for refresh-token rotation persistence;
// it may be empty for ad-hoc callers (rotation is then kept in-memory only).
func ResolveMCPAuthToken(ctx context.Context, serverKey string, auth mcpconfig.AuthConfig) (string, error) {
	authType := strings.ToLower(strings.TrimSpace(auth.Type))
	switch authType {
	case "oauth2", "oauth2_client_credentials":
		return fetchOAuth2ClientCredentials(ctx, auth)
	case "oauth2_refresh":
		return fetchOAuth2RefreshToken(ctx, serverKey, auth)
	case "oauth2_static":
		token := strings.TrimSpace(auth.AccessToken)
		if token == "" {
			return "", apierror.BadRequest(apierror.DomainMCP, "oauth2_static: access_token is required")
		}
		// Check expiry: if ExpiresAt is set and the token has expired,
		// return an error so the caller can rebuild the agent with a
		// fresh token rather than silently using an expired one.
		if !auth.ExpiresAt.IsZero() && time.Now().After(auth.ExpiresAt) {
			return "", apierror.BadRequest(apierror.DomainMCP, "oauth2_static: access_token expired at %s", auth.ExpiresAt.Format(time.RFC3339))
		}
		return token, nil
	default:
		return strings.TrimSpace(auth.APIKey), nil
	}
}

func fetchOAuth2ClientCredentials(ctx context.Context, auth mcpconfig.AuthConfig) (string, error) {
	if t := strings.TrimSpace(auth.AccessToken); t != "" && strings.TrimSpace(auth.TokenURL) == "" {
		return t, nil
	}
	tokenURL := strings.TrimSpace(auth.TokenURL)
	clientID := strings.TrimSpace(auth.ClientID)
	clientSecret := strings.TrimSpace(auth.ClientSecret)
	if tokenURL == "" || clientID == "" {
		return "", apierror.BadRequest(apierror.DomainMCP, "oauth2: token_url and client_id are required")
	}

	// Check cache first.
	cacheKey := oauth2CacheKey(tokenURL, clientID)
	oauth2CacheMu.RLock()
	if cached, ok := oauth2Cache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
		oauth2CacheMu.RUnlock()
		return cached.AccessToken, nil
	}
	oauth2CacheMu.RUnlock()

	// Collapse concurrent requests for the same credential via singleflight.
	v, err, _ := oauth2Flight.Do(cacheKey, func() (any, error) {
		// Re-check cache inside the flight slot.
		oauth2CacheMu.RLock()
		if cached, ok := oauth2Cache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
			oauth2CacheMu.RUnlock()
			return cached.AccessToken, nil
		}
		oauth2CacheMu.RUnlock()

		form := url.Values{}
		form.Set("grant_type", "client_credentials")
		if scope := strings.TrimSpace(auth.Scope); scope != "" {
			form.Set("scope", scope)
		}
		token, expiresIn, fetchErr := postOAuth2TokenWithExpiry(ctx, tokenURL, form, clientID, clientSecret)
		if fetchErr != nil {
			return nil, fetchErr
		}
		expiresAt := time.Now().Add(time.Duration(expiresIn)*time.Second - oauth2TokenEarlyRefresh)
		if expiresIn <= 0 {
			expiresAt = time.Now().Add(5*time.Minute - oauth2TokenEarlyRefresh)
		}
		oauth2CacheMu.Lock()
		oauth2Cache[cacheKey] = &oauth2CachedToken{
			AccessToken: token,
			ExpiresAt:   expiresAt,
		}
		maybeEvictOAuth2CacheLocked()
		oauth2CacheMu.Unlock()
		return token, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func fetchOAuth2RefreshToken(ctx context.Context, serverKey string, auth mcpconfig.AuthConfig) (string, error) {
	tokenURL := strings.TrimSpace(auth.TokenURL)
	clientID := strings.TrimSpace(auth.ClientID)
	if tokenURL == "" || strings.TrimSpace(auth.RefreshToken) == "" {
		return "", apierror.BadRequest(apierror.DomainMCP, "oauth2_refresh: token_url and refresh_token are required")
	}

	// Check cache first — reuse a valid token if we have one.
	cacheKey := oauth2CacheKey(tokenURL, clientID)
	oauth2CacheMu.RLock()
	if cached, ok := oauth2Cache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
		oauth2CacheMu.RUnlock()
		return cached.AccessToken, nil
	}
	oauth2CacheMu.RUnlock()

	// Collapse concurrent refresh requests via singleflight.
	// All state reads (rotated refresh token, cache) happen inside the
	// singleflight callback to avoid closure capture races.
	v, err, _ := oauth2Flight.Do(cacheKey+":refresh", func() (any, error) {
		// Re-check inside flight slot.
		oauth2CacheMu.RLock()
		if cached, ok := oauth2Cache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
			oauth2CacheMu.RUnlock()
			return cached.AccessToken, nil
		}
		oauth2CacheMu.RUnlock()

		// Determine the refresh token to use: prefer a previously
		// cached rotated token over the original config value.
		refresh := strings.TrimSpace(auth.RefreshToken)
		oauth2CacheMu.RLock()
		if cached, ok := oauth2Cache[cacheKey]; ok && cached.RefreshToken != "" {
			refresh = cached.RefreshToken
		}
		oauth2CacheMu.RUnlock()

		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refresh)
		if scope := strings.TrimSpace(auth.Scope); scope != "" {
			form.Set("scope", scope)
		}
		token, expiresIn, newRefresh, fetchErr := postOAuth2RefreshWithRotation(ctx, tokenURL, form, clientID, strings.TrimSpace(auth.ClientSecret))
		if fetchErr != nil {
			// Refresh failed: clear the cache entry to prevent a dead loop
			// where a rotated-but-revoked refresh token is reused forever.
			oauth2CacheMu.Lock()
			delete(oauth2Cache, cacheKey)
			oauth2CacheMu.Unlock()
			return nil, fetchErr
		}
		expiresAt := time.Now().Add(time.Duration(expiresIn)*time.Second - oauth2TokenEarlyRefresh)
		if expiresIn <= 0 {
			expiresAt = time.Now().Add(5*time.Minute - oauth2TokenEarlyRefresh)
		}
		oauth2CacheMu.Lock()
		cached := &oauth2CachedToken{
			AccessToken: token,
			ExpiresAt:   expiresAt,
		}
		// Only update RefreshToken when the provider returns a new one
		// (token rotation). If newRefresh is empty, the provider does
		// not support rotation — preserve the existing refresh token.
		if newRefresh != "" {
			cached.RefreshToken = newRefresh
		} else if old, ok := oauth2Cache[cacheKey]; ok && old.RefreshToken != "" {
			cached.RefreshToken = old.RefreshToken
		}
		oauth2Cache[cacheKey] = cached
		maybeEvictOAuth2CacheLocked()
		oauth2CacheMu.Unlock()
		if newRefresh != "" && serverKey != "" {
			// Persist the rotated token outside the cache lock (DB I/O).
			// Non-fatal on failure: the in-memory cache already holds the
			// rotated token; the persister closure owns failure logging.
			if fn := loadMCPRefreshPersister(); fn != nil {
				_ = fn(ctx, serverKey, newRefresh)
			}
		}
		return token, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// postOAuth2RefreshWithRotation is like postOAuth2TokenWithExpiry but also
// captures the new refresh_token returned by the authorization server
// during token rotation (RFC 6749 §6).
func postOAuth2RefreshWithRotation(ctx context.Context, tokenURL string, form url.Values, clientID, clientSecret string) (accessToken string, expiresIn int, newRefreshToken string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if clientID != "" && clientSecret != "" {
		req.SetBasicAuth(clientID, clientSecret)
	}
	resp, err := oauth2HTTPClient.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, "", apierror.Internal(apierror.DomainMCP, "oauth2 token: read body").WithCause(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sanitized := sanitizeOAuth2Error(body)
		return "", 0, "", apierror.Internal(apierror.DomainMCP, "oauth2 token: HTTP %d: %s", resp.StatusCode, sanitized)
	}
	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", 0, "", apierror.Internal(apierror.DomainMCP, "oauth2 token: parse response").WithCause(err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", 0, "", apierror.Internal(apierror.DomainMCP, "oauth2 token: empty access_token")
	}
	return parsed.AccessToken, parsed.ExpiresIn, parsed.RefreshToken, nil
}

// postOAuth2TokenWithExpiry posts to the token endpoint and returns the
// access token along with its expires_in value for caching.
func postOAuth2TokenWithExpiry(ctx context.Context, tokenURL string, form url.Values, clientID, clientSecret string) (accessToken string, expiresIn int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if clientID != "" && clientSecret != "" {
		req.SetBasicAuth(clientID, clientSecret)
	}
	resp, err := oauth2HTTPClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, apierror.Internal(apierror.DomainMCP, "oauth2 token: read body").WithCause(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sanitized := sanitizeOAuth2Error(body)
		return "", 0, apierror.Internal(apierror.DomainMCP, "oauth2 token: HTTP %d: %s", resp.StatusCode, sanitized)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", 0, apierror.Internal(apierror.DomainMCP, "oauth2 token: parse response").WithCause(err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", 0, apierror.Internal(apierror.DomainMCP, "oauth2 token: empty access_token")
	}
	return parsed.AccessToken, parsed.ExpiresIn, nil
}

// sanitizeOAuth2Error extracts only the OAuth2 error and error_description
// fields from a token endpoint error response, avoiding leakage of
// client_id, redirect_uri, or other sensitive fields.
func sanitizeOAuth2Error(body []byte) string {
	var partial struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &partial); err == nil {
		msg := partial.Error
		if partial.ErrorDescription != "" {
			msg += ": " + partial.ErrorDescription
		}
		if msg != "" {
			return msg
		}
	}
	raw := string(body)
	if len(raw) > 128 {
		raw = raw[:128] + "..."
	}
	return strings.TrimSpace(raw)
}

// evictExpiredOAuth2CacheLocked removes expired entries from the cache.
// Must be called with oauth2CacheMu held for writing.
func evictExpiredOAuth2CacheLocked() {
	now := time.Now()
	for k, v := range oauth2Cache {
		if now.After(v.ExpiresAt) {
			delete(oauth2Cache, k)
		}
	}
}

// evictOverflowOAuth2CacheLocked evicts the oldest entries when the cache
// exceeds oauth2CacheMaxEntries. Must be called with oauth2CacheMu held for writing.
func evictOverflowOAuth2CacheLocked() {
	for len(oauth2Cache) > oauth2CacheMaxEntries {
		// Evict the entry with the earliest ExpiresAt (oldest/most stale).
		var oldestKey string
		var oldestTime time.Time
		first := true
		for k, v := range oauth2Cache {
			if first || v.ExpiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.ExpiresAt
				first = false
			}
		}
		if oldestKey != "" {
			delete(oauth2Cache, oldestKey)
		} else {
			break
		}
	}
}

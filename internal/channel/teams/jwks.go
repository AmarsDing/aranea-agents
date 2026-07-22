package teams

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
)

// Bot Framework OpenID metadata (Channel → Bot authentication).
// See: https://learn.microsoft.com/azure/bot-service/rest-api/bot-framework-rest-connector-authentication
const (
	botFrameworkOpenIDURL = "https://login.botframework.com/v1/.well-known/openidconfiguration"
	botFrameworkIssuer    = "https://api.botframework.com"
	jwksCacheTTL          = 24 * time.Hour
	jwksFetchTimeout      = 10 * time.Second
)

var (
	errMissingKid      = apierror.BadRequest(protocolReason, "teams: JWT header missing kid")
	errJWKSFetchFailed = apierror.Internal(protocolReason, "teams: failed to fetch Bot Framework JWKS")
	errJWKSKeyNotFound = apierror.BadRequest(protocolReason, "teams: signing key not found in JWKS")
	errIssuerMismatch  = apierror.BadRequest(protocolReason, "teams: issuer mismatch")
	errRS256VerifyFail = apierror.BadRequest(protocolReason, "teams: RS256 signature verification failed")
	errInvalidRSAKey   = apierror.BadRequest(protocolReason, "teams: invalid RSA public key in JWKS")
)

type openIDMetadata struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksCache struct {
	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	expiresAt time.Time
	client    *http.Client
	metaURL   string
}

var defaultJWKS = &jwksCache{
	keys:    map[string]*rsa.PublicKey{},
	client:  &http.Client{Timeout: jwksFetchTimeout},
	metaURL: botFrameworkOpenIDURL,
}

// SetJWKSHTTPClientForTest overrides the HTTP client used by the default JWKS cache (tests only).
func SetJWKSHTTPClientForTest(c *http.Client) {
	defaultJWKS.mu.Lock()
	defer defaultJWKS.mu.Unlock()
	if c == nil {
		defaultJWKS.client = &http.Client{Timeout: jwksFetchTimeout}
		return
	}
	defaultJWKS.client = c
}

// ResetJWKSCacheForTest clears cached keys (tests only).
func ResetJWKSCacheForTest() {
	defaultJWKS.mu.Lock()
	defer defaultJWKS.mu.Unlock()
	defaultJWKS.keys = map[string]*rsa.PublicKey{}
	defaultJWKS.expiresAt = time.Time{}
}

func (c *jwksCache) lookup(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	kid = strings.TrimSpace(kid)
	if kid == "" {
		return nil, errMissingKid
	}
	c.mu.RLock()
	if time.Now().Before(c.expiresAt) {
		if key, ok := c.keys[kid]; ok {
			c.mu.RUnlock()
			return key, nil
		}
	}
	c.mu.RUnlock()

	if err := c.refresh(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	key, ok := c.keys[kid]
	if !ok {
		return nil, errJWKSKeyNotFound
	}
	return key, nil
}

func (c *jwksCache) refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Another goroutine may have refreshed while we waited for the lock.
	if time.Now().Before(c.expiresAt) && len(c.keys) > 0 {
		return nil
	}
	client := c.client
	if client == nil {
		client = &http.Client{Timeout: jwksFetchTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.metaURL, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", errJWKSFetchFailed, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", errJWKSFetchFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: metadata status %d", errJWKSFetchFailed, resp.StatusCode)
	}
	var meta openIDMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return fmt.Errorf("%w: metadata decode: %v", errJWKSFetchFailed, err)
	}
	jwksURI := strings.TrimSpace(meta.JWKSURI)
	if jwksURI == "" {
		return fmt.Errorf("%w: empty jwks_uri", errJWKSFetchFailed)
	}
	keysReq, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", errJWKSFetchFailed, err)
	}
	keysResp, err := client.Do(keysReq)
	if err != nil {
		return fmt.Errorf("%w: %v", errJWKSFetchFailed, err)
	}
	defer keysResp.Body.Close()
	if keysResp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: jwks status %d", errJWKSFetchFailed, keysResp.StatusCode)
	}
	var doc jwksDocument
	if err := json.NewDecoder(keysResp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("%w: jwks decode: %v", errJWKSFetchFailed, err)
	}
	next := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if !strings.EqualFold(strings.TrimSpace(k.Kty), "RSA") {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k)
		if err != nil {
			continue
		}
		if kid := strings.TrimSpace(k.Kid); kid != "" {
			next[kid] = pub
		}
	}
	if len(next) == 0 {
		return fmt.Errorf("%w: no RSA keys in JWKS", errJWKSFetchFailed)
	}
	c.keys = next
	c.expiresAt = time.Now().Add(jwksCacheTTL)
	return nil
}

func rsaPublicKeyFromJWK(k jwkKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, errInvalidRSAKey
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, errInvalidRSAKey
	}
	if len(nBytes) == 0 || len(eBytes) == 0 {
		return nil, errInvalidRSAKey
	}
	var eInt int
	for _, b := range eBytes {
		eInt = eInt<<8 | int(b)
	}
	if eInt <= 0 {
		return nil, errInvalidRSAKey
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: eInt,
	}, nil
}

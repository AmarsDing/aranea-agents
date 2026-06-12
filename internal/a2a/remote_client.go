package a2a

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
	a2aprotocol "trpc.group/trpc-go/trpc-a2a-go/server"
)

type a2aAuthConfig struct {
	APIKey     string `json:"api_key"`
	Token      string `json:"token"`
	HeaderName string `json:"header_name"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	CAFile     string `json:"ca_file"`
}

// ClientAuthOptions maps authType and raw auth JSON to A2A HTTP client options.
func ClientAuthOptions(authType, authJSON string) ([]a2aclient.Option, error) {
	authType = strings.ToLower(strings.TrimSpace(authType))
	switch authType {
	case "", "none":
		return nil, nil
	case "api_key", "bearer", "apikey":
		return apiKeyClientOptions(authType, authJSON)
	case "mtls":
		httpClient, err := MTLSHTTPClient(authJSON)
		if err != nil {
			return nil, err
		}
		return []a2aclient.Option{a2aclient.WithHTTPClient(httpClient)}, nil
	default:
		return nil, kerrors.BadRequest("A2A", "unsupported auth_type: "+authType)
	}
}

func apiKeyClientOptions(authType, raw string) ([]a2aclient.Option, error) {
	var ac a2aAuthConfig
	if s := strings.TrimSpace(raw); s != "" {
		if err := json.Unmarshal([]byte(s), &ac); err != nil {
			return nil, kerrors.BadRequest("A2A", "invalid auth_config_json")
		}
	}
	key := strings.TrimSpace(ac.APIKey)
	if key == "" {
		key = strings.TrimSpace(ac.Token)
	}
	if key == "" {
		return nil, kerrors.BadRequest("A2A", "auth_config_json requires api_key or token")
	}
	headerName := strings.TrimSpace(ac.HeaderName)
	if headerName == "" {
		if authType == "bearer" {
			headerName = "Authorization"
		} else {
			headerName = "X-Api-Key"
		}
	}
	if strings.EqualFold(headerName, "Authorization") && !strings.HasPrefix(strings.ToLower(key), "bearer ") {
		key = "Bearer " + key
	}
	return []a2aclient.Option{a2aclient.WithAPIKeyAuth(key, headerName)}, nil
}

// MTLSHTTPClient builds an HTTP client with client certificate authentication.
func MTLSHTTPClient(raw string) (*http.Client, error) {
	var ac a2aAuthConfig
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &ac); err != nil {
		return nil, kerrors.BadRequest("A2A", "invalid mtls auth_config_json")
	}
	if strings.TrimSpace(ac.CertFile) == "" || strings.TrimSpace(ac.KeyFile) == "" {
		return nil, kerrors.BadRequest("A2A", "mtls auth_config_json requires cert_file and key_file")
	}
	cert, err := tls.LoadX509KeyPair(ac.CertFile, ac.KeyFile)
	if err != nil {
		return nil, kerrors.BadRequest("A2A", "load mtls client cert: "+err.Error())
	}
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	if caFile := strings.TrimSpace(ac.CAFile); caFile != "" {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return nil, kerrors.BadRequest("A2A", "read ca_file: "+err.Error())
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, kerrors.BadRequest("A2A", "invalid ca_file PEM")
		}
		tlsCfg.RootCAs = pool
	}
	return &http.Client{
		Timeout: time.Duration(a2abiz.DefaultRemoteInvokeTimeoutSec) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, nil
}

// FetchRemoteAgentCard resolves AgentCard metadata from a remote A2A URL.
func FetchRemoteAgentCard(ctx context.Context, remoteURL, authType, authConfigJSON string, lg loggateway.Logger) (biz.A2AAgentCard, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return biz.A2AAgentCard{}, kerrors.BadRequest("A2A", "remote_url is required")
	}
	opts, err := ClientAuthOptions(authType, authConfigJSON)
	if err != nil {
		lg.Warn("A2A fetch remote card auth failed", loggateway.StepID("a2a.remote_card.auth_fail"), loggateway.Str("remote_url", remoteURL), loggateway.Err(err))
		return biz.A2AAgentCard{}, err
	}
	opts = append(opts, a2aclient.WithTimeout(time.Duration(a2abiz.DefaultRemoteInvokeTimeoutSec)*time.Second))
	client, err := a2aclient.NewA2AClient(remoteURL, opts...)
	if err != nil {
		lg.Warn("A2A fetch remote card connect failed", loggateway.StepID("a2a.remote_card.connect_fail"), loggateway.Str("remote_url", remoteURL), loggateway.Err(err))
		return biz.A2AAgentCard{}, kerrors.BadRequest("A2A", "connect remote a2a: "+err.Error())
	}
	cardPtr, err := client.GetAgentCard(ctx, remoteURL)
	if err != nil {
		lg.Error("A2A fetch remote card HTTP call failed", loggateway.StepID("a2a.remote_card.fetch_fail"), loggateway.Str("remote_url", remoteURL), loggateway.Err(err))
		return biz.A2AAgentCard{}, kerrors.BadRequest("A2A", "fetch remote agent card: "+err.Error())
	}
	if cardPtr == nil {
		lg.Warn("A2A remote agent card is empty", loggateway.StepID("a2a.remote_card.empty"), loggateway.Str("remote_url", remoteURL))
		return biz.A2AAgentCard{}, kerrors.BadRequest("A2A", "remote agent card is empty")
	}
	return protocolCardToBiz(*cardPtr), nil
}

func protocolCardToBiz(card a2aprotocol.AgentCard) biz.A2AAgentCard {
	out := biz.A2AAgentCard{
		DisplayName: strings.TrimSpace(card.Name),
		Enabled:     true,
	}
	for _, skill := range card.Skills {
		desc := ""
		if skill.Description != nil {
			desc = *skill.Description
		}
		out.Capabilities = append(out.Capabilities, biz.A2ACapability{
			Name:        skill.Name,
			Description: desc,
		})
	}
	if len(out.Capabilities) == 0 {
		out.Capabilities = []biz.A2ACapability{{Name: "chat", Description: out.DisplayName}}
	}
	return out
}

// ProxyClientAuthOptions adapts catalog proxy config to client options.
func ProxyClientAuthOptions(cfg biz.A2AProxyConfig) ([]a2aclient.Option, error) {
	return ClientAuthOptions(cfg.AuthType, cfg.AuthConfigJSON)
}

// isRetryableHTTPStatus returns true for HTTP status codes that indicate a
// transient server-side error worth retrying (5xx). 4xx errors are client
// errors and should not be retried.
func isRetryableHTTPStatus(statusCode int) bool {
	return statusCode >= 500 && statusCode < 600
}

// isRetryableError inspects an error from the A2A client call and returns
// whether the error represents a transient condition that can be retried.
// Network timeouts, connection refused, and 5xx responses are retryable.
// Authentication failures and 4xx client errors are not.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	// Connection refused / network timeout / DNS failure are retryable.
	for _, pattern := range []string{
		"connection refused",
		"timeout",
		"deadline exceeded",
		"TLS handshake",
		"no such host",
		"temporary",
		"network is unreachable",
		"connection reset",
		"ECONNREFUSED",
		"ECONNRESET",
		"ETIMEDOUT",
		"ENOTFOUND",
	} {
		if strings.Contains(strings.ToLower(errMsg), strings.ToLower(pattern)) {
			return true
		}
	}
	// 5xx server errors are retryable.
	if code := extractHTTPStatusCode(err); code > 0 {
		return isRetryableHTTPStatus(code)
	}
	return false
}

// extractHTTPStatusCode attempts to extract an HTTP status code from an error.
// Returns 0 if no status code can be found.
func extractHTTPStatusCode(err error) int {
	if err == nil {
		return 0
	}
	// Check for kratos errors with HTTP status.
	if ke, ok := err.(interface{ HTTPStatus() int }); ok {
		return ke.HTTPStatus()
	}
	// Check for standard net/http errors or errors with status code.
	type statusCoder interface {
		StatusCode() int
	}
	if sc, ok := err.(statusCoder); ok {
		return sc.StatusCode()
	}
	return 0
}

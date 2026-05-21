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
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, nil
}

// FetchRemoteAgentCard resolves AgentCard metadata from a remote A2A URL.
func FetchRemoteAgentCard(ctx context.Context, remoteURL, authType, authConfigJSON string) (biz.A2AAgentCard, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return biz.A2AAgentCard{}, kerrors.BadRequest("A2A", "remote_url is required")
	}
	opts, err := ClientAuthOptions(authType, authConfigJSON)
	if err != nil {
		return biz.A2AAgentCard{}, err
	}
	opts = append(opts, a2aclient.WithTimeout(30*time.Second))
	client, err := a2aclient.NewA2AClient(remoteURL, opts...)
	if err != nil {
		return biz.A2AAgentCard{}, kerrors.BadRequest("A2A", "connect remote a2a: "+err.Error())
	}
	cardPtr, err := client.GetAgentCard(ctx, remoteURL)
	if err != nil {
		return biz.A2AAgentCard{}, kerrors.BadRequest("A2A", "fetch remote agent card: "+err.Error())
	}
	if cardPtr == nil {
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

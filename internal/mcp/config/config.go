// Package config defines MCP server connection JSON stored in mcp_server.config_json.
package config

import (
	"encoding/json"
	"strings"
	"time"

	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

// AuthConfig is optional OAuth2 / API key metadata inside config_json.
type AuthConfig struct {
	Type         string `json:"type"`
	APIKey       string `json:"api_key"`
	HeaderName   string `json:"header_name"`
	TokenURL     string `json:"token_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Scope        string `json:"scope"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// ServerConfig is the canonical shape of mcp_server.config_json.
type ServerConfig struct {
	Transport              string            `json:"transport"`
	URL                    string            `json:"url"`
	Command                string            `json:"command"`
	Args                   []string          `json:"args"`
	Headers                map[string]string `json:"headers"`
	Env                    map[string]string `json:"env"`
	Auth                   AuthConfig        `json:"auth"`
	ToolPrefix             string            `json:"tool_prefix"`
	TimeoutSec             int               `json:"timeout_sec"`
	SessionReconnectMax    int               `json:"session_reconnect_max"`
	RequireUserCredentials bool              `json:"require_user_credentials"`
	AllowAdHocHTTP         bool              `json:"allow_adhoc_http"`
	AdHocTimeoutSec        int               `json:"adhoc_timeout_sec"`
}

// DefaultJSON returns "{}" when raw is empty.
func DefaultJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

// ParseServerConfigJSON unmarshals config_json; empty input becomes an empty config.
func ParseServerConfigJSON(raw string) (ServerConfig, error) {
	raw = DefaultJSON(raw)
	var c ServerConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return ServerConfig{}, err
	}
	return c, nil
}

// ToTRPCConnectionConfig is the SINGLE canonical mapping from platform ServerConfig
// to trpc-agent-go MCP connection settings. All runtime code paths (toolset.go,
// mcp_production.go) must call this instead of constructing trpcmcp.ConnectionConfig
// manually so transport/timeout/env stay aligned. TPM-P1-12.
//
// Note: Headers map is shared by reference; callers that mutate (e.g. inject
// Authorization) should clone first.
func ToTRPCConnectionConfig(sc ServerConfig) trpcmcp.ConnectionConfig {
	transport := NormalizeTransport(sc.Transport)
	if transport == "" {
		transport = TransportStdio
	}
	cfg := trpcmcp.ConnectionConfig{
		Transport: transport,
		ServerURL: strings.TrimSpace(sc.URL),
		Headers:   sc.Headers,
		Command:   strings.TrimSpace(sc.Command),
		Args:      sc.Args,
	}
	if sc.TimeoutSec > 0 {
		cfg.Timeout = DurationSec(sc.TimeoutSec)
	}
	return cfg
}

// DurationSec converts seconds to time.Duration; non-positive returns zero.
func DurationSec(sec int) time.Duration {
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

// Canonical transport string values used across probe / runtime / observer.
// Keep in sync with trpc-agent-go/tool/mcp/config.go which accepts both
// "streamable" and "streamable_http"; we always emit "streamable".
const (
	TransportStdio      = "stdio"
	TransportSSE        = "sse"
	TransportStreamable = "streamable"
)

// transportAliases is the single source of truth for transport name normalization.
// Add new aliases here so all callers (probe, runtime, observer, ToTRPCConnectionConfig)
// stay aligned. TPM-P1-10.
var transportAliases = map[string]string{
	"stdio":           TransportStdio,
	"sse":             TransportSSE,
	"streamable":      TransportStreamable,
	"streamable_http": TransportStreamable,
	"streamablehttp":  TransportStreamable,
	"http":            TransportStreamable, // backward compat with early configs
}

// NormalizeTransport maps any accepted UI/API transport spelling to its canonical
// value. Unknown values are returned as-is (lower-cased trimmed) so callers can
// validate via IsKnownTransport and return precise errors.
func NormalizeTransport(t string) string {
	key := strings.ToLower(strings.TrimSpace(t))
	if canon, ok := transportAliases[key]; ok {
		return canon
	}
	return key
}

// IsKnownTransport reports whether t (after normalization) is a supported transport.
func IsKnownTransport(t string) bool {
	switch NormalizeTransport(t) {
	case TransportStdio, TransportSSE, TransportStreamable:
		return true
	}
	return false
}

// KnownTransports returns canonical transport names for error messages and UI hints.
func KnownTransports() []string {
	return []string{TransportStdio, TransportSSE, TransportStreamable}
}

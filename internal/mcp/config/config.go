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

// ToTRPCConnectionConfig maps platform config to trpc-agent-go MCP connection settings.
func ToTRPCConnectionConfig(sc ServerConfig) trpcmcp.ConnectionConfig {
	cfg := trpcmcp.ConnectionConfig{
		Transport: NormalizeTransport(sc.Transport),
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

// NormalizeTransport maps UI/API transport names to trpc-agent-go values.
func NormalizeTransport(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "stdio":
		return "stdio"
	case "sse":
		return "sse"
	case "streamable_http", "streamable":
		return "streamable"
	default:
		return t
	}
}

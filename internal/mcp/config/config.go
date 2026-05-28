// Package config defines MCP server connection JSON stored in mcp_server.config_json.
package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Transport is a normalized MCP transport type. JSON deserialization automatically
// normalizes aliases (e.g. "streamable_http" → "streamable") so all consumers
// see a single canonical representation. TPM-P1-10.
type Transport string

const (
	TransportStdio      Transport = "stdio"
	TransportSSE        Transport = "sse"
	TransportStreamable Transport = "streamable"
)

// transportAliases is the single source of truth for transport name normalization.
var transportAliases = map[string]Transport{
	"stdio":           TransportStdio,
	"sse":             TransportSSE,
	"streamable":      TransportStreamable,
	"streamable_http": TransportStreamable,
	"streamablehttp":  TransportStreamable,
	"http":            TransportStreamable,
}

// ParseTransport normalizes and validates a transport string.
func ParseTransport(s string) (Transport, error) {
	key := strings.ToLower(strings.TrimSpace(s))
	if t, ok := transportAliases[key]; ok {
		return t, nil
	}
	return "", fmt.Errorf("unknown transport: %q (valid: %v)", s, KnownTransports())
}

// UnmarshalJSON implements json.Unmarshaler so any alias is auto-normalized.
func (t *Transport) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseTransport(s)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// MarshalJSON implements json.Marshaler to emit the canonical form.
func (t Transport) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(t))
}

// String implements fmt.Stringer.
func (t Transport) String() string { return string(t) }

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
	Transport              Transport         `json:"transport"`
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
	ProbeMode              string            `json:"probe_mode"`
}

// DefaultJSON returns "{}" when raw is empty.
func DefaultJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

// ParseServerConfigJSON unmarshals config_json; empty input becomes an empty config.
// Transport aliases are automatically normalized (e.g. "streamable_http" → "streamable").
func ParseServerConfigJSON(raw string) (ServerConfig, error) {
	raw = DefaultJSON(raw)
	var c ServerConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return ServerConfig{}, err
	}
	return c, nil
}

// DurationSec converts seconds to time.Duration; non-positive returns zero.
func DurationSec(sec int) time.Duration {
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

// NormalizeTransport maps any accepted UI/API transport spelling to its canonical
// value. Unknown values are returned as-is (lower-cased trimmed) so callers can
// validate via IsKnownTransport and return precise errors.
func NormalizeTransport(t string) string {
	key := strings.ToLower(strings.TrimSpace(t))
	if canon, ok := transportAliases[key]; ok {
		return string(canon)
	}
	return key
}

// IsKnownTransport reports whether t (after normalization) is a supported transport.
func IsKnownTransport(t string) bool {
	nt := NormalizeTransport(t)
	switch nt {
	case string(TransportStdio), string(TransportSSE), string(TransportStreamable):
		return true
	}
	return false
}

// KnownTransports returns canonical transport names for error messages and UI hints.
func KnownTransports() []string {
	return []string{string(TransportStdio), string(TransportSSE), string(TransportStreamable)}
}

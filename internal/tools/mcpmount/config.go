package mcpmount

import (
	"encoding/json"
	"strings"

	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

type AuthConfig struct {
	Type       string `json:"type"`
	APIKey     string `json:"api_key"`
	HeaderName string `json:"header_name"`
}

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
}

func parseServerConfigJSON(raw string) (ServerConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var c ServerConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return ServerConfig{}, err
	}
	return c, nil
}

func toTRPCConnectionConfig(sc ServerConfig) trpcmcp.ConnectionConfig {
	cfg := trpcmcp.ConnectionConfig{
		Transport: normalizeTransport(sc.Transport),
		ServerURL: strings.TrimSpace(sc.URL),
		Headers:   sc.Headers,
		Command:   strings.TrimSpace(sc.Command),
		Args:      sc.Args,
	}
	sec := sc.TimeoutSec
	if sec <= 0 {
		sec = 60
	}
	cfg.Timeout = parseDurationSec(sec)
	return cfg
}

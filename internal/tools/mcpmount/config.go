package mcpmount

import (
	"encoding/json"
	"strings"

	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

type ServerConfig struct {
	Transport              string            `json:"transport"`
	URL                    string            `json:"url"`
	Command                string            `json:"command"`
	Args                   []string          `json:"args"`
	Headers                map[string]string `json:"headers"`
	Env                    map[string]string `json:"env"`
	ToolPrefix             string            `json:"tool_prefix"`
	TimeoutSec             int               `json:"timeout_sec"`
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
	if sc.TimeoutSec > 0 {
		cfg.Timeout = parseDurationSec(sc.TimeoutSec)
	}
	return cfg
}

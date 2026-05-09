package mcpmount

import (
	"encoding/json"
	"strings"
)

// ServerConfig matches platform mcp_server.config_json (see internal/mcpprobe).
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

package runtime

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// InstanceConfig is parsed from channel.config_json for runtime decisions.
type InstanceConfig struct {
	Type        string
	ReceiveMode string
}

func ParseInstanceConfig(configJSON string, lg loggateway.Logger) InstanceConfig {
	var env struct {
		Type        string `json:"type"`
		ReceiveMode string `json:"receive_mode"`
	}
	if err := json.Unmarshal([]byte(configJSON), &env); err != nil {
		lg.Warn("解析 instance config 失败", loggateway.StepID("channel.runtime.config"), loggateway.Err(err))
	}
	return InstanceConfig{
		Type:        strings.TrimSpace(strings.ToLower(env.Type)),
		ReceiveMode: strings.TrimSpace(strings.ToLower(env.ReceiveMode)),
	}
}

// NeedsRuntimeConnector reports whether this channel instance requires a long-running goroutine.
func NeedsRuntimeConnector(ch biz.Channel, lg loggateway.Logger) bool {
	if !ch.Enabled || strings.TrimSpace(ch.DeletedAt) != "" {
		return false
	}
	cfg := ParseInstanceConfig(ch.ConfigJSON, lg)
	mode := cfg.ReceiveMode
	if mode == "" {
		mode = defaultReceiveMode(cfg.Type)
	}
	switch mode {
	case "webhook", "event", "onebot":
		return false
	case "websocket", "stream", "socket_mode", "polling", "gateway":
		return true
	default:
		return false
	}
}

func defaultReceiveMode(channelType string) string {
	switch strings.TrimSpace(strings.ToLower(channelType)) {
	case "feishu":
		return "websocket"
	case "dingtalk":
		return "webhook"
	case "slack":
		return "event"
	case "telegram":
		return "webhook"
	case "discord":
		return "gateway"
	case "personal_qq":
		return "onebot"
	case "line":
		return "webhook"
	case "mattermost":
		return "websocket"
	case "teams":
		return "webhook"
	default:
		return "webhook"
	}
}

func EffectiveReceiveMode(ch biz.Channel, lg loggateway.Logger) string {
	cfg := ParseInstanceConfig(ch.ConfigJSON, lg)
	if cfg.ReceiveMode != "" {
		return cfg.ReceiveMode
	}
	return defaultReceiveMode(cfg.Type)
}

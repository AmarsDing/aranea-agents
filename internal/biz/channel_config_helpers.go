package biz

import (
	"encoding/json"
	"strings"
)

const (
	defaultChannelAckMessage      = "收到，正在处理…"
	defaultChannelAckOnQueued     = "当前有任务进行中，你的消息已排队，将在当前任务完成后处理。"
	defaultChannelHeartbeatMsg    = "仍在处理中…"
	defaultChannelTurnTimeoutSec  = 0 // 0 = use service default (300s)
	defaultChannelFirstByteSec    = 0 // 0 = use service default (30s)
)

// ChannelLongTaskConfig holds IM long-running task settings from config_json.config.
type ChannelLongTaskConfig struct {
	AckMessage          string
	AckOnQueued         string
	TurnTimeoutSec      int
	FirstByteTimeoutSec int
	ProgressMode        string
	ProgressQuietSec    int
	HeartbeatMessage    string
	ExecutionMode       string
	AsyncGraphID        string
	AsyncTeamID         string
	AsyncCronTaskID     string
}

// ParseChannelLongTaskConfig reads long-task settings from channel config_json.
func ParseChannelLongTaskConfig(configJSON string) ChannelLongTaskConfig {
	cfg := ChannelLongTaskConfig{
		AckMessage:       defaultChannelAckMessage,
		AckOnQueued:      defaultChannelAckOnQueued,
		HeartbeatMessage: defaultChannelHeartbeatMsg,
		ProgressMode:     "off",
		ExecutionMode:    "sync",
	}
	var env struct {
		Config struct {
			AckMessage          *string `json:"ack_message"`
			AckOnQueued         *string `json:"ack_on_queued"`
			TurnTimeoutSec      *int    `json:"turn_timeout_sec"`
			FirstByteTimeoutSec *int    `json:"first_byte_timeout_sec"`
			ProgressMode        *string `json:"progress_mode"`
			ProgressQuietSec    *int    `json:"progress_quiet_sec"`
			HeartbeatMessage    *string `json:"heartbeat_message"`
			ExecutionMode       *string `json:"execution_mode"`
			AsyncGraphID        *string `json:"async_graph_id"`
			AsyncTeamID         *string `json:"async_team_id"`
			AsyncCronTaskID     *string `json:"async_cron_task_id"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(defaultJSON(configJSON)), &env) != nil {
		return cfg
	}
	if env.Config.AckMessage != nil {
		cfg.AckMessage = strings.TrimSpace(*env.Config.AckMessage)
	}
	if env.Config.AckOnQueued != nil {
		cfg.AckOnQueued = strings.TrimSpace(*env.Config.AckOnQueued)
	}
	if env.Config.TurnTimeoutSec != nil && *env.Config.TurnTimeoutSec > 0 {
		cfg.TurnTimeoutSec = *env.Config.TurnTimeoutSec
	}
	if env.Config.FirstByteTimeoutSec != nil && *env.Config.FirstByteTimeoutSec > 0 {
		cfg.FirstByteTimeoutSec = *env.Config.FirstByteTimeoutSec
	}
	if env.Config.ProgressMode != nil {
		cfg.ProgressMode = strings.TrimSpace(*env.Config.ProgressMode)
	}
	if env.Config.ProgressQuietSec != nil && *env.Config.ProgressQuietSec >= 0 {
		cfg.ProgressQuietSec = *env.Config.ProgressQuietSec
	}
	if env.Config.HeartbeatMessage != nil {
		cfg.HeartbeatMessage = strings.TrimSpace(*env.Config.HeartbeatMessage)
	}
	if env.Config.ExecutionMode != nil {
		cfg.ExecutionMode = strings.TrimSpace(*env.Config.ExecutionMode)
	}
	if env.Config.AsyncGraphID != nil {
		cfg.AsyncGraphID = strings.TrimSpace(*env.Config.AsyncGraphID)
	}
	if env.Config.AsyncTeamID != nil {
		cfg.AsyncTeamID = strings.TrimSpace(*env.Config.AsyncTeamID)
	}
	if env.Config.AsyncCronTaskID != nil {
		cfg.AsyncCronTaskID = strings.TrimSpace(*env.Config.AsyncCronTaskID)
	}
	return cfg
}

// ParseWeChatActiveMode reads config_json.config.active_mode for official account channels.
func ParseWeChatActiveMode(configJSON string) bool {
	var env struct {
		Config struct {
			ActiveMode bool `json:"active_mode"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(defaultJSON(configJSON)), &env) != nil {
		return false
	}
	return env.Config.ActiveMode
}

// ChannelSupportsLongTaskIngress reports whether webhook/runtime long-task features apply.
// WeChat passive reply must return XML in the HTTP response and cannot use outbound ACK/async.
func ChannelSupportsLongTaskIngress(platform, configJSON string) bool {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "wechat" && !ParseWeChatActiveMode(configJSON) {
		return false
	}
	return true
}

// ChannelStreamingEnabled reports config_json.config.streaming_enabled.
func ChannelStreamingEnabled(configJSON string) bool {
	var env struct {
		Config struct {
			StreamingEnabled bool `json:"streaming_enabled"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(defaultJSON(configJSON)), &env) != nil {
		return false
	}
	return env.Config.StreamingEnabled
}

// RenderChannelTemplate replaces {{key}} placeholders in outbound templates.
func RenderChannelTemplate(tmpl string, vars map[string]string) string {
	out := tmpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

// ProgressEnabled reports whether IM progress PATCH is active.
func (c ChannelLongTaskConfig) ProgressEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(c.ProgressMode)) {
	case "", "off":
		return false
	default:
		return true
	}
}

// ShouldRunAsync decides if inbound should dispatch Graph/Cron instead of sync Turn.
func (c ChannelLongTaskConfig) ShouldRunAsync(text string) bool {
	text = strings.TrimSpace(text)
	switch strings.ToLower(strings.TrimSpace(c.ExecutionMode)) {
	case "async":
		return c.AsyncGraphID != "" || c.AsyncTeamID != "" || c.AsyncCronTaskID != ""
	case "auto":
		return strings.HasPrefix(strings.ToLower(text), "/async") && (c.AsyncGraphID != "" || c.AsyncTeamID != "" || c.AsyncCronTaskID != "")
	default:
		return false
	}
}

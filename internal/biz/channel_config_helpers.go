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
	// DefaultContextAdmissionThreshold blocks new channel turns when session context exceeds this ratio (CH-BOR-11).
	DefaultContextAdmissionThreshold = 0.6
)

// DefaultChannelAsyncKeywords routes execution_mode=auto inbound to Job plane (CC-A-02).
var DefaultChannelAsyncKeywords = []string{"/async", "分析", "全量", "研报", "24h", "24小时"}

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
	AsyncKeywords       []string
	// RunPolicy (M55 §2.6 · CC-R-02)
	AutoEscalateAfterSoftBudget bool
	SoftEscalateConfirmSec      int
	DurableDeadlineSec          int
	BusyInputMode               string
	SessionMaxConcurrentDM      int
	SessionMaxConcurrentGroup   int
	// ContextAdmissionThreshold: 0 = disabled; unset defaults to DefaultContextAdmissionThreshold.
	ContextAdmissionThreshold float64
}

// ParseChannelLongTaskConfig reads long-task settings from channel config_json.
func ParseChannelLongTaskConfig(configJSON string) ChannelLongTaskConfig {
	cfg := ChannelLongTaskConfig{
		AckMessage:                defaultChannelAckMessage,
		AckOnQueued:               defaultChannelAckOnQueued,
		HeartbeatMessage:          defaultChannelHeartbeatMsg,
		ProgressMode:              "off",
		ExecutionMode:             "sync",
		ContextAdmissionThreshold: DefaultContextAdmissionThreshold,
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
			AsyncGraphID        *string   `json:"async_graph_id"`
			AsyncTeamID         *string   `json:"async_team_id"`
			AsyncCronTaskID     *string   `json:"async_cron_task_id"`
			AsyncKeywords       *[]string `json:"async_keywords"`
			BusyInputMode               *string `json:"busy_input_mode"`
			SessionMaxConcurrentDM      *int     `json:"session_max_concurrent_dm"`
			SessionMaxConcurrentGroup   *int     `json:"session_max_concurrent_group"`
			ContextAdmissionThreshold   *float64 `json:"context_admission_threshold"`
			AutoEscalateAfterSoftBudget *bool `json:"auto_escalate_after_soft_budget"`
			SoftEscalateConfirmSec      *int  `json:"soft_escalate_confirm_sec"`
			DurableDeadlineSec          *int  `json:"durable_deadline_sec"`
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
	if env.Config.AsyncKeywords != nil {
		cfg.AsyncKeywords = normalizeAsyncKeywords(*env.Config.AsyncKeywords)
	}
	if env.Config.AutoEscalateAfterSoftBudget != nil {
		cfg.AutoEscalateAfterSoftBudget = *env.Config.AutoEscalateAfterSoftBudget
	}
	if env.Config.SoftEscalateConfirmSec != nil && *env.Config.SoftEscalateConfirmSec > 0 {
		cfg.SoftEscalateConfirmSec = *env.Config.SoftEscalateConfirmSec
	}
	if env.Config.DurableDeadlineSec != nil && *env.Config.DurableDeadlineSec > 0 {
		cfg.DurableDeadlineSec = *env.Config.DurableDeadlineSec
	}
	if env.Config.BusyInputMode != nil {
		cfg.BusyInputMode = strings.TrimSpace(strings.ToLower(*env.Config.BusyInputMode))
	}
	if env.Config.SessionMaxConcurrentDM != nil && *env.Config.SessionMaxConcurrentDM > 0 {
		cfg.SessionMaxConcurrentDM = *env.Config.SessionMaxConcurrentDM
	}
	if env.Config.SessionMaxConcurrentGroup != nil && *env.Config.SessionMaxConcurrentGroup > 0 {
		cfg.SessionMaxConcurrentGroup = *env.Config.SessionMaxConcurrentGroup
	}
	if env.Config.ContextAdmissionThreshold != nil {
		if *env.Config.ContextAdmissionThreshold <= 0 {
			cfg.ContextAdmissionThreshold = 0
		} else {
			cfg.ContextAdmissionThreshold = *env.Config.ContextAdmissionThreshold
		}
	}
	return cfg
}

// ContextPressureActive reports whether session context usage should tighten admission (CH-BOR-11).
func ContextPressureActive(ratio, threshold float64) bool {
	return threshold > 0 && ratio >= threshold
}

const (
	defaultChannelMaxConcurrentDM    = 1
	defaultChannelMaxConcurrentGroup = 3
)

// ChannelBusyInputInterrupt reports config_json.config.busy_input_mode=interrupt (F-10).
func ChannelBusyInputInterrupt(configJSON string) bool {
	return ParseChannelLongTaskConfig(configJSON).BusyInputMode == "interrupt"
}

// ChannelBusyInputFollowup reports busy_input_mode=followup (CH-BOR-01).
func ChannelBusyInputFollowup(configJSON string) bool {
	return ParseChannelLongTaskConfig(configJSON).BusyInputMode == "followup"
}

// ChannelBusyInputQueue reports whether busy inbound should steer/enqueue (queue or followup).
func ChannelBusyInputQueue(configJSON string) bool {
	mode := ParseChannelLongTaskConfig(configJSON).BusyInputMode
	return mode == "" || mode == "queue" || mode == "followup"
}

// MaxConcurrentInbound returns DM/group concurrent turn cap for channel ingress (CH-BOR-02).
func (c ChannelLongTaskConfig) MaxConcurrentInbound(isGroup bool) int {
	if isGroup {
		if c.SessionMaxConcurrentGroup > 0 {
			return c.SessionMaxConcurrentGroup
		}
		return defaultChannelMaxConcurrentGroup
	}
	if c.SessionMaxConcurrentDM > 0 {
		return c.SessionMaxConcurrentDM
	}
	return defaultChannelMaxConcurrentDM
}

// DefaultSoftEscalateConfirmSec is the wait before auto-escalating after soft budget IM notice.
func DefaultSoftEscalateConfirmSec() int { return 60 }

// DefaultDurableDeadlineSec matches blueprint run_policy durable_deadline_sec.
func DefaultDurableDeadlineSec() int { return 86400 }

func (c ChannelLongTaskConfig) RunPolicy() SessionRunBudget {
	budget := DefaultSessionRunBudget()
	if c.TurnTimeoutSec > 0 && c.TurnTimeoutSec > budget.SoftBudgetSec {
		budget.HardBudgetSec = c.TurnTimeoutSec
	}
	return budget
}

func (c ChannelLongTaskConfig) SoftEscalateConfirmSecOrDefault() int {
	if c.SoftEscalateConfirmSec > 0 {
		return c.SoftEscalateConfirmSec
	}
	return DefaultSoftEscalateConfirmSec()
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

func normalizeAsyncKeywords(words []string) []string {
	out := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func (c ChannelLongTaskConfig) asyncKeywords() []string {
	if len(c.AsyncKeywords) > 0 {
		return c.AsyncKeywords
	}
	return DefaultChannelAsyncKeywords
}

func (c ChannelLongTaskConfig) hasAsyncTarget() bool {
	return c.AsyncGraphID != "" || c.AsyncTeamID != "" || c.AsyncCronTaskID != ""
}

func matchesChannelAsyncKeyword(text string, keywords []string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if strings.HasPrefix(kw, "/") {
			if strings.HasPrefix(lower, strings.ToLower(kw)) {
				return true
			}
			continue
		}
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// ShouldRunAsync decides if inbound should dispatch Graph/Cron instead of sync Turn.
// CC-R-05: only explicit execution_mode=async (or /async prefix) routes; keywords no longer route in auto mode.
func (c ChannelLongTaskConfig) ShouldRunAsync(text string) bool {
	if !c.hasAsyncTarget() {
		return false
	}
	text = strings.TrimSpace(text)
	switch strings.ToLower(strings.TrimSpace(c.ExecutionMode)) {
	case "async":
		return true
	case "auto":
		return strings.HasPrefix(strings.ToLower(text), "/async")
	default:
		return false
	}
}

const suggestDurableMinRuneLen = 4

// SuggestDurableRun reports whether inbound text looks like a long task (UX hint only, CC-R-05).
// Keyword matches only trigger when the message exceeds suggestDurableMinRuneLen runes,
// reducing false positives on very short messages like "分析".
func (c ChannelLongTaskConfig) SuggestDurableRun(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(text), "/async") || strings.HasPrefix(strings.ToLower(text), "/background") {
		return true
	}
	if len([]rune(text)) <= suggestDurableMinRuneLen {
		return false
	}
	return matchesChannelAsyncKeyword(text, c.asyncKeywords())
}

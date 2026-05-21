package biz

import (
	"encoding/json"
	"strings"
)

// HookConfig is the JSON shape stored in hooks.config_json (EP-CB-01).
type HookConfig struct {
	CallbackPoint string         `json:"callback_point"`
	Condition     HookCondition  `json:"condition"`
	Action        HookAction     `json:"action"`
}

// HookCondition scopes when a hook fires.
type HookCondition struct {
	AgentID   string `json:"agent_id"`
	ToolName  string `json:"tool_name"`
	EventType string `json:"event_type"`
}

// HookAction describes what to do when the hook fires.
type HookAction struct {
	Type             string         `json:"type"`
	WebhookURL       string         `json:"webhook_url"`
	ModifyPatch      map[string]any `json:"modify_patch"`
	LogLevel         string         `json:"log_level"`
	Message          string         `json:"message"`
	NotifyMaxRetries int            `json:"notify_max_retries"`
	NotifyTimeoutSec int            `json:"notify_timeout_sec"`
}

// ParseHookConfig unmarshals ConfigJSON; empty config is valid but has no point.
func ParseHookConfig(configJSON string) (HookConfig, error) {
	raw := strings.TrimSpace(configJSON)
	if raw == "" {
		return HookConfig{}, nil
	}
	var cfg HookConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return HookConfig{}, err
	}
	cfg.CallbackPoint = NormalizeCallbackPoint(cfg.CallbackPoint)
	return cfg, nil
}

// CallbackPoint returns the normalized lifecycle point from config_json.
func (h Hook) CallbackPoint() string {
	cfg, err := ParseHookConfig(h.ConfigJSON)
	if err != nil {
		return ""
	}
	return cfg.CallbackPoint
}

// HookConditionFromConfig returns condition fields from config_json.
func (h Hook) HookConditionFromConfig() HookCondition {
	cfg, err := ParseHookConfig(h.ConfigJSON)
	if err != nil {
		return HookCondition{}
	}
	return cfg.Condition
}

// HookActionFromConfig returns action fields from config_json.
func (h Hook) HookActionFromConfig() HookAction {
	cfg, err := ParseHookConfig(h.ConfigJSON)
	if err != nil {
		return HookAction{}
	}
	return cfg.Action
}

// NormalizeCallbackPoint maps aliases to canonical snake_case points.
func NormalizeCallbackPoint(point string) string {
	switch strings.ToLower(strings.TrimSpace(point)) {
	case "beforeagent", "before_agent":
		return "before_agent"
	case "afteragent", "after_agent":
		return "after_agent"
	case "beforemodel", "before_model":
		return "before_model"
	case "aftermodel", "after_model":
		return "after_model"
	case "beforetool", "before_tool":
		return "before_tool"
	case "aftertool", "after_tool":
		return "after_tool"
	case "onevent", "on_event":
		return "on_event"
	default:
		return strings.ToLower(strings.TrimSpace(point))
	}
}

// HookAppliesToAgent reports whether condition.agent_id matches the given ids.
func HookAppliesToAgent(cond HookCondition, agentID, agentKey string) bool {
	want := strings.TrimSpace(cond.AgentID)
	if want == "" {
		return true
	}
	if want == strings.TrimSpace(agentID) {
		return true
	}
	return want == strings.TrimSpace(agentKey)
}

// HookAppliesToTool reports whether condition.tool_name matches (empty = any).
func HookAppliesToTool(cond HookCondition, toolName string) bool {
	want := strings.TrimSpace(cond.ToolName)
	if want == "" {
		return true
	}
	return want == strings.TrimSpace(toolName)
}

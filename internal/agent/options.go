package agent

import (
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/agent/llmcompat"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"
)

// RFC3339Now returns a UTC RFC3339 timestamp for chat messages.
func RFC3339Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ProviderAPIConfig holds outbound HTTP credential hints deserialized from llm_provider_models.config_json.
type ProviderAPIConfig = llmcompat.ProviderAPIConfig

// MergeProviderConfigJSON overlays JSON config from LlmProviderModel.ConfigJSON.
var MergeProviderConfigJSON = llmcompat.MergeProviderConfigJSON

// ApplyThinkingCapability copies catalog capability_thinking onto ProviderAPIConfig.
var ApplyThinkingCapability = llmcompat.ApplyThinkingCapability

// TeamMemberAnchor is embedded into message options_json for team timelines.
type TeamMemberAnchor struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	TeamID  string `json:"team_id"` // 新增：标识消息属于哪个团队
}

func mathRoundCtxRatio(r float64) float64 {
	if r <= 0 {
		return 0
	}
	x := r * 100
	if x > 100 {
		return 100
	}
	return float64(int(x + 0.5))
}

// UserOptionsJSON builds options_json for an inbound user message.
func UserOptionsJSON(agent biz.Agent, dialogMode, provider, model string, ctxRatio float64, team *TeamMemberAnchor) (string, error) {
	opts := map[string]any{
		"dialog_mode": dialogMode,
		"provider":    provider,
		"model":       model,
		"agent": map[string]any{
			"id":           agent.ID,
			"agent_key":    agent.AgentKey,
			"display_name": agent.DisplayName,
			"name":         strutil.FirstNonEmpty(agent.DisplayName, agent.AgentKey),
			"icon":         agent.Icon,
		},
		"send_meta": map[string]any{
			// context_pct = session history ratio BEFORE this turn is sent.
			// It does NOT include the current turn's input tokens, so a new
			// session shows 0 even when the first prompt is large.
			"context_pct": mathRoundCtxRatio(ctxRatio),
		},
	}
	if team != nil && strings.TrimSpace(team.AgentID) != "" {
		opts["team_member"] = map[string]any{
			"agent_id": team.AgentID,
			"name":     team.Name,
			"role":     team.Role,
			"team_id":  team.TeamID, // 新增：传递 team_id
		}
	}
	raw, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// AssistantOptionsJSON builds options_json for an assistant message (catalog agent metadata).
func AssistantOptionsJSON(agent biz.Agent, team *TeamMemberAnchor) (string, error) {
	opts := map[string]any{
		"agent": map[string]any{
			"id":           agent.ID,
			"agent_key":    agent.AgentKey,
			"display_name": agent.DisplayName,
			"name":         strutil.FirstNonEmpty(agent.DisplayName, agent.AgentKey),
			"icon":         agent.Icon,
		},
	}
	if team != nil && strings.TrimSpace(team.AgentID) != "" {
		opts["team_member"] = map[string]any{
			"agent_id": team.AgentID,
			"name":     team.Name,
			"role":     team.Role,
			"team_id":  team.TeamID, // 新增：传递 team_id
		}
	}
	raw, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// ReasoningFromMessageOptionsJSON returns persisted provider reasoning (e.g. DeepSeek thinking mode) from message options_json.
func ReasoningFromMessageOptionsJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var opts map[string]any
	if json.Unmarshal([]byte(raw), &opts) != nil {
		return ""
	}
	v, ok := opts["reasoning_content"]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return ""
	}
}

// MergeReasoningIntoAssistantOptionsJSON merges reasoning into assistant options_json for UI replay and LLM history.
// Writes both reasoning_markdown (Chat UI) and reasoning_content (provider replay).
func MergeReasoningIntoAssistantOptionsJSON(base string, reasoning string) (string, error) {
	reasoning = strings.TrimSpace(reasoning)
	b := strings.TrimSpace(base)
	var opts map[string]any
	if b == "" {
		opts = map[string]any{}
	} else if err := json.Unmarshal([]byte(b), &opts); err != nil {
		return "", err
	}
	if reasoning == "" {
		delete(opts, "reasoning_content")
		delete(opts, "reasoning_markdown")
	} else {
		opts["reasoning_content"] = reasoning
		opts["reasoning_markdown"] = reasoning
	}
	out, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// MergeReasoningAsDisplayFlag sets the reasoning_as_display flag in options_json.
// When true, content_markdown is a reasoning fallback (LLM produced only reasoning, no separate reply).
// Frontend uses this to render ThinkActivity instead of SayActivity.
func MergeReasoningAsDisplayFlag(base string, flag bool) (string, error) {
	b := strings.TrimSpace(base)
	var opts map[string]any
	if b == "" {
		opts = map[string]any{}
	} else if err := json.Unmarshal([]byte(b), &opts); err != nil {
		return "", err
	}
	if flag {
		opts["reasoning_as_display"] = true
	} else {
		delete(opts, "reasoning_as_display")
	}
	out, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// MergeSourceIntoUserOptionsJSON stamps envelope source (web|channel|cron|a2a) for UserBubble badges (M55 CC-B-07).
func MergeSourceIntoUserOptionsJSON(optionsJSON, source string) (string, error) {
	return MergeInboundSourceIntoUserOptionsJSON(optionsJSON, source, "", "")
}

// MergeVoiceMetaIntoUserOptionsJSON stamps voice-input provenance (M74 V2-T6):
// input_modality="voice" + asr_provider + asr_duration_ms（空 provider / 零时长省略）。
func MergeVoiceMetaIntoUserOptionsJSON(optionsJSON, asrProvider string, durationMs int) (string, error) {
	opts := map[string]any{}
	if raw := strings.TrimSpace(optionsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &opts); err != nil {
			return optionsJSON, err
		}
	}
	opts["input_modality"] = "voice"
	if p := strings.TrimSpace(asrProvider); p != "" {
		opts["asr_provider"] = p
	}
	if durationMs > 0 {
		opts["asr_duration_ms"] = durationMs
	}
	out, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// MergeInboundSourceIntoUserOptionsJSON stamps source, platform, and channel_key (M55 Tier 0).
func MergeInboundSourceIntoUserOptionsJSON(optionsJSON, source, platform, channelKey string) (string, error) {
	source = strings.TrimSpace(source)
	platform = strings.TrimSpace(platform)
	channelKey = strings.TrimSpace(channelKey)
	if source == "" && platform == "" && channelKey == "" {
		return optionsJSON, nil
	}
	opts := map[string]any{}
	if raw := strings.TrimSpace(optionsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &opts); err != nil {
			return optionsJSON, err
		}
	}
	if source != "" {
		opts["source"] = source
	}
	if platform != "" {
		opts["platform"] = platform
	}
	if channelKey != "" {
		opts["channel_key"] = channelKey
	}
	out, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

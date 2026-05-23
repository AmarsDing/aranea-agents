package agent

import (
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"
)

// RFC3339Now returns a UTC RFC3339 timestamp for chat messages.
func RFC3339Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ProviderAPIConfig holds outbound HTTP credential hints deserialized from llm_provider_models.config_json.
type ProviderAPIConfig struct {
	ProviderType string `json:"provider_type"`
	APIBaseURL   string `json:"api_base_url"`
	APIKey       string `json:"api_key"`
}

// MergeProviderConfigJSON overlays JSON config from LlmProviderModel.ConfigJSON.
func MergeProviderConfigJSON(raw string, out *ProviderAPIConfig) {
	var c ProviderAPIConfig
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &c) != nil {
		return
	}
	if out.ProviderType == "" {
		out.ProviderType = c.ProviderType
	}
	if out.APIBaseURL == "" {
		out.APIBaseURL = c.APIBaseURL
	}
	if out.APIKey == "" {
		out.APIKey = c.APIKey
	}
}

// IsLikelyAnthropicNativeAPI is true when the configured base URL targets Anthropic's
// Messages API host. OpenAI-compatible /chat/completions proxies may still use
// provider_type "anthropic" for labeling — those must NOT be blocked here.
func IsLikelyAnthropicNativeAPI(baseURL, _ string) bool {
	b := strings.ToLower(strings.TrimSpace(baseURL))
	if strings.Contains(b, "openrouter") {
		return false
	}
	return strings.Contains(b, "api.anthropic.com")
}

// TeamMemberAnchor is embedded into message options_json for team timelines.
type TeamMemberAnchor struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
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
			"context_pct": mathRoundCtxRatio(ctxRatio),
		},
	}
	if team != nil && strings.TrimSpace(team.AgentID) != "" {
		opts["team_member"] = map[string]any{
			"agent_id": team.AgentID,
			"name":     team.Name,
			"role":     team.Role,
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

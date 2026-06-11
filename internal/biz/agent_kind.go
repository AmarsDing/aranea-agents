package biz

import (
	"encoding/json"
	"strings"

	"aranea-agents/pkg/loggateway"
)

const (
	AgentKindLLM      = "llm"
	AgentKindA2AProxy = "a2a_proxy"
)

// A2AProxyConfig holds remote A2A agent connection settings for agent_kind=a2a_proxy.
type A2AProxyConfig struct {
	RemoteURL       string
	AgentCardURL    string
	EnableStreaming bool
	AuthType        string
	AuthConfigJSON  string
	TimeoutSeconds  int
}

// NormalizeAgentKind returns llm for empty/unknown llm aliases; a2a_proxy for proxy aliases.
func NormalizeAgentKind(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", "llm", "open":
		return AgentKindLLM
	case AgentKindA2AProxy, "a2a", "a2a-proxy":
		return AgentKindA2AProxy
	default:
		return AgentKindLLM
	}
}

// IsA2AProxyAgent reports whether the agent should be built with a2aagent.
func IsA2AProxyAgent(ag Agent) bool {
	return NormalizeAgentKind(ag.AgentKind) == AgentKindA2AProxy
}

func agentKindFromConfigJSON(configJSON string) string {
	configJSON = strings.TrimSpace(configJSON)
	if configJSON == "" {
		return AgentKindLLM
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		return AgentKindLLM
	}
	if v, ok := m["agent_kind"].(string); ok {
		return NormalizeAgentKind(v)
	}
	return AgentKindLLM
}

func a2aProxyFromConfigJSON(configJSON string) *A2AProxyConfig {
	configJSON = strings.TrimSpace(configJSON)
	if configJSON == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		return nil
	}
	raw, ok := m["a2a_proxy"].(map[string]any)
	if !ok || raw == nil {
		return nil
	}
	cfg := &A2AProxyConfig{}
	if v, ok := raw["remote_url"].(string); ok {
		cfg.RemoteURL = strings.TrimSpace(v)
	}
	if v, ok := raw["agent_card_url"].(string); ok {
		cfg.AgentCardURL = strings.TrimSpace(v)
	}
	if v, ok := raw["enable_streaming"].(bool); ok {
		cfg.EnableStreaming = v
	}
	if v, ok := raw["auth_type"].(string); ok {
		cfg.AuthType = strings.TrimSpace(v)
	}
	if v, ok := raw["auth_config_json"].(string); ok {
		cfg.AuthConfigJSON = v
	}
	if v, ok := raw["timeout_seconds"].(float64); ok {
		cfg.TimeoutSeconds = int(v)
	}
	if cfg.RemoteURL == "" && cfg.AgentCardURL == "" {
		return nil
	}
	return cfg
}

// HydrateAgentKind reads agent_kind and a2a_proxy from config_json when AgentKind is empty.
func HydrateAgentKind(a *Agent) {
	if a == nil {
		return
	}
	if strings.TrimSpace(a.AgentKind) == "" {
		a.AgentKind = agentKindFromConfigJSON(a.ConfigJSON)
	} else {
		a.AgentKind = NormalizeAgentKind(a.AgentKind)
	}
	if a.A2AProxy == nil {
		a.A2AProxy = a2aProxyFromConfigJSON(a.ConfigJSON)
	}
}

// EmbedAgentKindInConfigJSON merges agent_kind and a2a_proxy into config_json.
func EmbedAgentKindInConfigJSON(configJSON, kind string, proxy *A2AProxyConfig, lg loggateway.Logger) string {
	kind = NormalizeAgentKind(kind)
	var m map[string]any
	if strings.TrimSpace(configJSON) != "" {
		if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
			// Preserve the original config_json on parse failure instead of
			// discarding it. Return the original with agent_kind appended as
			// a best-effort fallback, or just return the original if we can't
			// even do that.
			lg.Warn("解析 config_json 失败，保留原始内容", loggateway.StepID("agent_kind.embed"), loggateway.Err(err))
			return configJSON
		}
	}
	if m == nil {
		m = map[string]any{}
	}
	if kind == "" {
		kind = AgentKindLLM
	}
	m["agent_kind"] = kind
	if kind == AgentKindA2AProxy && proxy != nil {
		entry := map[string]any{
			"remote_url": proxy.RemoteURL,
		}
		if proxy.AgentCardURL != "" {
			entry["agent_card_url"] = proxy.AgentCardURL
		}
		if proxy.EnableStreaming {
			entry["enable_streaming"] = true
		}
		if proxy.AuthType != "" {
			entry["auth_type"] = proxy.AuthType
		}
		if proxy.AuthConfigJSON != "" {
			entry["auth_config_json"] = proxy.AuthConfigJSON
		}
		if proxy.TimeoutSeconds > 0 {
			entry["timeout_seconds"] = proxy.TimeoutSeconds
		}
		m["a2a_proxy"] = entry
	} else {
		delete(m, "a2a_proxy")
	}
	out, err := json.Marshal(m)
	if err != nil {
		return configJSON
	}
	return string(out)
}

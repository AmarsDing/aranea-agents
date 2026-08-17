package biz

import (
	"encoding/json"
	"strings"
)

// AgentKnowledgeConfig is stored under config_json.knowledge (no Ent column).
// GroundedOnly: the agent must answer from retrieved knowledge passages only
// and refuse when the knowledge base has no evidence.
type AgentKnowledgeConfig struct {
	GroundedOnly bool `json:"grounded_only"`
}

// ParseAgentKnowledgeConfig reads config_json.knowledge.
func ParseAgentKnowledgeConfig(configJSON string) AgentKnowledgeConfig {
	configJSON = strings.TrimSpace(configJSON)
	if configJSON == "" || configJSON == "{}" {
		return AgentKnowledgeConfig{}
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &root); err != nil {
		return AgentKnowledgeConfig{}
	}
	raw, ok := root["knowledge"]
	if !ok {
		return AgentKnowledgeConfig{}
	}
	var cfg AgentKnowledgeConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return AgentKnowledgeConfig{}
	}
	return cfg
}

package team

import (
	"encoding/json"
	"strings"
)

// Definition mirrors team DefinitionJSON (subset used by native runner).
type Definition struct {
	Version            int            `json:"version"`
	Mode               string         `json:"mode"`
	SynthesizerAgentID string         `json:"synthesizer_agent_id"`
	Members            []MemberDef    `json:"members"`
	MaxConcurrency     int            `json:"max_concurrency"`
	TimeoutSeconds     int            `json:"timeout_seconds"`
}

// MemberDef is one team member entry in DefinitionJSON.
type MemberDef struct {
	AgentID string `json:"agent_id"`
	Role    string `json:"role"`
	Enabled *bool  `json:"enabled"`
}

// ParseDefinition unmarshals team JSON; empty string yields default sequential with no members.
func ParseDefinition(raw string) (Definition, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Definition{Version: 1, Mode: "sequential"}, nil
	}
	var d Definition
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return Definition{}, err
	}
	if strings.TrimSpace(d.Mode) == "" {
		d.Mode = "sequential"
	}
	return d, nil
}

func memberEnabled(m MemberDef) bool {
	return m.Enabled == nil || *m.Enabled
}

// EnabledMembers returns members in order that are enabled and have agent_id.
func EnabledMembers(d Definition) []MemberDef {
	var out []MemberDef
	for _, m := range d.Members {
		if !memberEnabled(m) || strings.TrimSpace(m.AgentID) == "" {
			continue
		}
		out = append(out, m)
	}
	return out
}

// SynthesizerAgentID resolves synthesizer from definition or member role.
func SynthesizerAgentID(d Definition) string {
	if id := strings.TrimSpace(d.SynthesizerAgentID); id != "" {
		return id
	}
	for _, m := range d.Members {
		if !memberEnabled(m) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(m.Role), "synthesizer") {
			return strings.TrimSpace(m.AgentID)
		}
	}
	return ""
}

// ParallelWorkers excludes the synthesizer agent id from concurrent workers when set.
func ParallelWorkers(d Definition) []MemberDef {
	synth := strings.TrimSpace(SynthesizerAgentID(d))
	var out []MemberDef
	for _, m := range EnabledMembers(d) {
		if synth != "" && strings.TrimSpace(m.AgentID) == synth {
			continue
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return EnabledMembers(d)
	}
	return out
}

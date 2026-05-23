package team

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
)

// applyTeamRuntimeExecutionOptions enables checkpoint/HITL/interrupt for Graph team runs (M53 Phase 6).
func applyTeamRuntimeExecutionOptions(cfg biz.GraphBuildConfig, def Definition, rawDefinitionJSON string) biz.GraphBuildConfig {
	cfg.EnableCheckpoint = true
	if spec, ok := parseOrchestrationCheckpoint(rawDefinitionJSON); ok && !spec {
		cfg.EnableCheckpoint = false
	}
	before, after := collectGraphInterrupts(rawDefinitionJSON)
	if len(before) > 0 {
		cfg.InterruptBefore = appendUniqueStrings(cfg.InterruptBefore, before...)
	}
	if len(after) > 0 {
		cfg.InterruptAfter = appendUniqueStrings(cfg.InterruptAfter, after...)
	}
	applyEmbeddedNodePolicies(&cfg, rawDefinitionJSON)
	if def.FailurePolicy != nil && def.FailurePolicy.CircuitBreaker != nil {
		cfg = biz.ApplyCircuitBreakerPolicy(cfg, def.FailurePolicy.CircuitBreaker)
	}
	_ = def
	return cfg
}

func parseOrchestrationCheckpoint(raw string) (bool, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true, true
	}
	var body struct {
		EnableCheckpoint *bool `json:"enable_checkpoint"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return true, false
	}
	if body.EnableCheckpoint == nil {
		return true, true
	}
	return *body.EnableCheckpoint, true
}

type graphNodePolicy struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	InterruptBefore  bool     `json:"interrupt_before"`
	InterruptAfter   bool     `json:"interrupt_after"`
	Destinations     []string `json:"destinations"`
	RetryMaxAttempts int      `json:"retry_max_attempts"`
	FallbackAgent    string   `json:"fallback_agent"`
}

func collectGraphInterrupts(raw string) (before, after []string) {
	for _, n := range parseGraphNodes(raw) {
		id := strings.TrimSpace(n.ID)
		if id == "" {
			continue
		}
		if n.InterruptBefore {
			before = append(before, id)
		}
		if n.InterruptAfter {
			after = append(after, id)
		}
	}
	return before, after
}

func applyEmbeddedNodePolicies(cfg *biz.GraphBuildConfig, raw string) {
	if cfg == nil {
		return
	}
	policies := map[string]graphNodePolicy{}
	for _, n := range parseGraphNodes(raw) {
		id := strings.TrimSpace(n.ID)
		if id == "" {
			continue
		}
		policies[id] = n
	}
	for i := range cfg.Nodes {
		id := cfg.Nodes[i].ID
		n, ok := policies[id]
		if !ok {
			continue
		}
		if n.InterruptBefore {
			cfg.Nodes[i].InterruptBefore = true
		}
		if n.InterruptAfter {
			cfg.Nodes[i].InterruptAfter = true
		}
		if len(n.Destinations) > 0 {
			cfg.Nodes[i].Destinations = append([]string(nil), n.Destinations...)
		}
		if n.RetryMaxAttempts > 0 {
			cfg.Nodes[i].RetryMaxAttempts = n.RetryMaxAttempts
		}
		if fb := strings.TrimSpace(n.FallbackAgent); fb != "" {
			cfg.Nodes[i].FallbackAgent = fb
		}
	}
}

func parseGraphNodes(raw string) []graphNodePolicy {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var body struct {
		Graph struct {
			Nodes []graphNodePolicy `json:"nodes"`
		} `json:"graph"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil
	}
	return body.Graph.Nodes
}

func appendUniqueStrings(base []string, extra ...string) []string {
	seen := map[string]struct{}{}
	for _, s := range base {
		seen[s] = struct{}{}
	}
	for _, s := range extra {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		base = append(base, s)
	}
	return base
}

package team

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
)

// applyTeamRuntimeExecutionOptions enables checkpoint/HITL/interrupt for Graph team runs (M53 Phase 6).
type parsedRuntimeOptions struct {
	EnableCheckpoint  bool
	CheckpointPresent bool
	Nodes             []graphNodePolicy
}

func parseRuntimeOptions(raw string) parsedRuntimeOptions {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return parsedRuntimeOptions{EnableCheckpoint: true, CheckpointPresent: true}
	}
	var body struct {
		EnableCheckpoint *bool `json:"enable_checkpoint"`
		Graph            struct {
			Nodes []graphNodePolicy `json:"nodes"`
		} `json:"graph"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return parsedRuntimeOptions{EnableCheckpoint: true, CheckpointPresent: false}
	}
	opts := parsedRuntimeOptions{CheckpointPresent: true}
	if body.EnableCheckpoint != nil {
		opts.EnableCheckpoint = *body.EnableCheckpoint
	} else {
		opts.EnableCheckpoint = true
	}
	opts.Nodes = body.Graph.Nodes
	return opts
}

func applyTeamRuntimeExecutionOptions(cfg biz.GraphBuildConfig, def Definition, rawDefinitionJSON string) biz.GraphBuildConfig {
	opts := parseRuntimeOptions(rawDefinitionJSON)
	cfg.EnableCheckpoint = true
	if opts.CheckpointPresent && !opts.EnableCheckpoint {
		cfg.EnableCheckpoint = false
	}
	before, after := collectGraphInterruptsFromNodes(opts.Nodes)
	if len(before) > 0 {
		cfg.InterruptBefore = appendUniqueStrings(cfg.InterruptBefore, before...)
	}
	if len(after) > 0 {
		cfg.InterruptAfter = appendUniqueStrings(cfg.InterruptAfter, after...)
	}
	applyEmbeddedNodePoliciesFromNodes(&cfg, opts.Nodes)
	if def.FailurePolicy != nil && def.FailurePolicy.CircuitBreaker != nil {
		bizPolicy := failurePolicyToBiz(def.FailurePolicy).CircuitBreaker
		cfg = biz.ApplyCircuitBreakerPolicy(cfg, bizPolicy)
	}
	return cfg
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
	return collectGraphInterruptsFromNodes(parseGraphNodes(raw))
}

func collectGraphInterruptsFromNodes(nodes []graphNodePolicy) (before, after []string) {
	for _, n := range nodes {
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
	applyEmbeddedNodePoliciesFromNodes(cfg, parseGraphNodes(raw))
}

func applyEmbeddedNodePoliciesFromNodes(cfg *biz.GraphBuildConfig, nodes []graphNodePolicy) {
	if cfg == nil {
		return
	}
	policies := map[string]graphNodePolicy{}
	for _, n := range nodes {
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
		cfg.Nodes[i].InterruptBefore = n.InterruptBefore
		cfg.Nodes[i].InterruptAfter = n.InterruptAfter
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

package team

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

// Definition mirrors team DefinitionJSON (subset used by native runner).
type Definition struct {
	Version            int               `json:"version"`
	Mode               string            `json:"mode"`
	SynthesizerAgentID string            `json:"synthesizer_agent_id"`
	Members            []MemberDef       `json:"members"`
	MaxConcurrency     int               `json:"max_concurrency"`
	TimeoutSeconds     int               `json:"timeout_seconds"`
	LoopMaxIterations  int               `json:"loop_max_iterations,omitempty"`
	RuntimeEngine      string            `json:"runtime_engine,omitempty"`
	TeamGraphRuntime   bool              `json:"team_graph_runtime,omitempty"`
	CriticLoop         *CriticLoopConfig `json:"critic_loop,omitempty"`
	IntentAnchorAgentID string                `json:"intent_anchor_agent_id,omitempty"`
	Swarm               *SwarmConfigDef        `json:"swarm,omitempty"`
	MemberTool          *MemberToolDef         `json:"member_tool_config,omitempty"`
	FailurePolicy       *biz.TeamFailurePolicy `json:"failure_policy,omitempty"`
}

type SwarmConfigDef struct {
	MaxHandoffs                int  `json:"max_handoffs"`
	NodeTimeoutSeconds         int  `json:"node_timeout_seconds"`
	RepetitiveHandoffWindow    int  `json:"repetitive_handoff_window"`
	RepetitiveHandoffMinUnique int  `json:"repetitive_handoff_min_unique"`
	CrossRequestTransfer       bool `json:"cross_request_transfer"`
}

type MemberToolDef struct {
	StreamInner       bool   `json:"stream_inner"`
	InnerTextMode     string `json:"inner_text_mode"`
	SkipSummarization bool   `json:"skip_summarization"`
	HistoryScope      string `json:"history_scope"`
	ToolSetName       string `json:"tool_set_name"`
}

// CriticLoopConfig matches frontend critic_loop JSON (score_threshold reserved for future routing).
type CriticLoopConfig struct {
	MaxIterations  int     `json:"max_iterations"`
	ScoreThreshold float64 `json:"score_threshold"`
}

// MemberDef is one team member entry in DefinitionJSON.
type MemberDef struct {
	AgentID    string `json:"agent_id"`
	Role       string `json:"role"`
	Enabled    *bool  `json:"enabled"`
	SortOrder  int    `json:"sort_order"`
	Name       string `json:"name"`
	TaskPrompt string `json:"task_prompt,omitempty"`
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

type memberWithIndex struct {
	m MemberDef
	i int
}

// EnabledMembers returns enabled members with non-empty agent_id, ordered by sort_order then declaration order.
func EnabledMembers(d Definition) []MemberDef {
	var pairs []memberWithIndex
	for i, m := range d.Members {
		if !memberEnabled(m) || strings.TrimSpace(m.AgentID) == "" {
			continue
		}
		pairs = append(pairs, memberWithIndex{m: m, i: i})
	}
	sort.SliceStable(pairs, func(a, b int) bool {
		sa, sb := pairs[a].m.SortOrder, pairs[b].m.SortOrder
		switch {
		case sa > 0 && sb > 0 && sa != sb:
			return sa < sb
		case sa > 0 && sb <= 0:
			return true
		case sa <= 0 && sb > 0:
			return false
		default:
			return pairs[a].i < pairs[b].i
		}
	})
	out := make([]MemberDef, len(pairs))
	for j := range pairs {
		out[j] = pairs[j].m
	}
	return out
}

// SynthesizerAgentID resolves synthesizer from definition or member role.
func SynthesizerAgentID(d Definition) string {
	if id := strings.TrimSpace(d.SynthesizerAgentID); id != "" {
		return id
	}
	for _, m := range EnabledMembers(d) {
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

const (
	teamTurnMinSeconds = 120
	teamTurnMaxSeconds = 7200
)

// TurnDeadlineDuration bounds wall-clock time for the native team Run phase (orchestrated LLM
// workflow only), not intent preflight or workflow build. Intent uses its own short timeout (see intent.Run).
// Zero means do not add an extra deadline (still subject to parent context / gateway limits).
func TurnDeadlineDuration(d Definition) time.Duration {
	sec := d.TimeoutSeconds
	if sec <= 0 {
		return 0
	}
	if sec < teamTurnMinSeconds {
		sec = teamTurnMinSeconds
	}
	if sec > teamTurnMaxSeconds {
		sec = teamTurnMaxSeconds
	}
	return time.Duration(sec) * time.Second
}

package biz

import (
	"encoding/json"
	"strings"
)

const OrchestrationSpecVersion = 2

// OrchestrationSpec is the v2 strong-typed team orchestration document (TG-CMP-V2).
type OrchestrationSpec struct {
	Version               int                  `json:"version"`
	Mode                  string               `json:"mode"`
	Members               []OrchestrationMember `json:"members"`
	Graph                 *EmbeddedGraphSpec   `json:"graph,omitempty"`
	LinkedGraphID         string               `json:"linked_graph_id,omitempty"`
	FailurePolicy         *TeamFailurePolicy   `json:"failure_policy,omitempty"`
	RuntimeEngine         string               `json:"runtime_engine,omitempty"`
	TeamGraphRuntime      bool                 `json:"team_graph_runtime,omitempty"`
	TurnTimeoutSec        int                  `json:"turn_timeout_sec,omitempty"`
	FirstByteTimeoutSec   int                  `json:"first_byte_timeout_sec,omitempty"`
	IntentAnchorAgentID   string               `json:"intent_anchor_agent_id,omitempty"`
	Description           string               `json:"description,omitempty"`
	MaxConcurrency        int                  `json:"max_concurrency,omitempty"`
	TimeoutSeconds        int                  `json:"timeout_seconds,omitempty"`
	LoopMaxIterations     int                  `json:"loop_max_iterations,omitempty"`
	SynthesizerAgentID    string               `json:"synthesizer_agent_id,omitempty"`
	CriticLoop            *CriticLoopSpec      `json:"critic_loop,omitempty"`
	EnableCheckpoint      bool                 `json:"enable_checkpoint,omitempty"`
}

type OrchestrationMember struct {
	AgentID    string `json:"agent_id"`
	Role       string `json:"role"`
	Name       string `json:"name"`
	TaskPrompt string `json:"task_prompt,omitempty"`
	Enabled    bool   `json:"enabled"`
	SortOrder  int    `json:"sort_order"`
}

type EmbeddedGraphSpec struct {
	Version int                    `json:"version"`
	Layout  string                 `json:"layout"`
	Nodes   []EmbeddedGraphNodeSpec `json:"nodes"`
	Edges   []EmbeddedGraphEdgeSpec `json:"edges"`
}

type EmbeddedGraphNodeSpec struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	Label            string   `json:"label"`
	AgentID          string   `json:"agent_id,omitempty"`
	Role             string   `json:"role,omitempty"`
	InterruptBefore  bool     `json:"interrupt_before,omitempty"`
	InterruptAfter   bool     `json:"interrupt_after,omitempty"`
	Destinations     []string `json:"destinations,omitempty"`
	RetryMaxAttempts int      `json:"retry_max_attempts,omitempty"`
	FallbackAgent    string   `json:"fallback_agent,omitempty"`
}

type EmbeddedGraphEdgeSpec struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Label     string `json:"label,omitempty"`
	Condition string `json:"condition,omitempty"`
}

type CriticLoopSpec struct {
	MaxIterations  int     `json:"max_iterations"`
	ScoreThreshold float64 `json:"score_threshold"`
}

// CircuitBreakerPolicy configures node-level circuit breaking (M53 FP-02).
type CircuitBreakerPolicy struct {
	FailureThreshold    int    `json:"failure_threshold"`
	ResetTimeoutSeconds int    `json:"reset_timeout_seconds"`
	FallbackNode        string `json:"fallback_node,omitempty"`
}

// ParseOrchestrationSpec unmarshals definition_json into OrchestrationSpec v2.
func ParseOrchestrationSpec(raw string) (OrchestrationSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultOrchestrationSpec(), nil
	}
	var spec OrchestrationSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return OrchestrationSpec{}, err
	}
	NormalizeOrchestrationSpec(&spec)
	return spec, nil
}

// DefaultOrchestrationSpec returns v2 defaults for new teams (TG-RT-DEFAULT).
func DefaultOrchestrationSpec() OrchestrationSpec {
	return OrchestrationSpec{
		Version:          OrchestrationSpecVersion,
		Mode:             "sequential",
		RuntimeEngine:    "graph",
		TeamGraphRuntime: true,
		MaxConcurrency:   2,
		TimeoutSeconds:   600,
		Members:          []OrchestrationMember{},
	}
}

// NormalizeOrchestrationSpec fills defaults without dropping unknown JSON keys on round-trip.
func NormalizeOrchestrationSpec(spec *OrchestrationSpec) {
	if spec == nil {
		return
	}
	if spec.Version <= 0 {
		spec.Version = 1
	}
	if strings.TrimSpace(spec.Mode) == "" {
		spec.Mode = "sequential"
	}
	engine := strings.ToLower(strings.TrimSpace(spec.RuntimeEngine))
	if engine == "graph" || spec.TeamGraphRuntime {
		spec.RuntimeEngine = "graph"
		spec.TeamGraphRuntime = true
	} else if engine == "" && spec.Version >= OrchestrationSpecVersion {
		spec.RuntimeEngine = "graph"
		spec.TeamGraphRuntime = true
	} else if engine == "" {
		spec.RuntimeEngine = "graph"
		spec.TeamGraphRuntime = true
	} else {
		spec.RuntimeEngine = engine
	}
	if spec.MaxConcurrency <= 0 {
		spec.MaxConcurrency = 2
	}
	if spec.TimeoutSeconds <= 0 {
		spec.TimeoutSeconds = 600
	}
}

// OrchestrationSpecToDefinitionJSON serializes spec to definition_json (v2 canonical).
func OrchestrationSpecToDefinitionJSON(spec OrchestrationSpec) (string, error) {
	NormalizeOrchestrationSpec(&spec)
	if spec.Version < OrchestrationSpecVersion {
		spec.Version = OrchestrationSpecVersion
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// MergeOrchestrationSpecIntoDefinition applies v2 spec onto raw JSON, preserving unknown keys.
func MergeOrchestrationSpecIntoDefinition(raw string, spec OrchestrationSpec) (string, error) {
	base := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &base); err != nil {
			return "", err
		}
	}
	canonical, err := OrchestrationSpecToDefinitionJSON(spec)
	if err != nil {
		return "", err
	}
	var overlay map[string]any
	if err := json.Unmarshal([]byte(canonical), &overlay); err != nil {
		return "", err
	}
	for k, v := range overlay {
		base[k] = v
	}
	b, err := json.Marshal(base)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// EnsureGraphRuntimeDefault sets runtime_engine=graph for new teams when unset (TG-RT-DEFAULT).
func EnsureGraphRuntimeDefault(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		out, _ := OrchestrationSpecToDefinitionJSON(DefaultOrchestrationSpec())
		return out
	}
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		return raw
	}
	engine := strings.ToLower(strings.TrimSpace(spec.RuntimeEngine))
	if engine == "graph" || spec.TeamGraphRuntime {
		return raw
	}
	if engine == "native" && runtimeEngineExplicitlySet(raw) {
		return raw
	}
	spec.RuntimeEngine = "graph"
	spec.TeamGraphRuntime = true
	if spec.Version < OrchestrationSpecVersion {
		spec.Version = OrchestrationSpecVersion
	}
	out, err := MergeOrchestrationSpecIntoDefinition(raw, spec)
	if err != nil {
		return raw
	}
	return out
}

func runtimeEngineExplicitlySet(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return false
	}
	_, ok := body["runtime_engine"]
	return ok
}

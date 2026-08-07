package biz

import (
	"encoding/json"
	"strings"
)

const OrchestrationSpecVersion = 2

// Definition graph source values for OrchestrationSpec.Source（Team × Graph 一体化，Phase 11）。
const (
	// DefinitionGraphSourcePreset：拓扑由 Team 表单（mode/members）派生物化。
	DefinitionGraphSourcePreset = "preset"
	// DefinitionGraphSourceCustom：拓扑在 Graph 编辑器中被手改过。
	DefinitionGraphSourceCustom = "custom"
	// DefinitionGraphSourceLinkedExt：拓扑关联自独立 Graph 资产（非本 team 拥有）。
	DefinitionGraphSourceLinkedExt = "linked_external"
)

// OrchestrationSpec is the v2 strong-typed team orchestration document (TG-CMP-V2).
type OrchestrationSpec struct {
	Version       int                   `json:"version"`
	Mode          string                `json:"mode"`
	Members       []OrchestrationMember `json:"members"`
	Graph         *EmbeddedGraphSpec    `json:"graph,omitempty"`
	LinkedGraphID string                `json:"linked_graph_id,omitempty"`
	// Source 标记拓扑来源（preset/custom/linked_external），缺省按 preset 处理（见 GraphSource）。
	Source              string             `json:"source,omitempty"`
	FailurePolicy       *TeamFailurePolicy `json:"failure_policy,omitempty"`
	RuntimeEngine       string             `json:"runtime_engine,omitempty"`
	TeamGraphRuntime    bool               `json:"team_graph_runtime,omitempty"`
	TurnTimeoutSec      int                `json:"turn_timeout_sec,omitempty"`
	FirstByteTimeoutSec int                `json:"first_byte_timeout_sec,omitempty"`
	IntentAnchorAgentID string             `json:"intent_anchor_agent_id,omitempty"`
	Description         string             `json:"description,omitempty"`
	MaxConcurrency      int                `json:"max_concurrency,omitempty"`
	// Deprecated: use RunTimeoutSec
	TimeoutSeconds     int             `json:"timeout_seconds,omitempty"`
	RunTimeoutSec      int             `json:"run_timeout_sec,omitempty"`
	LoopMaxIterations  int             `json:"loop_max_iterations,omitempty"`
	SynthesizerAgentID string          `json:"synthesizer_agent_id,omitempty"`
	CriticLoop         *CriticLoopSpec `json:"critic_loop,omitempty"`
	EnableCheckpoint   bool            `json:"enable_checkpoint,omitempty"`
	// EnableStateDeliverable / DeliverableContract / VerificationGates 是
	// Spirit 装配链路写入 definition_json 的交付物通道字段（C1/C3/F5/F9）。
	// 2026-08-07 根因：本结构此前缺少这三个字段，materializeAndBind 经
	// OrchestrationSpecToDefinitionJSON 重序列化时将其静默丢弃，导致 DAG
	// 团队运行期无 set_deliverable 工具、真实交付物闸门判失败、下游节点
	// 永不派发。字段必须随 spec 往返保留。
	EnableStateDeliverable bool                       `json:"enable_state_deliverable,omitempty"`
	DeliverableContract    *MemberDeliverableContract `json:"deliverable_contract,omitempty"`
	VerificationGates      []VerificationGate         `json:"verification_gates,omitempty"`
}

// GraphSource returns the effective definition graph source (default preset).
func (s OrchestrationSpec) GraphSource() string {
	switch strings.TrimSpace(s.Source) {
	case DefinitionGraphSourceCustom:
		return DefinitionGraphSourceCustom
	case DefinitionGraphSourceLinkedExt:
		return DefinitionGraphSourceLinkedExt
	default:
		return DefinitionGraphSourcePreset
	}
}

type OrchestrationMember struct {
	AgentID    string `json:"agent_id"`
	Role       string `json:"role"`
	Name       string `json:"name"`
	TaskPrompt string `json:"task_prompt,omitempty"`
	// EnabledPtr is the raw JSON value: nil means "not set" (defaults to true
	// after normalization). Use Enabled() to get the resolved boolean.
	EnabledPtr *bool `json:"enabled"`
	SortOrder  int   `json:"sort_order"`
}

// Enabled returns the resolved enabled state. When EnabledPtr is nil (field
// absent from JSON), the member is considered enabled by default — this
// preserves backward compatibility with the *bool semantics used by the
// original teamDefinition and definitionForUpdate structs.
func (m OrchestrationMember) Enabled() bool {
	return m.EnabledPtr == nil || *m.EnabledPtr
}

type EmbeddedGraphSpec struct {
	Version int                     `json:"version"`
	Layout  string                  `json:"layout"`
	Nodes   []EmbeddedGraphNodeSpec `json:"nodes"`
	Edges   []EmbeddedGraphEdgeSpec `json:"edges"`
}

type EmbeddedGraphNodeSpec struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	Label            string   `json:"label"`
	AgentID          string   `json:"agent_id,omitempty"`
	Role             string   `json:"role,omitempty"`
	TaskPrompt       string   `json:"task_prompt,omitempty"`
	Enabled          *bool    `json:"enabled,omitempty"`
	InterruptBefore  bool     `json:"interrupt_before,omitempty"`
	InterruptAfter   bool     `json:"interrupt_after,omitempty"`
	Destinations     []string `json:"destinations,omitempty"`
	RetryMaxAttempts int      `json:"retry_max_attempts,omitempty"`
	FallbackAgent    string   `json:"fallback_agent,omitempty"`
	ReviewerAgent    string   `json:"reviewer_agent,omitempty"`
	ReviewRules      string   `json:"review_rules,omitempty"`
	FuncRef          string   `json:"func_ref,omitempty"`
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
		Mode:             TeamModeSequential,
		RuntimeEngine:    RuntimeEngineGraph,
		TeamGraphRuntime: true,
		MaxConcurrency:   2,
		RunTimeoutSec:    600,
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
		spec.Mode = TeamModeSequential
	}
	engine := strings.ToLower(strings.TrimSpace(spec.RuntimeEngine))
	if engine == RuntimeEngineGraph || spec.TeamGraphRuntime {
		spec.RuntimeEngine = RuntimeEngineGraph
		spec.TeamGraphRuntime = true
	} else if engine == "" && spec.Version >= OrchestrationSpecVersion {
		spec.RuntimeEngine = RuntimeEngineGraph
		spec.TeamGraphRuntime = true
	} else if engine == "" {
		spec.RuntimeEngine = RuntimeEngineGraph
		spec.TeamGraphRuntime = true
	} else {
		spec.RuntimeEngine = engine
	}
	if spec.MaxConcurrency <= 0 {
		spec.MaxConcurrency = 2
	}
	if spec.RunTimeoutSec == 0 && spec.TimeoutSeconds > 0 {
		spec.RunTimeoutSec = spec.TimeoutSeconds
	}
	if spec.RunTimeoutSec <= 0 {
		spec.RunTimeoutSec = 600
	}
	if spec.TimeoutSeconds <= 0 {
		spec.TimeoutSeconds = spec.RunTimeoutSec
	}
	// 空 entries 契约等价于无契约（与 team.ParseDefinition 归一化口径一致），
	// 避免 canonical JSON 携带 deliverable_contract:{"entries":[]}。
	if spec.DeliverableContract != nil && len(spec.DeliverableContract.Entries) == 0 {
		spec.DeliverableContract = nil
	}
	// Backfill members from graph.nodes when members is empty but graph has agent nodes.
	// This fixes data inconsistency where some teams were created with graph.nodes
	// but empty members array (e.g. certain pack import paths).
	if len(spec.Members) == 0 && spec.Graph != nil && len(spec.Graph.Nodes) > 0 {
		for _, n := range spec.Graph.Nodes {
			if n.Type == "agent" && strings.TrimSpace(n.AgentID) != "" {
				spec.Members = append(spec.Members, OrchestrationMember{
					AgentID:    n.AgentID,
					Role:       firstNonEmpty(strings.TrimSpace(n.Role), RoleWorker),
					Name:       firstNonEmpty(strings.TrimSpace(n.Label), "Agent"),
					TaskPrompt: strings.TrimSpace(n.TaskPrompt),
					EnabledPtr: n.Enabled,
					SortOrder:  len(spec.Members) + 1,
				})
			}
		}
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
		// Protect base members from being overwritten by empty overlay members.
		// When a partial update omits members (and graph), the overlay contains members:[]
		// which would wipe out existing members in base. Skip the overwrite in this case.
		if k == "members" {
			if overlayArr, ok := v.([]any); ok && len(overlayArr) == 0 {
				if baseArr, ok := base["members"].([]any); ok && len(baseArr) > 0 {
					continue
				}
			}
		}
		// C-21: deep-merge graph nodes by id so proto/partial overlays do not
		// drop retry/fallback/reviewer/func_ref preserved in definition_json.
		if k == "graph" {
			base[k] = mergeEmbeddedGraphMaps(base["graph"], v)
			continue
		}
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
	if engine == RuntimeEngineGraph || spec.TeamGraphRuntime {
		return raw
	}
	if engine == RuntimeEngineNative && runtimeEngineExplicitlySet(raw) {
		return raw
	}
	spec.RuntimeEngine = RuntimeEngineGraph
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

// mergeEmbeddedGraphMaps merges overlay graph onto base graph, preserving node
// fields from base when overlay omits them (C-21 JSON passthrough).
func mergeEmbeddedGraphMaps(baseAny, overlayAny any) any {
	overlay, ok := overlayAny.(map[string]any)
	if !ok {
		return overlayAny
	}
	base, _ := baseAny.(map[string]any)
	if base == nil {
		return overlay
	}
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if k == "nodes" {
			out[k] = mergeEmbeddedGraphNodes(base["nodes"], v)
			continue
		}
		out[k] = v
	}
	return out
}

func mergeEmbeddedGraphNodes(baseAny, overlayAny any) any {
	overlayNodes, ok := overlayAny.([]any)
	if !ok {
		return overlayAny
	}
	baseByID := map[string]map[string]any{}
	if baseNodes, ok := baseAny.([]any); ok {
		for _, n := range baseNodes {
			m, ok := n.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			if id != "" {
				baseByID[id] = m
			}
		}
	}
	out := make([]any, 0, len(overlayNodes))
	for _, n := range overlayNodes {
		om, ok := n.(map[string]any)
		if !ok {
			out = append(out, n)
			continue
		}
		id, _ := om["id"].(string)
		if bm, ok := baseByID[id]; ok {
			merged := make(map[string]any, len(bm)+len(om))
			for k, v := range bm {
				merged[k] = v
			}
			for k, v := range om {
				merged[k] = v
			}
			out = append(out, merged)
			continue
		}
		out = append(out, om)
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

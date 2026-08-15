// Package deliverable provides set_deliverable/get_deliverable/ack_deliverable
// tools for structured cross-agent handoff via graph state.
//
// When a Team Definition has EnableStateDeliverable=true, the graph runtime
// injects a "deliverable" StateField (MergeReducer) into the graph schema.
// Agent A calls set_deliverable to store its output; agent B calls
// get_deliverable to retrieve A's output as structured input, then
// ack_deliverable to formally accept/reject it.
package deliverable

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"aranea-agents/internal/biz"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Reserved keys of the deliverable state map. "summary" is the bridge source
// for the envelope summary (B.10.15.4); "cognition" carries the C1 cognitive
// process record. Both are extracted by WriteDeliverablesToSession and never
// land in StructuredJSON. Business data with the same keys is overwritten.
const (
	reservedKeySummary   = "summary"
	reservedKeyCognition = "cognition"
)

// topicNamePattern constrains the C3 topic namespace: Unicode letters and
// digits plus '_'/'-', so LLM-authored contract names (e.g. Chinese
// "恢复执行报告") round-trip through the MDC 1:1 topic mapping. Spaces and
// punctuation stay excluded so the deliverable map keys remain greppable and
// collision-free with the reserved keys ("summary"/"cognition" are ASCII and
// can only collide via exact byte equality).
var topicNamePattern = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}_-]{0,63}$`)

// validateTopicName rejects empty-checked topic names that contain characters
// outside the allowed set or collide with reserved keys. Caller guarantees
// topic is trimmed non-empty.
func validateTopicName(topic string) error {
	if !topicNamePattern.MatchString(topic) {
		return fmt.Errorf("invalid topic %q: must match ^[\\p{L}\\p{N}][\\p{L}\\p{N}_-]{0,63}$ (letters/digits in any language, '_' and '-'; no spaces or punctuation)", topic)
	}
	if topic == reservedKeySummary || topic == reservedKeyCognition {
		return fmt.Errorf("invalid topic %q: reserved key", topic)
	}
	return nil
}

// --- set_deliverable ---

// SetDeliverableTool lets an agent publish structured output to the graph
// state's "deliverable" field. The framework's flow layer detects the
// StateDelta method and merges the returned bytes into graph state via
// the state reducer (MergeReducer: top-level key merge).
//
// contract is the optional member-level deliverable contract (MDC); when
// non-nil, topic writes matching a declared entry are validated
// (required_keys + schema_json) and violations fail the Call with an
// LLM-actionable *biz.MemberContractViolationError.
type SetDeliverableTool struct {
	contract *biz.MemberDeliverableContract
}

// NewSetDeliverableTool creates the set_deliverable tool without a contract.
func NewSetDeliverableTool() *SetDeliverableTool { return &SetDeliverableTool{} }

// NewSetDeliverableToolWithContract creates the set_deliverable tool with a
// member-level deliverable contract governing topic writes.
func NewSetDeliverableToolWithContract(contract *biz.MemberDeliverableContract) *SetDeliverableTool {
	return &SetDeliverableTool{contract: contract}
}

// CacheKeyDiscriminator implements the agent build cache's implicit
// behavior-discriminator interface. A contract-installed set_deliverable
// rejects violating topic writes while the contract-free variant accepts
// them — same declaration name, different behavior — so the contract content
// must participate in the cache key. Returns "" for the contract-free
// variant (key falls back to the plain declaration name).
func (t *SetDeliverableTool) CacheKeyDiscriminator() string {
	if t == nil || t.contract == nil {
		return ""
	}
	raw, err := json.Marshal(t.contract)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "contract:" + hex.EncodeToString(sum[:])[:16]
}

// DeliverableContractTopics exposes the topic names declared by the installed
// member deliverable contract (nil when uncontracted). The graph-layer
// tool-limit summary fallback (adapter.summaryFallbackAgent) reads this via
// duck typing to file its synthesized deliverable under the contracted topic
// instead of inventing one.
func (t *SetDeliverableTool) DeliverableContractTopics() []string {
	if t == nil || t.contract == nil {
		return nil
	}
	out := make([]string, 0, len(t.contract.Entries))
	for _, e := range t.contract.Entries {
		out = append(out, e.Topic)
	}
	return out
}

type setDeliverableInput struct {
	Data map[string]any `json:"data" jsonschema:"description=The deliverable content as a JSON object.,required"`
	Note string         `json:"note" jsonschema:"description=Optional note describing this deliverable for downstream agents"`
	// Topic enables the C3 shared blackboard: non-empty stores data under
	// deliverable[topic] (merged with the current map) instead of overwriting
	// the whole map. Letters/digits in any language plus '_'/'-', and must
	// not collide with the reserved keys "summary"/"cognition".
	Topic string `json:"topic" jsonschema:"description=Optional namespace for this deliverable. When set, data is stored under deliverable[topic] and other topics are preserved. Use the exact contract name declared for your team (Chinese names are supported), e.g. research or 恢复执行报告."`
	// Cognition is the optional C1 cognitive-process record, stored under the
	// reserved "cognition" key of the deliverable map and bridged into the
	// DeliverableRef envelope for downstream teams.
	Cognition *biz.DeliverableCognition `json:"cognition" jsonschema:"description=Optional record of the cognitive process behind this deliverable: decisions with rationale, rejected options, assumptions, open questions."`
}

type setDeliverableOutput struct {
	Written bool           `json:"written"`
	Data    map[string]any `json:"data"`
	Keys    int            `json:"keys"`
	Note    string         `json:"note,omitempty"`
	Topic   string         `json:"topic,omitempty"`
}

// Declaration returns the tool metadata.
func (t *SetDeliverableTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "set_deliverable",
		Description: "Publish structured output to the shared graph state deliverable field. " +
			"Downstream agents in the same team run can retrieve it via get_deliverable. " +
			"Writes merge at top-level key granularity (MergeReducer): without topic, the keys you " +
			"write overwrite same-named keys but other members' topics are preserved. " +
			"With topic, data is stored under deliverable[topic] so multiple topics coexist — " +
			"always use a distinct topic per member in parallel teams. " +
			"Keys \"summary\" and \"cognition\" are reserved: business data using them is overwritten.",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"data"},
			Properties: map[string]*trpctool.Schema{
				"data": {
					Type:        "object",
					Description: "The deliverable content as a JSON object. Keys are arbitrary; downstream agents should agree on a schema.",
				},
				"note": {
					Type:        "string",
					Description: "Optional human-readable note about this deliverable.",
				},
				"topic": {
					Type:        "string",
					Description: "Optional namespace. When set, data is stored under deliverable[topic] and other topics are preserved. Letters/digits in any language plus '_'/'-' (Chinese supported, e.g. 恢复执行报告); NOT \"summary\"/\"cognition\" (reserved). Use the exact contract name declared for your team.",
				},
				"cognition": {
					Type:        "object",
					Description: "Optional cognitive-process record: why the deliverable is what it is, not just what it is.",
					Properties: map[string]*trpctool.Schema{
						"decisions": {
							Type:        "array",
							Description: "Decisions made while producing this deliverable.",
							Items: &trpctool.Schema{
								Type: "object",
								Properties: map[string]*trpctool.Schema{
									"choice":     {Type: "string", Description: "The chosen option."},
									"rationale":  {Type: "string", Description: "Why this option was chosen."},
									"confidence": {Type: "number", Description: "Optional confidence, 0 to 1."},
								},
								Required: []string{"choice", "rationale"},
							},
						},
						"rejected": {
							Type:        "array",
							Description: "Options considered and rejected.",
							Items: &trpctool.Schema{
								Type: "object",
								Properties: map[string]*trpctool.Schema{
									"option": {Type: "string", Description: "The rejected option."},
									"reason": {Type: "string", Description: "Why it was rejected."},
								},
								Required: []string{"option", "reason"},
							},
						},
						"assumptions": {
							Type:        "array",
							Description: "Assumptions this deliverable relies on.",
							Items:       &trpctool.Schema{Type: "string"},
						},
						"open_questions": {
							Type:        "array",
							Description: "Unresolved questions downstream agents should be aware of.",
							Items:       &trpctool.Schema{Type: "string"},
						},
					},
				},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Confirmation that the deliverable was written to graph state.",
			Required:    []string{"written", "keys"},
			Properties: map[string]*trpctool.Schema{
				"written": {Type: "boolean", Description: "Whether the deliverable was written."},
				"data":    {Type: "object", Description: "The deliverable map that was written (merged map when topic is set)."},
				"keys":    {Type: "integer", Description: "Number of top-level keys in the written map."},
				"note":    {Type: "string", Description: "Optional note echoed back."},
				"topic":   {Type: "string", Description: "The topic namespace used, if any."},
			},
		},
	}
}

// Call validates input and returns the output. The actual graph state write
// happens via StateDelta, which the flow layer calls after Call succeeds.
//
// C3: with a topic, Call reads the current deliverable map from the session
// (best-effort; an unreadable map is treated as empty), merges data under
// map[topic] and returns the merged map so StateDelta's whole-map Cover write
// preserves other topics. Without a topic the legacy semantics hold: data
// overwrites the whole map. C1: a provided cognition is stored under the
// reserved "cognition" key of the written map.
func (t *SetDeliverableTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t == nil {
		return nil, errors.New("set_deliverable is not configured")
	}
	var in setDeliverableInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, err
	}
	if in.Data == nil {
		return nil, errors.New("data is required and must be a JSON object")
	}
	topic := strings.TrimSpace(in.Topic)
	if topic != "" {
		if err := validateTopicName(topic); err != nil {
			return nil, err
		}
		if violations := t.contract.ValidateTopicData(topic, in.Data); len(violations) > 0 {
			return nil, &biz.MemberContractViolationError{Violations: violations}
		}
	}

	merged := in.Data
	if topic != "" {
		base, _ := readDeliverableMap(ctx)
		merged = make(map[string]any, len(base)+1)
		for k, v := range base {
			merged[k] = v
		}
		merged[topic] = in.Data
	}
	if in.Cognition != nil {
		if topic == "" {
			// Copy so the caller's input map is not mutated by the reserved key.
			merged = make(map[string]any, len(in.Data)+1)
			for k, v := range in.Data {
				merged[k] = v
			}
		}
		merged[reservedKeyCognition] = in.Cognition
	}
	storeLocalDeliverable(ctx, merged)
	return setDeliverableOutput{
		Written: true,
		Data:    merged,
		Keys:    len(merged),
		Note:    strings.TrimSpace(in.Note),
		Topic:   topic,
	}, nil
}

// readDeliverableMap resolves the current deliverable map for tool Calls.
// Precedence: (1) the invocation's RuntimeState — inside a graph run this is
// the node-start snapshot of graph state, which carries upstream members'
// writes (merged via the deliverable StateField reducer) plus same-node
// read-your-writes installed by set_deliverable; (2) the session state, the
// only copy outside graph runs and after run completion. When RuntimeState
// holds the key at all it is authoritative — even as an empty map — so stale
// session data from a previous turn never leaks into a running graph.
func readDeliverableMap(ctx context.Context) (map[string]any, bool) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return nil, false
	}
	if inv.RunOptions.RuntimeState != nil {
		if raw, found := inv.RunOptions.RuntimeState[biz.DeliverableStateKey]; found {
			if m, ok := toDeliverableMap(raw); ok {
				return m, true
			}
		}
	}
	if inv.Session != nil {
		if raw, found := inv.Session.GetState(biz.DeliverableStateKey); found && len(raw) > 0 {
			var out map[string]any
			if err := json.Unmarshal(raw, &out); err == nil {
				return out, true
			}
		}
	}
	return nil, false
}

// toDeliverableMap normalizes a deliverable state value: decoded maps pass
// through, raw JSON bytes (session-seeded state) are unmarshaled.
func toDeliverableMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case []byte:
		if len(t) == 0 {
			return nil, false
		}
		var out map[string]any
		if err := json.Unmarshal(t, &out); err != nil {
			return nil, false
		}
		return out, true
	}
	return nil, false
}

// storeLocalDeliverable installs the freshly written map into the
// invocation's RuntimeState so same-node get_deliverable calls observe
// read-your-writes. The key is replaced (never mutated in place) because the
// node-start snapshot may share sub-maps with sibling nodes. The
// authoritative cross-node write still flows via StateDelta → graph channels
// (MergeReducer); this only affects the node-local view.
func storeLocalDeliverable(ctx context.Context, merged map[string]any) {
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.RunOptions.RuntimeState == nil {
		return
	}
	inv.RunOptions.RuntimeState[biz.DeliverableStateKey] = merged
}

// StateDelta returns the graph state delta after a successful Call.
// The flow layer calls this via duck typing (see internal/flow/processor/functioncall.go).
// Returns nil when toolCallID is empty, resultJSON is invalid, or Written=false.
func (t *SetDeliverableTool) StateDelta(toolCallID string, _ []byte, resultJSON []byte) map[string][]byte {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" || len(resultJSON) == 0 {
		return nil
	}
	var out setDeliverableOutput
	if err := json.Unmarshal(resultJSON, &out); err != nil {
		return nil
	}
	if !out.Written || len(out.Data) == 0 {
		return nil
	}
	b, err := json.Marshal(out.Data)
	if err != nil {
		return nil
	}
	return map[string][]byte{
		biz.DeliverableStateKey: b,
	}
}

// --- get_deliverable ---

// GetDeliverableTool lets an agent read the current "deliverable" from
// the session state. This is read-only; it does not modify graph state.
type GetDeliverableTool struct{}

// NewGetDeliverableTool creates the get_deliverable tool.
func NewGetDeliverableTool() *GetDeliverableTool { return &GetDeliverableTool{} }

type getDeliverableInput struct {
	Key string `json:"key" jsonschema:"description=Optional specific key to read from the deliverable. Empty returns the full deliverable object."`
	// Topic selects the C3 namespace: non-empty reads deliverable[topic]
	// (a sub-object) and applies the key filter within it.
	Topic string `json:"topic" jsonschema:"description=Optional namespace to read from (the topic used in set_deliverable). Empty reads the top-level deliverable map."`
}

type getDeliverableOutput struct {
	Data  map[string]any `json:"data"`
	Found bool           `json:"found"`
	Key   string         `json:"key,omitempty"`
	Topic string         `json:"topic,omitempty"`
}

// Declaration returns the tool metadata.
func (t *GetDeliverableTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "get_deliverable",
		Description: "Read the shared deliverable from graph state. " +
			"Use this at the start of a team member's work to retrieve upstream agent output. " +
			"Returns found=false if no deliverable has been set yet.",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{},
			Properties: map[string]*trpctool.Schema{
				"key": {
					Type:        "string",
					Description: "Optional specific key to read from the deliverable. Empty returns the full object.",
				},
				"topic": {
					Type:        "string",
					Description: "Optional namespace to read from (the topic used in set_deliverable). Empty reads the top-level deliverable map.",
				},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "The deliverable data from graph state.",
			Required:    []string{"found"},
			Properties: map[string]*trpctool.Schema{
				"data":  {Type: "object", Description: "The deliverable data (full, topic-scoped, or single-key depending on input)."},
				"found": {Type: "boolean", Description: "Whether a deliverable was found."},
				"key":   {Type: "string", Description: "The specific key requested, if any."},
				"topic": {Type: "string", Description: "The topic namespace requested, if any."},
			},
		},
	}
}

// Call reads the deliverable via readDeliverableMap: the graph RuntimeState
// snapshot first (upstream members' writes mid-run), session state as the
// non-graph fallback.
// C3: with a topic, the read is scoped to the deliverable[topic] sub-object
// (missing topic or non-object value → found=false); the optional key filter
// then applies within that sub-object. Topic lookup is deliberately tolerant —
// an unknown topic yields found=false, not an error.
func (t *GetDeliverableTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t == nil {
		return nil, errors.New("get_deliverable is not configured")
	}
	var in getDeliverableInput
	_ = json.Unmarshal(jsonArgs, &in) // input is optional, empty struct is fine

	data, found := readDeliverableMap(ctx)
	if !found {
		return getDeliverableOutput{Found: false, Key: in.Key, Topic: in.Topic}, nil
	}
	topic := strings.TrimSpace(in.Topic)
	if topic != "" {
		sub, exists := data[topic]
		if !exists {
			return getDeliverableOutput{Found: false, Key: in.Key, Topic: topic}, nil
		}
		subMap, isMap := sub.(map[string]any)
		if !isMap {
			return getDeliverableOutput{Found: false, Key: in.Key, Topic: topic}, nil
		}
		data = subMap
	}
	key := strings.TrimSpace(in.Key)
	if key != "" {
		v, exists := data[key]
		if !exists {
			return getDeliverableOutput{Found: false, Key: key, Topic: topic}, nil
		}
		return getDeliverableOutput{
			Data:  map[string]any{key: v},
			Found: true,
			Key:   key,
			Topic: topic,
		}, nil
	}
	return getDeliverableOutput{
		Data:  data,
		Found: true,
		Topic: topic,
	}, nil
}

// --- interface guards ---

var (
	_ trpctool.Tool         = (*SetDeliverableTool)(nil)
	_ trpctool.CallableTool = (*SetDeliverableTool)(nil)
	_ trpctool.Tool         = (*GetDeliverableTool)(nil)
	_ trpctool.CallableTool = (*GetDeliverableTool)(nil)
)

// --- ToolSet adapter ---

// ToolSet implements trpctool.ToolSet for the deliverable tool group.
type ToolSet struct{}

// Tools returns the set_deliverable and get_deliverable tools.
func (ToolSet) Tools(_ context.Context) []trpctool.Tool {
	return Tools()
}

// Name returns the toolset name.
func (ToolSet) Name() string { return "deliverable" }

// Close releases resources held by the toolset (none for deliverable).
func (ToolSet) Close() error { return nil }

// Tools returns all deliverable tools as a flat slice (no member contract).
func Tools() []trpctool.Tool {
	return ToolsWithContract(nil)
}

// ToolsWithContract returns the deliverable tools with the member-level
// deliverable contract installed on set_deliverable (nil = uncontracted).
func ToolsWithContract(contract *biz.MemberDeliverableContract) []trpctool.Tool {
	return []trpctool.Tool{
		NewSetDeliverableToolWithContract(contract),
		NewGetDeliverableTool(),
		NewAckDeliverableTool(),
	}
}

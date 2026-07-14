// Package deliverable provides set_deliverable/get_deliverable tools for
// structured cross-agent handoff via graph state.
//
// When a Team Definition has EnableStateDeliverable=true, the graph runtime
// injects a "deliverable" StateField (CoverReducer) into the graph schema.
// Agent A calls set_deliverable to store its output; agent B calls
// get_deliverable to retrieve A's output as structured input.
package deliverable

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aranea-agents/internal/biz"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// --- set_deliverable ---

// SetDeliverableTool lets an agent publish structured output to the graph
// state's "deliverable" field. The framework's flow layer detects the
// StateDelta method and merges the returned bytes into graph state via
// the CoverReducer (latest writer wins).
type SetDeliverableTool struct{}

// NewSetDeliverableTool creates the set_deliverable tool.
func NewSetDeliverableTool() *SetDeliverableTool { return &SetDeliverableTool{} }

type setDeliverableInput struct {
	Data map[string]any `json:"data" jsonschema:"description=The deliverable content as a JSON object. This will overwrite the previous deliverable.,required"`
	Note string         `json:"note" jsonschema:"description=Optional note describing this deliverable for downstream agents"`
}

type setDeliverableOutput struct {
	Written bool           `json:"written"`
	Data    map[string]any `json:"data"`
	Keys    int            `json:"keys"`
	Note    string         `json:"note,omitempty"`
}

// Declaration returns the tool metadata.
func (t *SetDeliverableTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "set_deliverable",
		Description: "Publish structured output to the shared graph state deliverable field. " +
			"Downstream agents in the same team run can retrieve it via get_deliverable. " +
			"Each call overwrites the previous deliverable (CoverReducer semantics).",
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
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Confirmation that the deliverable was written to graph state.",
			Required:    []string{"written", "keys"},
			Properties: map[string]*trpctool.Schema{
				"written": {Type: "boolean", Description: "Whether the deliverable was written."},
				"data":    {Type: "object", Description: "The deliverable data that was written."},
				"keys":    {Type: "integer", Description: "Number of top-level keys in the deliverable."},
				"note":    {Type: "string", Description: "Optional note echoed back."},
			},
		},
	}
}

// Call validates input and returns the output. The actual graph state write
// happens via StateDelta, which the flow layer calls after Call succeeds.
func (t *SetDeliverableTool) Call(_ context.Context, jsonArgs []byte) (any, error) {
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
	return setDeliverableOutput{
		Written: true,
		Data:    in.Data,
		Keys:    len(in.Data),
		Note:    strings.TrimSpace(in.Note),
	}, nil
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
}

type getDeliverableOutput struct {
	Data  map[string]any `json:"data"`
	Found bool           `json:"found"`
	Key   string         `json:"key,omitempty"`
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
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "The deliverable data from graph state.",
			Required:    []string{"found"},
			Properties: map[string]*trpctool.Schema{
				"data":  {Type: "object", Description: "The deliverable data (full or single-key depending on input)."},
				"found": {Type: "boolean", Description: "Whether a deliverable was found."},
				"key":   {Type: "string", Description: "The specific key requested, if any."},
			},
		},
	}
}

// Call reads the deliverable from the session state via the invocation context.
func (t *GetDeliverableTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t == nil {
		return nil, errors.New("get_deliverable is not configured")
	}
	var in getDeliverableInput
	_ = json.Unmarshal(jsonArgs, &in) // input is optional, empty struct is fine

	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return getDeliverableOutput{Found: false, Key: in.Key}, nil
	}
	raw, found := inv.Session.GetState(biz.DeliverableStateKey)
	if !found || len(raw) == 0 {
		return getDeliverableOutput{Found: false, Key: in.Key}, nil
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return getDeliverableOutput{Found: false, Key: in.Key}, nil
	}
	key := strings.TrimSpace(in.Key)
	if key != "" {
		v, exists := data[key]
		if !exists {
			return getDeliverableOutput{Found: false, Key: key}, nil
		}
		return getDeliverableOutput{
			Data:  map[string]any{key: v},
			Found: true,
			Key:   key,
		}, nil
	}
	return getDeliverableOutput{
		Data:  data,
		Found: true,
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

// Tools returns all deliverable tools as a flat slice.
func Tools() []trpctool.Tool {
	return []trpctool.Tool{
		NewSetDeliverableTool(),
		NewGetDeliverableTool(),
	}
}

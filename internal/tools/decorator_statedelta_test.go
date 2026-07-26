package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// stateDeltaMockTool mirrors the framework's StateDelta duck-typing
// (flow.processor.attachStateDelta): tools implementing this exact
// signature turn their result into session/graph state.
type stateDeltaMockTool struct {
	name   string
	result any
}

func (m *stateDeltaMockTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: m.name}
}

func (m *stateDeltaMockTool) Call(_ context.Context, _ []byte) (any, error) {
	return m.result, nil
}

func (m *stateDeltaMockTool) StateDelta(_ string, _ []byte, resultJSON []byte) map[string][]byte {
	var out struct {
		Written bool           `json:"written"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resultJSON, &out); err != nil || !out.Written {
		return nil
	}
	b, err := json.Marshal(out.Data)
	if err != nil {
		return nil
	}
	return map[string][]byte{"deliverable": b}
}

// TestToolDecorator_SkipsTruncationForStateDeltaTools reproduces the
// bff43a17 incident: set_deliverable returned a 28.9KB result; the
// decorator wrapped it in a truncation envelope
// ({"content":"...","truncated":true}); the framework then fed the
// envelope — not the original JSON — into StateDelta, which failed to
// parse `written` and silently dropped the session-state write.
// StateDelta-providing tools must bypass the size budget: truncating
// their results corrupts both the state write and the LLM-visible JSON.
func TestToolDecorator_SkipsTruncationForStateDeltaTools(t *testing.T) {
	big := strings.Repeat("x", 30*1024)
	inner := &stateDeltaMockTool{
		name: "set_deliverable",
		result: map[string]any{
			"written": true,
			"data":    map[string]any{"doc": big},
		},
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
	})
	res, err := d.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", res)
	}
	if truncated, _ := m["truncated"].(bool); truncated {
		t.Fatalf("StateDelta tool result must not be truncated: %v", m)
	}
	if written, _ := m["written"].(bool); !written {
		t.Fatalf("result lost written=true: %v", m)
	}
	// The framework feeds the marshaled decorated result back into
	// StateDelta; it must still parse and produce the state delta.
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	delta := inner.StateDelta("call-1", nil, raw)
	if len(delta["deliverable"]) == 0 {
		t.Fatal("StateDelta returned no deliverable delta for decorated result")
	}
}

// TestToolDecorator_StillTruncatesPlainTools is the control: tools
// without StateDelta keep the size-budget truncation behavior.
func TestToolDecorator_StillTruncatesPlainTools(t *testing.T) {
	big := strings.Repeat("x", 30*1024)
	inner := &decoratorMockTool{
		name: "plain_tool",
		call: func(_ context.Context, _ []byte) (any, error) {
			return map[string]any{"data": big}, nil
		},
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
	})
	res, err := d.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", res)
	}
	if truncated, _ := m["truncated"].(bool); !truncated {
		t.Fatalf("plain tool result should be truncated: %v", m)
	}
	if _, hasContent := m["content"]; !hasContent {
		t.Fatalf("truncated envelope should carry content string: %v", m)
	}
}

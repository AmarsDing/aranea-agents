package plugintrpc

import (
	"encoding/json"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestApplyModelModifyPatch_generationAndSystem(t *testing.T) {
	temp := 0.2
	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "hi"}},
	}
	patch := map[string]any{
		"generation_config": map[string]any{"temperature": temp, "max_tokens": 128},
		"append_system":     "be concise",
	}
	ApplyModelModifyPatch(req, patch, loggateway.NewNoop())
	if req.Temperature == nil || *req.Temperature != temp {
		t.Fatalf("temperature=%v", req.Temperature)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 128 {
		t.Fatalf("max_tokens=%v", req.MaxTokens)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != trpcmodel.RoleSystem {
		t.Fatalf("messages=%+v", req.Messages)
	}
}

func TestApplyToolModifyPatch_mergeArguments(t *testing.T) {
	args := &trpctool.BeforeToolArgs{
		ToolName:  "search",
		Arguments: []byte(`{"q":"hello"}`),
	}
	patch := map[string]any{
		"merge_arguments": map[string]any{"limit": 5},
	}
	out := ApplyToolModifyPatch(args, patch, loggateway.NewNoop())
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["q"] != "hello" || m["limit"].(float64) != 5 {
		t.Fatalf("got %#v", m)
	}
}

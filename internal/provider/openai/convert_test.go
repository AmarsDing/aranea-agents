package openai

import (
	"encoding/json"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

func TestOpenAIMessagesFromContents_toolRoundTrip(t *testing.T) {
	contents := []*genai.Content{
		genai.NewContentFromText("hi", genai.RoleUser),
		{
			Role: string(genai.RoleModel),
			Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{ID: "call_1", Name: "read_file", Args: map[string]any{"path": "go.mod"}}},
			},
		},
		{
			Role: string(genai.RoleUser),
			Parts: []*genai.Part{
				{FunctionResponse: &genai.FunctionResponse{ID: "call_1", Name: "read_file", Response: map[string]any{"content": "module x"}}},
			},
		},
	}
	msgs, err := OpenAIMessagesFromContents(contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("msgs: %d", len(msgs))
	}
	if msgs[1]["role"] != "assistant" {
		t.Fatalf("want assistant, got %v", msgs[1])
	}
	if msgs[2]["role"] != "tool" {
		t.Fatalf("want tool, got %v", msgs[2])
	}
}

func TestSanitizeOpenAIChatMessagesToolSequence_stripsDanglingToolCalls(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "tool_calls": []any{map[string]any{
			"id": "call_1", "type": "function", "function": map[string]any{"name": "x", "arguments": "{}"},
		}}},
		{"role": "user", "content": "next"},
	}
	out := SanitizeOpenAIChatMessagesToolSequence(msgs)
	if len(out) != 3 {
		t.Fatalf("len %d %#v", len(out), out)
	}
	if out[1]["tool_calls"] != nil {
		t.Fatal("expected tool_calls removed")
	}
	c, _ := out[1]["content"].(string)
	if c == "" {
		t.Fatal("expected placeholder content")
	}
}

// OpenAIMessagesFromContents assigns tool_calls as []map[string]any; sanitizer must still catch dangling calls.
func TestSanitizeOpenAIChatMessagesToolSequence_toolCallsAsMapSlice(t *testing.T) {
	toolCalls := []map[string]any{{
		"id": "call_1", "type": "function", "function": map[string]any{"name": "x", "arguments": "{}"},
	}}
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "tool_calls": toolCalls},
		{"role": "user", "content": "next"},
	}
	out := SanitizeOpenAIChatMessagesToolSequence(msgs)
	if len(out) != 3 {
		t.Fatalf("len %d", len(out))
	}
	if out[1]["tool_calls"] != nil {
		t.Fatal("expected tool_calls stripped for []map[string]any form")
	}
}

func TestSanitizeOpenAIChatMessagesToolSequence_keepsCompleteToolTurn(t *testing.T) {
	msgs := []map[string]any{
		{"role": "assistant", "tool_calls": []any{map[string]any{
			"id": "c1", "type": "function", "function": map[string]any{"name": "n", "arguments": "{}"},
		}}},
		{"role": "tool", "tool_call_id": "c1", "content": "{}"},
		{"role": "user", "content": "ok"},
	}
	out := SanitizeOpenAIChatMessagesToolSequence(msgs)
	if len(out) != 3 {
		t.Fatalf("len %d", len(out))
	}
	if out[0]["tool_calls"] == nil {
		t.Fatal("expected tool_calls preserved")
	}
	if out[1]["role"] != "tool" {
		t.Fatal("expected tool message")
	}
}

func TestOpenAIChatToolsFromRequest_basic(t *testing.T) {
	req := &model.LLMRequest{
		Config: &genai.GenerateContentConfig{
			Tools: []*genai.Tool{
				{FunctionDeclarations: []*genai.FunctionDeclaration{
					{Name: "read_file", Description: "read", ParametersJsonSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string"},
						},
					}},
				}},
			},
		},
	}
	tools := OpenAIChatToolsFromRequest(req)
	if len(tools) != 1 {
		t.Fatalf("got %d tools", len(tools))
	}
	fn := tools[0]["function"].(map[string]any)
	if fn["name"] != "read_file" {
		t.Fatalf("%v", fn)
	}
	raw, _ := json.Marshal(tools[0])
	if len(raw) < 10 {
		t.Fatal(string(raw))
	}
}

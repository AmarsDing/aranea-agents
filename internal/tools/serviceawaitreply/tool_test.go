package serviceawaitreply

import (
	"context"
	"testing"
)

func TestWithReplyFunc_RoundTrip(t *testing.T) {
	called := false
	fn := ReplyFunc(func(_ context.Context) (string, error) {
		called = true
		return "hello", nil
	})
	ctx := WithReplyFunc(context.Background(), fn)
	got := ReplyFuncFromContext(ctx)
	if got == nil {
		t.Fatal("ReplyFuncFromContext returned nil")
	}
	reply, err := got(context.Background())
	if err != nil || reply != "hello" {
		t.Fatalf("got (%q, %v), want (%q, nil)", reply, err, "hello")
	}
	if !called {
		t.Fatal("recovered function was not called")
	}
}

func TestReplyFuncFromContext_MissingKey(t *testing.T) {
	got := ReplyFuncFromContext(context.Background())
	if got != nil {
		t.Fatal("expected nil when key is missing")
	}
}

func TestReplyFuncFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), replyFuncKey{}, "not a ReplyFunc")
	got := ReplyFuncFromContext(ctx)
	if got != nil {
		t.Fatal("expected nil when value is wrong type")
	}
}

func TestNew_ReturnsNonNil(t *testing.T) {
	tool := New()
	if tool == nil {
		t.Fatal("New() returned nil")
	}
}

func TestServiceTool_Declaration(t *testing.T) {
	tool := &ServiceTool{}
	d := tool.Declaration()
	if d == nil {
		t.Fatal("Declaration() returned nil")
	}
	if d.Name != "await_user_reply" {
		t.Fatalf("Name = %q, want %q", d.Name, "await_user_reply")
	}
	if d.Description == "" {
		t.Fatal("Description should not be empty")
	}
	if d.InputSchema == nil {
		t.Fatal("InputSchema should not be nil")
	}
	if d.InputSchema.Type != "object" {
		t.Fatalf("InputSchema.Type = %q, want %q", d.InputSchema.Type, "object")
	}
}

func TestServiceTool_Call_NoReplyFunc(t *testing.T) {
	tool := &ServiceTool{}
	result, err := tool.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["success"] != false {
		t.Fatalf("success = %v, want false", m["success"])
	}
}

func TestServiceTool_Call_WithReplyFunc(t *testing.T) {
	fn := ReplyFunc(func(_ context.Context) (string, error) {
		return "user reply text", nil
	})
	ctx := WithReplyFunc(context.Background(), fn)
	tool := &ServiceTool{}
	result, err := tool.Call(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["success"] != true {
		t.Fatalf("success = %v, want true", m["success"])
	}
	if m["reply"] != "user reply text" {
		t.Fatalf("reply = %v, want %q", m["reply"], "user reply text")
	}
}

func TestServiceTool_Call_ReplyFuncError(t *testing.T) {
	fn := ReplyFunc(func(_ context.Context) (string, error) {
		return "", context.Canceled
	})
	ctx := WithReplyFunc(context.Background(), fn)
	tool := &ServiceTool{}
	_, err := tool.Call(ctx, nil)
	if err == nil {
		t.Fatal("expected error from ReplyFunc")
	}
}

func TestWithToolConfirmRequest_MissingKey(t *testing.T) {
	_, ok := ToolConfirmRequestFromContext(context.Background())
	if ok {
		t.Fatal("expected false when key is missing")
	}
}

func TestWithToolConfirmRequest_EmptyToolKey(t *testing.T) {
	ctx := WithToolConfirmRequest(context.Background(), ToolConfirmRequest{ToolKey: "", ToolCallID: "call-1"})
	_, ok := ToolConfirmRequestFromContext(ctx)
	if ok {
		t.Fatal("expected false when ToolKey is empty")
	}
}

func TestWithToolConfirmRequest_WhitespaceToolKey(t *testing.T) {
	ctx := WithToolConfirmRequest(context.Background(), ToolConfirmRequest{ToolKey: "   ", ToolCallID: "call-1"})
	_, ok := ToolConfirmRequestFromContext(ctx)
	if ok {
		t.Fatal("expected false when ToolKey is whitespace-only")
	}
}

func TestWithToolConfirmRequest_TrimsFields(t *testing.T) {
	ctx := WithToolConfirmRequest(context.Background(), ToolConfirmRequest{ToolKey: "  bash  ", ToolCallID: "  call-1  "})
	req, ok := ToolConfirmRequestFromContext(ctx)
	if !ok {
		t.Fatal("expected true")
	}
	if req.ToolKey != "bash" {
		t.Fatalf("ToolKey = %q, want %q", req.ToolKey, "bash")
	}
	if req.ToolCallID != "call-1" {
		t.Fatalf("ToolCallID = %q, want %q", req.ToolCallID, "call-1")
	}
}

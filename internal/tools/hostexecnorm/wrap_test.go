package hostexecnorm

import (
	"context"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type mockToolSet struct {
	nameFn  func() string
	closeFn func() error
	toolsFn func(ctx context.Context) []trpctool.Tool
}

func (m *mockToolSet) Name() string {
	if m.nameFn != nil {
		return m.nameFn()
	}
	return "mock-toolset"
}

func (m *mockToolSet) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func (m *mockToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if m.toolsFn != nil {
		return m.toolsFn(ctx)
	}
	return nil
}

type mockCallableTool struct {
	declFn func() *trpctool.Declaration
	callFn func(ctx context.Context, jsonArgs []byte) (any, error)
}

func (m *mockCallableTool) Declaration() *trpctool.Declaration {
	if m.declFn != nil {
		return m.declFn()
	}
	return &trpctool.Declaration{Name: "mock-tool"}
}

func (m *mockCallableTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if m.callFn != nil {
		return m.callFn(ctx, jsonArgs)
	}
	return nil, nil
}

func TestWrapToolSet_NilInput(t *testing.T) {
	result := WrapToolSet(nil)
	if result != nil {
		t.Fatalf("expected nil for nil input, got %v", result)
	}
}

func TestWrapToolSet_DelegatesName(t *testing.T) {
	ts := &mockToolSet{nameFn: func() string { return "hostexec" }}
	wrapped := WrapToolSet(ts)
	if wrapped.Name() != "hostexec" {
		t.Fatalf("expected name hostexec, got %q", wrapped.Name())
	}
}

func TestWrapToolSet_DelegatesClose(t *testing.T) {
	closed := false
	ts := &mockToolSet{closeFn: func() error {
		closed = true
		return nil
	}}
	wrapped := WrapToolSet(ts)
	if err := wrapped.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !closed {
		t.Fatal("expected Close to be delegated")
	}
}

func TestWrapToolSet_DelegatesTools(t *testing.T) {
	innerTool := &mockCallableTool{}
	ts := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{innerTool}
	}}
	wrapped := WrapToolSet(ts)
	tools := wrapped.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
}

func TestWrapToolSet_ToolsEmpty(t *testing.T) {
	ts := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return nil
	}}
	wrapped := WrapToolSet(ts)
	tools := wrapped.Tools(context.Background())
	if tools != nil {
		t.Fatalf("expected nil for empty tools, got %v", tools)
	}
}

func TestWrapCallable_NormalizesArgs(t *testing.T) {
	var receivedArgs []byte
	inner := &mockCallableTool{
		callFn: func(_ context.Context, jsonArgs []byte) (any, error) {
			receivedArgs = jsonArgs
			return "ok", nil
		},
	}

	wrapped := WrapToolSet(&mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{inner}
	}})

	tools := wrapped.Tools(context.Background())
	ct, ok := tools[0].(trpctool.CallableTool)
	if !ok {
		t.Fatal("expected CallableTool")
	}

	input := []byte(`{"command":"pwd","working_dir":"/tmp"}`)
	_, err := ct.Call(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(receivedArgs) != `{"command":"pwd","workdir":"/tmp"}` {
		t.Fatalf("expected normalized args, got %s", receivedArgs)
	}
}

func TestWrapCallable_DelegatesDeclaration(t *testing.T) {
	inner := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "exec_command"}
		},
	}

	wrapped := WrapToolSet(&mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{inner}
	}})

	tools := wrapped.Tools(context.Background())
	ct, ok := tools[0].(trpctool.CallableTool)
	if !ok {
		t.Fatal("expected CallableTool")
	}

	decl := ct.Declaration()
	if decl == nil || decl.Name != "exec_command" {
		t.Fatalf("expected declaration name exec_command, got %v", decl)
	}
}

func TestWrapCallable_NoNormalizationNeeded(t *testing.T) {
	var receivedArgs []byte
	inner := &mockCallableTool{
		callFn: func(_ context.Context, jsonArgs []byte) (any, error) {
			receivedArgs = jsonArgs
			return "ok", nil
		},
	}

	wrapped := WrapToolSet(&mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{inner}
	}})

	tools := wrapped.Tools(context.Background())
	ct := tools[0].(trpctool.CallableTool)

	input := []byte(`{"command":"pwd","workdir":"/tmp"}`)
	_, err := ct.Call(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(receivedArgs) != string(input) {
		t.Fatalf("expected unchanged args when workdir already present, got %s", receivedArgs)
	}
}

func TestWrapToolSet_NonCallableToolPassthrough(t *testing.T) {
	nonCallable := &struct{ trpctool.Tool }{}
	ts := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{nonCallable}
	}}
	wrapped := WrapToolSet(ts)
	tools := wrapped.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0] != nonCallable {
		t.Fatal("expected non-callable tool to pass through unchanged")
	}
}

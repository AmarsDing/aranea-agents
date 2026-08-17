package filenorm

import (
	"context"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type mockCallable struct {
	name string
	got  []byte
}

func (m *mockCallable) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: m.name}
}

func (m *mockCallable) Call(_ context.Context, jsonArgs []byte) (any, error) {
	m.got = append([]byte(nil), jsonArgs...)
	return "ok", nil
}

type mockSet struct{ tools []trpctool.Tool }

func (m *mockSet) Name() string                          { return "file" }
func (m *mockSet) Close() error                          { return nil }
func (m *mockSet) Tools(context.Context) []trpctool.Tool { return m.tools }

func TestWrapCallable_NormalizesPathAlias(t *testing.T) {
	inner := &mockCallable{name: "read_file"}
	wrapped := WrapToolSet(&mockSet{tools: []trpctool.Tool{inner}})
	ct := wrapped.Tools(context.Background())[0].(trpctool.CallableTool)
	if _, err := ct.Call(context.Background(), []byte(`{"path":"a.go"}`)); err != nil {
		t.Fatal(err)
	}
	if string(inner.got) != `{"file_name":"a.go"}` {
		t.Fatalf("got %s", inner.got)
	}
}

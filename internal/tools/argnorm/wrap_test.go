package argnorm

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

func TestWrapTool_NormalizesURLAlias(t *testing.T) {
	inner := &mockCallable{name: "web_fetch"}
	wrapped := WrapTool(inner).(trpctool.CallableTool)
	if _, err := wrapped.Call(context.Background(), []byte(`{"url":"https://example.com"}`)); err != nil {
		t.Fatal(err)
	}
	var want = `{"urls":["https://example.com"]}`
	if string(inner.got) != want {
		t.Fatalf("got %s want %s", inner.got, want)
	}
}

func TestWrapTool_SkipsUnknown(t *testing.T) {
	inner := &mockCallable{name: "save_file"}
	if WrapTool(inner) != inner {
		t.Fatal("unknown tools must keep identity so Streamable is not stripped")
	}
}

package editstamp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestRecordAndList(t *testing.T) {
	dir := t.TempDir()
	Record(dir, "src\\a.go")
	Record(dir, "src/a.go")
	Record(dir, "b.py")
	got := List(dir)
	if len(got) != 2 || got[0] != "src/a.go" || got[1] != "b.py" {
		t.Fatalf("%v", got)
	}
}

func TestWrapToolSetRecordsOnSuccess(t *testing.T) {
	dir := t.TempDir()
	inner := &stubSet{tools: []trpctool.Tool{&stubSave{}}}
	ts := WrapToolSet(inner, dir)
	ct := ts.Tools(context.Background())[0].(trpctool.CallableTool)
	if _, err := ct.Call(context.Background(), []byte(`{"file_name":"pkg/x.go"}`)); err != nil {
		t.Fatal(err)
	}
	got := List(dir)
	if len(got) != 1 || got[0] != "pkg/x.go" {
		t.Fatalf("%v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".aranea", "edited-paths.txt")); err != nil {
		t.Fatal(err)
	}
}

type stubSet struct{ tools []trpctool.Tool }

func (s *stubSet) Tools(context.Context) []trpctool.Tool { return s.tools }
func (s *stubSet) Close() error                          { return nil }
func (s *stubSet) Name() string                          { return "file" }

type stubSave struct{}

func (s *stubSave) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: "save_file"}
}
func (s *stubSave) Call(context.Context, []byte) (any, error) {
	return map[string]any{"ok": true}, nil
}

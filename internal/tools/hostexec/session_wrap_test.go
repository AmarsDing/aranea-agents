package hostexec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestWrapSessionEnhance_WritesOutputFileAndNotify(t *testing.T) {
	dir := t.TempDir()
	exec := &stubExec{
		result: map[string]any{"status": "running", "session_id": "sess-1", "output": "boot\n"},
	}
	stdin := &stubStdin{outputs: []map[string]any{
		{"status": "running", "output": "boot\nready\n", "session_id": "sess-1"},
	}}
	ts := WrapSessionEnhance(&stubSet{tools: []trpctool.Tool{exec, stdin}}, dir, loggateway.NewNoop())
	var ct trpctool.CallableTool
	for _, t0 := range ts.Tools(context.Background()) {
		if t0.Declaration().Name == "exec_command" {
			ct = t0.(trpctool.CallableTool)
		}
	}
	out, err := ct.Call(context.Background(), []byte(`{"command":"echo","notify_pattern":"ready","yield_time_ms":2000}`))
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["notified"] != true {
		t.Fatalf("notified=%v", m["notified"])
	}
	if _, ok := m["running_for_ms"]; !ok {
		t.Fatal("missing running_for_ms")
	}
	path, _ := m["output_file"].(string)
	if path == "" {
		t.Fatal("missing output_file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ready") {
		t.Fatalf("log=%q", body)
	}
	if _, err := os.Stat(filepath.Join(dir, ".aranea", "shell", "sess-1.log")); err != nil {
		t.Fatal(err)
	}
}

func TestWrapSessionEnhance_DeclarationHasNotify(t *testing.T) {
	ts := WrapSessionEnhance(&stubSet{tools: []trpctool.Tool{&stubExec{}}}, t.TempDir(), loggateway.NewNoop())
	d := ts.Tools(context.Background())[0].Declaration()
	if d.InputSchema == nil || d.InputSchema.Properties["notify_pattern"] == nil {
		t.Fatalf("schema=%+v", d.InputSchema)
	}
}

func TestWrapSessionEnhance_StreamableCall(t *testing.T) {
	dir := t.TempDir()
	exec := &stubExec{
		result: map[string]any{"status": "exited", "session_id": "sess-stream", "output": "hello stream\n"},
	}
	ts := WrapSessionEnhance(&stubSet{tools: []trpctool.Tool{exec}}, dir, loggateway.NewNoop())
	st, ok := ts.Tools(context.Background())[0].(trpctool.StreamableTool)
	if !ok {
		t.Fatal("exec_command must be StreamableTool")
	}
	reader, err := st.StreamableCall(context.Background(), []byte(`{"command":"echo"}`))
	if err != nil {
		t.Fatal(err)
	}
	chunk, cerr := reader.Recv()
	if cerr != nil {
		t.Fatalf("recv: %v", cerr)
	}
	text, _ := chunk.Content.(string)
	if !strings.Contains(text, "hello stream") {
		t.Fatalf("chunk=%v", chunk.Content)
	}
}

func TestExtractNotify(t *testing.T) {
	p, w := extractNotify([]byte(`{"notify_on_output":"DONE","block_until_ms":1500}`))
	if p != "DONE" || w.Milliseconds() != 1500 {
		t.Fatalf("%s %v", p, w)
	}
}

type stubSet struct{ tools []trpctool.Tool }

func (s *stubSet) Tools(context.Context) []trpctool.Tool { return s.tools }
func (s *stubSet) Close() error                          { return nil }
func (s *stubSet) Name() string                          { return "hostexec" }

type stubExec struct {
	result map[string]any
}

func (s *stubExec) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name:        "exec_command",
		InputSchema: &trpctool.Schema{Type: "object", Properties: map[string]*trpctool.Schema{"command": {Type: "string"}}},
	}
}
func (s *stubExec) Call(context.Context, []byte) (any, error) { return s.result, nil }

type stubStdin struct {
	outputs []map[string]any
	i       int
}

func (s *stubStdin) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: "write_stdin"}
}
func (s *stubStdin) Call(context.Context, []byte) (any, error) {
	if s.i >= len(s.outputs) {
		return s.outputs[len(s.outputs)-1], nil
	}
	out := s.outputs[s.i]
	s.i++
	return out, nil
}

func TestSessionMetaStore_Hung(t *testing.T) {
	s := newSessionMetaStore()
	t0 := time.Unix(1_700_000_000, 0)
	_, hung := s.observe("s1", "boot", "running", t0, time.Second)
	if hung {
		t.Fatal("fresh session must not be hung")
	}
	_, hung = s.observe("s1", "boot", "running", t0.Add(2*time.Second), time.Second)
	if !hung {
		t.Fatal("unchanged output past threshold must be hung")
	}
	ms, hung := s.observe("s1", "boot\nmore", "running", t0.Add(3*time.Second), time.Second)
	if hung {
		t.Fatal("new output clears hung")
	}
	if ms < 3000 {
		t.Fatalf("running_for_ms=%d", ms)
	}
}

func TestWrapSessionEnhance_WriteStdinNotify(t *testing.T) {
	dir := t.TempDir()
	exec := &stubExec{
		result: map[string]any{"status": "running", "session_id": "sess-2", "output": "boot\n"},
	}
	stdin := &stubStdin{outputs: []map[string]any{
		{"status": "running", "output": "boot\n", "session_id": "sess-2"},
		{"status": "running", "output": "boot\nready\n", "session_id": "sess-2"},
	}}
	ts := WrapSessionEnhance(&stubSet{tools: []trpctool.Tool{exec, stdin}}, dir, loggateway.NewNoop())
	var wt trpctool.CallableTool
	for _, t0 := range ts.Tools(context.Background()) {
		if t0.Declaration().Name == "write_stdin" {
			wt = t0.(trpctool.CallableTool)
		}
	}
	if wt == nil {
		t.Fatal("missing write_stdin")
	}
	if wt.Declaration().InputSchema == nil || wt.Declaration().InputSchema.Properties["notify_pattern"] == nil {
		t.Fatal("write_stdin schema missing notify_pattern")
	}
	out, err := wt.Call(context.Background(), []byte(`{"session_id":"sess-2","chars":"","notify_pattern":"ready","block_until_ms":2000}`))
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["notified"] != true {
		t.Fatalf("notified=%v", m["notified"])
	}
}

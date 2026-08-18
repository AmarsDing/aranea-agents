package rgsearch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestParseRipgrepJSON(t *testing.T) {
	stdout := `{"type":"match","data":{"path":{"text":"a.go"},"lines":{"text":"hello TODO\n"},"line_number":3}}
{"type":"begin","data":{}}
{"type":"match","data":{"path":{"text":"a.go"},"lines":{"text":"second TODO"},"line_number":8}}
{"type":"match","data":{"path":{"text":"b.go"},"lines":{"text":"other"},"line_number":1}}
`
	files, trunc := parseRipgrepJSON(stdout, "", "")
	if trunc {
		t.Fatal("small result must not truncate")
	}
	if len(files) != 2 {
		t.Fatalf("files=%d", len(files))
	}
	if files[0].FilePath != "a.go" || len(files[0].Matches) != 2 {
		t.Fatalf("a.go = %+v", files[0])
	}
	if files[0].Matches[0].LineNumber != 3 || files[0].Matches[0].LineContent != "hello TODO" {
		t.Fatalf("line = %+v", files[0].Matches[0])
	}
}

func TestParseRipgrepJSON_ContextLines(t *testing.T) {
	stdout := `{"type":"context","data":{"path":{"text":"a.go"},"lines":{"text":"before\n"},"line_number":2}}
{"type":"match","data":{"path":{"text":"a.go"},"lines":{"text":"hello TODO\n"},"line_number":3}}
{"type":"context","data":{"path":{"text":"a.go"},"lines":{"text":"after\n"},"line_number":4}}
`
	files, trunc := parseRipgrepJSON(stdout, "", "")
	if trunc {
		t.Fatal("small result must not truncate")
	}
	if len(files) != 1 || len(files[0].Matches) != 3 {
		t.Fatalf("%+v", files)
	}
	if files[0].Matches[0].Kind != "context" || files[0].Matches[1].Kind != "match" {
		t.Fatalf("kinds = %+v", files[0].Matches)
	}
}

func TestPaginateFileMatches(t *testing.T) {
	in := []*fileMatch{{
		FilePath: "a.go",
		Matches: []*lineMatch{
			{LineNumber: 1, LineContent: "c0", Kind: "context"},
			{LineNumber: 2, LineContent: "m0", Kind: "match"},
			{LineNumber: 3, LineContent: "c1", Kind: "context"},
			{LineNumber: 4, LineContent: "m1", Kind: "match"},
			{LineNumber: 5, LineContent: "c2", Kind: "context"},
			{LineNumber: 6, LineContent: "m2", Kind: "match"},
		},
	}}
	out, trunc := paginateFileMatches(in, 1, 1)
	if !trunc {
		t.Fatal("expected truncated")
	}
	if len(out) != 1 || len(out[0].Matches) != 3 {
		t.Fatalf("%+v", out[0].Matches)
	}
	if out[0].Matches[0].LineContent != "c1" || out[0].Matches[1].LineContent != "m1" || out[0].Matches[2].LineContent != "c2" {
		t.Fatalf("kept = %+v", out[0].Matches)
	}
}

func TestCapFileMatches(t *testing.T) {
	in := make([]*fileMatch, 0, 60)
	for i := 0; i < 60; i++ {
		in = append(in, &fileMatch{FilePath: "f", Matches: []*lineMatch{{LineNumber: 1, LineContent: "x"}}})
	}
	out, trunc := capFileMatches(in)
	if !trunc || len(out) != maxFiles {
		t.Fatalf("len=%d trunc=%v", len(out), trunc)
	}
}

func TestSearchContentRipgrep(t *testing.T) {
	inner := &stubSearch{result: map[string]any{"file_matches": []any{}, "message": "inner"}}
	run := func(ctx context.Context, dir string, args []string) (string, error) {
		if !contains(args, "TODO") {
			t.Fatalf("args=%v", args)
		}
		return `{"type":"match","data":{"path":{"text":"src/a.go"},"lines":{"text":"TODO here"},"line_number":4}}`, nil
	}
	ts := wrapToolSet(&stubSet{tools: []trpctool.Tool{inner}}, t.TempDir(), loggateway.NewNoop(), run)
	ct := ts.Tools(context.Background())[0].(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{"content_pattern":"TODO","file_pattern":"*.go","path":"src"}`))
	if err != nil {
		t.Fatal(err)
	}
	rsp := out.(searchResponse)
	if rsp.Engine != "ripgrep" || len(rsp.FileMatches) != 1 {
		t.Fatalf("%+v", rsp)
	}
	if inner.calls != 0 {
		t.Fatal("ripgrep hit must not call inner")
	}
}

func TestSearchContentFallbackWhenRgMissing(t *testing.T) {
	inner := &stubSearch{result: map[string]any{
		"file_matches": []map[string]any{{"file_path": "a.go", "matches": []map[string]any{{"line_number": 1, "line_content": "x"}}}},
		"message":      "Found 1 files matching",
	}}
	run := func(context.Context, string, []string) (string, error) {
		return "", errors.New("rg not found")
	}
	ts := wrapToolSet(&stubSet{tools: []trpctool.Tool{inner}}, t.TempDir(), loggateway.NewNoop(), run)
	ct := ts.Tools(context.Background())[0].(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{"content_pattern":"x","file_pattern":"*"}`))
	if err != nil {
		t.Fatal(err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner calls=%d", inner.calls)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("type %T", out)
	}
	if m["message"] != "Found 1 files matching" {
		t.Fatalf("%v", m)
	}
}

func TestSearchContentVirtualRefUsesInner(t *testing.T) {
	inner := &stubSearch{result: map[string]any{"file_matches": []any{}}}
	run := func(context.Context, string, []string) (string, error) {
		t.Fatal("rg must not run for artifact refs")
		return "", nil
	}
	ts := wrapToolSet(&stubSet{tools: []trpctool.Tool{inner}}, t.TempDir(), loggateway.NewNoop(), run)
	ct := ts.Tools(context.Background())[0].(trpctool.CallableTool)
	_, err := ct.Call(context.Background(), []byte(`{"content_pattern":"x","file_pattern":"artifact://a"}`))
	if err != nil {
		t.Fatal(err)
	}
	if inner.calls != 1 {
		t.Fatal("expected inner")
	}
}

func TestSearchContentRipgrepContextFlags(t *testing.T) {
	inner := &stubSearch{result: map[string]any{"file_matches": []any{}}}
	run := func(ctx context.Context, dir string, args []string) (string, error) {
		if !contains(args, "-A") || !contains(args, "-B") || !contains(args, "--type") || !contains(args, "-U") {
			t.Fatalf("args=%v", args)
		}
		return `{"type":"match","data":{"path":{"text":"a.go"},"lines":{"text":"TODO"},"line_number":1}}`, nil
	}
	ts := wrapToolSet(&stubSet{tools: []trpctool.Tool{inner}}, t.TempDir(), loggateway.NewNoop(), run)
	ct := ts.Tools(context.Background())[0].(trpctool.CallableTool)
	d := ct.Declaration()
	if d.InputSchema == nil || d.InputSchema.Properties["after"] == nil || d.InputSchema.Properties["head_limit"] == nil {
		t.Fatalf("schema=%+v", d.InputSchema)
	}
	_, err := ct.Call(context.Background(), []byte(`{"content_pattern":"TODO","file_pattern":"*.go","after":2,"before":1,"type":"go","multiline":true,"head_limit":3}`))
	if err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeRGType(t *testing.T) {
	if got := sanitizeRGType("go"); got != "go" {
		t.Fatalf("got %q", got)
	}
	if got := sanitizeRGType("go; rm -rf"); got != "" {
		t.Fatalf("rejected type leaked: %q", got)
	}
}

func TestExecRipgrepNoMatchIsSuccess(t *testing.T) {
	if _, err := os.Stat("."); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	inner := &stubSearch{result: map[string]any{"file_matches": []any{}}}
	ts := WrapToolSet(&stubSet{tools: []trpctool.Tool{inner}}, dir, loggateway.NewNoop())
	ct := ts.Tools(context.Background())[0].(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{"content_pattern":"zzz_not_present_zzz","file_pattern":"*"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "files matching") && inner.calls == 0 {
		t.Fatalf("unexpected %s", raw)
	}
}

type stubSet struct{ tools []trpctool.Tool }

func (s *stubSet) Tools(context.Context) []trpctool.Tool { return s.tools }
func (s *stubSet) Close() error                          { return nil }
func (s *stubSet) Name() string                          { return "file" }

type stubSearch struct {
	calls  int
	result any
}

func (s *stubSearch) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: "search_content"}
}
func (s *stubSearch) Call(context.Context, []byte) (any, error) {
	s.calls++
	return s.result, nil
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

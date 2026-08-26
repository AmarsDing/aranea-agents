package memory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcmemtool "trpc.group/trpc-go/trpc-agent-go/memory/tool"
)

// fakeInnerTool stubs the upstream delegate.
type fakeInnerTool struct {
	decl *trpctool.Declaration
	res  any
	err  error
}

func (f *fakeInnerTool) Declaration() *trpctool.Declaration { return f.decl }
func (f *fakeInnerTool) Call(context.Context, []byte) (any, error) {
	return f.res, f.err
}

func TestEmptyNoteSearchTool_ZeroHitAnnotates(t *testing.T) {
	inner := &fakeInnerTool{
		decl: &trpctool.Declaration{Name: "memory_search"},
		res:  &trpcmemtool.SearchMemoryResponse{Query: "q", Results: []trpcmemtool.Result{}, Count: 0},
	}
	tool := &emptyNoteSearchTool{inner: inner}
	res, err := tool.Call(context.Background(), []byte(`{"query":"q"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	wrapped, ok := res.(*searchMemoryResponseWithNote)
	if !ok {
		t.Fatalf("zero-hit response type = %T, want *searchMemoryResponseWithNote", res)
	}
	if !strings.Contains(wrapped.Note, "0 结果不代表未发生/未收录") {
		t.Fatalf("note must carry the R3 guidance phrase, got %q", wrapped.Note)
	}
	// JSON 序列化平铺：note 与 query/results/count 同级。
	raw, _ := json.Marshal(res)
	var flat map[string]any
	if err := json.Unmarshal(raw, &flat); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, ok := flat["note"]; !ok {
		t.Fatalf("serialized response must expose note field: %s", raw)
	}
	if flat["count"].(float64) != 0 {
		t.Fatalf("count must stay 0: %s", raw)
	}
}

func TestEmptyNoteSearchTool_HitPassThrough(t *testing.T) {
	hit := &trpcmemtool.SearchMemoryResponse{
		Query:   "q",
		Results: []trpcmemtool.Result{{ID: "m1", Memory: "likes tea"}},
		Count:   1,
	}
	tool := &emptyNoteSearchTool{inner: &fakeInnerTool{decl: &trpctool.Declaration{Name: "memory_search"}, res: hit}}
	res, err := tool.Call(context.Background(), []byte(`{"query":"q"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res != any(hit) {
		t.Fatalf("non-zero-hit must pass through the upstream response untouched, got %T", res)
	}
}

func TestEmptyNoteSearchTool_ErrorPassThrough(t *testing.T) {
	wantErr := errors.New("search backend down")
	tool := &emptyNoteSearchTool{inner: &fakeInnerTool{decl: &trpctool.Declaration{Name: "memory_search"}, err: wantErr}}
	if _, err := tool.Call(context.Background(), []byte(`{"query":"q"}`)); !errors.Is(err, wantErr) {
		t.Fatalf("error must pass through, got %v", err)
	}
}

func TestEmptyNoteLoadTool_ZeroHitAnnotates(t *testing.T) {
	inner := &fakeInnerTool{
		decl: &trpctool.Declaration{Name: "memory_load"},
		res:  &trpcmemtool.LoadMemoryResponse{Limit: 10, Results: []trpcmemtool.Result{}, Count: 0},
	}
	tool := &emptyNoteLoadTool{inner: inner}
	res, err := tool.Call(context.Background(), []byte(`{"limit":10}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	wrapped, ok := res.(*loadMemoryResponseWithNote)
	if !ok {
		t.Fatalf("zero-hit response type = %T, want *loadMemoryResponseWithNote", res)
	}
	if !strings.Contains(wrapped.Note, "0 结果不代表未发生/未收录") {
		t.Fatalf("note must carry the R3 guidance phrase, got %q", wrapped.Note)
	}
}

func TestEmptyNoteLoadTool_HitPassThrough(t *testing.T) {
	hit := &trpcmemtool.LoadMemoryResponse{
		Limit:   10,
		Results: []trpcmemtool.Result{{ID: "m1"}},
		Count:   1,
	}
	tool := &emptyNoteLoadTool{inner: &fakeInnerTool{decl: &trpctool.Declaration{Name: "memory_load"}, res: hit}}
	res, err := tool.Call(context.Background(), []byte(`{"limit":10}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res != any(hit) {
		t.Fatalf("non-zero-hit must pass through untouched, got %T", res)
	}
}

func TestEmptyNoteTools_DeclarationDelegated(t *testing.T) {
	decl := &trpctool.Declaration{Name: "memory_search", Description: "d"}
	tool := &emptyNoteSearchTool{inner: &fakeInnerTool{decl: decl}}
	if tool.Declaration() != decl {
		t.Fatal("declaration must be delegated verbatim (name/schema parity with upstream)")
	}
}

// TestDefaultTools_WrappedReadTools pins the assembly: the read tools in
// DefaultTools are the zero-hit-annotating wrappers, the write tools are not.
func TestDefaultTools_WrappedReadTools(t *testing.T) {
	tools := DefaultTools()
	var sawSearch, sawLoad bool
	for _, tl := range tools {
		switch tl.Declaration().Name {
		case "memory_search":
			if _, ok := tl.(*emptyNoteSearchTool); !ok {
				t.Fatalf("memory_search must be the note wrapper, got %T", tl)
			}
			sawSearch = true
		case "memory_load":
			if _, ok := tl.(*emptyNoteLoadTool); !ok {
				t.Fatalf("memory_load must be the note wrapper, got %T", tl)
			}
			sawLoad = true
		}
	}
	if !sawSearch || !sawLoad {
		t.Fatalf("read tools missing from DefaultTools: search=%v load=%v", sawSearch, sawLoad)
	}
}

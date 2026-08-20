package knowledge

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/knowledge"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestNewSearchTool_ReturnsCallableTool(t *testing.T) {
	tool := NewSearchTool(nil)
	if tool == nil {
		t.Fatal("NewSearchTool returned nil")
	}

	var _ trpctool.CallableTool = tool

	decl := tool.Declaration()
	if decl == nil {
		t.Fatal("Declaration() returned nil")
	}
	if decl.Name != "knowledge_search" {
		t.Fatalf("expected declaration name %q, got %q", "knowledge_search", decl.Name)
	}
}

func TestWithRetriever_RoundTrip(t *testing.T) {
	ctx := context.Background()

	if got := RetrieverFromContext(ctx); got != nil {
		t.Fatalf("expected nil retriever from empty context, got %v", got)
	}

	r := &knowledge.Retriever{}
	ctx = WithRetriever(ctx, r)

	got := RetrieverFromContext(ctx)
	if got != r {
		t.Fatalf("expected retriever %p, got %p", r, got)
	}
}

func TestWithKnowledgeCollections_FiltersEmptyIDs(t *testing.T) {
	ctx := context.Background()

	ctx = WithKnowledgeCollections(ctx, []string{"", "  ", "col-1", "", "col-2", "   "})
	got := knowledgeCollectionsFromContext(ctx)

	if len(got) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(got))
	}
	if got[0] != "col-1" || got[1] != "col-2" {
		t.Fatalf("expected [col-1, col-2], got %v", got)
	}
}

func TestWithKnowledgeCollections_AllEmpty_ReturnsUnmodifiedContext(t *testing.T) {
	ctx := context.Background()

	ctx2 := WithKnowledgeCollections(ctx, []string{"", "  ", "\t"})
	got := knowledgeCollectionsFromContext(ctx2)

	if len(got) != 0 {
		t.Fatalf("expected 0 collections, got %d", len(got))
	}

	if ctx2 != ctx {
		t.Fatal("expected context to be unmodified when all IDs are empty")
	}
}

func TestCallTool_NoRetriever_ReturnsError(t *testing.T) {
	tool := NewSearchTool(nil)
	ctx := context.Background()

	args, _ := json.Marshal(searchInput{
		CollectionID: "col-1",
		Query:        "hello",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error when no retriever in context, got nil")
	}
}

func TestCallTool_EmptyCollectionID_NoScopedCollections_ReturnsError(t *testing.T) {
	tool := NewSearchTool(nil)
	ctx := context.Background()

	args, _ := json.Marshal(searchInput{
		CollectionID: "",
		Query:        "hello",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error when collection_id is empty and no scoped collections, got nil")
	}
}

func TestNewReflectTool_ReturnsCallableTool(t *testing.T) {
	tool := NewReflectTool(nil)
	if tool == nil {
		t.Fatal("NewReflectTool returned nil")
	}

	var _ trpctool.CallableTool = tool

	decl := tool.Declaration()
	if decl == nil {
		t.Fatal("Declaration() returned nil")
	}
	if decl.Name != "knowledge_reflect" {
		t.Fatalf("expected declaration name %q, got %q", "knowledge_reflect", decl.Name)
	}
}

func TestReflectTool_MissingCollectionIDs(t *testing.T) {
	tool := NewReflectTool(nil)
	ctx := context.Background()

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: []string{},
		Query:         "hello",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error when collection_ids is empty and no scoped context, got nil")
	}
}

func TestReflectTool_MissingQuery(t *testing.T) {
	tool := NewReflectTool(nil)
	ctx := context.Background()

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: []string{"col-1"},
		Query:         "",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error when query is empty, got nil")
	}
}

func TestReflectTool_UnscopedCollectionID(t *testing.T) {
	tool := NewReflectTool(nil)
	ctx := context.Background()
	ctx = WithKnowledgeCollections(ctx, []string{"col-1", "col-2"})

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: []string{"col-1", "col-3"},
		Query:         "hello",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error when collection_id not in scoped context, got nil")
	}
}

func TestSearchTool_UnscopedCollectionID(t *testing.T) {
	tool := NewSearchTool(nil)
	ctx := context.Background()
	ctx = WithKnowledgeCollections(ctx, []string{"col-1", "col-2"})

	args, _ := json.Marshal(searchInput{
		CollectionID: "col-3",
		Query:        "hello",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error when collection_id not in scoped context, got nil")
	}
}

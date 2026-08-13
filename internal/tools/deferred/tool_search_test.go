package deferred

import (
	"context"
	"testing"
)

func TestToolSearchTool_SearchByQuery(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "arxiv_search", Description: "Search academic papers on arXiv", Category: "search"},
		{Name: "wikipedia_search", Description: "Search Wikipedia articles", Category: "search"},
		{Name: "email_send", Description: "Send email messages", Category: "communication"},
	}
	tool := NewToolSearchTool(catalog)
	result, err := tool.Call(context.Background(), []byte(`{"query": "search"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output, ok := result.(toolSearchOutput)
	if !ok {
		t.Fatalf("expected toolSearchOutput, got %T", result)
	}
	if len(output.Tools) != 2 {
		t.Fatalf("expected 2 results, got %d", len(output.Tools))
	}
}

func TestToolSearchTool_NoMatch(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "arxiv_search", Description: "Search academic papers", Category: "search"},
	}
	tool := NewToolSearchTool(catalog)
	result, err := tool.Call(context.Background(), []byte(`{"query": "nonexistent"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output, ok := result.(toolSearchOutput)
	if !ok {
		t.Fatalf("expected toolSearchOutput, got %T", result)
	}
	if len(output.Tools) != 0 {
		t.Fatalf("expected 0 results, got %d", len(output.Tools))
	}
	if output.Suggestion == "" {
		t.Fatal("expected suggestion for no results")
	}
}

func TestToolSearchTool_Declaration(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "test_tool", Description: "A test tool", Category: "test"},
	}
	tool := NewToolSearchTool(catalog)
	decl := tool.Declaration()
	if decl.Name != "tool_search" {
		t.Fatalf("expected name tool_search, got %s", decl.Name)
	}
}

func TestToolSearchTool_CatalogNames(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "tool_a", Description: "A", Category: "test"},
		{Name: "tool_b", Description: "B", Category: "test"},
	}
	tool := NewToolSearchTool(catalog)
	names := tool.CatalogNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

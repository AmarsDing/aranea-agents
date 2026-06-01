package trpc_test

import (
	"context"
	"testing"

	"aranea-agents/internal/tools/trpc"
)

func TestBuildToolsets_EmptyConfig(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if len(out.Tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(out.Tools))
	}
	if len(out.ToolSets) != 0 {
		t.Fatalf("expected 0 toolsets, got %d", len(out.ToolSets))
	}
}

func TestBuildToolsets_TodoEnabled(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		Todo: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "todo_write" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected todo_write tool in assembled tools")
	}
}

func TestBuildToolsets_WebSearchEnabled(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		WebSearch: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "duckduckgo_search" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected duckduckgo_search tool in assembled tools")
	}
}

func TestBuildToolsets_AwaitReplyNoHook(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		AwaitReply: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "await_user_reply" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected await_user_reply tool when AwaitReply=true and AwaitHook=nil")
	}
}

func TestBuildToolsets_AwaitReplyWithHook(t *testing.T) {
	var hook trpc.ReplyFunc = func(ctx context.Context) (string, error) { return "", nil }
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		AwaitReply: true,
		AwaitHook:  hook,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	foundFramework := false
	foundService := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil {
			if d.Name == "await_user_reply" {
				foundFramework = true
			}
		}
	}
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "await_user_reply" {
			foundService = true
		}
	}
	if foundFramework && !foundService {
		t.Fatal("expected await_user_reply service tool when AwaitHook is set")
	}
}

func TestBuildToolsets_CustomTools(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		CustomTools: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestBuildToolsets_KnowledgeSearchEnabled(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		KnowledgeSearch: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "knowledge_search" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected knowledge_search tool when KnowledgeSearch=true")
	}
}

func TestBuildToolsets_KnowledgeReflectEnabled(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		KnowledgeReflect: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "knowledge_reflect" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected knowledge_reflect tool when KnowledgeReflect=true")
	}
}

func TestBuildToolsets_CallAgentEnabled(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		CallAgent: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "call_agent" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected call_agent tool when CallAgent=true")
	}
}

func TestBuildToolsets_MemoryEnabled(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		MemoryEnabled: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	memNames := map[string]bool{
		"memory_add":    true,
		"memory_update": true,
		"memory_load":   true,
		"memory_search": true,
		"memory_delete": true,
	}
	found := 0
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && memNames[d.Name] {
			found++
		}
	}
	if found < 5 {
		t.Fatalf("expected 5 memory tools, found %d", found)
	}
}

func TestBuildToolsets_WebResearchNotReady(t *testing.T) {
	_, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		WebResearch: true,
	})
	if err == nil {
		t.Fatal("expected error when WebResearch=true but config not ready")
	}
}

func TestBuildToolsets_MultipleFlags(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		Todo:      true,
		WebSearch: true,
		Email:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Tools) == 0 && len(out.ToolSets) == 0 {
		t.Fatal("expected some tools/toolsets with multiple flags enabled")
	}
}

func TestBuildToolsets_ArxivSearchEnabled(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		ArxivSearch: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ToolSets) == 0 {
		t.Fatal("expected at least one toolset for arxiv_search")
	}
}

func TestBuildToolsets_WikipediaEnabled(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		Wikipedia: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ToolSets) == 0 {
		t.Fatal("expected at least one toolset for wikipedia")
	}
}

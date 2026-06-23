package trpc_test

import (
	"context"
	"testing"

	"aranea-agents/internal/tools/subagent"
	"aranea-agents/internal/tools/trpc"
)

func TestBuildToolsets_EmptyConfig(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{}, nil)
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
	}, nil)
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
	}, nil)
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
	}, nil)
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
	}, nil)
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
	}, nil)
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
	}, nil)
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
	}, nil)
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
	}, nil)
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
	}, nil)
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
	}, nil)
	if err == nil {
		t.Fatal("expected error when WebResearch=true but config not ready")
	}
}

func TestBuildToolsets_MultipleFlags(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		Todo:      true,
		WebSearch: true,
		Email:     true,
	}, nil)
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
	}, nil)
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
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.ToolSets) == 0 {
		t.Fatal("expected at least one toolset for wikipedia")
	}
}

func TestBuildToolsets_SubAgentNotAutoEnabled(t *testing.T) {
	// SubAgentService wired but no subagent flag: tools must NOT be auto-added.
	// This prevents every agent from gaining subagent capability just because
	// the service is available.
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		Todo:            true,
		SubAgentService: &subagent.Service{},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil {
			if d.Name == "subagents_spawn" || d.Name == "subagents_list" || d.Name == "subagents_get" || d.Name == "subagents_cancel" {
				t.Fatalf("subagent tool %q should not be auto-enabled when SubAgent=false", d.Name)
			}
		}
	}
}

func TestBuildToolsets_SubAgentEnabledWhenFlagSet(t *testing.T) {
	out, err := trpc.BuildToolsets(context.Background(), trpc.ToolsetConfig{
		SubAgent:        true,
		SubAgentService: &subagent.Service{},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, tool := range out.Tools {
		if d := tool.Declaration(); d != nil && d.Name == "subagents_spawn" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected subagents_spawn when SubAgent=true and service is wired")
	}
}

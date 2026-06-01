package tools

import (
	"strings"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type disambigMockTool struct {
	decl *trpctool.Declaration
}

func (m *disambigMockTool) Declaration() *trpctool.Declaration {
	return m.decl
}

func TestApplyDisambiguationHints_WithExamples(t *testing.T) {
	_ = Registry()
	origRegistry := registry
	defer func() { registry = origRegistry }()
	registry = []*ToolRegistration{
		{
			Name:        "duckduckgo",
			Description: "Search the web using DuckDuckGo",
			Examples: []ToolUseExample{
				{UserQuery: "search for recent AI news", ToolName: "duckduckgo", Explanation: "general web search"},
			},
		},
	}
	tools := []trpctool.Tool{
		&disambigMockTool{decl: &trpctool.Declaration{Name: "duckduckgo", Description: "Search the web using DuckDuckGo"}},
	}
	ApplyDisambiguationHints(tools)
	desc := tools[0].Declaration().Description
	if !strings.Contains(desc, "When user asks") {
		t.Fatalf("expected example hint in description, got: %s", desc)
	}
}

func TestApplyDisambiguationHints_WithGroup(t *testing.T) {
	_ = Registry()
	origRegistry := registry
	defer func() { registry = origRegistry }()
	registry = []*ToolRegistration{
		{Name: "web_research", Description: "Web research tool", Group: "web_search"},
		{Name: "duckduckgo", Description: "DuckDuckGo search", Group: "web_search"},
	}
	tools := []trpctool.Tool{
		&disambigMockTool{decl: &trpctool.Declaration{Name: "web_research", Description: "Web research tool"}},
	}
	ApplyDisambiguationHints(tools)
	desc := tools[0].Declaration().Description
	if !strings.Contains(desc, "Alternatives") {
		t.Fatalf("expected group alternatives hint, got: %s", desc)
	}
}

func TestApplyDisambiguationHints_NoHints(t *testing.T) {
	_ = Registry()
	origRegistry := registry
	defer func() { registry = origRegistry }()
	registry = []*ToolRegistration{
		{Name: "file", Description: "File operations"},
	}
	tools := []trpctool.Tool{
		&disambigMockTool{decl: &trpctool.Declaration{Name: "file", Description: "File operations"}},
	}
	ApplyDisambiguationHints(tools)
	desc := tools[0].Declaration().Description
	if desc != "File operations" {
		t.Fatalf("expected unchanged description, got: %s", desc)
	}
}

package tools

import (
	"context"
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

type disambigMockCallableTool struct {
	decl *trpctool.Declaration
}

func (m *disambigMockCallableTool) Declaration() *trpctool.Declaration { return m.decl }
func (m *disambigMockCallableTool) Call(_ context.Context, _ []byte) (any, error) {
	return "called", nil
}

type disambigMockStreamableTool struct {
	decl *trpctool.Declaration
}

func (m *disambigMockStreamableTool) Declaration() *trpctool.Declaration { return m.decl }
func (m *disambigMockStreamableTool) Call(_ context.Context, _ []byte) (any, error) {
	return "called", nil
}
func (m *disambigMockStreamableTool) StreamableCall(_ context.Context, _ []byte) (*trpctool.StreamReader, error) {
	s := trpctool.NewStream(1)
	go func() {
		s.Writer.Send(trpctool.StreamChunk{Content: "chunk"}, nil)
		s.Writer.Close()
	}()
	return s.Reader, nil
}

// Regression (2026-07-18): disambiguatedTool must not unconditionally satisfy
// trpctool.StreamableTool — see TestApplyRuntimeNameAliases_nonStreamableInner_notMisclassified.
func TestApplyDisambiguationHints_nonStreamableInner_notMisclassified(t *testing.T) {
	_ = Registry()
	origRegistry := registry
	defer func() { registry = origRegistry }()
	registry = []*ToolRegistration{
		{
			Name:     "duckduckgo",
			Examples: []ToolUseExample{{UserQuery: "q", ToolName: "duckduckgo"}},
		},
	}
	tools := []trpctool.Tool{
		&disambigMockCallableTool{decl: &trpctool.Declaration{Name: "duckduckgo"}},
	}
	ApplyDisambiguationHints(tools)
	if _, ok := tools[0].(trpctool.StreamableTool); ok {
		t.Fatal("disambiguated non-streamable tool must not satisfy StreamableTool")
	}
}

func TestApplyDisambiguationHints_streamableInner_staysStreamable(t *testing.T) {
	_ = Registry()
	origRegistry := registry
	defer func() { registry = origRegistry }()
	registry = []*ToolRegistration{
		{
			Name:     "duckduckgo",
			Examples: []ToolUseExample{{UserQuery: "q", ToolName: "duckduckgo"}},
		},
	}
	tools := []trpctool.Tool{
		&disambigMockStreamableTool{decl: &trpctool.Declaration{Name: "duckduckgo"}},
	}
	ApplyDisambiguationHints(tools)
	st, ok := tools[0].(trpctool.StreamableTool)
	if !ok {
		t.Fatal("disambiguated streamable tool must satisfy StreamableTool")
	}
	r, err := st.StreamableCall(context.Background(), []byte(`{}`))
	if err != nil || r == nil {
		t.Fatalf("StreamableCall should delegate to inner: err=%v reader=%v", err, r)
	}
}

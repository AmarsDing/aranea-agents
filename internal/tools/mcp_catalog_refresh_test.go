package tools

import (
	"context"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type fakeMCPCache struct{ n int }

func (f *fakeMCPCache) Name() string                          { return "mcp" }
func (f *fakeMCPCache) Tools(context.Context) []trpctool.Tool { return nil }
func (f *fakeMCPCache) Close() error                          { return nil }
func (f *fakeMCPCache) InvalidateToolsCache()                 { f.n++ }

func TestCollectMCPCacheInvalidators(t *testing.T) {
	t.Parallel()
	inner := &fakeMCPCache{}
	sets := []trpctool.ToolSet{
		&mcpSchemaGovernedToolSet{inner: inner},
		&governFakeToolSet{name: "files"},
	}
	got := CollectMCPCacheInvalidators(sets)
	if len(got) != 1 {
		t.Fatalf("want 1 invalidator, got %d", len(got))
	}
	got[0].InvalidateToolsCache()
	if inner.n != 1 {
		t.Fatalf("forwarded Invalidate calls = %d", inner.n)
	}
}

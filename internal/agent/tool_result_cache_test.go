package agent

import (
	"context"
	"testing"

	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/tools/cache"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// PERF-1 回归：缓存策略必须来自构建期快照（toolBuildCatalog 的
// MetadataJSON/ConfigJSON），禁止在每次工具调用时跑 GetTool 聚合查询
// （此前每次调用 2 个 hook 各一次 ~49ms 聚合 SQL）。快照语义安全：
// 工具 CRUD 会 invalidateAllAgentBuildCaches，策略变更随重建生效。
func TestToolResultCacheHooks_PolicyFromSnapshot(t *testing.T) {
	fk := &fakeToolLookup{}
	deps := TRPCBuilderDeps{}
	deps.ToolUC = fk
	deps.ResultCache = cache.NewResultCache(8)

	catalog := &toolBuildCatalog{
		entries: map[string]biztool.ToolCatalogEntry{
			"web_fetch": {Key: "web_fetch", MetadataJSON: `{"cache_enabled":true,"cache_ttl_sec":60}`},
			"plain":     {Key: "plain"},
		},
		overrides: map[string]biztool.ToolAgentOverride{},
	}

	before := newToolResultCacheBeforeHook(deps, catalog)
	after := newToolResultCacheAfterHook(deps, catalog)
	ctx := context.Background()
	args := []byte(`{"url":"https://example.com"}`)

	// 首次未命中 → 无 CustomResult。
	res, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "web_fetch", Arguments: args})
	if err != nil || res == nil || res.CustomResult != nil {
		t.Fatalf("first call must miss, got %+v err=%v", res, err)
	}
	// after hook 按快照策略写入缓存。
	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "web_fetch", Arguments: args, Result: "PAGE"}); err != nil {
		t.Fatal(err)
	}
	// 二次命中。
	res, err = before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "web_fetch", Arguments: args})
	if err != nil || res == nil || res.CustomResult != "PAGE" {
		t.Fatalf("second call must hit with cached PAGE, got %+v err=%v", res, err)
	}
	if fk.getToolCalls != 0 {
		t.Fatalf("GetTool called %d times; policy must come from the build snapshot", fk.getToolCalls)
	}

	// 策略未启用缓存的工具：不写不读。
	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "plain", Arguments: args, Result: "X"}); err != nil {
		t.Fatal(err)
	}
	res, _ = before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "plain", Arguments: args})
	if res == nil || res.CustomResult != nil {
		t.Fatalf("plain must not be cached, got %+v", res)
	}

	// 快照外的工具（框架内建/无目录行）：不缓存、不查库。
	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "ghost", Arguments: args, Result: "X"}); err != nil {
		t.Fatal(err)
	}
	res, _ = before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "ghost", Arguments: args})
	if res == nil || res.CustomResult != nil {
		t.Fatalf("ghost must not be cached, got %+v", res)
	}
	if fk.getToolCalls != 0 {
		t.Fatalf("GetTool called %d times for snapshot misses", fk.getToolCalls)
	}
}

// nil 快照（无启用工具等）：hooks 安全跳过且不触库。
func TestToolResultCacheHooks_NilCatalog(t *testing.T) {
	fk := &fakeToolLookup{}
	deps := TRPCBuilderDeps{}
	deps.ToolUC = fk
	deps.ResultCache = cache.NewResultCache(8)

	before := newToolResultCacheBeforeHook(deps, nil)
	after := newToolResultCacheAfterHook(deps, nil)
	ctx := context.Background()

	res, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "web_fetch", Arguments: []byte(`{}`)})
	if err != nil || res == nil || res.CustomResult != nil {
		t.Fatalf("nil catalog must miss, got %+v err=%v", res, err)
	}
	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "web_fetch", Arguments: []byte(`{}`), Result: "X"}); err != nil {
		t.Fatal(err)
	}
	if fk.getToolCalls != 0 {
		t.Fatalf("nil catalog must not query GetTool, got %d calls", fk.getToolCalls)
	}
}

// 未注入 ResultCache 时回退到包级默认实例（等价历史 cache.Global 单例），
// 保证 prod 无 Wire 注入路径行为不变。
func TestToolResultCacheHooks_DefaultCacheFallback(t *testing.T) {
	deps := TRPCBuilderDeps{} // ResultCache 未注入
	catalog := &toolBuildCatalog{
		entries: map[string]biztool.ToolCatalogEntry{
			"web_fetch": {Key: "web_fetch", ConfigJSON: `{"cache_enabled":true,"cache_ttl_sec":60}`},
	},
	overrides: map[string]biztool.ToolAgentOverride{},
	}
	before := newToolResultCacheBeforeHook(deps, catalog)
	after := newToolResultCacheAfterHook(deps, catalog)
	ctx := context.Background()
	args := []byte(`{"k":"default-fallback"}`)

	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "web_fetch", Arguments: args, Result: "D"}); err != nil {
		t.Fatal(err)
	}
	res, _ := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "web_fetch", Arguments: args})
	if res == nil || res.CustomResult != "D" {
		t.Fatalf("default cache fallback must store/hit, got %+v", res)
	}
}

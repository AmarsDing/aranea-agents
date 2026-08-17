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
//
// 装饰器已缓存 IsCacheable 网络工具（web_fetch 等），回调 ResultCache
// 不得再写一份。写工具与 file 族即使 cache_enabled 也不进回调缓存。
func TestToolResultCacheHooks_PolicyFromSnapshot(t *testing.T) {
	fk := &fakeToolLookup{}
	deps := TRPCBuilderDeps{}
	deps.ToolUC = fk
	deps.ResultCache = cache.NewResultCache(8)

	catalog := &toolBuildCatalog{
		entries: map[string]biztool.ToolCatalogEntry{
			"web_fetch": {Key: "web_fetch", MetadataJSON: `{"cache_enabled":true,"cache_ttl_sec":60}`},
			"save_file": {Key: "save_file", MetadataJSON: `{"cache_enabled":true,"cache_ttl_sec":60}`},
			"plain":     {Key: "plain"},
		},
		overrides: map[string]biztool.ToolAgentOverride{},
	}

	before := newToolResultCacheBeforeHook(deps, catalog)
	after := newToolResultCacheAfterHook(deps, catalog)
	ctx := context.Background()
	args := []byte(`{"url":"https://example.com"}`)

	res, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "web_fetch", Arguments: args})
	if err != nil || res == nil || res.CustomResult != nil {
		t.Fatalf("first call must miss, got %+v err=%v", res, err)
	}
	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "web_fetch", Arguments: args, Result: "PAGE"}); err != nil {
		t.Fatal(err)
	}
	res, err = before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "web_fetch", Arguments: args})
	if err != nil || res == nil || res.CustomResult != nil {
		t.Fatalf("web_fetch must not use callback cache (decorator owns it), got %+v err=%v", res, err)
	}
	if fk.getToolCalls != 0 {
		t.Fatalf("GetTool called %d times; policy must come from the build snapshot", fk.getToolCalls)
	}

	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "save_file", Arguments: args, Result: "W"}); err != nil {
		t.Fatal(err)
	}
	res, _ = before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "save_file", Arguments: args})
	if res == nil || res.CustomResult != nil {
		t.Fatalf("save_file must not be cached, got %+v", res)
	}

	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "plain", Arguments: args, Result: "X"}); err != nil {
		t.Fatal(err)
	}
	res, _ = before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "plain", Arguments: args})
	if res == nil || res.CustomResult != nil {
		t.Fatalf("plain must not be cached, got %+v", res)
	}

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

// 未注入 ResultCache 时回退到包级默认实例（等价历史 cache.Global 单例）。
func TestToolResultCacheHooks_DefaultCacheFallback(t *testing.T) {
	deps := TRPCBuilderDeps{} // ResultCache 未注入
	if deps.resultCache() != defaultToolResultCache {
		t.Fatal("nil ResultCache must use the process-wide default instance")
	}
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
	if res == nil || res.CustomResult != nil {
		t.Fatalf("web_fetch must not populate callback cache, got %+v", res)
	}
}

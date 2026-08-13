package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/cache"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// defaultToolResultCache is the process-wide result cache used when
// deps.ResultCache is not injected. It replaces the deprecated
// cache.Global() singleton (same 512-entry capacity, same shared-across-agents
// semantics); tests inject isolated instances via TRPCExtensionDeps.ResultCache.
var defaultToolResultCache = cache.NewResultCache(512)

// resultCache returns the result cache for this build: constructor-injected
// when present, else the process-wide default.
func (d TRPCBuilderDeps) resultCache() *cache.ResultCache {
	if d.ResultCache != nil {
		return d.ResultCache
	}
	return defaultToolResultCache
}

// cachePolicyFromSnapshot resolves the result-cache policy from the per-build
// catalog snapshot (PERF-1), replacing the previous per-invocation GetTool
// aggregation query (~49ms × 2 hooks per tool call). Snapshot semantics are
// safe: tool CRUD evicts all cached agents (service/tool.go →
// invalidateAllAgentBuildCaches), so policy changes take effect on rebuild.
// Tools absent from the snapshot (framework builtins, rows deleted mid-build)
// get a disabled policy — fail-soft, matching the old GetTool error path.
func cachePolicyFromSnapshot(catalog *toolBuildCatalog, toolKey string) cache.CachePolicy {
	if catalog == nil {
		return cache.CachePolicy{}
	}
	e, ok := catalog.entries[toolKey]
	if !ok {
		return cache.CachePolicy{}
	}
	return cache.PolicyFromToolJSON(e.MetadataJSON, e.ConfigJSON)
}

func newToolResultCacheBeforeHook(deps TRPCBuilderDeps, catalog *toolBuildCatalog) callbacks.BeforeToolHook {
	lg := deps.Logger()
	rc := deps.resultCache()
	return callbacks.NewBeforeToolHook(4, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		toolKey := strings.TrimSpace(args.ToolName)
		if toolKey == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		if !cachePolicyFromSnapshot(catalog, toolKey).Enabled {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		if hit, ok := rc.Get(toolKey, args.Arguments); ok {
			lg.Info("工具结果缓存命中", loggateway.StepID("agent.tool.cache_hit"), loggateway.Phase("done"), loggateway.Str("tool", toolKey))
			return &trpctool.BeforeToolResult{Context: ctx, CustomResult: hit}, nil
		}
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	})
}

func newToolResultCacheAfterHook(deps TRPCBuilderDeps, catalog *toolBuildCatalog) callbacks.AfterToolHook {
	rc := deps.resultCache()
	return callbacks.NewToolRecorderCallback(45, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil || args.Error != nil || args.Result == nil {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		toolKey := strings.TrimSpace(args.ToolName)
		if toolKey == "" {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		policy := cachePolicyFromSnapshot(catalog, toolKey)
		if !policy.Enabled {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		rc.Put(toolKey, args.Arguments, args.Result, policy.TTL)
		return &trpctool.AfterToolResult{Context: ctx}, nil
	})
}

// classifyToolInvocationStreaming derives streaming flags from tool catalog + MCP meta.
func classifyToolInvocationStreaming(tool biz.Tool, meta map[string]any) (streaming bool, chunkCount int) {
	if tool.SupportsStreaming {
		streaming = true
	}
	if meta == nil {
		return streaming, 0
	}
	if v, ok := meta["chunk_count"]; ok {
		switch n := v.(type) {
		case float64:
			chunkCount = int(n)
		case int:
			chunkCount = n
		}
	}
	if v, ok := meta["streaming"]; ok {
		if b, ok := v.(bool); ok && b {
			streaming = true
		}
	}
	if chunkCount > 0 {
		streaming = true
	}
	return streaming, chunkCount
}

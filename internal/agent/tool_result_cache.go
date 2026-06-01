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

func newToolResultCacheBeforeHook(deps TRPCBuilderDeps) callbacks.BeforeToolHook {
	lg := deps.Logger()
	return callbacks.NewBeforeToolHook(4, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil || deps.ToolUC == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		toolKey := strings.TrimSpace(args.ToolName)
		if toolKey == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		tool, err := deps.ToolUC.GetTool(ctx, toolKey)
		if err != nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		policy := cache.PolicyFromToolJSON(tool.MetadataJSON, tool.ConfigJSON)
		if !policy.Enabled {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		if hit, ok := cache.Global().Get(toolKey, args.Arguments); ok {
			lg.Info("工具结果缓存命中", loggateway.StepID("agent.tool.cache_hit"), loggateway.Phase("done"), loggateway.Str("tool", toolKey))
			return &trpctool.BeforeToolResult{Context: ctx, CustomResult: hit}, nil
		}
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	})
}

func newToolResultCacheAfterHook(deps TRPCBuilderDeps) callbacks.AfterToolHook {
	return callbacks.NewToolRecorderCallback(45, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil || args.Error != nil || args.Result == nil || deps.ToolUC == nil {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		toolKey := strings.TrimSpace(args.ToolName)
		if toolKey == "" {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		tool, err := deps.ToolUC.GetTool(ctx, toolKey)
		if err != nil {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		policy := cache.PolicyFromToolJSON(tool.MetadataJSON, tool.ConfigJSON)
		if !policy.Enabled {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		cache.Global().Put(toolKey, args.Arguments, args.Result, policy.TTL)
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

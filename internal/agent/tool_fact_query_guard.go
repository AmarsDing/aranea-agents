package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// factQueryWebGuardBeforeHook blocks deferred page-fetch / generic search
// tools on light-gear fact queries so the Spirit uses resident web_research
// (or datetime) instead of scraping search-engine HTML.
type factQueryWebGuardBeforeHook struct{}

func newFactQueryWebGuardBeforeHook(webResearchReady bool) *factQueryWebGuardBeforeHook {
	if !webResearchReady {
		return nil
	}
	return &factQueryWebGuardBeforeHook{}
}

func (h *factQueryWebGuardBeforeHook) Point() callbacks.CallbackPoint {
	return callbacks.PointBeforeTool
}

func (h *factQueryWebGuardBeforeHook) Priority() int { return 3 }

func (h *factQueryWebGuardBeforeHook) HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil {
		return &trpctool.BeforeToolResult{}, nil
	}
	switch args.ToolName {
	case "web_fetch", "duckduckgo_search", "gemini_web_fetch", "google_search":
	default:
		return &trpctool.BeforeToolResult{}, nil
	}
	if !biz.LooksLikeFactQuery(userTextFromInvocation(ctx)) {
		return &trpctool.BeforeToolResult{}, nil
	}
	return &trpctool.BeforeToolResult{
		CustomResult: "事实查询请直接调用本轮已注册的 web_research（时间用 datetime），不要 tool_load。不要用 web_fetch / duckduckgo_search 抓搜索页。",
	}, nil
}

func userTextFromInvocation(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	return inv.Message.Content
}

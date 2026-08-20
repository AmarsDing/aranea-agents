package session

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
)

// ModelConfigCatalog resolves provider/model config JSON for context window lookup.
type ModelConfigCatalog interface {
	GetModelConfigJSON(ctx context.Context, provider, model string) string
}

// ResolveContextWindowTokens picks the context window for the active model call.
func ResolveContextWindowTokens(ctx context.Context, catalog ModelConfigCatalog, sess biz.Session, ag biz.Agent, prov, mod string) int {
	cfgJSON := ""
	if catalog != nil {
		cfgJSON = catalog.GetModelConfigJSON(ctx, prov, mod)
	}
	return llmcontext.ResolveWindow(llmcontext.ResolveInput{
		ProviderModelConfigJSON: cfgJSON,
		SessionDefaultWindow:    sess.DefaultContextWindowTokens,
		AgentWindow:             ag.ContextWindow,
	})
}

// PatchContextFromLLMUsage updates session context metrics and optionally triggers L0 compression.
func PatchContextFromLLMUsage(
	ctx context.Context,
	sessions biz.SessionTurnExtrasPort,
	compress biz.NativeTurnCompressor,
	catalog ModelConfigCatalog,
	sessionID string,
	sess biz.Session,
	ag biz.Agent,
	prov, mod string,
	promptTok, completionTok int,
	lg loggateway.Logger,
) {
	if sessions == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	// 上下文用量是压缩触发与上下文面板的数据源：客户端断连时仍须落库（P1，
	// 2026-08-20）。Compress.AfterNativeTurn 内部已自行 Background 解耦，接收
	// detached ctx 仅取 values（TRPC user key），不受 10s 兜底影响。
	ctx, cancel := appctx.Detach(ctx)
	defer cancel()
	win := ResolveContextWindowTokens(ctx, catalog, sess, ag, prov, mod)
	if err := sessions.UpdateSessionContextFromLLMUsage(ctx, sessionID, promptTok, completionTok, win); err != nil {
		lg.Warn("更新会话上下文用量失败",
			loggateway.StepID("session.context_usage"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err),
			loggateway.Int("prompt_tokens", promptTok),
			loggateway.Int("context_window", win))
	}
	if compress != nil {
		compress.AfterNativeTurn(ctx, sessionID, ag)
	}
}

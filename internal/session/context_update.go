package session

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
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
	sessions *biz.SessionUsecase,
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

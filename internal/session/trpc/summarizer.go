package session

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcsummary "trpc.group/trpc-go/trpc-agent-go/session/summary"
)

// SummarizerConfig holds the dependencies for constructing the framework
// session summarizer. The summarizer is injected into the SQLite Session
// Service to enable async LLM-based session summarization.
type SummarizerConfig struct {
	Catalog *biz.LlmProviderModelUsecase
	RT      *provider.RoundTrip
	Lg      loggateway.Logger
}

// NewDynamicSummarizer creates a summary.SessionSummarizer that resolves
// the LLM model at summary time using the project's model catalog.
//
// It uses the DynamicSummarizer pattern because different sessions may
// need different models. The resolver picks the first available model
// from the catalog (same strategy as session title generation).
//
// If no model is available, the resolver returns nil, which makes the
// framework skip automatic summary checks — the existing Compressor
// pipeline remains the primary compression mechanism.
func NewDynamicSummarizer(cfg SummarizerConfig) trpcsummary.SessionSummarizer {
	if cfg.Catalog == nil {
		return nil
	}
	return trpcsummary.NewDynamicSummarizer(func(ctx context.Context, sess *trpcsession.Session) (trpcsummary.SessionSummarizer, error) {
		m, err := resolveSummaryModel(ctx, cfg)
		if err != nil {
			cfg.Lg.Warn("session summary: resolve model failed", loggateway.StepID("session.summary_resolve"), loggateway.Err(err))
			return nil, nil // nil = skip automatic summary, don't block
		}
		return trpcsummary.NewSummarizer(m,
			trpcsummary.WithName("aranea-session-summary"),
			trpcsummary.WithContextThreshold(),
			// 关闭 cache-safe fork（评审 C3-1，2026-08-19）：本函数刻意用
			// PickTitleModel 选轻量模型做摘要，与会话聊天模型大概率不同，
			// KV cache 前缀复用的前提（同模型+同前缀）不成立，fork 无收益，
			// 反而继承父请求的 Tools（白付 token）与 GenerationConfig/
			// ExtraFields（跨供应商可能 400；框架 shouldRetrySummary 不重试
			// 400 → 摘要静默失败）。standalone 路径请求干净（无 Tools、无继承
			// 配置），行为可预期。若未来摘要模型与聊天模型同源再评估开启。
			trpcsummary.WithCacheSafeForking(false),
		), nil
	})
}

// resolveSummaryModel picks a model from the catalog for summarization.
// It uses the same strategy as session title generation: PickTitleModel
// from biz, falling back to the first enabled model.
func resolveSummaryModel(ctx context.Context, cfg SummarizerConfig) (trpcmodel.Model, error) {
	models, err := cfg.Catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, nil
	}
	pm, ok := biz.PickTitleModel(models)
	if !ok {
		return nil, nil
	}
	return provider.TRPCModelForProviderModel(ctx, cfg.Catalog, cfg.RT, pm.Provider, pm.Model, cfg.Lg)
}

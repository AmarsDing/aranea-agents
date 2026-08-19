package knowledge

import (
	"context"
	"time"

	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/loggateway"
)

// EmbedUsageInput carries one embedding network call's consumption to the
// usage recorder (P1-3, 2026-08-19).
type EmbedUsageInput struct {
	Provider  string
	Model     string
	Tokens    int
	BatchSize int
	Latency   time.Duration
	// TaskType is the Gemini-style task hint ("" / "RETRIEVAL_DOCUMENT" /
	// "RETRIEVAL_QUERY"); persisted into metadata for ingest/query split.
	TaskType string
	// UsageSource is "response" (provider-reported usage) or "estimated"
	// (chars-based estimate for providers without usage reporting).
	UsageSource string
	Err         error
	// Prewarm marks probe pings (startup / voice-session warm-up) as opposed
	// to real ingest/retrieval traffic; persisted into metadata so analytics
	// can exclude probe rows (P1-3 review fix, 2026-08-19).
	Prewarm bool
}

// EmbedUsageRecorder records embedding consumption (e.g. into
// model_token_usage_events). nil = recording disabled.
type EmbedUsageRecorder interface {
	RecordEmbedUsage(ctx context.Context, in EmbedUsageInput)
}

// SetUsageRecorder 注入 embedding 用量记录器（装配层调用，同 SetMonitorBus
// 模式；nil = 不记录）。记录点收敛在 embedBatchWithTaskType —— 所有网络调用
// （文档批嵌入 Embed、缓存未命中的查询嵌入 EmbedSingle/EmbedWithTaskType、
// Prewarm ping）的唯一汇聚点，缓存命中不产生网络调用自然不计。
func (e *MultiProviderEmbedder) SetUsageRecorder(r EmbedUsageRecorder) {
	if e == nil {
		return
	}
	e.usageRec = r
}

// recordUsage fires the usage recorder best-effort; recorder errors are Warn-logged
// and never fail the embedding call.
func (e *MultiProviderEmbedder) recordUsage(ctx context.Context, texts []string, taskType string, tokens int, latency time.Duration, callErr error, prewarm bool) {
	rec := e.usageRec
	if rec == nil {
		return
	}
	provider, _, _, model, _ := e.snapshot()
	source := "response"
	if tokens <= 0 {
		// Provider returned no usage (ollama / huggingface / openai-compat
		// gateways that omit the field): estimate from text so consumption
		// stays visible; usage_source=estimated keeps rows identifiable.
		if callErr == nil {
			chars := 0
			for _, t := range texts {
				chars += len([]rune(t))
			}
			tokens = llmcontext.EstimateTokensFromChars(chars)
		}
		source = "estimated"
	}
	if tokens <= 0 && callErr == nil {
		return
	}
	in := EmbedUsageInput{
		Provider: provider, Model: model, Tokens: tokens, BatchSize: len(texts),
		Latency: latency, TaskType: taskType, UsageSource: source, Err: callErr,
		Prewarm: prewarm,
	}
	defer func() {
		if r := recover(); r != nil {
			e.lg.Warn("embed usage recorder panicked", loggateway.StepID("knowledge.embed_usage"), loggateway.Any("panic", r))
		}
	}()
	rec.RecordEmbedUsage(ctx, in)
}

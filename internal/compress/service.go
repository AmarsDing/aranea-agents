package compress

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/loggateway"
)

// Request is the minimal input for one summarization call (no session DB).
type Request struct {
	Transcript string
	// PriorSummary optional chunk from earlier compression rounds.
	PriorSummary string
	Provider     string
	Model        string
	// SystemPrompt overrides DefaultSystemPrompt when non-empty.
	SystemPrompt string
}

// Result is model output plus resolved routing metadata.
type Result struct {
	Markdown         string
	PromptTokens     int
	CompletionTokens int
	Provider         string
	Model            string
	PromptVersion    string
}

// Compressor summarizes transcripts via the configured LLM catalog row.
type Compressor interface {
	Compress(ctx context.Context, req Request) (Result, error)
}

// LLMService implements Compressor using OpenAI-compatible chat completions.
type LLMService struct {
	ModelCatalog *biz.LlmProviderModelUsecase
	HTTPClient   *http.Client
	lg           loggateway.Logger
}

// NewLLMService builds a compressor; httpClient must be non-nil for outbound calls.
func NewLLMService(catalog *biz.LlmProviderModelUsecase, httpClient *http.Client, lg loggateway.Logger) *LLMService {
	return &LLMService{ModelCatalog: catalog, HTTPClient: httpClient, lg: lg}
}

var _ Compressor = (*LLMService)(nil)

// Compress implements [Compressor].
func (s *LLMService) Compress(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.ModelCatalog == nil {
		return Result{}, ErrCatalogRequired
	}
	s.lg.Info("L1 压缩开始", loggateway.StepID("compress.start"), loggateway.Str("provider", req.Provider), loggateway.Str("model", req.Model))
	out := Result{Provider: strings.TrimSpace(req.Provider), Model: strings.TrimSpace(req.Model), PromptVersion: PromptVersion}
	if s.HTTPClient == nil {
		return out, ErrHTTPClientRequired
	}
	if out.Provider == "" || out.Model == "" {
		return out, ErrProviderModelRequired
	}
	transcript := strings.TrimSpace(req.Transcript)
	if transcript == "" {
		return out, ErrEmptyTranscript
	}

	row, err := s.ModelCatalog.GetByProviderAndModel(ctx, out.Provider, out.Model)
	if err != nil {
		s.lg.Error("L2 压缩模型查询失败", loggateway.StepID("compress.catalog_fail"), loggateway.Str("provider", out.Provider), loggateway.Str("model", out.Model), loggateway.Err(err))
		return out, err
	}
	var cfg chatagent.ProviderAPIConfig
	chatagent.MergeProviderConfigJSON(row.ConfigJSON, &cfg)
	chatagent.ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)

	sys := strings.TrimSpace(req.SystemPrompt)
	if sys == "" {
		sys = DefaultSystemPrompt
	}
	msgs := []chatagent.OpenAICompatMessage{{Role: "system", Content: sys}}
	if ps := strings.TrimSpace(req.PriorSummary); ps != "" {
		msgs = append(msgs, chatagent.OpenAICompatMessage{
			Role:    "user",
			Content: "Previously summarized context:\n\n" + ps,
		})
	}
	msgs = append(msgs, chatagent.OpenAICompatMessage{
		Role:    "user",
		Content: "Conversation excerpt to absorb:\n\n" + transcript,
	})

	callCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
	}

	text, _, ptok, ctok, err := chatagent.CallOpenAICompatChat(callCtx, s.HTTPClient, cfg, out.Model, msgs)
	if err != nil {
		s.lg.Error("L4 压缩 LLM 调用失败", loggateway.StepID("compress.llm_call_fail"), loggateway.Str("provider", out.Provider), loggateway.Str("model", out.Model), loggateway.Err(err))
		return out, err
	}
	out.Markdown = strings.TrimSpace(text)
	out.PromptTokens = ptok
	out.CompletionTokens = ctok
	if ptok > 0 {
		// 双锚点校准：用权威 prompt_tokens 回填共享估算器（rune 数与调用点单位一致）。
		llmcontext.RecordAuthoritativeUsage(ptok, inputChars(sys, req.PriorSummary, transcript))
	}
	return out, nil
}

// inputChars sums rune counts of prompt input parts (authoritative anchor chars).
func inputChars(parts ...string) int {
	n := 0
	for _, p := range parts {
		n += utf8.RuneCountInString(p)
	}
	return n
}

package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	// compressorLLMTimeout caps the LLM call duration for summarisation.
	// Matches the budget used by link_evolution.go for consistency.
	compressorLLMTimeout = 30 * time.Second
)

// LLMContextCompressor is the default biz.ContextCompressor implementation.
// It uses an LLM to produce a recursive summary of evicted messages, merging
// any existing summary so successive compressions stay information-dense.
//
// When llm is nil, ShouldCompress still works (pure threshold check) but
// Compress returns an error — callers wiring the compressor for automatic
// hook-driven compression should guard accordingly.
type LLMContextCompressor struct {
	llm trpcmodel.Model
	lg  loggateway.Logger
}

// NewLLMContextCompressor creates an LLMContextCompressor.
//
// Parameters:
//   - llm: the LLM used for summarisation. May be nil (Compress will error).
//   - lg:  the logger. Falls back to a no-op logger if nil.
func NewLLMContextCompressor(llm trpcmodel.Model, lg loggateway.Logger) *LLMContextCompressor {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &LLMContextCompressor{
		llm: llm,
		lg:  lg.With(loggateway.Domain("context_compressor")),
	}
}

// ShouldCompress reports whether the context window usage ratio has crossed
// the compression threshold (llmcontext.ContextStatusCriticalThreshold = 0.80).
func (c *LLMContextCompressor) ShouldCompress(usedRatio float64) bool {
	return usedRatio >= llmcontext.ContextStatusCriticalThreshold
}

// Compress produces a recursive summary by feeding the existing summary
// (if any) together with the evicted messages to the LLM.
//
// Contract (see biz.ContextCompressor):
//   - empty evictedMessages → empty result, no LLM call
//   - nil LLM → error
//   - LLM failure → error (propagated for caller graceful-degradation)
//   - empty LLM response → empty Summary, metrics still populated
func (c *LLMContextCompressor) Compress(ctx context.Context, existingSummary string, evictedMessages []biz.ConsolidateMessage) (biz.ContextCompressionResult, error) {
	if len(evictedMessages) == 0 {
		return biz.ContextCompressionResult{}, nil
	}
	if c.llm == nil {
		return biz.ContextCompressionResult{}, fmt.Errorf("context compressor: LLM not wired")
	}

	beforeChars := len(existingSummary)
	for _, m := range evictedMessages {
		beforeChars += len(m.Content)
	}

	prompt := buildCompressionPrompt(existingSummary, evictedMessages)
	callCtx, cancel := context.WithTimeout(ctx, compressorLLMTimeout)
	defer cancel()

	req := trpcmodel.NewRequest([]trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: compressionSystemPrompt},
		{Role: trpcmodel.RoleUser, Content: prompt},
	})

	respCh, err := c.llm.GenerateContent(callCtx, req)
	if err != nil {
		return biz.ContextCompressionResult{}, fmt.Errorf("context compressor: LLM generate content: %w", err)
	}

	content, err := consumeLLMResponse(respCh)
	if err != nil {
		return biz.ContextCompressionResult{}, fmt.Errorf("context compressor: %w", err)
	}

	summary := strings.TrimSpace(content)
	return biz.ContextCompressionResult{
		Summary:      summary,
		EvictedCount: len(evictedMessages),
		BeforeChars:  beforeChars,
		AfterChars:   len(summary),
	}, nil
}

// buildCompressionPrompt constructs the user prompt for recursive summarisation.
// When an existing summary is present, it is included so the LLM can merge it
// with the new messages (recursive summarisation per Letta's pattern).
func buildCompressionPrompt(existingSummary string, messages []biz.ConsolidateMessage) string {
	var sb strings.Builder
	if existingSummary != "" {
		sb.WriteString("Existing summary from previous compression:\n")
		sb.WriteString(existingSummary)
		sb.WriteString("\n\nNew messages to incorporate:\n")
	} else {
		sb.WriteString("Messages to summarise:\n")
	}
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s] %s\n", m.Role, m.Content))
	}
	return sb.String()
}

// compressionSystemPrompt instructs the LLM on how to produce a dense,
// information-preserving recursive summary. The "do not add information"
// constraint is critical to prevent hallucination during recursive passes.
const compressionSystemPrompt = `You are a context summarisation agent. Produce a concise summary that preserves:
1. Key user decisions and preferences
2. Important facts and context
3. Unfinished tasks or open questions
4. The chronological flow of the conversation

If an existing summary is provided, merge it with the new messages into a single coherent summary.
Keep the summary dense — prefer specifics over generalities. Do not add information not present in the input.`

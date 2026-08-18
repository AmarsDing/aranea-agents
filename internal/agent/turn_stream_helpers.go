package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// ErrFirstByteTimeout indicates the model did not produce a meaningful event in time.
var ErrFirstByteTimeout = errors.New("first byte timeout")

// DisplayMarkdownFromStream returns assistant-visible text from a consumed event stream.
// Reasoning-only replies are included when main content is empty (aligned with Team turns).
// The second return value indicates whether the display text is a reasoning fallback
// (i.e. the LLM produced only reasoning with no separate reply content).
func DisplayMarkdownFromStream(result EventStreamResult) (string, bool) {
	reply := strings.TrimSpace(result.Reply.String())
	if reply != "" {
		return reply, false
	}
	reasoning := strings.TrimSpace(result.Reasoning.String())
	return reasoning, reasoning != ""
}

// EstimateTokensIfMissing fills missing token counts from text using a symmetric
// "fill missing" semantic: each side is estimated independently from its own text.
// Caller must pass non-empty inputPreview for prompt estimation to work (chat path
// historically passed "" which suppressed prompt estimation — fixed at call sites).
func EstimateTokensIfMissing(promptTok, completionTok int, inputPreview, displayMarkdown string) (int, int) {
	if promptTok <= 0 {
		promptTok = RoughTokenEstimate(inputPreview)
	}
	if completionTok <= 0 {
		completionTok = RoughTokenEstimate(displayMarkdown)
	}
	return promptTok, completionTok
}

// ConsumeWithFirstByteGuard runs the turn stream consumer with a first-byte deadline.
func ConsumeWithFirstByteGuard(
	parentCtx context.Context,
	firstByteTimeout time.Duration,
	events <-chan *trpcevent.Event,
	meta ProjectMeta,
	opts *StreamConsumeOptions,
	lg loggateway.Logger,
) (EventStreamResult, error) {
	if firstByteTimeout <= 0 {
		firstByteTimeout = DefaultFirstByteTimeout
	}
	firstByteCtx, cancel := context.WithTimeout(parentCtx, firstByteTimeout)
	defer cancel()
	received := false
	result := ConsumeEventStreamWithFirstByte(firstByteCtx, parentCtx, events, meta, &received, opts, lg)
	if result.FirstByteTimedOut || (!received && parentCtx.Err() == nil) {
		return result, fmt.Errorf("%w after %s", ErrFirstByteTimeout, firstByteTimeout)
	}
	return result, nil
}

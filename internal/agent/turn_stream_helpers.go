package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/event"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// ErrFirstByteTimeout indicates the model did not produce a meaningful event in time.
var ErrFirstByteTimeout = errors.New("first byte timeout")

// DisplayMarkdownFromStream returns assistant-visible text from a consumed event stream.
// Reasoning-only replies are included when main content is empty (aligned with Team turns).
func DisplayMarkdownFromStream(result EventStreamResult) string {
	reply := strings.TrimSpace(result.Reply.String())
	if reply != "" {
		return reply
	}
	return strings.TrimSpace(result.Reasoning.String())
}

// EstimateTokensIfMissing fills token counts from text when the model omitted usage.
func EstimateTokensIfMissing(promptTok, completionTok int, inputPreview, displayMarkdown string) (int, int) {
	if promptTok > 0 && completionTok > 0 {
		return promptTok, completionTok
	}
	if promptTok <= 0 && completionTok > 0 && strings.TrimSpace(inputPreview) != "" {
		promptTok = RoughTokenEstimate(inputPreview)
	}
	if promptTok > 0 || completionTok > 0 || displayMarkdown == "" {
		return promptTok, completionTok
	}
	return RoughTokenEstimate(inputPreview+displayMarkdown), RoughTokenEstimate(displayMarkdown)
}

// ConsumeWithFirstByteGuard runs the turn stream consumer with a first-byte deadline.
func ConsumeWithFirstByteGuard(
	parentCtx context.Context,
	firstByteTimeout time.Duration,
	events <-chan *trpcevent.Event,
	bus event.Bus,
	meta ProjectMeta,
	opts *StreamConsumeOptions,
) (EventStreamResult, error) {
	if firstByteTimeout <= 0 {
		firstByteTimeout = DefaultFirstByteTimeout
	}
	firstByteCtx, cancel := context.WithTimeout(parentCtx, firstByteTimeout)
	defer cancel()
	received := false
	result := ConsumeEventStreamWithFirstByte(firstByteCtx, parentCtx, events, bus, meta, &received, opts)
	if !received && parentCtx.Err() == nil {
		return result, fmt.Errorf("%w after %s", ErrFirstByteTimeout, firstByteTimeout)
	}
	return result, nil
}

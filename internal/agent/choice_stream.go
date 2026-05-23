package agent

import (
	"strings"
	"time"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// DefaultFirstByteTimeout is the maximum wait for the first model event before failing the turn.
const DefaultFirstByteTimeout = 30 * time.Second

// DefaultTurnTimeout is the maximum wall-clock duration for a single chat turn.
const DefaultTurnTimeout = 5 * time.Minute

// ChoiceStreamContent extracts text and reasoning from a streaming choice.
// Partial chunks preserve delta spacing; non-partial values are trimmed.
func ChoiceStreamContent(choice trpcmodel.Choice, partial bool) (text, reasoning string) {
	msg := choice.Message
	delta := choice.Delta
	if partial {
		text = delta.Content
		reasoning = delta.ReasoningContent
		if text == "" {
			text = strings.TrimSpace(msg.Content)
		}
		if reasoning == "" {
			reasoning = strings.TrimSpace(msg.ReasoningContent)
		}
		return text, reasoning
	}
	return firstNonEmptyTrimmed(msg.Content, delta.Content),
		firstNonEmptyTrimmed(msg.ReasoningContent, delta.ReasoningContent)
}

// ChoiceHasStreamContent reports whether a choice carries text or reasoning payload.
func ChoiceHasStreamContent(choice trpcmodel.Choice, partial bool) bool {
	text, reasoning := ChoiceStreamContent(choice, partial)
	return text != "" || reasoning != ""
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

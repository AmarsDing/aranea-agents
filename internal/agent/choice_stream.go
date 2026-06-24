package agent

import (
	"strings"
	"time"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// DefaultFirstByteTimeout is the maximum wait for the first model event before failing the turn.
const DefaultFirstByteTimeout = 30 * time.Second

// DefaultTurnTimeout is the maximum wall-clock duration for a single chat turn.
const DefaultTurnTimeout = 10 * time.Minute

// ChoiceStreamContent extracts text and reasoning from a streaming choice.
// Partial chunks preserve delta spacing; non-partial values are trimmed.
// Falls back to ContentParts[].Text for providers that put stream text there.
func ChoiceStreamContent(choice trpcmodel.Choice, partial bool) (text, reasoning string) {
	msg := choice.Message
	delta := choice.Delta
	if partial {
		text = delta.Content
		reasoning = delta.ReasoningContent
		if text == "" {
			text = strings.TrimSpace(msg.Content)
		}
		if text == "" {
			text = textFromContentParts(delta.ContentParts)
		}
		if reasoning == "" {
			reasoning = strings.TrimSpace(msg.ReasoningContent)
		}
		return text, reasoning
	}
	text = firstNonEmptyTrimmed(msg.Content, delta.Content)
	if text == "" {
		text = firstNonEmptyTrimmed(textFromContentParts(msg.ContentParts), textFromContentParts(delta.ContentParts))
	}
	reasoning = firstNonEmptyTrimmed(msg.ReasoningContent, delta.ReasoningContent)
	return text, reasoning
}

// textFromContentParts concatenates text parts from multimodal content parts.
func textFromContentParts(parts []trpcmodel.ContentPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" && p.Text != nil {
			b.WriteString(*p.Text)
		}
	}
	return b.String()
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

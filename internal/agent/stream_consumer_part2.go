package agent

import (
	"aranea-agents/internal/provider"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func accumulateChoiceStream(result *EventStreamResult, choice trpcmodel.Choice, partial bool) {
	if result == nil {
		return
	}
	text, reasoning := ChoiceStreamContent(choice, partial)
	if text != "" {
		if partial {
			_ = provider.VisibleStreamingDelta(&result.Reply, text)
		} else {

			result.Reply.Reset()
			result.Reply.WriteString(text)
		}
		result.HasContent = true
	}
	if reasoning != "" {
		if partial {
			_ = provider.VisibleStreamingDelta(&result.Reasoning, reasoning)
		} else {
			result.Reasoning.Reset()
			result.Reasoning.WriteString(reasoning)
		}
		result.HasContent = true
	}
}

func isChatCompletionStreamObject(objType string) bool {
	switch objType {
	case "", trpcmodel.ObjectTypeChatCompletionChunk, trpcmodel.ObjectTypeChatCompletion:
		return true
	default:
		return false
	}
}

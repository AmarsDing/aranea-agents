package agent

import (
	"context"
	"net/http"

	"aranea-agents/internal/agent/llmcompat"
)

type OpenAICompatMessage = llmcompat.OpenAICompatMessage

type OpenAICompatToolCallResult = llmcompat.OpenAICompatToolCallResult

func CallOpenAICompatChat(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage) (text string, reasoning string, promptTok, completionTok int, err error) {
	return llmcompat.CallOpenAICompatChat(ctx, hc, cfg, modelName, messages)
}

func CallOpenAICompatChatStream(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage, onDelta func(piece string) error) (fullText string, reasoningText string, promptTok, completionTok int, err error) {
	return llmcompat.CallOpenAICompatChatStream(ctx, hc, cfg, modelName, messages, onDelta)
}

func CallOpenAICompatChatWithTools(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage, tools []map[string]any) (text string, toolCalls []OpenAICompatToolCallResult, promptTok, completionTok int, err error) {
	return llmcompat.CallOpenAICompatChatWithTools(ctx, hc, cfg, modelName, messages, tools)
}

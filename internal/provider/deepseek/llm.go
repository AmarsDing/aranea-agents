package deepseek

import (
	"net/http"

	"aranea-agents/internal/provider/openai"

	"google.golang.org/adk/model"
)

// NewLLM returns model.LLM for DeepSeek (OpenAI-compatible API; diverge in this package later if needed).
func NewLLM(hc *http.Client, baseURL, apiKey string) model.LLM {
	return openai.NewLLM(hc, baseURL, apiKey, openai.WithName("deepseek"))
}

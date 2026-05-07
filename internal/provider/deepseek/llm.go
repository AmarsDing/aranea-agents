package deepseek

import (
	"net/http"
	"strings"

	"aranea-agents/internal/provider/openai"

	"google.golang.org/adk/model"
)

// NewLLM returns model.LLM for DeepSeek (OpenAI-compatible API).
// modelAPIID is the upstream model name (e.g. deepseek-v4-pro); if empty, defaults to "deepseek".
// Prefer provider.DefaultRegistry() with provider_type "deepseek" so ModelAPI comes from the catalog row.
func NewLLM(hc *http.Client, baseURL, apiKey, modelAPIID string) model.LLM {
	name := strings.TrimSpace(modelAPIID)
	if name == "" {
		name = "deepseek"
	}
	return openai.NewLLM(hc, baseURL, apiKey, openai.WithName(name))
}

// Package gemini wires catalog-backed credentials to google.golang.org/adk/model/gemini (official Gen AI client).
package gemini

import (
	"context"
	"net/http"
	"strings"

	adkgemini "google.golang.org/adk/model/gemini"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// NewLLM returns model.LLM backed by the Gemini API (google.golang.org/genai).
// apiKey may be empty if the process sets GOOGLE_API_KEY or GEMINI_API_KEY.
// baseURL is optional (custom or proxy endpoint).
// modelName is the default model id when LLMRequest.Model is empty; if empty, uses gemini-2.0-flash.
func NewLLM(ctx context.Context, hc *http.Client, apiKey, baseURL, modelName string) (model.LLM, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = "gemini-2.0-flash"
	}
	cfg := &genai.ClientConfig{
		APIKey:     strings.TrimSpace(apiKey),
		HTTPClient: hc,
	}
	if u := strings.TrimRight(strings.TrimSpace(baseURL), "/"); u != "" {
		cfg.HTTPOptions.BaseURL = u
	}
	return adkgemini.NewModel(ctx, modelName, cfg)
}

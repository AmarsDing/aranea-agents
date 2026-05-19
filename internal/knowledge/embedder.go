// Package knowledge provides embedding generation for document chunks.
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Embedder generates embeddings for text using a remote API.
type Embedder struct {
	mu       sync.RWMutex
	Provider string // "openai" | "ollama" | "custom"
	BaseURL  string
	APIKey   string
	Model    string
	Dim      int
}

// NewEmbedder creates an Embedder with the given configuration.
func NewEmbedder(provider, baseURL, apiKey, model string, dim int) *Embedder {
	if dim <= 0 {
		dim = 1536
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &Embedder{
		Provider: provider,
		BaseURL:  strings.TrimRight(baseURL, "/"),
		APIKey:   apiKey,
		Model:    model,
		Dim:      dim,
	}
}

// Config returns a redacted view of embedder settings (EP-KN-01).
func (e *Embedder) Config() (provider, baseURL, model string, dim int, configured bool, hasAPIKey bool) {
	if e == nil {
		return "", "", "", 0, false, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	hasAPIKey = e.APIKey != ""
	configured = e.Provider != "" && (hasAPIKey || e.Provider == "ollama")
	return e.Provider, e.BaseURL, e.Model, e.Dim, configured, hasAPIKey
}

// Update applies runtime embedder settings from admin UI (EP-KN-01).
func (e *Embedder) Update(provider, baseURL, apiKey, model string, dim int) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if p := strings.TrimSpace(provider); p != "" {
		e.Provider = p
	}
	if b := strings.TrimRight(strings.TrimSpace(baseURL), "/"); b != "" {
		e.BaseURL = b
	}
	if k := strings.TrimSpace(apiKey); k != "" {
		e.APIKey = k
	}
	if m := strings.TrimSpace(model); m != "" {
		e.Model = m
	}
	if dim > 0 {
		e.Dim = dim
	}
}

func (e *Embedder) snapshot() (provider, baseURL, apiKey, model string, dim int) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Provider, e.BaseURL, e.APIKey, e.Model, e.Dim
}

// Embed returns a single embedding vector for the input text.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("embedder: text is empty")
	}
	provider, baseURL, apiKey, model, dim := e.snapshot()
	_ = dim
	switch provider {
	case "ollama":
		return e.embedOllamaWith(ctx, baseURL, model, text)
	default:
		return e.embedOpenAIWith(ctx, baseURL, apiKey, model, text)
	}
}

// EmbedBatch returns embeddings for a slice of texts.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		vec, err := e.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, vec)
	}
	return out, nil
}

// embedOpenAI calls the OpenAI-compatible /v1/embeddings endpoint.
func (e *Embedder) embedOpenAI(ctx context.Context, text string) ([]float32, error) {
	_, baseURL, apiKey, model, _ := e.snapshot()
	return e.embedOpenAIWith(ctx, baseURL, apiKey, model, text)
}

func (e *Embedder) embedOpenAIWith(ctx context.Context, baseURL, apiKey, model, text string) ([]float32, error) {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	type req struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	type embObj struct {
		Embedding []float32 `json:"embedding"`
	}
	type resp struct {
		Data []embObj `json:"data"`
	}
	body, err := jsonPOST(ctx, http.DefaultClient, baseURL+"/v1/embeddings", apiKey, req{
		Model: model,
		Input: []string{text},
	})
	if err != nil {
		return nil, err
	}
	var r resp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("embedder openai: %w", err)
	}
	if len(r.Data) == 0 {
		return nil, fmt.Errorf("embedder openai: empty response")
	}
	return r.Data[0].Embedding, nil
}

// embedOllama calls the Ollama /api/embeddings endpoint.
func (e *Embedder) embedOllama(ctx context.Context, text string) ([]float32, error) {
	_, baseURL, _, model, _ := e.snapshot()
	return e.embedOllamaWith(ctx, baseURL, model, text)
}

func (e *Embedder) embedOllamaWith(ctx context.Context, baseURL, model, text string) ([]float32, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	type req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	type resp struct {
		Embedding []float32 `json:"embedding"`
	}
	body, err := jsonPOST(ctx, http.DefaultClient, baseURL+"/api/embeddings", "", req{
		Model:  model,
		Prompt: text,
	})
	if err != nil {
		return nil, err
	}
	var r resp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("embedder ollama: %w", err)
	}
	return r.Embedding, nil
}

// jsonPOST sends an authenticated JSON POST request and returns the body.
func jsonPOST(ctx context.Context, client *http.Client, url, apiKey string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedder http: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("embedder http %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

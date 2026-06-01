package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/genai"
)

func init() {
	timeoutSec := 60
	if v := os.Getenv("KRATOS_KNOWLEDGE_EMBED_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}
	embedHTTPClient = &http.Client{
		Timeout: time.Duration(timeoutSec) * time.Second,
	}
}

var embedHTTPClient *http.Client

const (
	ProviderOpenAI      = "openai"
	ProviderOllama      = "ollama"
	ProviderGemini      = "gemini"
	ProviderHuggingFace = "huggingface"
)

// BatchEmbedder embeds multiple texts efficiently when supported by the backend.
type BatchEmbedder interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// Embedder generates embeddings for text using a remote API.
type Embedder struct {
	mu       sync.RWMutex
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	Dim      int
	lg       loggateway.Logger
}

// NewEmbedder creates an Embedder with the given configuration.
func NewEmbedder(provider, baseURL, apiKey, model string, dim int, lg loggateway.Logger) *Embedder {
	if dim <= 0 {
		dim = 1536
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = ProviderOpenAI
	}
	if model == "" {
		switch provider {
		case ProviderGemini:
			model = "gemini-embedding-001"
		case ProviderOllama:
			model = "nomic-embed-text"
		default:
			model = "text-embedding-3-small"
		}
	}
	if provider == ProviderHuggingFace && baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &Embedder{
		Provider: provider,
		BaseURL:  strings.TrimRight(baseURL, "/"),
		APIKey:   apiKey,
		Model:    model,
		Dim:      dim,
		lg:       lg,
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
	switch e.Provider {
	case ProviderOllama:
		configured = e.Provider != ""
	case ProviderHuggingFace:
		configured = e.BaseURL != ""
	default:
		configured = e.Provider != "" && hasAPIKey
	}
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
		return nil, kerrors.BadRequest("KNOWLEDGE", "embedder: text is empty")
	}
	vecs, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil, kerrors.InternalServer("KNOWLEDGE", "embedder: empty embedding")
	}
	return vecs[0], nil
}

// EmbedWithTaskType returns a single embedding with a task type hint (e.g. "RETRIEVAL_QUERY").
func (e *Embedder) EmbedWithTaskType(ctx context.Context, text string, taskType string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, kerrors.BadRequest("KNOWLEDGE", "embedder: text is empty")
	}
	vecs, err := e.EmbedBatchWithTaskType(ctx, []string{text}, taskType)
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil, kerrors.InternalServer("KNOWLEDGE", "embedder: empty embedding")
	}
	return vecs[0], nil
}

// EmbedBatch returns embeddings for a slice of texts using provider batch APIs when available.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return e.EmbedBatchWithTaskType(ctx, texts, "")
}

// EmbedBatchWithTaskType returns embeddings with an optional task type hint (e.g. "RETRIEVAL_QUERY").
// Currently only Gemini uses task type; other providers ignore it.
func (e *Embedder) EmbedBatchWithTaskType(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	provider, baseURL, apiKey, model, dim := e.snapshot()
	switch provider {
	case ProviderOllama:
		return e.embedOllamaBatch(ctx, baseURL, model, texts)
	case ProviderGemini:
		return e.embedGeminiBatch(ctx, apiKey, model, dim, texts, taskType)
	case ProviderHuggingFace:
		return e.embedHuggingFaceBatch(ctx, baseURL, dim, texts)
	default:
		return e.embedOpenAIBatch(ctx, baseURL, apiKey, model, texts)
	}
}

func (e *Embedder) embedOpenAIBatch(ctx context.Context, baseURL, apiKey, model string, texts []string) ([][]float32, error) {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += defaultEmbedBatchSize {
		end := start + defaultEmbedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]
		type req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		type embObj struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		type resp struct {
			Data []embObj `json:"data"`
		}
		body, err := jsonPOST(ctx, embedHTTPClient, baseURL+"/v1/embeddings", apiKey, req{
			Model: model,
			Input: batch,
		})
		if err != nil {
			return nil, err
		}
		var r resp
		if err := json.Unmarshal(body, &r); err != nil {
			e.lg.Error("knowledge embedder openai parse failed", loggateway.StepID("knowledge.embed_fail"), loggateway.Err(err))
			return nil, kerrors.InternalServer("KNOWLEDGE", "embedder openai parse failed: "+err.Error())
		}
		if len(r.Data) != len(batch) {
			e.lg.Error("knowledge embedder openai count mismatch", loggateway.StepID("knowledge.embed_fail"), loggateway.Int("expected", len(batch)), loggateway.Int("got", len(r.Data)))
			return nil, kerrors.InternalServer("KNOWLEDGE", fmt.Sprintf("embedder openai: expected %d embeddings, got %d", len(batch), len(r.Data)))
		}
		ordered := make([][]float32, len(batch))
		for _, item := range r.Data {
			if item.Index < 0 || item.Index >= len(batch) {
				return nil, kerrors.InternalServer("KNOWLEDGE", fmt.Sprintf("embedder openai: invalid index %d", item.Index))
			}
			ordered[item.Index] = item.Embedding
		}
		for i, vec := range ordered {
			if len(vec) == 0 {
				return nil, kerrors.InternalServer("KNOWLEDGE", fmt.Sprintf("embedder openai: empty embedding at %d", start+i))
			}
			out = append(out, vec)
		}
	}
	return out, nil
}

func (e *Embedder) embedOllamaBatch(ctx context.Context, baseURL, model string, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec, err := e.embedOllamaWith(ctx, baseURL, model, text)
		if err != nil {
			return nil, err
		}
		out = append(out, vec)
	}
	return out, nil
}

func (e *Embedder) embedGeminiBatch(ctx context.Context, apiKey, model string, dim int, texts []string, taskType string) ([][]float32, error) {
	if apiKey == "" {
		e.lg.Error("knowledge embedder gemini API key required", loggateway.StepID("knowledge.embed_fail"))
		return nil, kerrors.BadRequest("KNOWLEDGE", "embedder gemini: API key required")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		e.lg.Error("knowledge embedder gemini client failed", loggateway.StepID("knowledge.embed_fail"), loggateway.Err(err))
		return nil, kerrors.InternalServer("KNOWLEDGE", "embedder gemini client failed: "+err.Error())
	}
	model = strings.TrimPrefix(model, "models/")
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += defaultEmbedBatchSize {
		end := start + defaultEmbedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]
		contents := make([]*genai.Content, len(batch))
		for i, t := range batch {
			contents[i] = genai.NewContentFromText(t, genai.RoleUser)
		}
		cfg := &genai.EmbedContentConfig{TaskType: "RETRIEVAL_DOCUMENT"}
		if taskType != "" {
			cfg.TaskType = taskType
		}
		if dim > 0 && dim <= math.MaxInt32 {
			d := int32(dim)
			cfg.OutputDimensionality = &d
		}
		resp, err := client.Models.EmbedContent(ctx, model, contents, cfg)
		if err != nil {
			e.lg.Error("knowledge embedder gemini API failed", loggateway.StepID("knowledge.embed_fail"), loggateway.Err(err))
			return nil, kerrors.InternalServer("KNOWLEDGE", "embedder gemini API failed: "+err.Error())
		}
		if len(resp.Embeddings) != len(batch) {
			return nil, kerrors.InternalServer("KNOWLEDGE", fmt.Sprintf("embedder gemini: expected %d embeddings, got %d", len(batch), len(resp.Embeddings)))
		}
		for _, emb := range resp.Embeddings {
			if len(emb.Values) == 0 {
				return nil, kerrors.InternalServer("KNOWLEDGE", "embedder gemini: empty embedding")
			}
			vec := make([]float32, len(emb.Values))
			for i, v := range emb.Values {
				vec[i] = float32(v)
			}
			out = append(out, vec)
		}
	}
	return out, nil
}

func (e *Embedder) embedHuggingFaceBatch(ctx context.Context, baseURL string, dim int, texts []string) ([][]float32, error) {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += defaultEmbedBatchSize {
		end := start + defaultEmbedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]
		type req struct {
			Inputs     []string `json:"inputs"`
			Dimensions int      `json:"dimensions,omitempty"`
		}
		payload := req{Inputs: batch}
		if dim > 0 {
			payload.Dimensions = dim
		}
		body, err := jsonPOST(ctx, embedHTTPClient, baseURL+"/embed", "", payload)
		if err != nil {
			return nil, err
		}
		var data [][]float64
		if err := json.Unmarshal(body, &data); err != nil {
			e.lg.Warn("解析 huggingface embed 响应失败", loggateway.StepID("knowledge.embed_fail"), loggateway.Err(err))
			return nil, kerrors.InternalServer("KNOWLEDGE", "embedder huggingface parse failed: "+err.Error())
		}
		if len(data) != len(batch) {
			return nil, kerrors.InternalServer("KNOWLEDGE", fmt.Sprintf("embedder huggingface: expected %d embeddings, got %d", len(batch), len(data)))
		}
		for _, row := range data {
			vec := make([]float32, len(row))
			for i, v := range row {
				vec[i] = float32(v)
			}
			out = append(out, vec)
		}
	}
	return out, nil
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
	body, err := jsonPOST(ctx, embedHTTPClient, baseURL+"/api/embeddings", "", req{
		Model:  model,
		Prompt: text,
	})
	if err != nil {
		return nil, err
	}
	var r resp
	if err := json.Unmarshal(body, &r); err != nil {
		e.lg.Warn("解析 ollama embed 响应失败", loggateway.StepID("knowledge.embed_fail"), loggateway.Err(err))
		return nil, kerrors.InternalServer("KNOWLEDGE", "embedder ollama parse failed: "+err.Error())
	}
	return r.Embedding, nil
}

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
		return nil, kerrors.InternalServer("KNOWLEDGE", "embedder http request failed: "+err.Error())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, kerrors.InternalServer("KNOWLEDGE", fmt.Sprintf("embedder http %d: %s", resp.StatusCode, string(body)))
	}
	return body, nil
}

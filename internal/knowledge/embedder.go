package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"golang.org/x/sync/singleflight"
	"google.golang.org/genai"
)

func init() {
	timeoutSec := 5
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

// ErrEmbedderNotConfigured is returned when the embedder has not been fully
// configured via the memory module UI (missing base_url or api_key for
// providers that require them). Callers should skip embedding and degrade
// gracefully instead of attempting a network call that will time out.
var ErrEmbedderNotConfigured = apierror.Unavailable("KNOWLEDGE", "embedder not configured: set provider/base_url/api_key in memory module UI")

// Embedder is the interface for text embedding providers.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
}

// EmbedderAdmin provides runtime configuration access for embedding providers.
type EmbedderAdmin interface {
	Config() (provider, baseURL, model string, dim int, configured bool, hasAPIKey bool)
	Update(provider, baseURL, apiKey, model string, dim int)
}

// MultiProviderEmbedder generates embeddings for text using a remote API.
type MultiProviderEmbedder struct {
	mu       sync.RWMutex
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	dim      int
	lg       loggateway.Logger
	// lastPrewarmAt 记录最近一次成功预热时间（60s 窗口去重；仅成功才写入）。
	lastPrewarmAt time.Time
	// monitorBus 流程日志总线（装配层经 SetMonitorBus 注入；nil 时跳过流程日志）。
	monitorBus contract.MonitorBus
	// queryCache 仅缓存查询侧单条 embedding；配置变更立即清空。键为哈希，
	// 避免在进程堆中额外保留查询明文。
	queryCache  map[[sha256.Size]byte]queryEmbeddingCacheEntry
	queryFlight singleflight.Group
}

type queryEmbeddingCacheEntry struct {
	vector    []float32
	expiresAt time.Time
}

const (
	queryEmbeddingCacheTTL = 10 * time.Minute
	queryEmbeddingCacheMax = 512
)

var _ Embedder = (*MultiProviderEmbedder)(nil)
var _ EmbedderAdmin = (*MultiProviderEmbedder)(nil)

// NewMultiProviderEmbedder creates a MultiProviderEmbedder with the given configuration.
func NewMultiProviderEmbedder(provider, baseURL, apiKey, model string, dim int, lg loggateway.Logger) *MultiProviderEmbedder {
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
	return &MultiProviderEmbedder{
		Provider:   provider,
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		Model:      model,
		dim:        dim,
		lg:         lg,
		queryCache: make(map[[sha256.Size]byte]queryEmbeddingCacheEntry),
	}
}

// Config returns a redacted view of embedder settings (EP-KN-01).
func (e *MultiProviderEmbedder) Config() (provider, baseURL, model string, dim int, configured bool, hasAPIKey bool) {
	if e == nil {
		return "", "", "", 0, false, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	hasAPIKey = e.APIKey != ""
	switch e.Provider {
	case ProviderOllama:
		// Ollama runs locally; only base_url is required.
		configured = e.BaseURL != ""
	case ProviderHuggingFace:
		configured = e.BaseURL != ""
	case ProviderGemini:
		configured = hasAPIKey
	default:
		// OpenAI-compatible: both base_url and api_key must be set via the
		// memory module UI. Empty base_url means "not configured" (previously
		// defaulted to https://api.openai.com, which caused 21s timeouts
		// under DNS pollution).
		configured = e.BaseURL != "" && hasAPIKey
	}
	return e.Provider, e.BaseURL, e.Model, e.dim, configured, hasAPIKey
}

// Update applies runtime embedder settings from admin UI (EP-KN-01).
func (e *MultiProviderEmbedder) Update(provider, baseURL, apiKey, model string, dim int) {
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
		e.dim = dim
	}
	clear(e.queryCache)
}

func (e *MultiProviderEmbedder) snapshot() (provider, baseURL, apiKey, model string, dim int) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Provider, e.BaseURL, e.APIKey, e.Model, e.dim
}

// Dim returns the embedding dimension.
func (e *MultiProviderEmbedder) Dim() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.dim
}

// prewarmMinInterval bounds how often Prewarm re-sends its ping request.
const prewarmMinInterval = 60 * time.Second

// Prewarm sends a minimal embedding request ("ping") to move the cold-start
// cost (TCP/TLS handshake + remote model warm-up, 实测 ~2.4s) off the first
// real memory-recall turn. Mounted on process startup and voice.start.
//
// Semantics (C3):
//   - nil receiver / not configured → silent no-op (embedding is optional);
//   - success is deduplicated for prewarmMinInterval;
//   - failure logs Warn (K3) and is NOT deduplicated — the next call retries
//     (K4). The error is returned for testability; callers ignore it.
//
// Concurrency: check-then-ping without holding the lock during network I/O;
// a rare concurrent double-ping is harmless.
func (e *MultiProviderEmbedder) Prewarm(ctx context.Context) error {
	if e == nil {
		return nil
	}
	if _, _, _, _, configured, _ := e.Config(); !configured {
		return nil
	}
	e.mu.RLock()
	last := e.lastPrewarmAt
	e.mu.RUnlock()
	if !last.IsZero() && time.Since(last) < prewarmMinInterval {
		return nil
	}
	// RETRIEVAL_QUERY task type: semantically a query-side warm-up, and it
	// keeps the ping out of the ingest flow log (EmbedBatchWithTaskType only
	// flow-logs non-query batches).
	if _, err := e.embedBatchWithTaskType(ctx, []string{"ping"}, "RETRIEVAL_QUERY"); err != nil {
		e.lg.Warn("embedding prewarm failed", loggateway.StepID("knowledge.embed_prewarm"), loggateway.Err(err))
		return err
	}
	e.mu.Lock()
	e.lastPrewarmAt = time.Now()
	e.mu.Unlock()
	e.lg.Info("embedding prewarm done", loggateway.StepID("knowledge.embed_prewarm"))
	return nil
}

// EmbedSingle returns a single embedding vector for the input text.
func (e *MultiProviderEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, apierror.BadRequest(apierror.DomainKnowledge, "embedder: text is empty")
	}
	return e.embedQueryCached(ctx, text, "")
}

// EmbedWithTaskType returns a single embedding with a task type hint (e.g. "RETRIEVAL_QUERY").
func (e *MultiProviderEmbedder) EmbedWithTaskType(ctx context.Context, text string, taskType string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, apierror.BadRequest(apierror.DomainKnowledge, "embedder: text is empty")
	}
	return e.embedQueryCached(ctx, text, taskType)
}

func (e *MultiProviderEmbedder) embedQueryCached(ctx context.Context, text, taskType string) ([]float32, error) {
	provider, baseURL, _, model, dim := e.snapshot()
	key := sha256.Sum256([]byte(provider + "\x00" + baseURL + "\x00" + model + "\x00" +
		strconv.Itoa(dim) + "\x00" + taskType + "\x00" + strings.TrimSpace(text)))
	now := time.Now()
	e.mu.RLock()
	cached, ok := e.queryCache[key]
	e.mu.RUnlock()
	if ok && now.Before(cached.expiresAt) {
		return append([]float32(nil), cached.vector...), nil
	}

	value, err, _ := e.queryFlight.Do(string(key[:]), func() (any, error) {
		now := time.Now()
		e.mu.RLock()
		cached, ok := e.queryCache[key]
		e.mu.RUnlock()
		if ok && now.Before(cached.expiresAt) {
			return append([]float32(nil), cached.vector...), nil
		}
		vecs, err := e.embedBatchWithTaskType(ctx, []string{text}, taskType)
		if err != nil {
			return nil, err
		}
		if len(vecs) == 0 || len(vecs[0]) == 0 {
			return nil, apierror.Internal(apierror.DomainKnowledge, "embedder: empty embedding")
		}
		vector := append([]float32(nil), vecs[0]...)
		e.mu.Lock()
		if len(e.queryCache) >= queryEmbeddingCacheMax {
			for cacheKey, entry := range e.queryCache {
				if now.After(entry.expiresAt) {
					delete(e.queryCache, cacheKey)
				}
			}
			if len(e.queryCache) >= queryEmbeddingCacheMax {
				for cacheKey := range e.queryCache {
					delete(e.queryCache, cacheKey)
					break
				}
			}
		}
		e.queryCache[key] = queryEmbeddingCacheEntry{
			vector: vector, expiresAt: now.Add(queryEmbeddingCacheTTL),
		}
		e.mu.Unlock()
		return append([]float32(nil), vector...), nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]float32), nil
}

// Embed returns embeddings for a slice of texts using provider batch APIs when available.
func (e *MultiProviderEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return e.EmbedBatchWithTaskType(ctx, texts, "")
}

// SetMonitorBus 注入流程日志总线（装配层调用；nil = 不发射流程日志）。
func (e *MultiProviderEmbedder) SetMonitorBus(bus contract.MonitorBus) {
	if e == nil {
		return
	}
	e.monitorBus = bus
}

// EmbedBatchWithTaskType returns embeddings with an optional task type hint (e.g. "RETRIEVAL_QUERY").
// Currently only Gemini uses task type; other providers ignore it.
func (e *MultiProviderEmbedder) EmbedBatchWithTaskType(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
	start := time.Now()
	vecs, err := e.embedBatchWithTaskType(ctx, texts, taskType)
	// 查询嵌入（RETRIEVAL_QUERY）属检索热路径，已由 knowledge.search 覆盖；
	// 这里只记录文档批嵌入（摄取路径 taskType 为空 / RETRIEVAL_DOCUMENT）。
	if e.monitorBus != nil && len(texts) > 0 && taskType != "RETRIEVAL_QUERY" {
		flow := newKnowledgeFlow(ctx, e.monitorBus, nil)
		if err != nil {
			flow.LogError("system.knowledge.embed_fail", "知识嵌入失败，降级处理",
				event.P("batch_size", len(texts)),
				event.P("duration_ms", time.Since(start).Milliseconds()),
				event.P("error", err.Error()))
		} else {
			flow.LogDone("knowledge.ingest.embed", "文档向量嵌入完成",
				event.P("batch_size", len(texts)),
				event.P("duration_ms", time.Since(start).Milliseconds()))
		}
	}
	return vecs, err
}

func (e *MultiProviderEmbedder) embedBatchWithTaskType(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	// Skip when the embedder has not been fully configured via the memory
	// module UI. This avoids blocking the LLM main flow on a doomed network
	// call (e.g. default https://api.openai.com under DNS pollution).
	if _, _, _, _, configured, _ := e.Config(); !configured {
		return nil, ErrEmbedderNotConfigured
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

func (e *MultiProviderEmbedder) embedOpenAIBatch(ctx context.Context, baseURL, apiKey, model string, texts []string) ([][]float32, error) {
	if baseURL == "" {
		return nil, ErrEmbedderNotConfigured
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
			return nil, apierror.Internal(apierror.DomainKnowledge, "embedder openai parse failed").WithCause(err)
		}
		if len(r.Data) != len(batch) {
			e.lg.Error("knowledge embedder openai count mismatch", loggateway.StepID("knowledge.embed_fail"), loggateway.Int("expected", len(batch)), loggateway.Int("got", len(r.Data)))
			return nil, apierror.Internal(apierror.DomainKnowledge, "embedder openai: expected %d embeddings, got %d", len(batch), len(r.Data))
		}
		ordered := make([][]float32, len(batch))
		for _, item := range r.Data {
			if item.Index < 0 || item.Index >= len(batch) {
				return nil, apierror.Internal(apierror.DomainKnowledge, "embedder openai: invalid index %d", item.Index)
			}
			ordered[item.Index] = item.Embedding
		}
		for i, vec := range ordered {
			if len(vec) == 0 {
				return nil, apierror.Internal(apierror.DomainKnowledge, "embedder openai: empty embedding at %d", start+i)
			}
			out = append(out, vec)
		}
	}
	return out, nil
}

func (e *MultiProviderEmbedder) embedOllamaBatch(ctx context.Context, baseURL, model string, texts []string) ([][]float32, error) {
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

func (e *MultiProviderEmbedder) embedGeminiBatch(ctx context.Context, apiKey, model string, dim int, texts []string, taskType string) ([][]float32, error) {
	if apiKey == "" {
		e.lg.Error("knowledge embedder gemini API key required", loggateway.StepID("knowledge.embed_fail"))
		return nil, apierror.BadRequest(apierror.DomainKnowledge, "embedder gemini: API key required")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		e.lg.Error("knowledge embedder gemini client failed", loggateway.StepID("knowledge.embed_fail"), loggateway.Err(err))
		return nil, apierror.Internal(apierror.DomainKnowledge, "embedder gemini client failed").WithCause(err)
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
			return nil, apierror.Internal(apierror.DomainKnowledge, "embedder gemini API failed").WithCause(err)
		}
		if len(resp.Embeddings) != len(batch) {
			return nil, apierror.Internal(apierror.DomainKnowledge, "embedder gemini: expected %d embeddings, got %d", len(batch), len(resp.Embeddings))
		}
		for _, emb := range resp.Embeddings {
			if len(emb.Values) == 0 {
				return nil, apierror.Internal(apierror.DomainKnowledge, "embedder gemini: empty embedding")
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

func (e *MultiProviderEmbedder) embedHuggingFaceBatch(ctx context.Context, baseURL string, dim int, texts []string) ([][]float32, error) {
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
			return nil, apierror.Internal(apierror.DomainKnowledge, "embedder huggingface parse failed").WithCause(err)
		}
		if len(data) != len(batch) {
			return nil, apierror.Internal(apierror.DomainKnowledge, "embedder huggingface: expected %d embeddings, got %d", len(batch), len(data))
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

func (e *MultiProviderEmbedder) embedOllamaWith(ctx context.Context, baseURL, model, text string) ([]float32, error) {
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
		return nil, apierror.Internal(apierror.DomainKnowledge, "embedder ollama parse failed").WithCause(err)
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
		return nil, apierror.Internal(apierror.DomainKnowledge, "embedder http request failed").WithCause(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, apierror.Internal(apierror.DomainKnowledge, "embedder http %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

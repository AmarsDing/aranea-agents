package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ---------- VLM image captioning (multimodal memory) ----------
//
// Cycle 2 academic-track compliance: this captioner is the only LLM
// component in the memory system, and the default model is gpt-4o-mini as
// the rules require (EVAL_MEDIA_VLM_MODEL exists for industry-track runs
// only). Images are captioned into dense factual text at Add/Search time;
// Add-side captions are stored as ordinary text atoms and embedded with
// text-embedding-v4, so retrieval reuses the existing textual path.
//
// The component is disabled unless EVAL_MEDIA_VLM_BASE_URL and
// EVAL_MEDIA_VLM_API_KEY are both set; when disabled the server is
// byte-identical to the caption-only route. Every failure (timeout, 5xx,
// bad payload) degrades softly: the image falls back to acknowledge-only
// and never fails Add/Search (FAQ Q23).

const (
	defaultEvalVLMModel        = "gpt-4o-mini"
	defaultEvalVLMTimeoutSec   = 25
	defaultEvalVLMConcurrency  = 4
	defaultEvalVLMMaxImages    = 12
	defaultEvalVLMBudgetSec    = 45
	defaultEvalVLMCacheEntries = 4096
)

// evalVLMCaptionPrompt optimizes for downstream text retrieval: dense,
// factual, OCR-inclusive, no speculation.
const evalVLMCaptionPrompt = `You are captioning an image for a long-term memory system. Write one dense, factual paragraph (under 200 words) that lets a text-only retrieval system answer future questions about this image. Cover: overall scene or subject; people (count, appearance, actions); notable objects and their spatial arrangement; colors and style; and for charts, diagrams, or screenshots, the axes, labels, data trends, and UI structure. Transcribe all clearly visible text verbatim in its original language inside quotes. Describe only what is visible: no guesses, no interpretation, no aesthetic judgment.`

// evalImageKey locates one image part inside an Add/Search request: message
// index plus the part's position in the original content array.
type evalImageKey struct{ msg, part int }

// evalCaptionJob is one contract-valid image awaiting a caption.
type evalCaptionJob struct {
	key     evalImageKey
	mime    string
	payload string // base64 body of the Data URI
}

type evalVLMCaptioner struct {
	baseURL   string
	apiKey    string
	model     string
	client    *http.Client
	sem       chan struct{} // global in-flight bound
	maxImages int           // per-request caption cap (budget guard)
	budget    time.Duration // per-request total caption time budget
	maxBytes  int64         // decoded-size cap, mirrors the contract limit
	cacheCap  int
	lg        loggateway.Logger

	mu     sync.Mutex
	cache  map[string]string // sha256(payload) -> caption
	cacheQ []string          // FIFO eviction order
}

// newEvalVLMCaptionerFromEnv returns nil (component disabled) unless both
// the endpoint and key are configured.
func newEvalVLMCaptionerFromEnv(lg loggateway.Logger, maxImageBytes int64) *evalVLMCaptioner {
	baseURL := strings.TrimSpace(os.Getenv("EVAL_MEDIA_VLM_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("EVAL_MEDIA_VLM_API_KEY"))
	if baseURL == "" || apiKey == "" {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	timeoutSec := envInt64("EVAL_MEDIA_VLM_TIMEOUT_SEC", defaultEvalVLMTimeoutSec)
	if timeoutSec < 1 {
		timeoutSec = defaultEvalVLMTimeoutSec
	}
	conc := envInt64("EVAL_MEDIA_VLM_MAX_CONCURRENCY", defaultEvalVLMConcurrency)
	if conc < 1 {
		conc = defaultEvalVLMConcurrency
	}
	budgetSec := envInt64("EVAL_MEDIA_VLM_BUDGET_SEC", defaultEvalVLMBudgetSec)
	if budgetSec < 1 {
		budgetSec = defaultEvalVLMBudgetSec
	}
	c := &evalVLMCaptioner{
		baseURL:   baseURL,
		apiKey:    apiKey,
		model:     envOr("EVAL_MEDIA_VLM_MODEL", defaultEvalVLMModel),
		client:    &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		sem:       make(chan struct{}, conc),
		maxImages: int(envInt64("EVAL_MEDIA_VLM_MAX_IMAGES", defaultEvalVLMMaxImages)),
		budget:    time.Duration(budgetSec) * time.Second,
		maxBytes:  maxImageBytes,
		cacheCap:  int(envInt64("EVAL_MEDIA_VLM_CACHE_ENTRIES", defaultEvalVLMCacheEntries)),
		lg:        lg,
		cache:     map[string]string{},
	}
	if c.maxImages < 1 {
		c.maxImages = defaultEvalVLMMaxImages
	}
	if c.cacheCap < 16 {
		c.cacheCap = defaultEvalVLMCacheEntries
	}
	lg.Info("vlm media captioning enabled",
		loggateway.StepID("memoryeval.startup"), loggateway.Str("model", c.model))
	return c
}

// collectEvalCaptionJobs extracts contract-valid images in message/part
// order; invalid payloads are skipped here exactly as persistMediaParts
// skips them, so the two paths never diverge.
func collectEvalCaptionJobs(msgs []rawMessage) []evalCaptionJob {
	var jobs []evalCaptionJob
	for i, m := range msgs {
		for _, img := range m.Content.images {
			mime, payload, ok := parseEvalDataURI(img.URL)
			if !ok {
				continue
			}
			jobs = append(jobs, evalCaptionJob{
				key:     evalImageKey{msg: i, part: img.Index},
				mime:    mime,
				payload: payload,
			})
		}
	}
	return jobs
}

// captionAll fans jobs out concurrently under the global semaphore and the
// per-request budget. Nil-receiver safe: a disabled captioner yields nil.
func (c *evalVLMCaptioner) captionAll(ctx context.Context, jobs []evalCaptionJob) map[evalImageKey]string {
	if c == nil || len(jobs) == 0 {
		return nil
	}
	if len(jobs) > c.maxImages {
		c.lg.Warn("eval media caption truncated to per-request cap",
			loggateway.StepID("memoryeval.media_caption"),
			loggateway.Int("images", len(jobs)), loggateway.Int("cap", c.maxImages))
		jobs = jobs[:c.maxImages]
	}
	ctx, cancel := context.WithTimeout(ctx, c.budget)
	defer cancel()
	// Singleflight: identical payloads (replayed Adds, duplicated parts)
	// share one VLM call; the result is fanned out to every matching key.
	groups := make(map[string][]evalCaptionJob, len(jobs))
	var order []string
	for _, j := range jobs {
		sum := sha256.Sum256([]byte(j.payload))
		h := hex.EncodeToString(sum[:])
		if _, seen := groups[h]; !seen {
			order = append(order, h)
		}
		groups[h] = append(groups[h], j)
	}
	out := make(map[evalImageKey]string, len(jobs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, h := range order {
		group := groups[h]
		wg.Add(1)
		go func() {
			defer wg.Done()
			caption, ok := c.caption(ctx, group[0].mime, group[0].payload)
			if !ok {
				return
			}
			mu.Lock()
			for _, j := range group {
				out[j.key] = caption
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// caption returns the cached or freshly generated caption. ok=false means
// the caller falls back to acknowledge-only for this image.
func (c *evalVLMCaptioner) caption(ctx context.Context, mime, payload string) (string, bool) {
	// Reject over-cap payloads from the encoded length without decoding
	// (base64 inflates ~4/3), mirroring decodeEvalImagePayload.
	if c.maxBytes > 0 && int64(len(payload)) > (c.maxBytes+2)/3*4+2 {
		return "", false
	}
	sum := sha256.Sum256([]byte(payload))
	key := hex.EncodeToString(sum[:])
	if v, ok := c.lookup(key); ok {
		return v, true
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return "", false
	}
	// Another goroutine may have captioned the same image while we waited.
	if v, ok := c.lookup(key); ok {
		return v, true
	}
	text, err := c.vlmChat(ctx, mime, payload)
	if err != nil {
		c.lg.Warn("eval media caption failed",
			loggateway.StepID("memoryeval.media_caption"), loggateway.Err(err))
		return "", false
	}
	if text = strings.TrimSpace(text); text == "" {
		return "", false
	}
	c.remember(key, text)
	return text, true
}

func (c *evalVLMCaptioner) lookup(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.cache[key]
	return v, ok
}

func (c *evalVLMCaptioner) remember(key, caption string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.cache[key]; exists {
		return
	}
	for len(c.cacheQ) >= c.cacheCap {
		oldest := c.cacheQ[0]
		c.cacheQ = c.cacheQ[1:]
		delete(c.cache, oldest)
	}
	c.cache[key] = caption
	c.cacheQ = append(c.cacheQ, key)
}

// vlmChat issues one OpenAI-compatible vision chat completion. The image is
// forwarded as the original Data URI; the server never decodes the bytes.
func (c *evalVLMCaptioner) vlmChat(ctx context.Context, mime, payload string) (string, error) {
	body := map[string]any{
		"model":       c.model,
		"temperature": 0,
		"max_tokens":  400,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": evalVLMCaptionPrompt},
				{"type": "image_url", "image_url": map[string]any{
					"url": "data:" + mime + ";base64," + payload,
				}},
			},
		}},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(c.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vlm chat status %d: %s", resp.StatusCode, strings.TrimSpace(string(data[:min(len(data), 200)])))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("vlm chat returned no choices")
	}
	return vlmContentText(parsed.Choices[0].Message.Content), nil
}

// vlmContentText tolerates both the plain-string content form and the
// parts-array form some OpenAI-compatible providers return.
func vlmContentText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b []string
	for _, p := range parts {
		if p.Type == "text" && strings.TrimSpace(p.Text) != "" {
			b = append(b, p.Text)
		}
	}
	return strings.TrimSpace(strings.Join(b, "\n"))
}

// evalCaptionLines renders captions as newline-joined atoms, one line per
// image in part order, so statement splitting stores each caption as its own
// searchable memory row next to the message text.
func evalCaptionLines(msgIdx int, images []rawImageRef, captions map[evalImageKey]string) string {
	if len(captions) == 0 {
		return ""
	}
	var lines []string
	for _, img := range images {
		caption, ok := captions[evalImageKey{msg: msgIdx, part: img.Index}]
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("Image attachment (part %d): %s", img.Index, caption))
	}
	return strings.Join(lines, "\n")
}

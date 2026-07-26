package provider

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// usageTapTransport rewrites DeepSeek-style `prompt_cache_hit_tokens` in LLM
// API responses into the OpenAI-standard `prompt_tokens_details.cached_tokens`
// field, so the openai-go SDK parses it and the framework exposes it as
// model.Usage.PromptTokensDetails.CachedTokens.
//
// Background: DeepSeek returns usage as
//
//	"usage": {"prompt_tokens":100, "prompt_cache_hit_tokens":80, "prompt_cache_miss_tokens":20}
//
// The openai-go SDK only maps `usage.prompt_tokens_details.cached_tokens`.
// Without this tap, cache-hit tokens are invisible downstream and get billed
// at the full input price (see usage.ApplyTokenUsageCosts).
//
// Adapter-layer only (red line #27: framework source is read-only).
type usageTapTransport struct {
	base http.RoundTripper
}

// newUsageTapTransport wraps base so responses carrying DeepSeek cache-hit
// fields are normalized to the OpenAI-standard shape. nil base → DefaultTransport.
func newUsageTapTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &usageTapTransport{base: base}
}

func (t *usageTapTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	ct := resp.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "text/event-stream"):
		resp.Body = &sseCacheTapReadCloser{
			r:    bufio.NewReader(resp.Body),
			body: resp.Body,
		}
		return resp, nil
	case strings.Contains(ct, "application/json"):
		data, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr != nil {
			return nil, rerr
		}
		if bytes.Contains(data, []byte("prompt_cache_hit_tokens")) {
			data = rewriteCacheHitTokens(data)
		}
		resp.Body = io.NopCloser(bytes.NewReader(data))
		resp.ContentLength = int64(len(data))
		resp.Header.Set("Content-Length", strconv.Itoa(len(data)))
		return resp, nil
	default:
		return resp, nil
	}
}

// needsUsageTap reports whether the provider may return DeepSeek-style
// cache-hit usage fields. Matches explicit variant, deepseek base URL, or a
// deepseek model name routed through an OpenAI-compatible proxy.
func needsUsageTap(cfg ProviderModelConfig) bool {
	if InferVariant(cfg) == "deepseek" {
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(cfg.ModelAPI)), "deepseek")
}

// rewriteCacheHitTokens moves usage.prompt_cache_hit_tokens into
// usage.prompt_tokens_details.cached_tokens within one JSON object.
// Returns the input unchanged when the field is absent or JSON is invalid.
func rewriteCacheHitTokens(payload []byte) []byte {
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return payload
	}
	if !moveCacheHitField(root) {
		return payload
	}
	out, err := json.Marshal(root)
	if err != nil {
		return payload
	}
	return out
}

func moveCacheHitField(root map[string]any) bool {
	usage, ok := root["usage"].(map[string]any)
	if !ok {
		return false
	}
	hitRaw, ok := usage["prompt_cache_hit_tokens"]
	if !ok {
		return false
	}
	hit, ok := hitRaw.(float64)
	if !ok || hit <= 0 {
		return false
	}
	details, _ := usage["prompt_tokens_details"].(map[string]any)
	if details == nil {
		details = make(map[string]any, 1)
		usage["prompt_tokens_details"] = details
	}
	// Provider already reports the standard field — don't overwrite real data.
	if existing, ok := details["cached_tokens"].(float64); ok && existing > 0 {
		return false
	}
	details["cached_tokens"] = hit
	return true
}

// sseCacheTapReadCloser transforms SSE data lines on the fly, rewriting
// DeepSeek cache-hit usage into the OpenAI-standard field. Line-oriented:
// buffers partial lines across Read calls; non-usage lines pass through.
type sseCacheTapReadCloser struct {
	r    *bufio.Reader
	body io.Closer
	out  []byte // pending transformed bytes
	eof  bool
	err  error // terminal error to deliver after out is drained
}

func (s *sseCacheTapReadCloser) Read(p []byte) (int, error) {
	for len(s.out) == 0 {
		if s.eof {
			if s.err != nil {
				err := s.err
				s.err = nil
				return 0, err
			}
			return 0, io.EOF
		}
		line, err := s.r.ReadBytes('\n')
		if len(line) > 0 {
			s.out = transformSSEDataLine(line)
		}
		if err != nil {
			s.eof = true
			if err != io.EOF {
				s.err = err
			}
		}
	}
	n := copy(p, s.out)
	s.out = s.out[n:]
	return n, nil
}

func (s *sseCacheTapReadCloser) Close() error {
	return s.body.Close()
}

// transformSSEDataLine rewrites one SSE line when it is a `data:` payload
// containing prompt_cache_hit_tokens. Everything else passes through verbatim.
func transformSSEDataLine(line []byte) []byte {
	const prefix = "data:"
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return line
	}
	if !bytes.Contains(line, []byte("prompt_cache_hit_tokens")) {
		return line
	}
	payload := bytes.TrimSpace(line[len(prefix):])
	rewritten := rewriteCacheHitTokens(payload)
	if bytes.Equal(rewritten, payload) {
		return line
	}
	out := make([]byte, 0, len(rewritten)+8)
	out = append(out, prefix...)
	out = append(out, ' ')
	out = append(out, rewritten...)
	out = append(out, '\n')
	return out
}

package provider

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestRewriteCacheHitTokens(t *testing.T) {
	t.Run("moves_hit_tokens_into_details", func(t *testing.T) {
		in := `{"id":"x","usage":{"prompt_tokens":41783,"completion_tokens":2339,"total_tokens":44122,"prompt_cache_hit_tokens":29248,"prompt_cache_miss_tokens":12535}}`
		out := rewriteCacheHitTokens([]byte(in))
		s := string(out)
		if !strings.Contains(s, `"prompt_tokens_details"`) || !strings.Contains(s, `"cached_tokens":29248`) {
			t.Fatalf("expected cached_tokens=29248 in prompt_tokens_details, got: %s", s)
		}
		// Original DeepSeek fields are preserved (harmless; SDK ignores unknown fields).
		if !strings.Contains(s, `"prompt_cache_hit_tokens":29248`) {
			t.Fatalf("expected original hit field preserved, got: %s", s)
		}
	})

	t.Run("creates_details_when_missing", func(t *testing.T) {
		in := `{"usage":{"prompt_tokens":10,"prompt_cache_hit_tokens":8}}`
		out := rewriteCacheHitTokens([]byte(in))
		if !strings.Contains(string(out), `"cached_tokens":8`) {
			t.Fatalf("expected cached_tokens=8, got: %s", string(out))
		}
	})

	t.Run("keeps_existing_cached_tokens", func(t *testing.T) {
		in := `{"usage":{"prompt_tokens":10,"prompt_cache_hit_tokens":8,"prompt_tokens_details":{"cached_tokens":5}}}`
		out := rewriteCacheHitTokens([]byte(in))
		if string(out) != in {
			t.Fatalf("expected unchanged when cached_tokens already present, got: %s", string(out))
		}
	})

	t.Run("zero_hit_is_noop", func(t *testing.T) {
		in := `{"usage":{"prompt_tokens":10,"prompt_cache_hit_tokens":0}}`
		out := rewriteCacheHitTokens([]byte(in))
		if string(out) != in {
			t.Fatalf("expected unchanged for zero hit, got: %s", string(out))
		}
	})

	t.Run("no_usage_is_noop", func(t *testing.T) {
		in := `{"choices":[{"delta":{"content":"hi"}}]}`
		out := rewriteCacheHitTokens([]byte(in))
		if string(out) != in {
			t.Fatalf("expected unchanged without usage, got: %s", string(out))
		}
	})

	t.Run("invalid_json_is_noop", func(t *testing.T) {
		in := `[DONE]`
		out := rewriteCacheHitTokens([]byte(in))
		if string(out) != in {
			t.Fatalf("expected unchanged for invalid JSON, got: %s", string(out))
		}
	})
}

func TestTransformSSEDataLine(t *testing.T) {
	t.Run("rewrites_usage_chunk", func(t *testing.T) {
		line := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"prompt_cache_hit_tokens\":80}}\n"
		out := string(transformSSEDataLine([]byte(line)))
		if !strings.HasPrefix(out, "data: ") || !strings.HasSuffix(out, "\n") {
			t.Fatalf("expected data: prefix and newline suffix, got: %q", out)
		}
		if !strings.Contains(out, `"cached_tokens":80`) {
			t.Fatalf("expected cached_tokens=80, got: %q", out)
		}
	})

	t.Run("passes_through_done", func(t *testing.T) {
		line := "data: [DONE]\n"
		out := transformSSEDataLine([]byte(line))
		if string(out) != line {
			t.Fatalf("expected [DONE] unchanged, got: %q", string(out))
		}
	})

	t.Run("passes_through_non_data_lines", func(t *testing.T) {
		line := ": keep-alive comment\n"
		out := transformSSEDataLine([]byte(line))
		if string(out) != line {
			t.Fatalf("expected comment unchanged, got: %q", string(out))
		}
	})

	t.Run("passes_through_content_chunks_fast", func(t *testing.T) {
		line := "data: {\"choices\":[{\"delta\":{\"content\":\"prompt_cache_hit_tokens mention in text\"}}]}\n"
		// Field name appears inside content text, not as a usage field — rewrite
		// triggers but finds no usage.prompt_cache_hit_tokens, so line unchanged.
		out := transformSSEDataLine([]byte(line))
		if string(out) != line {
			t.Fatalf("expected content chunk unchanged, got: %q", string(out))
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUsageTapTransport_SSE(t *testing.T) {
	sseBody := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":41783,"completion_tokens":2339,"total_tokens":44122,"prompt_cache_hit_tokens":29248,"prompt_cache_miss_tokens":12535}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sseBody)),
		}, nil
	})

	rt := newUsageTapTransport(base)
	resp, err := rt.RoundTrip(&http.Request{})
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, `"cached_tokens":29248`) {
		t.Fatalf("expected rewritten usage chunk, got: %s", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("expected [DONE] preserved, got: %s", out)
	}
	if !strings.Contains(out, `"content":"hello"`) {
		t.Fatalf("expected content chunk preserved, got: %s", out)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}

func TestUsageTapTransport_SSE_PartialLines(t *testing.T) {
	// Force tiny reads so lines split across Read calls.
	sseBody := `data: {"choices":[],"usage":{"prompt_cache_hit_tokens":42}}` + "\n"
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sseBody)),
		}, nil
	})
	rt := newUsageTapTransport(base)
	resp, err := rt.RoundTrip(&http.Request{})
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	var buf strings.Builder
	tmp := make([]byte, 7) // smaller than any line → exercises buffering
	for {
		n, rerr := resp.Body.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			t.Fatalf("Read error: %v", rerr)
		}
	}
	if !strings.Contains(buf.String(), `"cached_tokens":42`) {
		t.Fatalf("expected rewritten chunk across partial reads, got: %s", buf.String())
	}
}

func TestUsageTapTransport_JSON(t *testing.T) {
	body := `{"usage":{"prompt_tokens":100,"prompt_cache_hit_tokens":80}}`
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	rt := newUsageTapTransport(base)
	resp, err := rt.RoundTrip(&http.Request{})
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if !strings.Contains(string(data), `"cached_tokens":80`) {
		t.Fatalf("expected rewritten JSON body, got: %s", string(data))
	}
	if resp.ContentLength != int64(len(data)) {
		t.Fatalf("ContentLength = %d, want %d", resp.ContentLength, len(data))
	}
	if got := resp.Header.Get("Content-Length"); got != strconv.Itoa(len(data)) {
		t.Fatalf("Content-Length header = %q, want %d", got, len(data))
	}
}

func TestUsageTapTransport_OtherContentTypePassthrough(t *testing.T) {
	body := "raw text"
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	rt := newUsageTapTransport(base)
	resp, err := rt.RoundTrip(&http.Request{})
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	data, _ := io.ReadAll(resp.Body)
	if string(data) != body {
		t.Fatalf("expected passthrough, got: %s", string(data))
	}
}

func TestNeedsUsageTap(t *testing.T) {
	cases := []struct {
		name string
		cfg  ProviderModelConfig
		want bool
	}{
		{"variant_deepseek", ProviderModelConfig{ProviderType: "deepseek"}, true},
		{"base_url_deepseek", ProviderModelConfig{BaseURL: "https://api.deepseek.com/v1"}, true},
		{"model_name_deepseek", ProviderModelConfig{ModelAPI: "deepseek-chat"}, true},
		{"openai_plain", ProviderModelConfig{ProviderType: "openai", ModelAPI: "gpt-4o"}, false},
		{"empty", ProviderModelConfig{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsUsageTap(c.cfg); got != c.want {
				t.Fatalf("needsUsageTap = %v, want %v", got, c.want)
			}
		})
	}
}

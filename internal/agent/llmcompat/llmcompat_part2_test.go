package llmcompat

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// catalog 静态 thinking_disabled=true 被调用方显式 effort 覆盖（P2-1 同约：
// 显式路由策略优先于静态配置）。
func TestCallOpenAICompatChatStream_ThinkingEffort_OverridesThinkingDisabled(t *testing.T) {
	var body []byte
	chunks := []string{
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`,
	}
	srv := sseServer(t, chunks, &body)

	cfg := ProviderAPIConfig{ProviderType: "deepseek", APIBaseURL: srv.URL, APIKey: "test-key",
		ThinkingDisabled: true, ThinkingEffort: "max"}
	_, _, _, _, _, err := CallOpenAICompatChatStream(context.Background(), srv.Client(), cfg, "deepseek-v4-flash",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}}, StreamCallbacks{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(body), `"reasoning_effort"`) {
		t.Fatalf("explicit effort must inject reasoning_effort: %s", string(body))
	}
	if strings.Contains(string(body), `"disabled"`) {
		t.Fatalf("explicit effort must override thinking_disabled: %s", string(body))
	}
}

// ─── 2026-10-20 P0-1：aux 调用解析缓存命中 token ─────────────────────────────
//
// 背景：llmcompat 此前只透传 prompt/completion，Usage.PromptTokensDetails.
// CachedTokens 被丢弃——aux 行（intent/title/planner/allocator/memory）cached
// 恒 0，且 aux 行参与命中率聚合，DeepSeek 命中率被系统性低估。

// OpenAI 标准字段 prompt_tokens_details.cached_tokens 必须透传到 cachedTok。
func TestCallOpenAICompatChat_CachedTokens_Propagated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"m",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,`+
			`"prompt_tokens_details":{"cached_tokens":40}}}`)
	}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})

	cfg := ProviderAPIConfig{ProviderType: "openai", APIBaseURL: srv.URL, APIKey: "test-key"}
	_, _, promptTok, _, cachedTok, err := CallOpenAICompatChat(context.Background(), srv.Client(), cfg, "test-model",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promptTok != 100 {
		t.Fatalf("promptTok = %d, want 100", promptTok)
	}
	if cachedTok != 40 {
		t.Fatalf("cachedTok = %d, want 40 (prompt_tokens_details.cached_tokens must propagate)", cachedTok)
	}
}

// DeepSeek 私有字段 prompt_cache_hit_tokens 经 usageTapTransport 归一化后
// 同样必须透传到 cachedTok（side-channel 路径此前未装 tap）。
func TestCallOpenAICompatChat_DeepSeekCacheHit_Normalized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"m",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,`+
			`"prompt_cache_hit_tokens":80,"prompt_cache_miss_tokens":20}}`)
	}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})

	cfg := ProviderAPIConfig{ProviderType: "deepseek", APIBaseURL: srv.URL, APIKey: "test-key"}
	_, _, promptTok, _, cachedTok, err := CallOpenAICompatChat(context.Background(), srv.Client(), cfg, "deepseek-v4-flash",
		[]OpenAICompatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promptTok != 100 {
		t.Fatalf("promptTok = %d, want 100", promptTok)
	}
	if cachedTok != 80 {
		t.Fatalf("cachedTok = %d, want 80 (DeepSeek prompt_cache_hit_tokens must be normalized by usage tap)", cachedTok)
	}
}

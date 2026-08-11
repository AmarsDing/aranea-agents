package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// embedderPrewarmServer emulates the OpenAI-compatible embeddings endpoint,
// counting incoming requests. status != 200 makes every request fail.
func embedderPrewarmServer(t *testing.T, status int) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float32{0.1, 0.2, 0.3}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// C3：nil embedder → no-op（接线方 nil-safe）。
func TestEmbedderPrewarm_NilEmbedderNoop(t *testing.T) {
	var e *MultiProviderEmbedder
	if err := e.Prewarm(context.Background()); err != nil {
		t.Fatalf("nil embedder prewarm must be a no-op, got %v", err)
	}
}

// C3：未配置（OpenAI 兼容 provider 缺 base_url）→ 静默跳过，不发网络请求。
func TestEmbedderPrewarm_NotConfiguredSkips(t *testing.T) {
	e := NewMultiProviderEmbedder(ProviderOpenAI, "", "", "", 0, loggateway.NewNoop())
	if err := e.Prewarm(context.Background()); err != nil {
		t.Fatalf("not-configured prewarm must skip silently, got %v", err)
	}
}

// C3：成功预热发一次最小 ping；60s 窗口内重复调用去重（不重复发请求）。
func TestEmbedderPrewarm_PingsAndDedups(t *testing.T) {
	srv, hits := embedderPrewarmServer(t, http.StatusOK)
	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "", 0, loggateway.NewNoop())
	if err := e.Prewarm(context.Background()); err != nil {
		t.Fatalf("first prewarm: %v", err)
	}
	if err := e.Prewarm(context.Background()); err != nil {
		t.Fatalf("second prewarm: %v", err)
	}
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Fatalf("prewarm within dedup window must send exactly 1 ping, got %d", got)
	}
}

// C3：失败的预热不参与去重——下次调用必须重试（K4 重试语义）。
func TestEmbedderPrewarm_FailureAllowsRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float32{0.1}}},
		})
	}))
	t.Cleanup(srv.Close)
	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "", 0, loggateway.NewNoop())
	if err := e.Prewarm(context.Background()); err == nil {
		t.Fatal("first prewarm must surface the server error")
	}
	if err := e.Prewarm(context.Background()); err != nil {
		t.Fatalf("failed prewarm must not be deduplicated — retry must run, got %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected 2 requests (failure + retry), got %d", got)
	}
}

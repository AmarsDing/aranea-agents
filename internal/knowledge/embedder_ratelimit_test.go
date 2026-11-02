package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// embedderFlakyServer 模拟 OpenAI 兼容嵌入端点：前 failTimes 次返回给定
// 状态码，之后返回 200；hits 记录总请求数。
func embedderFlakyServer(t *testing.T, status int, failTimes int32) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n <= failTimes {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"limit_requests"}}`))
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

// 429 限流在重试预算内恢复 → 调用成功且重试次数正确。
func TestEmbedder_RateLimitRetrySucceeds(t *testing.T) {
	defer func(orig func() int64) { embedBackoffJitterNanos = orig }(embedBackoffJitterNanos)
	embedBackoffJitterNanos = func() int64 { return 0 }

	srv, hits := embedderFlakyServer(t, http.StatusTooManyRequests, 2)
	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "", 0, loggateway.NewNoop())

	vecs, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("embed must succeed after 429 retries, got %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 3 {
		t.Fatalf("unexpected vectors: %v", vecs)
	}
	if got := atomic.LoadInt32(hits); got != 3 {
		t.Fatalf("expected 3 requests (1 initial + 2 retries), got %d", got)
	}
}

// 429 持续超过重试上限 → 维持原有 fail-soft（返回错误，由调用方降级）。
func TestEmbedder_RateLimitRetryExhausted(t *testing.T) {
	defer func(orig func() int64) { embedBackoffJitterNanos = orig }(embedBackoffJitterNanos)
	embedBackoffJitterNanos = func() int64 { return 0 }
	t.Setenv("KRATOS_KNOWLEDGE_EMBED_RETRY_MAX", "2")

	srv, hits := embedderFlakyServer(t, http.StatusTooManyRequests, 100)
	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "", 0, loggateway.NewNoop())

	if _, err := e.Embed(context.Background(), []string{"hello"}); err == nil {
		t.Fatal("embed must fail after exhausting 429 retries")
	}
	if got := atomic.LoadInt32(hits); got != 3 {
		t.Fatalf("expected 3 requests (1 initial + 2 retries), got %d", got)
	}
}

// 非限流错误（如 500）不重试，单次失败即返回。
func TestEmbedder_NonRateLimitErrorNoRetry(t *testing.T) {
	defer func(orig func() int64) { embedBackoffJitterNanos = orig }(embedBackoffJitterNanos)
	embedBackoffJitterNanos = func() int64 { return 0 }

	srv, hits := embedderFlakyServer(t, http.StatusInternalServerError, 100)
	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "", 0, loggateway.NewNoop())

	if _, err := e.Embed(context.Background(), []string{"hello"}); err == nil {
		t.Fatal("embed must fail on 500")
	}
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Fatalf("non-429 error must not retry, got %d requests", got)
	}
}

// 并发闸门：maxInflight=2 时，8 个并发 Embed 的服务端并发数不得超过 2。
func TestEmbedder_InflightGateBoundsConcurrency(t *testing.T) {
	defer func(orig func() int64) { embedBackoffJitterNanos = orig }(embedBackoffJitterNanos)
	embedBackoffJitterNanos = func() int64 { return 0 }
	t.Setenv("KRATOS_KNOWLEDGE_EMBED_MAX_INFLIGHT", "2")

	var current, maxSeen int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&current, 1)
		for {
			m := atomic.LoadInt32(&maxSeen)
			if n <= m || atomic.CompareAndSwapInt32(&maxSeen, m, n) {
				break
			}
		}
		time.Sleep(120 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float32{0.1, 0.2, 0.3}}},
		})
	}))
	t.Cleanup(srv.Close)

	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "", 0, loggateway.NewNoop())
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.Embed(context.Background(), []string{"hello"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent embed failed: %v", err)
		}
	}
	if got := atomic.LoadInt32(&maxSeen); got > 2 {
		t.Fatalf("inflight gate broken: server saw %d concurrent requests, want <= 2", got)
	}
}

// isEmbedRateLimited 判定覆盖：jsonPOST 包装与阿里云 limit_requests 文案。
func TestIsEmbedRateLimited(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{`embedder http 429: {"error":{"code":"limit_requests"}}`, true},
		{`Error 429, rate limit exceeded`, true},
		{`embedder http 500: internal`, false},
		{`embedder service unreachable`, false},
	}
	for _, c := range cases {
		err := &fakeErr{msg: c.msg}
		if got := isEmbedRateLimited(err); got != c.want {
			t.Fatalf("isEmbedRateLimited(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
	if isEmbedRateLimited(nil) {
		t.Fatal("nil error must not be rate-limited")
	}
	if isEmbedRateLimited(&fakeErr{msg: "embedder http 4290: weird"}) && !strings.Contains("4290", "429") {
		t.Fatal("sanity")
	}
}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }

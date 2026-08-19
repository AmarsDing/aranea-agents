package knowledge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// P1-3 (2026-08-19): embedding 用量记录——OpenAI 响应 usage 解析为真实
// prompt_tokens（usage_source=response）；无 usage 的 provider/网关降级
// chars 估算（usage_source=estimated）；调用失败记录失败行但不估算 token。

type captureUsageRecorder struct {
	mu    sync.Mutex
	calls []EmbedUsageInput
}

func (c *captureUsageRecorder) RecordEmbedUsage(_ context.Context, in EmbedUsageInput) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, in)
}

func (c *captureUsageRecorder) last() EmbedUsageInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[len(c.calls)-1]
}

func openaiEmbedServer(t *testing.T, withUsage bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		usage := ""
		if withUsage {
			usage = `,"usage":{"prompt_tokens":42,"total_tokens":42}`
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2,0.3]}]` + usage + `}`))
	}))
}

func TestEmbedUsage_OpenAIUsageRecorded(t *testing.T) {
	srv := openaiEmbedServer(t, true)
	defer srv.Close()
	rec := &captureUsageRecorder{}
	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "text-embedding-3-small", 3, loggateway.NewNoop())
	e.SetUsageRecorder(rec)

	if _, err := e.Embed(context.Background(), []string{"hello world"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("calls=%d want 1", len(rec.calls))
	}
	got := rec.last()
	if got.Tokens != 42 {
		t.Fatalf("Tokens=%d want 42 (provider-reported)", got.Tokens)
	}
	if got.UsageSource != "response" {
		t.Fatalf("UsageSource=%q want response", got.UsageSource)
	}
	if got.Provider != ProviderOpenAI || got.Model != "text-embedding-3-small" {
		t.Fatalf("attribution=%s/%s", got.Provider, got.Model)
	}
	if got.Err != nil {
		t.Fatalf("Err=%v want nil", got.Err)
	}
	if got.Latency <= 0 {
		t.Fatal("Latency must be positive")
	}
}

func TestEmbedUsage_NoUsageFallsBackToEstimated(t *testing.T) {
	srv := openaiEmbedServer(t, false) // gateway omits usage
	defer srv.Close()
	rec := &captureUsageRecorder{}
	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "m", 3, loggateway.NewNoop())
	e.SetUsageRecorder(rec)

	text := strings.Repeat("abcdefghij", 10) // 100 chars
	if _, err := e.Embed(context.Background(), []string{text}); err != nil {
		t.Fatal(err)
	}
	got := rec.last()
	if got.Tokens <= 0 {
		t.Fatalf("Tokens=%d want >0 estimated", got.Tokens)
	}
	if got.UsageSource != "estimated" {
		t.Fatalf("UsageSource=%q want estimated", got.UsageSource)
	}
}

func TestEmbedUsage_FailureRecordsFailedRowWithoutTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	rec := &captureUsageRecorder{}
	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "m", 3, loggateway.NewNoop())
	e.SetUsageRecorder(rec)

	if _, err := e.Embed(context.Background(), []string{"boom"}); err == nil {
		t.Fatal("want error")
	}
	if len(rec.calls) != 1 {
		t.Fatalf("calls=%d want 1", len(rec.calls))
	}
	got := rec.last()
	if got.Err == nil {
		t.Fatal("Err must carry the call failure")
	}
	if got.Tokens != 0 {
		t.Fatalf("Tokens=%d want 0 on failure (no estimation)", got.Tokens)
	}
}

func TestEmbedUsage_NilRecorderSkips(t *testing.T) {
	srv := openaiEmbedServer(t, true)
	defer srv.Close()
	e := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "k", "m", 3, loggateway.NewNoop())
	if _, err := e.Embed(context.Background(), []string{"hello"}); err != nil {
		t.Fatal(err)
	}
}

func TestEmbedUsage_NotConfiguredSkipsRecording(t *testing.T) {
	rec := &captureUsageRecorder{}
	e := NewMultiProviderEmbedder(ProviderOpenAI, "", "", "m", 3, loggateway.NewNoop())
	e.SetUsageRecorder(rec)
	if _, err := e.Embed(context.Background(), []string{"hello"}); !errors.Is(err, ErrEmbedderNotConfigured) {
		t.Fatalf("err=%v want ErrEmbedderNotConfigured", err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("calls=%d want 0 (no network call was made)", len(rec.calls))
	}
}

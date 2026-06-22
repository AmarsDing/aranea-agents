package compress

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func TestCompressCacheKey_Deterministic(t *testing.T) {
	req := Request{
		Transcript:   "hello world",
		PriorSummary: "prev",
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		SystemPrompt: "sys",
	}
	k1 := CompressCacheKey("sess-1", req)
	k2 := CompressCacheKey("sess-1", req)
	if k1 != k2 {
		t.Fatalf("same input should produce same key, got %s vs %s", k1, k2)
	}
}

func TestCompressCacheKey_DifferentSession(t *testing.T) {
	req := Request{Transcript: "hello", Provider: "openai", Model: "gpt-4o-mini"}
	k1 := CompressCacheKey("sess-1", req)
	k2 := CompressCacheKey("sess-2", req)
	if k1 == k2 {
		t.Fatalf("different sessions should produce different keys")
	}
}

func TestCompressCacheKey_DifferentTranscript(t *testing.T) {
	r1 := Request{Transcript: "hello", Provider: "openai", Model: "gpt-4o-mini"}
	r2 := Request{Transcript: "world", Provider: "openai", Model: "gpt-4o-mini"}
	k1 := CompressCacheKey("sess-1", r1)
	k2 := CompressCacheKey("sess-1", r2)
	if k1 == k2 {
		t.Fatalf("different transcripts should produce different keys")
	}
}

func TestCompressCache_PutAndGet(t *testing.T) {
	c := NewCompressCache(256, 10*time.Minute, loggateway.NewNoop())
	result := Result{Markdown: "summary", PromptTokens: 100, CompletionTokens: 50}
	c.Put("key-1", result)
	got, ok := c.Get("key-1")
	if !ok {
		t.Fatalf("expected cache hit")
	}
	if got.Markdown != result.Markdown {
		t.Fatalf("got %q want %q", got.Markdown, result.Markdown)
	}
}

func TestCompressCache_Miss(t *testing.T) {
	c := NewCompressCache(256, 10*time.Minute, loggateway.NewNoop())
	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatalf("expected cache miss")
	}
}

func TestCompressCache_TTLExpiry(t *testing.T) {
	c := NewCompressCache(256, 50*time.Millisecond, loggateway.NewNoop())
	result := Result{Markdown: "summary"}
	c.Put("key-1", result)
	_, ok := c.Get("key-1")
	if !ok {
		t.Fatalf("expected cache hit before TTL")
	}
	time.Sleep(80 * time.Millisecond)
	_, ok = c.Get("key-1")
	if ok {
		t.Fatalf("expected cache miss after TTL expiry")
	}
}

func TestCompressCache_LRUEviction(t *testing.T) {
	c := NewCompressCache(3, 10*time.Minute, loggateway.NewNoop())
	c.Put("a", Result{Markdown: "a"})
	c.Put("b", Result{Markdown: "b"})
	c.Put("c", Result{Markdown: "c"})
	c.Get("a")
	c.Put("d", Result{Markdown: "d"})
	_, ok := c.Get("b")
	if ok {
		t.Fatalf("expected b to be evicted (LRU), but got hit")
	}
	if c.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", c.Len())
	}
}

func TestCompressCache_Overwrite(t *testing.T) {
	c := NewCompressCache(256, 10*time.Minute, loggateway.NewNoop())
	c.Put("key-1", Result{Markdown: "v1"})
	c.Put("key-1", Result{Markdown: "v2"})
	got, ok := c.Get("key-1")
	if !ok {
		t.Fatalf("expected cache hit")
	}
	if got.Markdown != "v2" {
		t.Fatalf("got %q want %q", got.Markdown, "v2")
	}
}

func TestCompressCache_ConcurrentSafe(t *testing.T) {
	c := NewCompressCache(256, 10*time.Minute, loggateway.NewNoop())
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			c.Put(string(rune('a'+idx%26)), Result{Markdown: "v"})
		}(i)
		go func(idx int) {
			defer wg.Done()
			c.Get(string(rune('a' + idx%26)))
		}(i)
	}
	wg.Wait()
}

func TestContextWithSessionID(t *testing.T) {
	ctx := context.Background()
	if sid := SessionIDFromCtx(ctx); sid != "" {
		t.Fatalf("expected empty sessionID, got %q", sid)
	}
	ctx = ContextWithSessionID(ctx, "sess-123")
	if sid := SessionIDFromCtx(ctx); sid != "sess-123" {
		t.Fatalf("got %q want %q", sid, "sess-123")
	}
}

type mockCompressor struct {
	calls  int
	result Result
	err    error
}

func (m *mockCompressor) Compress(ctx context.Context, req Request) (Result, error) {
	m.calls++
	return m.result, m.err
}

func TestCachingCompressor_CacheHit(t *testing.T) {
	mock := &mockCompressor{result: Result{Markdown: "summary"}}
	cache := NewCompressCache(256, 10*time.Minute, loggateway.NewNoop())
	cc := NewCachingCompressor(mock, cache, loggateway.NewNoop())

	ctx := ContextWithSessionID(context.Background(), "sess-1")
	req := Request{Transcript: "hello", Provider: "openai", Model: "gpt-4o-mini"}

	res, err := cc.Compress(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Markdown != "summary" {
		t.Fatalf("got %q want %q", res.Markdown, "summary")
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 LLM call, got %d", mock.calls)
	}

	res2, err := cc.Compress(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.Markdown != "summary" {
		t.Fatalf("got %q want %q", res2.Markdown, "summary")
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 LLM call (cache hit), got %d", mock.calls)
	}
}

func TestCachingCompressor_CacheMiss_DifferentSession(t *testing.T) {
	mock := &mockCompressor{result: Result{Markdown: "summary"}}
	cache := NewCompressCache(256, 10*time.Minute, loggateway.NewNoop())
	cc := NewCachingCompressor(mock, cache, loggateway.NewNoop())

	req := Request{Transcript: "hello", Provider: "openai", Model: "gpt-4o-mini"}

	ctx1 := ContextWithSessionID(context.Background(), "sess-1")
	ctx2 := ContextWithSessionID(context.Background(), "sess-2")

	cc.Compress(ctx1, req)
	cc.Compress(ctx2, req)
	if mock.calls != 2 {
		t.Fatalf("expected 2 LLM calls (different sessions), got %d", mock.calls)
	}
}

func TestCachingCompressor_CompressWithCacheHit(t *testing.T) {
	mock := &mockCompressor{result: Result{Markdown: "summary"}}
	cache := NewCompressCache(256, 10*time.Minute, loggateway.NewNoop())
	cc := NewCachingCompressor(mock, cache, loggateway.NewNoop())

	ctx := ContextWithSessionID(context.Background(), "sess-1")
	req := Request{Transcript: "hello", Provider: "openai", Model: "gpt-4o-mini"}

	_, hit, err := cc.CompressWithCacheHit(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hit {
		t.Fatalf("expected cache miss on first call")
	}

	_, hit, err = cc.CompressWithCacheHit(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hit {
		t.Fatalf("expected cache hit on second call")
	}
}

func TestCachingCompressor_InnerError(t *testing.T) {
	mock := &mockCompressor{err: ErrEmptyTranscript}
	cache := NewCompressCache(256, 10*time.Minute, loggateway.NewNoop())
	cc := NewCachingCompressor(mock, cache, loggateway.NewNoop())

	ctx := ContextWithSessionID(context.Background(), "sess-1")
	req := Request{Transcript: "hello", Provider: "openai", Model: "gpt-4o-mini"}

	_, err := cc.Compress(ctx, req)
	if err != ErrEmptyTranscript {
		t.Fatalf("got %v want ErrEmptyTranscript", err)
	}
}

func TestNewCompressCache_Defaults(t *testing.T) {
	c := NewCompressCache(0, 0, nil)
	if c.cap != compressCacheDefaultCap {
		t.Fatalf("expected default cap %d, got %d", compressCacheDefaultCap, c.cap)
	}
	if c.ttl != compressCacheDefaultTTL {
		t.Fatalf("expected default ttl %v, got %v", compressCacheDefaultTTL, c.ttl)
	}
}

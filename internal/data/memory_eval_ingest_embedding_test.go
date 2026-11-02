package data

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestEvalStore_ImmutableEvidenceSkipsGovernanceEmbedding(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	emb := newEvalIntentTestEmbedder(nil)
	emb.failEmbedding = true
	// Isolate synchronous governance from the separately tested async index.
	s.emb = emb
	ctx := context.Background()
	n, err := s.AddMessages(ctx, "immutable", "session", []biz.EvalMessage{
		{Role: "user", MessageID: "commit", Content: "fix(auth): reject expired refresh tokens"},
		{Role: "user", MessageID: "event", Content: "I inspected the quartz prism."},
	})
	if err != nil || n != 2 {
		t.Fatalf("AddMessages = %d, %v", n, err)
	}
	if emb.embedCalls+emb.batchCalls != 0 {
		t.Fatalf("immutable Add waited for embedding: single=%d batch=%d", emb.embedCalls, emb.batchCalls)
	}
	s.emb = nil
	items, err := s.SearchMemories(ctx, "immutable", "What was the fix in module auth?", 5)
	if err != nil || !strings.Contains(evalContents(items), "expired refresh tokens") {
		t.Fatalf("durable evidence missing: %+v, %v", items, err)
	}
}

func TestEvalStore_GovernableEvidenceKeepsGovernanceEmbedding(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	emb := newEvalIntentTestEmbedder(nil)
	s.emb = emb
	ctx := context.Background()
	_, err := s.AddMessages(ctx, "governable", "session", []biz.EvalMessage{
		{Role: "user", MessageID: "old", Content: "I inspected the quartz prism. My favorite color is blue", Timestamp: "2024-01-01T00:00:00Z"},
		{Role: "user", MessageID: "new", Content: "My favorite color is red", Timestamp: "2024-02-01T00:00:00Z"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if emb.embedCalls+emb.batchCalls == 0 {
		t.Fatal("governable Add lost intent embedding")
	}
	s.emb = nil
	items, err := s.SearchMemories(ctx, "governable", "What is my favorite color now?", 5)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "favorite color is red") || strings.Contains(text, "favorite color is blue") {
		t.Fatalf("preference governance changed: %s, %v", text, err)
	}
}

type evalTransactionProbeEmbedder struct {
	inner      *evalIntentTestEmbedder
	inTx       bool
	batchSizes []int
}

func (p *evalTransactionProbeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	p.inTx = p.inTx || ctx.Value(rawTxKey{}) != nil
	return p.inner.Embed(ctx, text)
}

func (p *evalTransactionProbeEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	p.inTx = p.inTx || ctx.Value(rawTxKey{}) != nil
	p.batchSizes = append(p.batchSizes, len(texts))
	return p.inner.EmbedBatch(ctx, texts)
}

func TestEvalStore_GovernanceEmbeddingsBatchBeforeTransaction(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	probe := &evalTransactionProbeEmbedder{inner: newEvalIntentTestEmbedder(nil)}
	s.emb = probe
	ctx := context.Background()
	n, err := s.AddMessages(ctx, "batch-governance", "session", []biz.EvalMessage{
		{Role: "user", MessageID: "old", Content: "I inspected the quartz prism. My favorite color is blue", Timestamp: "2024-01-01T00:00:00Z"},
		{Role: "user", MessageID: "fix", Content: "fix(auth): reject expired refresh tokens"},
		{Role: "user", MessageID: "new", Content: "My favorite color is red", Timestamp: "2024-02-01T00:00:00Z"},
	})
	if err != nil || n != 3 {
		t.Fatalf("AddMessages = %d, %v", n, err)
	}
	if probe.inTx {
		t.Fatal("network embedding ran inside the write transaction")
	}
	// One anchor build, followed by one cross-message batch: two mutable
	// statements in both prefixed/routing forms. Immutable evidence is omitted.
	if len(probe.batchSizes) != 2 || probe.batchSizes[1] != 4 {
		t.Fatalf("batch sizes = %v, want anchors then four mutable texts", probe.batchSizes)
	}
	s.emb = nil
	items, err := s.SearchMemories(ctx, "batch-governance", "What is my favorite color now?", 5)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "favorite color is red") || strings.Contains(text, "favorite color is blue") {
		t.Fatalf("batched statement mapping damaged preference governance: %s, %v", text, err)
	}
}

func TestEvalStore_GovernanceOutageKeepsTransactionOffline(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	probe := &evalTransactionProbeEmbedder{inner: newEvalIntentTestEmbedder(nil)}
	probe.inner.failEmbedding = true
	s.emb = probe
	s.vec = biz.NewMemoryUsecase(NewMemoryRepo(r.data), probe)
	s.facts.vectorStore = &fakeVectorStore{}
	ctx := context.Background()
	n, err := s.AddMessages(ctx, "governance-outage", "session", []biz.EvalMessage{
		{Role: "user", Content: "My favorite color is blue", Timestamp: "2024-01-01T00:00:00Z"},
		{Role: "user", Content: "Actually, my favorite color is red", Timestamp: "2024-02-01T00:00:00Z"},
	})
	if err != nil || n != 2 {
		t.Fatalf("AddMessages during outage = %d, %v", n, err)
	}
	if probe.inTx {
		t.Fatal("embedding outage triggered a provider retry inside the write transaction")
	}
	s.emb, s.vec = nil, nil
	items, err := s.SearchMemories(ctx, "governance-outage", "What is my favorite color now?", 5)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "favorite color is red") || strings.Contains(text, "favorite color is blue") {
		t.Fatalf("outage damaged same-slot governance: %s, %v", text, err)
	}
}

type evalBlockingGovernanceEmbedder struct {
	inner *evalIntentTestEmbedder
	calls int
}

func (p *evalBlockingGovernanceEmbedder) Embed(ctx context.Context, _ string) ([]float32, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (p *evalBlockingGovernanceEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	p.calls++
	if p.calls == 1 {
		return p.inner.EmbedBatch(ctx, texts) // Router anchors are already healthy.
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestEvalStore_GovernanceTimeoutStillCommitsFacts(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	s.emb = &evalBlockingGovernanceEmbedder{inner: newEvalIntentTestEmbedder(nil)}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	n, err := s.AddMessages(ctx, "governance-timeout", "session", []biz.EvalMessage{
		{Role: "user", Content: "My favorite color is blue", Timestamp: "2024-01-01T00:00:00Z"},
		{Role: "user", Content: "My favorite color is red", Timestamp: "2024-02-01T00:00:00Z"},
	})
	if err != nil || n != 2 {
		t.Fatalf("optional governance exhausted the caller's deadline: AddMessages = %d, %v", n, err)
	}
	s.emb = nil
	items, err := s.SearchMemories(ctx, "governance-timeout", "What is my favorite color now?", 5)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "favorite color is red") || strings.Contains(text, "favorite color is blue") {
		t.Fatalf("governance timeout damaged durable/current evidence: %s, %v", text, err)
	}
}

func TestEvalStore_BusyRouterDoesNotBlockFactCommit(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	s.emb = newEvalIntentTestEmbedder(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	// A concurrent cold Search holds this mutex during remote anchor loading.
	s.routerMu.Lock()
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := s.AddMessages(ctx, "busy-router", "session", []biz.EvalMessage{
			{Role: "user", Content: "My favorite color is red"},
		})
		done <- result{n, err}
	}()
	var got result
	blocked := false
	select {
	case got = <-done:
	case <-time.After(time.Second):
		blocked = true
	}
	s.routerMu.Unlock()
	if blocked {
		<-done // Release the test writer before the schema cleanup.
		t.Fatal("optional router initialization blocked fact commit behind another request")
	}
	if got.err != nil || got.n != 1 {
		t.Fatalf("AddMessages while router busy = %d, %v", got.n, got.err)
	}
}

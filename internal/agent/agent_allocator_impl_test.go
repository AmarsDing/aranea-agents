package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// AgentAllocator Layer 2 (pgvector semantic matching) tests — P1-3.
//
// Covers three correctness goals:
//  1. Embedding success → allocator uses vector cosine similarity for matching.
//  2. Embedding failure → gracefully falls back to TF-IDF (no hard error).
//  3. Empty candidates → returns empty allocation (no panic).
//
// Plus a nil-embedder case verifying the TF-IDF path is used when no embedder
// is wired (preserves backward compatibility with existing callers).
// ---------------------------------------------------------------------------

// mockEmbedder implements knowledge.Embedder for tests. It returns a
// preconfigured vector slice (one vector per input text) or a fixed error.
type mockEmbedder struct {
	mu      sync.Mutex
	calls   int
	vectors [][]float32
	err     error
}

var _ knowledge.Embedder = (*mockEmbedder)(nil)

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	// Return preconfigured vectors; tests must set len(vectors) == len(texts).
	return m.vectors, nil
}

func (m *mockEmbedder) Dim() int {
	if len(m.vectors) > 0 && len(m.vectors[0]) > 0 {
		return len(m.vectors[0])
	}
	return 0
}

func (m *mockEmbedder) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// TestAgentAllocator_Layer2_EmbeddingSuccess_UsesVectorSimilarity verifies that
// when the embedder is configured and succeeds, the allocator picks the agent
// whose capability vector is most cosine-similar to the task vector.
//
// Setup: task text has NO keyword overlap with any agent (so TF-IDF would score
// 0 for all). The mock embedder returns vectors where agent-a is similar to the
// task and agent-b is orthogonal. Only the embedding path can produce a match.
func TestAgentAllocator_Layer2_EmbeddingSuccess_UsesVectorSimilarity(t *testing.T) {
	// Vectors (3 texts: task + agent-a + agent-b).
	// task   = [1.0, 0.0]
	// agentA = [0.9, 0.1]  → cosine ≈ 0.994
	// agentB = [0.0, 1.0]  → cosine = 0.0
	emb := &mockEmbedder{
		vectors: [][]float32{
			{1.0, 0.0},
			{0.9, 0.1},
			{0.0, 1.0},
		},
	}

	allocator := &agentAllocatorImpl{
		embedder: emb,
		lg:       loggateway.NewNoop(),
	}

	// Task text uses tokens that don't appear in any agent's profile so TF-IDF
	// would return 0 for every candidate.
	capabilities := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "Alpha", Description: "alpha specialist"},
		{AgentKey: "agent-b", DisplayName: "Beta", Description: "beta specialist"},
	}
	subTask := biz.SubTask{
		ID:          "st_emb_1",
		Name:        "zzz unrelated task",
		Description: "qqq xyz placeholder",
	}

	cap, score, reason := allocator.matchLayer2(context.Background(), subTask, capabilities, "trace-emb-1")
	if cap.AgentKey == "" {
		t.Fatal("expected a match via embedding path, got empty capability")
	}
	if cap.AgentKey != "agent-a" {
		t.Fatalf("expected agent-a (closest vector), got %s", cap.AgentKey)
	}
	if score <= 0 {
		t.Fatalf("expected positive score from embedding, got %v", score)
	}
	if !strings.Contains(reason, "向量") {
		t.Fatalf("expected reason to indicate vector path, got %q", reason)
	}
	if emb.Calls() != 1 {
		t.Fatalf("expected embedder to be called once, got %d", emb.Calls())
	}
}

// TestAgentAllocator_Layer2_EmbedderError_FallsBackToTFIDF verifies that when
// the embedder returns an error, the allocator logs a warning and falls back to
// the existing TF-IDF keyword matching — no hard error is surfaced.
func TestAgentAllocator_Layer2_EmbedderError_FallsBackToTFIDF(t *testing.T) {
	emb := &mockEmbedder{err: errors.New("embedder unavailable")}

	allocator := &agentAllocatorImpl{
		embedder: emb,
		lg:       loggateway.NewNoop(),
	}

	// Use task text that DOES have keyword overlap with agent-a so the TF-IDF
	// fallback produces a non-zero score.
	capabilities := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "Translation", Description: "translation expert"},
		{AgentKey: "agent-b", DisplayName: "Music", Description: "music player"},
	}
	subTask := biz.SubTask{
		ID:          "st_emb_2",
		Name:        "translate document",
		Description: "translation task",
	}

	cap, score, reason := allocator.matchLayer2(context.Background(), subTask, capabilities, "trace-emb-2")
	if cap.AgentKey == "" {
		t.Fatal("expected a fallback TF-IDF match, got empty capability")
	}
	if cap.AgentKey != "agent-a" {
		t.Fatalf("expected agent-a via TF-IDF fallback, got %s", cap.AgentKey)
	}
	if score <= 0 {
		t.Fatalf("expected positive TF-IDF score, got %v", score)
	}
	if !strings.Contains(reason, "TF-IDF") {
		t.Fatalf("expected reason to indicate TF-IDF fallback, got %q", reason)
	}
	if emb.Calls() != 1 {
		t.Fatalf("expected embedder to be called once before fallback, got %d", emb.Calls())
	}
}

// TestAgentAllocator_Layer2_NilEmbedder_FallsBackToTFIDF verifies that when no
// embedder is wired (nil), the allocator uses the TF-IDF path directly. This
// preserves backward compatibility with callers that haven't been updated.
func TestAgentAllocator_Layer2_NilEmbedder_FallsBackToTFIDF(t *testing.T) {
	allocator := &agentAllocatorImpl{
		embedder: nil,
		lg:       loggateway.NewNoop(),
	}

	capabilities := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "Translation", Description: "translation expert"},
	}
	subTask := biz.SubTask{
		ID:          "st_emb_3",
		Name:        "translate document",
		Description: "translation task",
	}

	cap, score, reason := allocator.matchLayer2(context.Background(), subTask, capabilities, "trace-emb-3")
	if cap.AgentKey == "" {
		t.Fatal("expected a TF-IDF match, got empty capability")
	}
	if cap.AgentKey != "agent-a" {
		t.Fatalf("expected agent-a, got %s", cap.AgentKey)
	}
	if score <= 0 {
		t.Fatalf("expected positive TF-IDF score, got %v", score)
	}
	if !strings.Contains(reason, "TF-IDF") {
		t.Fatalf("expected reason to indicate TF-IDF, got %q", reason)
	}
}

// TestAgentAllocator_Layer2_EmptyCandidates_ReturnsEmpty verifies that an empty
// candidate list returns an empty capability with score 0 — no panic.
func TestAgentAllocator_Layer2_EmptyCandidates_ReturnsEmpty(t *testing.T) {
	emb := &mockEmbedder{
		vectors: [][]float32{{1.0, 0.0}},
	}
	allocator := &agentAllocatorImpl{
		embedder: emb,
		lg:       loggateway.NewNoop(),
	}

	cap, score, reason := allocator.matchLayer2(context.Background(), biz.SubTask{}, nil, "trace-emb-4")
	if cap.AgentKey != "" {
		t.Fatalf("expected empty capability for empty candidates, got %+v", cap)
	}
	if score != 0 {
		t.Fatalf("expected score 0 for empty candidates, got %v", score)
	}
	if reason != "" {
		t.Fatalf("expected empty reason for empty candidates, got %q", reason)
	}
	// Embedder must not be called when there are no candidates.
	if emb.Calls() != 0 {
		t.Fatalf("expected embedder not to be called for empty candidates, got %d calls", emb.Calls())
	}
}

// TestAgentAllocator_Layer2_EmbeddingDimensionMismatch_FallsBackToTFIDF
// verifies that when the embedder returns a mismatched number of vectors, the
// allocator falls back to TF-IDF instead of panicking on index access.
func TestAgentAllocator_Layer2_EmbeddingDimensionMismatch_FallsBackToTFIDF(t *testing.T) {
	// Return only 1 vector for 3 input texts — mismatched count.
	emb := &mockEmbedder{
		vectors: [][]float32{{1.0, 0.0}},
	}

	allocator := &agentAllocatorImpl{
		embedder: emb,
		lg:       loggateway.NewNoop(),
	}

	capabilities := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "Translation", Description: "translation expert"},
		{AgentKey: "agent-b", DisplayName: "Music", Description: "music player"},
	}
	subTask := biz.SubTask{
		ID:          "st_emb_5",
		Name:        "translate document",
		Description: "translation task",
	}

	cap, score, reason := allocator.matchLayer2(context.Background(), subTask, capabilities, "trace-emb-5")
	if cap.AgentKey == "" {
		t.Fatal("expected a fallback TF-IDF match after dimension mismatch, got empty capability")
	}
	if cap.AgentKey != "agent-a" {
		t.Fatalf("expected agent-a via TF-IDF fallback, got %s", cap.AgentKey)
	}
	if score <= 0 {
		t.Fatalf("expected positive TF-IDF score after fallback, got %v", score)
	}
	if !strings.Contains(reason, "TF-IDF") {
		t.Fatalf("expected reason to indicate TF-IDF fallback, got %q", reason)
	}
}

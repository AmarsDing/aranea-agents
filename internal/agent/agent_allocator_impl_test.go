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

// ---------------------------------------------------------------------------
// P-ORCH: orchestration progress events + SpiritSessionID propagation
// ---------------------------------------------------------------------------

// allocatorCaptureBus captures published v2 Events for assertions.
type allocatorCaptureBus struct {
	mu        sync.Mutex
	published []biz.Event
}

func (b *allocatorCaptureBus) Publish(_ context.Context, ev biz.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, ev)
}

func (b *allocatorCaptureBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return nil, func() {}
}

func (b *allocatorCaptureBus) getPublished() []biz.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.Event, len(b.published))
	copy(out, b.published)
	return out
}

// fakeAllocatorAgentFactory captures TaskProfile for SpiritSessionID assertions.
type fakeAllocatorAgentFactory struct {
	profiles []biz.TaskProfile
	agentKey string
	err      error
}

func (f *fakeAllocatorAgentFactory) EnsureAgent(_ context.Context, p biz.TaskProfile) (string, error) {
	f.profiles = append(f.profiles, p)
	if f.err != nil {
		return "", f.err
	}
	if f.agentKey != "" {
		return f.agentKey, nil
	}
	return "factory-agent-1", nil
}

// TestAgentAllocator_PublishOrchestrationProgress verifies the progress event
// helper publishes a well-formed orchestration_progress SystemNoticeEvent and
// is nil-safe for both nil bus and empty session.
func TestAgentAllocator_PublishOrchestrationProgress(t *testing.T) {
	// nil bus → no panic, no publish.
	nilBusAllocator := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	nilBusAllocator.publishOrchestrationProgress(context.Background(), "sess-1", "allocating", map[string]any{"index": 1})

	// empty session → skipped.
	bus := &allocatorCaptureBus{}
	a := &agentAllocatorImpl{bus: bus, lg: loggateway.NewNoop()}
	a.publishOrchestrationProgress(context.Background(), "", "allocating", map[string]any{"index": 1})
	if len(bus.getPublished()) != 0 {
		t.Fatal("empty session must skip publish")
	}

	// normal publish.
	a.publishOrchestrationProgress(context.Background(), "sess-orch", "allocating", map[string]any{
		"index":    2,
		"total":    5,
		"sub_task": "数据预处理",
	})
	published := bus.getPublished()
	if len(published) != 1 {
		t.Fatalf("published=%d want 1", len(published))
	}
	notice, ok := published[0].(*biz.SystemNoticeEvent)
	if !ok {
		t.Fatalf("expected *biz.SystemNoticeEvent, got %T", published[0])
	}
	if notice.NoticeType != "orchestration_progress" {
		t.Errorf("NoticeType=%q want %q", notice.NoticeType, "orchestration_progress")
	}
	if notice.Meta["phase"] != "allocating" {
		t.Errorf("phase=%v want %q", notice.Meta["phase"], "allocating")
	}
	if notice.Meta["index"] != 2 {
		t.Errorf("index=%v want 2", notice.Meta["index"])
	}
	if notice.Meta["total"] != 5 {
		t.Errorf("total=%v want 5", notice.Meta["total"])
	}
	if notice.SpiritSessionID() != "sess-orch" {
		t.Errorf("sessionID=%q want %q", notice.SpiritSessionID(), "sess-orch")
	}
}

// TestAgentAllocator_TryAgentFactory_PassesSpiritSessionID verifies the
// allocator forwards the plan's SpiritSessionID into the factory TaskProfile
// so the factory can route progress events to the owning session.
func TestAgentAllocator_TryAgentFactory_PassesSpiritSessionID(t *testing.T) {
	factory := &fakeAllocatorAgentFactory{agentKey: "factory-agent-x"}
	a := &agentAllocatorImpl{
		agentFactory: factory,
		lg:           loggateway.NewNoop(),
	}
	subTask := biz.SubTask{
		ID:                   "st_1",
		Name:                 "数据预处理",
		Description:          "清洗并预处理原始数据",
		RequiredCapabilities: []string{"data-eng"},
	}

	alloc, ok := a.tryAgentFactoryForSubTask(context.Background(), subTask, "sess-spirit-9", "trace-1")
	if !ok {
		t.Fatal("tryAgentFactoryForSubTask returned ok=false")
	}
	if alloc.AssignedKey != "factory-agent-x" {
		t.Errorf("AssignedKey=%q want %q", alloc.AssignedKey, "factory-agent-x")
	}
	if len(factory.profiles) != 1 {
		t.Fatalf("factory calls=%d want 1", len(factory.profiles))
	}
	if factory.profiles[0].SpiritSessionID != "sess-spirit-9" {
		t.Errorf("SpiritSessionID=%q want %q", factory.profiles[0].SpiritSessionID, "sess-spirit-9")
	}
}

// ---------------------------------------------------------------------------
// P-ORCH.5: Allocate() two-phase parallelization tests.
//
// Phase A parallelizes matchSubTask across subtasks (errgroup + index writes).
// Phase B serializes factory creation (which needs user confirmation). The
// final allocations slice must preserve subtask input order regardless of
// parallel execution timing.
// ---------------------------------------------------------------------------

// fakeAllocatorRepo is an in-memory AllocationPlanRepository for Allocate tests.
type fakeAllocatorRepo struct {
	saved *biz.AllocationPlan
}

func (f *fakeAllocatorRepo) Create(_ context.Context, plan *biz.AllocationPlan) (*biz.AllocationPlan, error) {
	f.saved = plan
	plan.ID = "ap_test"
	return plan, nil
}
func (f *fakeAllocatorRepo) GetByID(_ context.Context, _ string) (*biz.AllocationPlan, error) {
	return f.saved, nil
}
func (f *fakeAllocatorRepo) Update(_ context.Context, plan *biz.AllocationPlan) (*biz.AllocationPlan, error) {
	f.saved = plan
	return plan, nil
}
func (f *fakeAllocatorRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]*biz.AllocationPlan, error) {
	if f.saved == nil {
		return nil, nil
	}
	return []*biz.AllocationPlan{f.saved}, nil
}

// TestAgentAllocator_Allocate_PreservesOrder_ParallelMatch verifies that
// when Allocate processes multiple subtasks in parallel (Phase A), the
// resulting allocations slice preserves the input subtask order.
//
// Without index-based writes, parallel goroutines appending to a shared slice
// would produce nondeterministic order. This test catches that regression by
// checking SubTaskID order matches the input.
func TestAgentAllocator_Allocate_PreservesOrder_ParallelMatch(t *testing.T) {
	agents := []biz.Agent{
		{AgentKey: "agent-a", DisplayName: "Agent A", Roles: []string{"backend"}, Status: "active"},
		{AgentKey: "agent-b", DisplayName: "Agent B", Roles: []string{"frontend"}, Status: "active"},
		{AgentKey: "agent-c", DisplayName: "Agent C", Roles: []string{"data"}, Status: "active"},
	}
	reader := &stubAgentReader{agents: agents}
	capBuilder := NewAgentCapabilityBuilder(reader, loggateway.NewNoop())
	repo := &fakeAllocatorRepo{}

	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		capBuilder:  capBuilder,
		lg:          loggateway.NewNoop(),
		// nil perfRepo/embedder/httpClient → Layer 1 exact match is the only
		// path that can succeed; Layer 2 falls back to TF-IDF (no overlap → 0),
		// Layer 3 returns empty (nil httpClient).
	}

	subTasks := []biz.SubTask{
		{ID: "st_1", Name: "后端", RequiredCapabilities: []string{"backend"}},
		{ID: "st_2", Name: "前端", RequiredCapabilities: []string{"frontend"}},
		{ID: "st_3", Name: "数据", RequiredCapabilities: []string{"data"}},
	}

	plan := &biz.TaskPlan{
		ID:              "tp_test",
		SpiritSessionID: "sess-orch-par",
		TraceID:         "trace-par",
		SubTasks:        subTasks,
		Strategy:        biz.StrategyParallel,
	}

	saved, err := impl.Allocate(context.Background(), plan)
	if err != nil {
		t.Fatalf("Allocate returned error: %v", err)
	}
	if len(saved.Allocations) != 3 {
		t.Fatalf("allocations count=%d want 3", len(saved.Allocations))
	}

	// Verify order preserved.
	wantIDs := []string{"st_1", "st_2", "st_3"}
	gotIDs := []string{saved.Allocations[0].SubTaskID, saved.Allocations[1].SubTaskID, saved.Allocations[2].SubTaskID}
	for i, want := range wantIDs {
		if saved.Allocations[i].SubTaskID != want {
			t.Errorf("allocations[%d].SubTaskID=%q want %q (full order: %v)", i, saved.Allocations[i].SubTaskID, want, gotIDs)
		}
	}

	// Verify each subtask matched the correct agent.
	wantKeys := map[string]string{"st_1": "agent-a", "st_2": "agent-b", "st_3": "agent-c"}
	for _, alloc := range saved.Allocations {
		want := wantKeys[alloc.SubTaskID]
		if alloc.AssignedKey != want {
			t.Errorf("SubTaskID=%s AssignedKey=%q want %q", alloc.SubTaskID, alloc.AssignedKey, want)
		}
	}
}

// TestAgentAllocator_Allocate_FactorySerial_OnAllFailed verifies that when
// all subtasks fail Phase A matching, Phase B invokes the factory serially
// (one factory call per subtask) and the allocations preserve input order.
//
// Setup: agents list is empty so every Layer 1-3 match fails → factory is
// the only path. fakeAllocatorAgentFactory returns a deterministic key per
// call. We assert factory.profiles length == subtask count and allocations
// are in input order.
func TestAgentAllocator_Allocate_FactorySerial_OnAllFailed(t *testing.T) {
	reader := &stubAgentReader{agents: nil} // empty catalog → all matches fail
	capBuilder := NewAgentCapabilityBuilder(reader, loggateway.NewNoop())
	repo := &fakeAllocatorRepo{}
	factory := &fakeAllocatorAgentFactory{agentKey: "factory-agent"}

	impl := &agentAllocatorImpl{
		repo:         repo,
		agentReader:  reader,
		capBuilder:   capBuilder,
		agentFactory: factory,
		lg:           loggateway.NewNoop(),
	}

	subTasks := []biz.SubTask{
		{ID: "st_1", Name: "任务1", RequiredCapabilities: []string{"x"}},
		{ID: "st_2", Name: "任务2", RequiredCapabilities: []string{"y"}},
	}

	plan := &biz.TaskPlan{
		ID:              "tp_factory",
		SpiritSessionID: "sess-orch-fac",
		TraceID:         "trace-fac",
		SubTasks:        subTasks,
		Strategy:        biz.StrategyParallel,
	}

	saved, err := impl.Allocate(context.Background(), plan)
	if err != nil {
		t.Fatalf("Allocate returned error: %v", err)
	}
	if len(saved.Allocations) != 2 {
		t.Fatalf("allocations count=%d want 2", len(saved.Allocations))
	}

	// Factory called exactly once per subtask (serial).
	if len(factory.profiles) != 2 {
		t.Errorf("factory calls=%d want 2 (serial)", len(factory.profiles))
	}

	// Order preserved.
	if saved.Allocations[0].SubTaskID != "st_1" || saved.Allocations[1].SubTaskID != "st_2" {
		t.Errorf("order not preserved: got %s, %s",
			saved.Allocations[0].SubTaskID, saved.Allocations[1].SubTaskID)
	}
}

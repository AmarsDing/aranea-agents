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
// When fn is set it takes precedence, allowing per-text vector assignment
// (P3b: agent batch embed and per-subtask taskText embed are separate calls).
type mockEmbedder struct {
	mu      sync.Mutex
	calls   int
	vectors [][]float32
	err     error
	fn      func(texts []string) [][]float32
}

var _ knowledge.Embedder = (*mockEmbedder)(nil)

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	if m.fn != nil {
		return m.fn(texts), nil
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
	// P3b: 两次独立嵌入调用——agent 批量（2 文本，与 capabilities 顺序对齐）
	// 和 taskText 单条。
	// task   = [1.0, 0.0]
	// agentA = [0.9, 0.1]  → cosine ≈ 0.994
	// agentB = [0.0, 1.0]  → cosine = 0.0
	emb := &mockEmbedder{
		fn: func(texts []string) [][]float32 {
			if len(texts) == 2 { // agent batch: [agent-a, agent-b]
				return [][]float32{{0.9, 0.1}, {0.0, 1.0}}
			}
			return [][]float32{{1.0, 0.0}} // taskText
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
	if emb.Calls() != 2 {
		t.Fatalf("expected embedder to be called twice (agent batch + taskText), got %d", emb.Calls())
	}
}

// countingPerfRepo wraps memAgentPerformanceRepo and counts Get calls
// (P3b perf 去重缓存验证）。
type countingPerfRepo struct {
	inner *memAgentPerformanceRepo
	mu    sync.Mutex
	gets  int
}

func (c *countingPerfRepo) Get(ctx context.Context, agentKey, taskType string) (*biz.AgentPerformance, error) {
	c.mu.Lock()
	c.gets++
	c.mu.Unlock()
	return c.inner.Get(ctx, agentKey, taskType)
}

func (c *countingPerfRepo) GetBestForTaskType(ctx context.Context, taskType string, limit int) ([]*biz.AgentPerformance, error) {
	return c.inner.GetBestForTaskType(ctx, taskType, limit)
}

func (c *countingPerfRepo) Upsert(ctx context.Context, perf *biz.AgentPerformance) error {
	return c.inner.Upsert(ctx, perf)
}

func (c *countingPerfRepo) Gets() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gets
}

// TestAgentAllocator_Layer2_SharedState_EmbedsAgentsOnce verifies the P3b
// shared-state optimization: across multiple subtasks sharing one layer2Shared,
// the agent capability batch is embedded exactly once (each subtask only embeds
// its own taskText), and perf lookups are deduplicated per agentKey.
func TestAgentAllocator_Layer2_SharedState_EmbedsAgentsOnce(t *testing.T) {
	// All vectors identical → cosine = 1 for every candidate (match always succeeds).
	emb := &mockEmbedder{
		fn: func(texts []string) [][]float32 {
			out := make([][]float32, len(texts))
			for i := range out {
				out[i] = []float32{1.0, 0.0}
			}
			return out
		},
	}
	perfRepo := &countingPerfRepo{inner: newMemAgentPerformanceRepo()}
	allocator := &agentAllocatorImpl{
		embedder: emb,
		perfRepo: perfRepo,
		lg:       loggateway.NewNoop(),
	}

	capabilities := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "Alpha", Description: "alpha specialist"},
		{AgentKey: "agent-b", DisplayName: "Beta", Description: "beta specialist"},
	}
	shared := newLayer2Shared()

	for _, id := range []string{"st_shared_1", "st_shared_2"} {
		cap, score, _ := allocator.matchLayer2(context.Background(), biz.SubTask{
			ID:          id,
			Name:        "some task",
			Description: "some description",
		}, capabilities, "trace-shared", shared)
		if cap.AgentKey == "" || score <= 0 {
			t.Fatalf("subtask %s: expected embedding-path match, got key=%q score=%v", id, cap.AgentKey, score)
		}
	}

	// 1 agent batch + 2 taskText = 3（未共享时为 2×(1 batch) + 2 = 4）。
	if emb.Calls() != 3 {
		t.Fatalf("expected 3 embed calls with shared state (1 agent batch + 2 taskText), got %d", emb.Calls())
	}
	// perf 按 agentKey 去重：2 个 Agent 全周期各查 1 次（未共享时为 2×2=4）。
	if perfRepo.Gets() != 2 {
		t.Fatalf("expected 2 perf Get calls with shared cache (one per agent), got %d", perfRepo.Gets())
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

	alloc, ok := a.tryAgentFactoryForSubTask(context.Background(), subTask, "sess-spirit-9", "trace-1", nil)
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
		repo:               repo,
		agentReader:        reader,
		capBuilder:         capBuilder,
		agentFactory:       factory,
		allowFactoryCreate: true,
		lg:                 loggateway.NewNoop(),
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

func TestAgentAllocator_Allocate_HotPathDoesNotCreateAgents(t *testing.T) {
	factory := &fakeAllocatorAgentFactory{agentKey: "factory-agent"}
	impl := &agentAllocatorImpl{
		repo:         &fakeAllocatorRepo{},
		agentReader:  &stubAgentReader{agents: nil},
		capBuilder:   NewAgentCapabilityBuilder(&stubAgentReader{agents: nil}, loggateway.NewNoop()),
		agentFactory: factory,
		lg:           loggateway.NewNoop(),
	}
	_, err := impl.Allocate(context.Background(), &biz.TaskPlan{
		ID:       "tp_closed",
		Strategy: biz.StrategyParallel,
		SubTasks: []biz.SubTask{{ID: "st_1", Name: "任务1", DomainPath: "创作/文案"}},
	})
	if err == nil {
		t.Fatal("hot path miss must fail closed")
	}
	if len(factory.profiles) != 0 {
		t.Fatalf("factory calls=%d want 0", len(factory.profiles))
	}
}

func TestAgentAllocator_MatchSubTask_NoLowScoreAssign(t *testing.T) {
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	caps := []biz.AgentCapability{{
		AgentKey: "be", DisplayName: "后端", Roles: []string{"backend"},
	}}
	_, err := impl.matchSubTask(context.Background(), biz.SubTask{
		ID: "st1", Name: "文案", RequiredCapabilities: []string{"unrelated-xyz"},
	}, caps, "tr")
	if err == nil {
		t.Fatal("below-threshold overlap must be a miss")
	}
}

// ---------------------------------------------------------------------------
// AllocateExplicit: Spirit LLM 显式指定 agent_keys（IDENTITY.md 契约：
// 系统管家/记忆管家/技能管家通过 plan_and_execute + agent_keys=[...] 调用）。
// 显式路由必须跳过启发式匹配（系统 Agent 本就被 BuildAll 过滤，启发式层
// 永远选不到 __system_admin__）。
// ---------------------------------------------------------------------------

func TestAgentAllocator_AllocateExplicit_Subtasks(t *testing.T) {
	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "__system_admin__", DisplayName: "系统管家", Status: "active"},
	}}
	repo := &fakeAllocatorRepo{}
	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		lg:          loggateway.NewNoop(),
	}

	plan := &biz.TaskPlan{
		ID:              "tp_explicit",
		SpiritSessionID: "sess-explicit",
		TraceID:         "trace-explicit",
		SubTasks: []biz.SubTask{
			{ID: "st_1", Name: "安装 Skill A"},
			{ID: "st_2", Name: "安装 Skill B"},
		},
		Strategy: biz.StrategyParallel,
	}

	saved, err := impl.AllocateExplicit(context.Background(), plan, []string{"__system_admin__"})
	if err != nil {
		t.Fatalf("AllocateExplicit returned error: %v", err)
	}
	if len(saved.Allocations) != 2 {
		t.Fatalf("allocations count=%d want 2", len(saved.Allocations))
	}
	for i, alloc := range saved.Allocations {
		if alloc.AssignedKey != "__system_admin__" {
			t.Errorf("allocations[%d].AssignedKey=%q want __system_admin__", i, alloc.AssignedKey)
		}
		if alloc.AssignedName != "系统管家" {
			t.Errorf("allocations[%d].AssignedName=%q want 系统管家", i, alloc.AssignedName)
		}
		if alloc.MatchLayer != "explicit" {
			t.Errorf("allocations[%d].MatchLayer=%q want explicit", i, alloc.MatchLayer)
		}
		if alloc.AssignedType != "agent" {
			t.Errorf("allocations[%d].AssignedType=%q want agent", i, alloc.AssignedType)
		}
	}
	if saved.Allocations[0].SubTaskID != "st_1" || saved.Allocations[1].SubTaskID != "st_2" {
		t.Errorf("order not preserved: %s, %s", saved.Allocations[0].SubTaskID, saved.Allocations[1].SubTaskID)
	}
	if repo.saved == nil {
		t.Fatal("AllocateExplicit did not persist the allocation plan")
	}
}

func TestAgentAllocator_AllocateExplicit_DagTeamMembers(t *testing.T) {
	reader := &stubAgentReader{agents: nil} // names fall back to keys
	repo := &fakeAllocatorRepo{}
	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		lg:          loggateway.NewNoop(),
	}

	plan := &biz.TaskPlan{
		ID:              "tp_explicit_dag",
		SpiritSessionID: "sess-explicit-dag",
		SubTasks:        []biz.SubTask{{ID: "st_1", Name: "协作任务"}},
		Strategy:        biz.StrategyDAG,
	}

	saved, err := impl.AllocateExplicit(context.Background(), plan, []string{"lead-x", "member-y", "member-z"})
	if err != nil {
		t.Fatalf("AllocateExplicit returned error: %v", err)
	}
	if len(saved.Allocations) != 1 {
		t.Fatalf("allocations count=%d want 1", len(saved.Allocations))
	}
	alloc := saved.Allocations[0]
	if alloc.AssignedKey != "lead-x" {
		t.Errorf("AssignedKey=%q want lead-x", alloc.AssignedKey)
	}
	if alloc.AssignedType != "team" {
		t.Errorf("AssignedType=%q want team (dag + multiple keys)", alloc.AssignedType)
	}
	if len(alloc.TeamMemberKeys) != 2 || alloc.TeamMemberKeys[0] != "member-y" || alloc.TeamMemberKeys[1] != "member-z" {
		t.Errorf("TeamMemberKeys=%v want [member-y member-z]", alloc.TeamMemberKeys)
	}
}

func TestAgentAllocator_AllocateExplicit_WholePlan(t *testing.T) {
	reader := &stubAgentReader{agents: nil}
	repo := &fakeAllocatorRepo{}
	impl := &agentAllocatorImpl{
		repo:        repo,
		agentReader: reader,
		lg:          loggateway.NewNoop(),
	}

	plan := &biz.TaskPlan{
		ID:              "tp_explicit_whole",
		SpiritSessionID: "sess-explicit-whole",
		SubTasks:        nil,
		Strategy:        biz.StrategyParallel,
	}

	saved, err := impl.AllocateExplicit(context.Background(), plan, []string{"__memory__"})
	if err != nil {
		t.Fatalf("AllocateExplicit returned error: %v", err)
	}
	if len(saved.Allocations) != 1 {
		t.Fatalf("allocations count=%d want 1", len(saved.Allocations))
	}
	if saved.Allocations[0].SubTaskID != "whole" {
		t.Errorf("SubTaskID=%q want whole", saved.Allocations[0].SubTaskID)
	}
	if saved.Allocations[0].AssignedKey != "__memory__" {
		t.Errorf("AssignedKey=%q want __memory__", saved.Allocations[0].AssignedKey)
	}
}

func TestAgentAllocator_AllocateExplicit_Validation(t *testing.T) {
	impl := &agentAllocatorImpl{lg: loggateway.NewNoop()}
	if _, err := impl.AllocateExplicit(context.Background(), nil, []string{"a"}); err == nil {
		t.Fatal("nil plan: want error")
	}
	plan := &biz.TaskPlan{ID: "tp_v", SpiritSessionID: "s"}
	if _, err := impl.AllocateExplicit(context.Background(), plan, nil); err == nil {
		t.Fatal("empty agent_keys: want error")
	}
	if _, err := impl.AllocateExplicit(context.Background(), plan, []string{"  ", ""}); err == nil {
		t.Fatal("blank agent_keys: want error")
	}
}

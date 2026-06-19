package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// --- Fakes ---

// fakeMemoryService implements MemoryReaderWriter for testing.
type fakeMemoryService struct {
	mu      sync.Mutex
	entries map[string][]*trpcmemory.Entry // keyed by userID
	added   []fakeAddedMemory
	updated map[string]string // memoryID -> new content
	deleted []string
	readErr error
	addErr  error // T3.1: mutation error for AddMemory (reflect/update_core)
	updErr  error // T3.1: mutation error for UpdateMemory (merge)
}

type fakeAddedMemory struct {
	content string
	topics  []string
}

func (f *fakeMemoryService) ReadMemories(_ context.Context, uk trpcmemory.UserKey, limit int) ([]*trpcmemory.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return nil, f.readErr
	}
	entries := f.entries[uk.UserID]
	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}
	out := make([]*trpcmemory.Entry, len(entries))
	copy(out, entries)
	return out, nil
}

func (f *fakeMemoryService) AddMemory(_ context.Context, _ trpcmemory.UserKey, memory string, topics []string, _ ...trpcmemory.AddOption) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, fakeAddedMemory{content: memory, topics: topics})
	return nil
}

func (f *fakeMemoryService) UpdateMemory(_ context.Context, mk trpcmemory.Key, memory string, _ []string, _ ...trpcmemory.UpdateOption) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updErr != nil {
		return f.updErr
	}
	if f.updated == nil {
		f.updated = make(map[string]string)
	}
	f.updated[mk.MemoryID] = memory
	return nil
}

func (f *fakeMemoryService) DeleteMemory(_ context.Context, mk trpcmemory.Key) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, mk.MemoryID)
	return nil
}

// fakeModel implements trpcmodel.Model for testing.
type fakeModel struct {
	response *trpcmodel.Response
	err      error
}

func (m *fakeModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan *trpcmodel.Response, 1)
	if m.response != nil {
		ch <- m.response
	}
	close(ch)
	return ch, nil
}

func (m *fakeModel) Info() trpcmodel.Info {
	return trpcmodel.Info{Name: "fake-model"}
}

// fakeConsolidationQueue implements ConsolidationQueue for testing.
type fakeConsolidationQueue struct {
	mu   sync.Mutex
	jobs []ConsolidationJobRequest
	ch   chan ConsolidationJobRequest
}

func newFakeConsolidationQueue() *fakeConsolidationQueue {
	return &fakeConsolidationQueue{ch: make(chan ConsolidationJobRequest, 10)}
}

func (f *fakeConsolidationQueue) Enqueue(r ConsolidationJobRequest) {
	f.mu.Lock()
	f.jobs = append(f.jobs, r)
	f.mu.Unlock()
	select {
	case f.ch <- r:
	default:
	}
}

func (f *fakeConsolidationQueue) Chan() <-chan ConsolidationJobRequest {
	return f.ch
}

func (f *fakeConsolidationQueue) jobCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

// --- Helpers ---

func buildLLMResponse(jsonContent string) *trpcmodel.Response {
	return &trpcmodel.Response{
		Choices: []trpcmodel.Choice{
			{
				Message: trpcmodel.Message{
					Role:    trpcmodel.RoleAssistant,
					Content: jsonContent,
				},
			},
		},
	}
}

func makeEntry(id, content string, topics ...string) *trpcmemory.Entry {
	return &trpcmemory.Entry{
		ID:      id,
		AppName: "agent-1",
		Memory: &trpcmemory.Memory{
			Memory: content,
			Topics: topics,
		},
		UserID: "user-1",
	}
}

// --- Tests ---

// TestSleepTime_EnqueueConsolidationJob verifies that EnqueueConsolidationJob
// enqueues a job successfully.
func TestSleepTime_EnqueueConsolidationJob(t *testing.T) {
	q := newFakeConsolidationQueue()
	svc := NewSleepTimeService(nil, nil, q, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.EnqueueConsolidationJob(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := q.jobCount(); got != 1 {
		t.Fatalf("expected 1 job enqueued, got %d", got)
	}
}

// TestSleepTime_EnqueueConsolidationJob_NilQueue verifies that a nil queue
// does not panic.
func TestSleepTime_EnqueueConsolidationJob_NilQueue(t *testing.T) {
	svc := NewSleepTimeService(nil, nil, nil, loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.EnqueueConsolidationJob(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error with nil queue, got %v", err)
	}
}

// TestSleepTime_Consolidate_Merge verifies that a merge operation updates the
// target memory and deletes the source memories.
func TestSleepTime_Consolidate_Merge(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {
				makeEntry("mem-1", "User likes Go"),
				makeEntry("mem-2", "User prefers Go language"),
			},
		},
	}
	llmResp := buildLLMResponse(`{"operations":[{"type":"merge","target_id":"mem-1","source_ids":["mem-2"],"merged_content":"User likes Go programming language"}]}`)
	llm := &fakeModel{response: llmResp}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := ms.updated["mem-1"]; got != "User likes Go programming language" {
		t.Errorf("expected mem-1 updated to merged content, got %q", got)
	}
	if len(ms.deleted) != 1 || ms.deleted[0] != "mem-2" {
		t.Errorf("expected mem-2 deleted, got %v", ms.deleted)
	}
}

// TestSleepTime_Consolidate_Reflect verifies that a reflect operation adds a
// new reflection memory.
func TestSleepTime_Consolidate_Reflect(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {
				makeEntry("mem-1", "User asked about Go"),
				makeEntry("mem-2", "User asked about Rust"),
			},
		},
	}
	llmResp := buildLLMResponse(`{"operations":[{"type":"reflect","reflection":"User is exploring programming languages","topics":["insight"]}]}`)
	llm := &fakeModel{response: llmResp}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(ms.added) != 1 {
		t.Fatalf("expected 1 memory added, got %d", len(ms.added))
	}
	if ms.added[0].content != "User is exploring programming languages" {
		t.Errorf("unexpected reflection content: %q", ms.added[0].content)
	}
	if len(ms.added[0].topics) != 1 || ms.added[0].topics[0] != "insight" {
		t.Errorf("unexpected topics: %v", ms.added[0].topics)
	}
}

// TestSleepTime_Consolidate_UpdateCore verifies that an update_core operation
// adds core memory entries.
func TestSleepTime_Consolidate_UpdateCore(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {
				makeEntry("mem-1", "My name is Alice"),
			},
		},
	}
	llmResp := buildLLMResponse(`{"operations":[{"type":"update_core","updates":[{"key":"name","value":"Alice"},{"key":"role","value":"engineer"}]}]}`)
	llm := &fakeModel{response: llmResp}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(ms.added) != 2 {
		t.Fatalf("expected 2 core memory entries added, got %d", len(ms.added))
	}
	// Each core memory entry should have the "core" topic
	for i, a := range ms.added {
		if len(a.topics) != 1 || a.topics[0] != coreMemoryTopic {
			t.Errorf("entry %d: expected topic %q, got %v", i, coreMemoryTopic, a.topics)
		}
	}
}

// TestSleepTime_Consolidate_LLMFailure verifies that LLM failure results in
// graceful degradation (logs warn, no panic, no error returned).
func TestSleepTime_Consolidate_LLMFailure(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {makeEntry("mem-1", "User likes Go")},
		},
	}
	llm := &fakeModel{err: errors.New("LLM service unavailable")}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	// Should not return an error (graceful degradation)
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error on LLM failure (graceful degradation), got %v", err)
	}
	// No operations should have been executed
	if len(ms.added) != 0 || len(ms.updated) != 0 || len(ms.deleted) != 0 {
		t.Errorf("expected no memory mutations on LLM failure, got added=%d updated=%d deleted=%d",
			len(ms.added), len(ms.updated), len(ms.deleted))
	}
}

// TestSleepTime_Consolidate_LLMResponseError verifies that an API-level error
// in the LLM response also results in graceful degradation.
func TestSleepTime_Consolidate_LLMResponseError(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {makeEntry("mem-1", "User likes Go")},
		},
	}
	llmResp := &trpcmodel.Response{
		Error: &trpcmodel.ResponseError{Message: "rate limit exceeded"},
	}
	llm := &fakeModel{response: llmResp}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error on LLM response error (graceful degradation), got %v", err)
	}
	if len(ms.added) != 0 || len(ms.updated) != 0 || len(ms.deleted) != 0 {
		t.Errorf("expected no memory mutations on LLM response error")
	}
}

// TestSleepTime_Consolidate_EmptyMemories verifies that empty memories result
// in a no-op (not an error) and the LLM is not called.
func TestSleepTime_Consolidate_EmptyMemories(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {},
		},
	}
	llm := &fakeModel{err: errors.New("LLM should not be called")}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error on empty memories, got %v", err)
	}
	if len(ms.added) != 0 || len(ms.updated) != 0 || len(ms.deleted) != 0 {
		t.Errorf("expected no memory mutations on empty memories")
	}
}

// TestSleepTime_Consolidate_NoLLM verifies that a nil LLM results in a no-op
// (not an error) when memories exist.
func TestSleepTime_Consolidate_NoLLM(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {makeEntry("mem-1", "User likes Go")},
		},
	}
	svc := NewSleepTimeService(ms, nil, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error with nil LLM, got %v", err)
	}
	if len(ms.added) != 0 || len(ms.updated) != 0 || len(ms.deleted) != 0 {
		t.Errorf("expected no memory mutations with nil LLM")
	}
}

// TestSleepTime_Consolidate_MultipleOperations verifies that multiple
// operations of different types are all executed.
func TestSleepTime_Consolidate_MultipleOperations(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {
				makeEntry("mem-1", "User likes Go"),
				makeEntry("mem-2", "User likes Go lang"),
				makeEntry("mem-3", "User asked about testing"),
			},
		},
	}
	ops := `{"operations":[
		{"type":"merge","target_id":"mem-1","source_ids":["mem-2"],"merged_content":"User likes Go programming"},
		{"type":"reflect","reflection":"User is interested in testing practices","topics":["insight"]},
		{"type":"update_core","updates":[{"key":"interest","value":"Go"}]}
	]}`
	llm := &fakeModel{response: buildLLMResponse(ops)}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	// merge: 1 update + 1 delete
	if got := ms.updated["mem-1"]; got != "User likes Go programming" {
		t.Errorf("expected mem-1 updated, got %q", got)
	}
	if len(ms.deleted) != 1 || ms.deleted[0] != "mem-2" {
		t.Errorf("expected mem-2 deleted, got %v", ms.deleted)
	}
	// reflect: 1 add, update_core: 1 add → total 2 adds
	if len(ms.added) != 2 {
		t.Fatalf("expected 2 memories added (reflect + core), got %d", len(ms.added))
	}
}

// TestSleepTime_Consolidate_MalformedJSON verifies that malformed LLM JSON
// output results in graceful degradation.
func TestSleepTime_Consolidate_MalformedJSON(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {makeEntry("mem-1", "User likes Go")},
		},
	}
	llm := &fakeModel{response: buildLLMResponse("not valid json")}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error on malformed JSON (graceful degradation), got %v", err)
	}
	if len(ms.added) != 0 || len(ms.updated) != 0 || len(ms.deleted) != 0 {
		t.Errorf("expected no memory mutations on malformed JSON")
	}
}

// TestSleepTime_Consolidate_ReadFailure verifies that a memory read failure
// returns an error (not graceful degradation — read failures are not LLM failures).
func TestSleepTime_Consolidate_ReadFailure(t *testing.T) {
	ms := &fakeMemoryService{
		readErr: errors.New("database unavailable"),
	}
	svc := NewSleepTimeService(ms, nil, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err == nil {
		t.Fatal("expected error on read failure, got nil")
	}
}

// TestSleepTime_Consolidate_EmptyOperations verifies that an empty operations
// array from the LLM is a valid no-op.
func TestSleepTime_Consolidate_EmptyOperations(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {makeEntry("mem-1", "User likes Go")},
		},
	}
	llm := &fakeModel{response: buildLLMResponse(`{"operations":[]}`)}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(ms.added) != 0 || len(ms.updated) != 0 || len(ms.deleted) != 0 {
		t.Errorf("expected no memory mutations with empty operations")
	}
}

// TestConsolidationQueue_EnqueueAndChan verifies the in-memory queue
// implementation.
func TestConsolidationQueue_EnqueueAndChan(t *testing.T) {
	q := NewConsolidationQueue(10)
	ch := q.Chan()
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	uk := trpcmemory.UserKey{AppName: "a", UserID: "u"}
	q.Enqueue(ConsolidationJobRequest{UserKey: uk, EnqueuedAt: time.Now()})
	select {
	case req := <-ch:
		if req.UserKey.AppName != "a" || req.UserKey.UserID != "u" {
			t.Errorf("unexpected job: %+v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for job")
	}
}

// TestSleepTime_ConsolidateResult_Unmarshal verifies the JSON structure of
// ConsolidationResult.
func TestSleepTime_ConsolidateResult_Unmarshal(t *testing.T) {
	raw := `{"operations":[
		{"type":"merge","target_id":"t1","source_ids":["s1","s2"],"merged_content":"merged","merged_topics":["a"]},
		{"type":"reflect","reflection":"ref","topics":["b"]},
		{"type":"update_core","updates":[{"key":"k","value":"v"}]}
	]}`
	var result ConsolidationResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(result.Operations))
	}
	if result.Operations[0].Type != "merge" || result.Operations[0].TargetID != "t1" {
		t.Errorf("unexpected op 0: %+v", result.Operations[0])
	}
	if len(result.Operations[0].SourceIDs) != 2 {
		t.Errorf("expected 2 source IDs, got %d", len(result.Operations[0].SourceIDs))
	}
	if result.Operations[1].Type != "reflect" || result.Operations[1].Reflection != "ref" {
		t.Errorf("unexpected op 1: %+v", result.Operations[1])
	}
	if result.Operations[2].Type != "update_core" || len(result.Operations[2].Updates) != 1 {
		t.Errorf("unexpected op 2: %+v", result.Operations[2])
	}
}

// TestSleepTime_Consolidate_MutationFailure_GracefulDegradation verifies that
// mutation failures (AddMemory/UpdateMemory) are treated as graceful
// degradation (return nil) rather than returning an error.
//
// T3.1: This ensures JobRunner retry is safe — only read failures return an
// error (idempotent, safe to retry). Mutation operations are NOT idempotent,
// so they must not be retried.
func TestSleepTime_Consolidate_MutationFailure_GracefulDegradation(t *testing.T) {
	// Test reflect (AddMemory) failure.
	t.Run("reflect_add_failure", func(t *testing.T) {
		ms := &fakeMemoryService{
			entries: map[string][]*trpcmemory.Entry{
				"user-1": {makeEntry("mem-1", "User likes Go")},
			},
			addErr: errors.New("add memory failed"),
		}
		llm := &fakeModel{response: buildLLMResponse(`{"operations":[{"type":"reflect","reflection":"new insight","topics":["t"]}]}`)}
		svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

		uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
		if err := svc.Consolidate(context.Background(), uk); err != nil {
			t.Fatalf("expected nil error on mutation failure (graceful degradation), got %v", err)
		}
	})

	// Test merge (UpdateMemory) failure.
	t.Run("merge_update_failure", func(t *testing.T) {
		ms := &fakeMemoryService{
			entries: map[string][]*trpcmemory.Entry{
				"user-1": {makeEntry("mem-1", "User likes Go")},
			},
			updErr: errors.New("update memory failed"),
		}
		llm := &fakeModel{response: buildLLMResponse(`{"operations":[{"type":"merge","target_id":"mem-1","source_ids":[],"merged_content":"merged","merged_topics":["t"]}]}`)}
		svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())

		uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
		if err := svc.Consolidate(context.Background(), uk); err != nil {
			t.Fatalf("expected nil error on mutation failure (graceful degradation), got %v", err)
		}
	})
}

// TestSleepTime_QueueChan verifies that QueueChan returns the queue channel
// when a queue is wired, and nil otherwise.
func TestSleepTime_QueueChan(t *testing.T) {
	// No queue wired → nil.
	svc := NewSleepTimeService(&fakeMemoryService{}, nil, nil, loggateway.NewNoop())
	if ch := svc.QueueChan(); ch != nil {
		t.Errorf("expected nil channel when no queue wired, got non-nil")
	}

	// Queue wired → non-nil.
	q := NewConsolidationQueue(10)
	svc2 := NewSleepTimeService(&fakeMemoryService{}, nil, q, loggateway.NewNoop())
	ch := svc2.QueueChan()
	if ch == nil {
		t.Fatal("expected non-nil channel when queue wired")
	}

	// Verify the channel is the same as queue.Chan().
	uk := trpcmemory.UserKey{AppName: "a", UserID: "u"}
	q.Enqueue(ConsolidationJobRequest{UserKey: uk, EnqueuedAt: time.Now()})
	select {
	case req := <-ch:
		if req.UserKey.AppName != "a" || req.UserKey.UserID != "u" {
			t.Errorf("unexpected job: %+v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for job from QueueChan")
	}
}

package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// --- Fakes ---

// fakeFactReader implements biz.L3FactReader for testing.
type fakeFactReader struct {
	mu            sync.Mutex
	rowsByUser    map[string][][]byte // userID -> raw JSON rows
	rowsByID      map[string][][]byte // factID -> raw JSON rows
	readErr       error
	getByIDErr    error
	listCallCount int
}

func newFakeFactReader() *fakeFactReader {
	return &fakeFactReader{
		rowsByUser: make(map[string][][]byte),
		rowsByID:   make(map[string][][]byte),
	}
}

func (f *fakeFactReader) ListFactRows(_ context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	return nil, 0, 0, 0, nil
}

func (f *fakeFactReader) ListFactRowsForUser(_ context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCallCount++
	if f.readErr != nil {
		return nil, f.readErr
	}
	rows := f.rowsByUser[userID]
	if limit > 0 && int(limit) < len(rows) {
		rows = rows[:limit]
	}
	out := make([][]byte, len(rows))
	copy(out, rows)
	return out, nil
}

func (f *fakeFactReader) ListFactRowsForUserAll(_ context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	return f.ListFactRowsForUser(nil, scopeType, scopeID, userID, keyword, limit, offset)
}

func (f *fakeFactReader) GetFactRowsByIDs(_ context.Context, factIDs []string) ([][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	var out [][]byte
	for _, id := range factIDs {
		if rows, ok := f.rowsByID[id]; ok {
			out = append(out, rows...)
		}
	}
	return out, nil
}

func (f *fakeFactReader) RecallL3Facts(_ context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	return nil, nil
}

// fakeFactWriter implements biz.L3FactWriter for testing.
type fakeFactWriter struct {
	mu         sync.Mutex
	upserts    []biz.FactUpsert
	upsertErr  error
	upsertByID map[string]biz.FactUpsert
}

func newFakeFactWriter() *fakeFactWriter {
	return &fakeFactWriter{
		upsertByID: make(map[string]biz.FactUpsert),
	}
}

func (f *fakeFactWriter) UpsertFactRow(_ context.Context, in biz.FactUpsert) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	f.upserts = append(f.upserts, in)
	f.upsertByID[in.ID] = in
	// Return a minimal JSON row for the upserted fact.
	row := map[string]any{
		"id":         in.ID,
		"statement":  in.Statement,
		"tags_json":  in.TagsJSON,
		"links":      in.LinksJSON,
		"keywords":   in.KeywordsJSON,
		"updated_at": in.UpdatedAt,
	}
	b, _ := json.Marshal(row)
	return b, nil
}

func (f *fakeFactWriter) DeleteFactRow(_ context.Context, factID string) error {
	return nil
}

func (f *fakeFactWriter) DeleteFactRowsByIDs(_ context.Context, factIDs []string) (int, error) {
	return 0, nil
}

func (f *fakeFactWriter) ClearFactsByScope(_ context.Context, scopeType, scopeID, userID string) ([]string, error) {
	return nil, nil
}

func (f *fakeFactWriter) InvalidateFact(_ context.Context, factID string) ([]byte, error) {
	return nil, nil
}

// InvalidateAndUpsertFactTx simulates the atomic invalidate + upsert operation.
// For testing purposes, it delegates to UpsertFactRow and ignores oldFactID
// (the fake writer does not track valid_until state).
func (f *fakeFactWriter) InvalidateAndUpsertFactTx(ctx context.Context, oldFactID string, in biz.FactUpsert) ([]byte, error) {
	return f.UpsertFactRow(ctx, in)
}

// fakeEvolutionQueue implements EvolutionQueue for testing.
type fakeEvolutionQueue struct {
	mu   sync.Mutex
	jobs []EvolutionJobRequest
	ch   chan EvolutionJobRequest
}

func newFakeEvolutionQueue() *fakeEvolutionQueue {
	return &fakeEvolutionQueue{ch: make(chan EvolutionJobRequest, 10)}
}

func (f *fakeEvolutionQueue) Enqueue(r EvolutionJobRequest) {
	f.mu.Lock()
	f.jobs = append(f.jobs, r)
	f.mu.Unlock()
	select {
	case f.ch <- r:
	default:
	}
}

func (f *fakeEvolutionQueue) Chan() <-chan EvolutionJobRequest {
	return f.ch
}

func (f *fakeEvolutionQueue) jobCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

// fakeTxProvider implements TxProvider for testing. By default it executes fn
// directly (no real transaction). The execHook can be set to customise
// behaviour (e.g. simulate rollback, capture ctx). callCount tracks how many
// times ExecInTx was invoked.
type fakeTxProvider struct {
	mu        sync.Mutex
	callCount int
	execHook  func(ctx context.Context, fn func(ctx context.Context) error) error
}

func newFakeTxProvider() *fakeTxProvider {
	return &fakeTxProvider{
		execHook: func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		},
	}
}

func (f *fakeTxProvider) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	f.mu.Lock()
	f.callCount++
	hook := f.execHook
	f.mu.Unlock()
	if hook == nil {
		return fn(ctx)
	}
	return hook(ctx, fn)
}

func (f *fakeTxProvider) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}

// --- Helpers ---

// buildFactRowJSON builds a raw JSON fact row for testing, matching the
// shape produced by scanFactRowJSON in the data layer.
func buildFactRowJSON(id, statement string, opts ...func(map[string]any)) []byte {
	row := map[string]any{
		"id":            id,
		"scope_type":    "agent",
		"scope_id":      "agent-1",
		"user_id":       "user-1",
		"agent_id":      "agent-1",
		"statement":     statement,
		"fingerprint":   "fp-" + id,
		"fact_kind":     "fact",
		"tags_json":     "[]",
		"importance":    0.5,
		"confidence":    1.0,
		"source_kind":   "trpc_memory",
		"status":        "active",
		"metadata_json": "{}",
		"created_at":    "2026-01-01T00:00:00Z",
		"updated_at":    "2026-01-01T00:00:00Z",
		"valid_from":    "2026-01-01T00:00:00Z",
		"valid_until":   "",
		"links":         "[]",
		"keywords":      "[]",
	}
	for _, opt := range opts {
		opt(row)
	}
	b, _ := json.Marshal(row)
	return b
}

func withLinks(links string) func(map[string]any) {
	return func(m map[string]any) { m["links"] = links }
}

func withKeywords(keywords string) func(map[string]any) {
	return func(m map[string]any) { m["keywords"] = keywords }
}

func makeEvolutionEntry(id, content string) *trpcmemory.Entry {
	return &trpcmemory.Entry{
		ID:      id,
		AppName: "agent-1",
		Memory: &trpcmemory.Memory{
			Memory: content,
		},
		UserID: "user-1",
	}
}

// --- Tests ---

// TestLinkEvolution_EvolveLinks_Success verifies the normal flow: keywords
// are extracted, links are generated, and historical memories are updated
// with backlinks.
func TestLinkEvolution_EvolveLinks_Success(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go programming"),
		buildFactRowJSON("mem-old-2", "User prefers tea over coffee"),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()

	llmResp := buildLLMResponse(`{"keywords":["go","programming"],"links":[{"target_id":"mem-old-1","reason":"both about Go"}]}`)
	llm := &fakeModel{response: llmResp}

	svc := NewLinkEvolutionService(llm, reader, writer, nil, newFakeTxProvider(), loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, newEntry)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(links) != 1 || links[0] != "mem-old-1" {
		t.Fatalf("expected links [mem-old-1], got %v", links)
	}

	// Verify historical memory was updated with backlink.
	writer.mu.Lock()
	defer writer.mu.Unlock()
	upserted, ok := writer.upsertByID["mem-old-1"]
	if !ok {
		t.Fatal("expected mem-old-1 to be updated via factWriter")
	}
	// The backlink to mem-new-1 should be in the links.
	if !containsString(decodeStringArray(upserted.LinksJSON), "mem-new-1") {
		t.Errorf("expected mem-old-1 links to contain mem-new-1, got %q", upserted.LinksJSON)
	}
}

// TestLinkEvolution_EvolveLinks_LLMFailure verifies that LLM failure results
// in graceful degradation (empty links, no error, no mutations).
func TestLinkEvolution_EvolveLinks_LLMFailure(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	llm := &fakeModel{err: errors.New("LLM service unavailable")}

	svc := NewLinkEvolutionService(llm, reader, writer, nil, newFakeTxProvider(), loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, newEntry)
	if err != nil {
		t.Fatalf("expected nil error on LLM failure (graceful degradation), got %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected empty links on LLM failure, got %v", links)
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.upserts) != 0 {
		t.Errorf("expected no memory mutations on LLM failure, got %d upserts", len(writer.upserts))
	}
}

// TestLinkEvolution_EvolveLinks_NilLLM verifies that a nil LLM results in
// graceful degradation (empty links, no error).
func TestLinkEvolution_EvolveLinks_NilLLM(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
	}
	writer := newFakeFactWriter()

	svc := NewLinkEvolutionService(nil, reader, writer, nil, nil, loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, newEntry)
	if err != nil {
		t.Fatalf("expected nil error with nil LLM, got %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected empty links with nil LLM, got %v", links)
	}
}

// TestLinkEvolution_EvolveLinks_NilEntry verifies that a nil newEntry is
// handled safely (red line #26: nil defense).
func TestLinkEvolution_EvolveLinks_NilEntry(t *testing.T) {
	reader := newFakeFactReader()
	writer := newFakeFactWriter()
	llm := &fakeModel{err: errors.New("LLM should not be called")}

	svc := NewLinkEvolutionService(llm, reader, writer, nil, newFakeTxProvider(), loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}

	links, err := svc.EvolveLinks(context.Background(), uk, nil)
	if err != nil {
		t.Fatalf("expected nil error on nil entry, got %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected empty links on nil entry, got %v", links)
	}
	// LLM should not have been called.
	if reader.listCallCount != 0 {
		t.Errorf("expected no factReader calls on nil entry, got %d", reader.listCallCount)
	}
}

// TestLinkEvolution_EvolveLinks_NoHistory verifies that when there are no
// historical memories, the service returns empty links without calling the LLM.
func TestLinkEvolution_EvolveLinks_NoHistory(t *testing.T) {
	reader := newFakeFactReader()
	// No historical memories for user-1.
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	llm := &fakeModel{err: errors.New("LLM should not be called when no history")}

	svc := NewLinkEvolutionService(llm, reader, writer, nil, newFakeTxProvider(), loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, newEntry)
	if err != nil {
		t.Fatalf("expected nil error on no history, got %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected empty links on no history, got %v", links)
	}
}

// TestLinkEvolution_EvolveLinks_MalformedJSON verifies that malformed LLM
// JSON output results in graceful degradation.
func TestLinkEvolution_EvolveLinks_MalformedJSON(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	llm := &fakeModel{response: buildLLMResponse("not valid json")}

	svc := NewLinkEvolutionService(llm, reader, writer, nil, newFakeTxProvider(), loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, newEntry)
	if err != nil {
		t.Fatalf("expected nil error on malformed JSON (graceful degradation), got %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected empty links on malformed JSON, got %v", links)
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.upserts) != 0 {
		t.Errorf("expected no memory mutations on malformed JSON, got %d upserts", len(writer.upserts))
	}
}

// TestLinkEvolution_EnqueueEvolution verifies that EnqueueEvolution enqueues
// a job successfully and the worker processes it.
func TestLinkEvolution_EnqueueEvolution(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	llmResp := buildLLMResponse(`{"keywords":["go"],"links":[{"target_id":"mem-old-1","reason":"related"}]}`)
	llm := &fakeModel{response: llmResp}

	q := newFakeEvolutionQueue()
	svc := NewLinkEvolutionService(llm, reader, writer, q, newFakeTxProvider(), loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	if err := svc.EnqueueEvolution(context.Background(), uk, newEntry); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := q.jobCount(); got != 1 {
		t.Fatalf("expected 1 job enqueued, got %d", got)
	}

	// Verify the job can be consumed from the channel.
	select {
	case req := <-q.Chan():
		if req.UserKey.UserID != "user-1" {
			t.Errorf("unexpected user key: %+v", req.UserKey)
		}
		if req.NewEntry == nil || req.NewEntry.ID != "mem-new-1" {
			t.Errorf("unexpected new entry: %+v", req.NewEntry)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for job")
	}
}

// TestLinkEvolution_EnqueueEvolution_NilQueue verifies that a nil queue
// does not panic.
func TestLinkEvolution_EnqueueEvolution_NilQueue(t *testing.T) {
	svc := NewLinkEvolutionService(nil, nil, nil, nil, nil, loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")
	if err := svc.EnqueueEvolution(context.Background(), uk, newEntry); err != nil {
		t.Fatalf("expected nil error with nil queue, got %v", err)
	}
}

// TestLinkEvolution_ConcurrentAccess verifies that the service is safe for
// concurrent use (run with -race).
func TestLinkEvolution_ConcurrentAccess(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
		buildFactRowJSON("mem-old-2", "User likes Rust"),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	llmResp := buildLLMResponse(`{"keywords":["go"],"links":[{"target_id":"mem-old-1","reason":"related"}]}`)
	llm := &fakeModel{response: llmResp}

	q := newFakeEvolutionQueue()
	svc := NewLinkEvolutionService(llm, reader, writer, q, newFakeTxProvider(), loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	var wg sync.WaitGroup
	const goroutines = 10
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Concurrent EnqueueEvolution
			_ = svc.EnqueueEvolution(context.Background(), uk, newEntry)
			// Concurrent EvolveLinks
			_, _ = svc.EvolveLinks(context.Background(), uk, newEntry)
		}()
	}
	wg.Wait()

	// Verify no panic / race condition. The factWriter should have received
	// upserts from the concurrent EvolveLinks calls.
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.upserts) == 0 {
		t.Error("expected at least one upsert from concurrent EvolveLinks calls")
	}
}

// TestLinkEvolution_EvolveLinks_PreservesExistingLinks verifies that when
// updating a historical memory's links, existing links are preserved.
func TestLinkEvolution_EvolveLinks_PreservesExistingLinks(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go",
			withLinks(`["mem-other"]`),
			withKeywords(`["programming"]`)),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	llmResp := buildLLMResponse(`{"keywords":["go","learning"],"links":[{"target_id":"mem-old-1","reason":"both about Go"}]}`)
	llm := &fakeModel{response: llmResp}

	svc := NewLinkEvolutionService(llm, reader, writer, nil, newFakeTxProvider(), loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, newEntry)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	upserted, ok := writer.upsertByID["mem-old-1"]
	if !ok {
		t.Fatal("expected mem-old-1 to be updated")
	}
	// Existing link "mem-other" should be preserved, and "mem-new-1" added.
	linksArr := decodeStringArray(upserted.LinksJSON)
	if !containsString(linksArr, "mem-other") {
		t.Errorf("expected existing link mem-other to be preserved, got %v", linksArr)
	}
	if !containsString(linksArr, "mem-new-1") {
		t.Errorf("expected new link mem-new-1 to be added, got %v", linksArr)
	}
	// Existing keyword "programming" should be preserved, and new keywords merged.
	keywordsArr := decodeStringArray(upserted.KeywordsJSON)
	if !containsString(keywordsArr, "programming") {
		t.Errorf("expected existing keyword programming to be preserved, got %v", keywordsArr)
	}
}

// TestLinkEvolution_EvolveLinks_TxWrapped verifies that when a TxProvider is
// wired, EvolveLinks wraps the backlink batch in a single transaction.
func TestLinkEvolution_EvolveLinks_TxWrapped(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	llmResp := buildLLMResponse(`{"keywords":["go"],"links":[{"target_id":"mem-old-1","reason":"related"}]}`)
	llm := &fakeModel{response: llmResp}

	tx := newFakeTxProvider()
	svc := NewLinkEvolutionService(llm, reader, writer, nil, tx, loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, newEntry)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(links) != 1 || links[0] != "mem-old-1" {
		t.Fatalf("expected links [mem-old-1], got %v", links)
	}
	if got := tx.calls(); got != 1 {
		t.Errorf("expected tx.ExecInTx to be called once, got %d", got)
	}
}

// TestLinkEvolution_EvolveLinks_TxRollbackOnFailure verifies that when a
// backlink update fails inside the transaction, EvolveLinks returns empty
// links (best-effort) and the transaction is rolled back (simulated by
// clearing the fake writer's upserts).
func TestLinkEvolution_EvolveLinks_TxRollbackOnFailure(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
		buildFactRowJSON("mem-old-2", "User likes Rust"),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	writer.upsertErr = errors.New("simulated DB failure")
	llmResp := buildLLMResponse(`{"keywords":["go"],"links":[{"target_id":"mem-old-1","reason":"related"},{"target_id":"mem-old-2","reason":"related"}]}`)
	llm := &fakeModel{response: llmResp}

	tx := newFakeTxProvider()
	tx.execHook = func(ctx context.Context, fn func(ctx context.Context) error) error {
		err := fn(ctx)
		if err != nil {
			// Simulate rollback: clear any upserts applied before the failure.
			writer.mu.Lock()
			writer.upserts = nil
			writer.upsertByID = make(map[string]biz.FactUpsert)
			writer.mu.Unlock()
		}
		return err
	}
	svc := NewLinkEvolutionService(llm, reader, writer, nil, tx, loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, newEntry)
	if err != nil {
		t.Fatalf("expected nil error (best-effort), got %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected empty links on tx rollback, got %v", links)
	}
	if got := tx.calls(); got != 1 {
		t.Errorf("expected tx.ExecInTx to be called once, got %d", got)
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.upserts) != 0 {
		t.Errorf("expected no upserts after rollback, got %d", len(writer.upserts))
	}
}

// TestLinkEvolution_EvolveLinks_NilTx verifies that when tx is nil, EvolveLinks
// falls back to non-atomic best-effort updates (no panic).
func TestLinkEvolution_EvolveLinks_NilTx(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	llmResp := buildLLMResponse(`{"keywords":["go"],"links":[{"target_id":"mem-old-1","reason":"related"}]}`)
	llm := &fakeModel{response: llmResp}

	svc := NewLinkEvolutionService(llm, reader, writer, nil, nil, loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	newEntry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, newEntry)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link with nil tx, got %d", len(links))
	}
}

// TestEvolutionQueue_EnqueueAndChan verifies the in-memory queue implementation.
func TestEvolutionQueue_EnqueueAndChan(t *testing.T) {
	q := NewEvolutionQueue(10)
	ch := q.Chan()
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	uk := trpcmemory.UserKey{AppName: "a", UserID: "u"}
	entry := makeEvolutionEntry("mem-1", "test")
	q.Enqueue(EvolutionJobRequest{UserKey: uk, NewEntry: entry, EnqueuedAt: time.Now()})
	select {
	case req := <-ch:
		if req.UserKey.AppName != "a" || req.UserKey.UserID != "u" {
			t.Errorf("unexpected job: %+v", req.UserKey)
		}
		if req.NewEntry == nil || req.NewEntry.ID != "mem-1" {
			t.Errorf("unexpected new entry: %+v", req.NewEntry)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for job")
	}
}

// TestLinkEvolutionResult_Unmarshal verifies the JSON structure of
// linkAnalysisResult.
func TestLinkEvolutionResult_Unmarshal(t *testing.T) {
	raw := `{"keywords":["go","programming"],"links":[{"target_id":"mem-1","reason":"related"}]}`
	var result linkAnalysisResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Keywords) != 2 {
		t.Fatalf("expected 2 keywords, got %d", len(result.Keywords))
	}
	if len(result.Links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(result.Links))
	}
	if result.Links[0].TargetID != "mem-1" {
		t.Errorf("unexpected target_id: %q", result.Links[0].TargetID)
	}
}

// containsString checks if a string slice contains a specific value.
func containsString(arr []string, target string) bool {
	for _, s := range arr {
		if s == target {
			return true
		}
	}
	return false
}

// --- Phase 6A-03 T8/T9 tests ---

// TestLinkEvolution_Throttle verifies that when throttleInterval is set,
// consecutive EvolveLinks calls for the same agent within the interval
// are skipped (Phase 6A-03 T8).
func TestLinkEvolution_Throttle(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
	}
	reader.rowsByID["mem-new-1"] = [][]byte{
		buildFactRowJSON("mem-new-1", "User is learning Go"),
	}
	writer := newFakeFactWriter()
	llm := &fakeModel{response: buildLLMResponse(`{"keywords":["go"],"links":[{"target_id":"mem-old-1","reason":"related"}]}`)}
	svc := NewLinkEvolutionService(llm, reader, writer, nil, newFakeTxProvider(), loggateway.NewNoop())
	svc.SetThrottleInterval(10 * time.Second) // 10s throttle

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	entry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	// First call: should succeed (no prior call).
	links1, err := svc.EvolveLinks(context.Background(), uk, entry)
	if err != nil {
		t.Fatalf("first call: expected nil error, got %v", err)
	}
	if len(links1) != 1 {
		t.Fatalf("first call: expected 1 link, got %d", len(links1))
	}

	// Second call within throttle window: should be skipped.
	links2, err := svc.EvolveLinks(context.Background(), uk, entry)
	if err != nil {
		t.Fatalf("second call: expected nil error, got %v", err)
	}
	if len(links2) != 0 {
		t.Fatalf("second call: expected 0 links (throttled), got %d", len(links2))
	}

	// Different agent: should NOT be throttled (per-agent throttle).
	uk2 := trpcmemory.UserKey{AppName: "agent-2", UserID: "user-1"}
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
	}
	links3, err := svc.EvolveLinks(context.Background(), uk2, entry)
	if err != nil {
		t.Fatalf("third call (different agent): expected nil error, got %v", err)
	}
	if len(links3) != 1 {
		t.Fatalf("third call (different agent): expected 1 link, got %d", len(links3))
	}
}

// TestLinkEvolution_Disabled verifies that SetLinkEvolutionEnabled(false)
// makes EvolveLinks a no-op (Phase 6A-03 T9).
func TestLinkEvolution_Disabled(t *testing.T) {
	reader := newFakeFactReader()
	reader.rowsByUser["user-1"] = [][]byte{
		buildFactRowJSON("mem-old-1", "User likes Go"),
	}
	writer := newFakeFactWriter()
	// LLM would error if called — verifies it's NOT called when disabled.
	llm := &fakeModel{err: errors.New("LLM should not be called when disabled")}
	svc := NewLinkEvolutionService(llm, reader, writer, nil, newFakeTxProvider(), loggateway.NewNoop())
	svc.SetLinkEvolutionEnabled(false)

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	entry := makeEvolutionEntry("mem-new-1", "User is learning Go")

	links, err := svc.EvolveLinks(context.Background(), uk, entry)
	if err != nil {
		t.Fatalf("expected nil error when disabled, got %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected 0 links when disabled, got %d", len(links))
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.upserts) != 0 {
		t.Errorf("expected 0 upserts when disabled, got %d", len(writer.upserts))
	}
}

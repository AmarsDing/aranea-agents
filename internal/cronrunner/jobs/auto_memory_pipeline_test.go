package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	sessionsess "aranea-agents/internal/biz/session"
	"aranea-agents/internal/conf"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/pkg/loggateway"
)

// --- fakes for the unified write pipeline (P1-3) ---

type fakePipelineFactWriter struct {
	upserts       []biz.FactUpsert
	txOldIDs      []string
	txUpserts     []biz.FactUpsert
	invalidations []string
}

func (f *fakePipelineFactWriter) UpsertFactRow(_ context.Context, in biz.FactUpsert) ([]byte, error) {
	f.upserts = append(f.upserts, in)
	b, _ := json.Marshal(map[string]any{"id": in.ID, "statement": in.Statement, "agent_id": in.AgentID, "user_id": in.UserID})
	return b, nil
}
func (f *fakePipelineFactWriter) DeleteFactRow(_ context.Context, _ string) error { return nil }
func (f *fakePipelineFactWriter) DeleteFactRowsByIDs(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (f *fakePipelineFactWriter) ClearFactsByScope(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakePipelineFactWriter) InvalidateFact(_ context.Context, factID string) ([]byte, error) {
	f.invalidations = append(f.invalidations, factID)
	return nil, nil
}
func (f *fakePipelineFactWriter) InvalidateAndUpsertFactTx(_ context.Context, oldFactID string, in biz.FactUpsert) ([]byte, error) {
	f.txOldIDs = append(f.txOldIDs, oldFactID)
	f.txUpserts = append(f.txUpserts, in)
	b, _ := json.Marshal(map[string]any{"id": in.ID, "statement": in.Statement, "agent_id": in.AgentID, "user_id": in.UserID})
	return b, nil
}

type fakePipelineAccessCounter struct {
	ids []string
}

func (f *fakePipelineAccessCounter) IncrementFactRecalledCount(_ context.Context, ids []string) error {
	f.ids = append(f.ids, ids...)
	return nil
}

type fakePipelineNeighborSearcher struct {
	neighbors []biz.MemoryConflictNeighbor
}

func (f *fakePipelineNeighborSearcher) SearchFactNeighbors(_ context.Context, _, _ string, _ []float32, _ int, _ float64) ([]biz.MemoryConflictNeighbor, error) {
	return f.neighbors, nil
}

type fakePipelineEmbedder struct{}

func (f *fakePipelineEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

type fakePipelineRowReader struct {
	rows [][]byte
}

func (f *fakePipelineRowReader) GetFactRowsByIDs(_ context.Context, _ []string) ([][]byte, error) {
	return f.rows, nil
}

// stubConsolidator emits fixed proposals regardless of input.
type stubConsolidator struct {
	proposals []biz.MemoryProposal
}

func (s *stubConsolidator) Extract(_ context.Context, _ biz.ConsolidateInput) ([]biz.MemoryProposal, error) {
	return s.proposals, nil
}

func newPipelineTestWorker(t *testing.T, writer biz.MemoryConsolidationWriter, pipeline *biz.FactWritePipeline, consolidator biz.MemoryConsolidator) *AutoMemoryWorker {
	t.Helper()
	const (
		sessID  = "sess-pipe-1"
		agentID = "agent-pipe-1"
		userID  = "user-pipe-1"
	)
	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: sessID, AgentID: agentID, UserID: userID},
		msgs: []sessionsess.ChatMessage{{
			ID: "m-pipe-1", SessionID: sessID, Role: "user", ContentMarkdown: "I prefer dark mode",
		}},
	}
	sessionsUC := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop(), nil)
	agentsUC := newMemoryEnabledAgentsUC(agentID)
	q := memtrpc.NewMemoryJobQueue(&conf.Runtime{}, 4, 0, loggateway.NewNoop())
	w, err := NewAutoMemoryWorker(AutoMemoryWorkerConfig{
		RuntimeConf:  &conf.Runtime{},
		Sessions:     sessionsUC,
		Agents:       agentsUC,
		Writer:       writer,
		Consolidator: consolidator,
		Queue:        q,
		FactPipeline: pipeline,
		Logger:       loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewAutoMemoryWorker: %v", err)
	}
	return w
}

func pipelineTestRequest() memtrpc.AutoMemoryJobRequest {
	return memtrpc.AutoMemoryJobRequest{SessionID: "sess-pipe-1", UserID: "user-pipe-1", AppName: "agent-pipe-1"}
}

func TestAutoMemoryWorker_ExtractPipelinesFacts(t *testing.T) {
	writer := &fakeConsolidationWriter{}
	factWriter := &fakePipelineFactWriter{}
	pipeline := biz.NewFactWritePipeline(biz.FactWritePipelineDeps{
		Writer: factWriter,
		LG:     loggateway.NewNoop(),
	})
	w := newPipelineTestWorker(t, writer, pipeline, biz.NewHeuristicConsolidator())

	if err := w.extract(context.Background(), pipelineTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(factWriter.upserts) != 1 {
		t.Fatalf("expected 1 pipeline upsert, got %d", len(factWriter.upserts))
	}
	up := factWriter.upserts[0]
	if up.SourceKind != "auto_memory" {
		t.Fatalf("source_kind=%q want auto_memory", up.SourceKind)
	}
	if up.SourceMessageID != "m-pipe-1" {
		t.Fatalf("source_message_id=%q want m-pipe-1", up.SourceMessageID)
	}
	if up.FactKind != "preference" {
		t.Fatalf("fact_kind=%q want preference", up.FactKind)
	}
	ep := writer.getEpisode()
	if ep == nil {
		t.Fatal("expected episode, got nil")
	}
	if ep.ConsolidatedL3 != 1 {
		t.Fatalf("consolidated_l3=%d want 1", ep.ConsolidatedL3)
	}
}

func TestAutoMemoryWorker_ExtractMergeSkipsInsertAndEpisode(t *testing.T) {
	writer := &fakeConsolidationWriter{}
	factWriter := &fakePipelineFactWriter{}
	access := &fakePipelineAccessCounter{}
	neighborRow, _ := json.Marshal(map[string]any{"id": "old-1", "fact_kind": "preference", "statement": "dark mode preference"})
	pipeline := biz.NewFactWritePipeline(biz.FactWritePipelineDeps{
		Searcher: &fakePipelineNeighborSearcher{neighbors: []biz.MemoryConflictNeighbor{
			{FactID: "old-1", Score: 0.95, FactKind: "preference"},
		}},
		Embedder: &fakePipelineEmbedder{},
		Reader:   &fakePipelineRowReader{rows: [][]byte{neighborRow}},
		Writer:   factWriter,
		Access:   access,
		LG:       loggateway.NewNoop(),
	})
	w := newPipelineTestWorker(t, writer, pipeline, biz.NewHeuristicConsolidator())

	if err := w.extract(context.Background(), pipelineTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(factWriter.upserts) != 0 {
		t.Fatalf("merge must not insert, got %d upserts", len(factWriter.upserts))
	}
	if len(access.ids) != 1 || access.ids[0] != "old-1" {
		t.Fatalf("access bump = %v, want [old-1]", access.ids)
	}
	if ep := writer.getEpisode(); ep != nil {
		t.Fatalf("merged-only run must not write an episode, got %+v", ep)
	}
}

func TestAutoMemoryWorker_ExtractGateDropsNonWhitelistedKind(t *testing.T) {
	writer := &fakeConsolidationWriter{}
	factWriter := &fakePipelineFactWriter{}
	pipeline := biz.NewFactWritePipeline(biz.FactWritePipelineDeps{
		Writer: factWriter,
		LG:     loggateway.NewNoop(),
	})
	consolidator := &stubConsolidator{proposals: []biz.MemoryProposal{{
		Statement:   "用户昨天点了一次外卖",
		SubjectType: "event",
		Scope:       "user",
		Confidence:  0.9,
	}}}
	w := newPipelineTestWorker(t, writer, pipeline, consolidator)

	if err := w.extract(context.Background(), pipelineTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(factWriter.upserts) != 0 {
		t.Fatalf("event kind must be dropped by the whitelist gate, got %d upserts", len(factWriter.upserts))
	}
	if ep := writer.getEpisode(); ep != nil {
		t.Fatalf("all-dropped run must not write an episode, got %+v", ep)
	}
}

func TestAutoMemoryWorker_ExtractWithoutPipelineSkipsFacts(t *testing.T) {
	writer := &fakeConsolidationWriter{}
	w := newPipelineTestWorker(t, writer, nil, biz.NewHeuristicConsolidator())
	if err := w.extract(context.Background(), pipelineTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(writer.getFacts()) != 0 {
		t.Fatalf("no fact writes expected without pipeline, got %d", len(writer.getFacts()))
	}
}

func TestAutoMemoryWorker_FeedbackPipelinesFacts(t *testing.T) {
	writer := &fakeConsolidationWriter{}
	factWriter := &fakePipelineFactWriter{}
	pipeline := biz.NewFactWritePipeline(biz.FactWritePipelineDeps{
		Writer: factWriter,
		LG:     loggateway.NewNoop(),
	})
	w := newPipelineTestWorker(t, writer, pipeline, biz.NewHeuristicConsolidator())

	req := pipelineTestRequest()
	req.FeedbackMessageID = "m-pipe-1"
	req.FeedbackRating = "negative"
	if err := w.extract(context.Background(), req); err != nil {
		t.Fatalf("extract feedback: %v", err)
	}
	if len(factWriter.upserts) != 1 {
		t.Fatalf("expected 1 feedback upsert via pipeline, got %d", len(factWriter.upserts))
	}
	if factWriter.upserts[0].FactKind != "preference" {
		t.Fatalf("feedback fact_kind=%q want preference", factWriter.upserts[0].FactKind)
	}
}

type capturingWriteBack struct {
	calls int
	got   biz.KnowledgeWriteBackInput
	err   error
}

func (c *capturingWriteBack) WriteBackSessionFacts(_ context.Context, in biz.KnowledgeWriteBackInput) (biz.KnowledgeWriteBackResult, error) {
	c.calls++
	c.got = in
	return biz.KnowledgeWriteBackResult{Appended: len(in.Facts)}, c.err
}

func TestAutoMemoryWorker_WriteBackAfterFacts(t *testing.T) {
	writer := &fakeConsolidationWriter{}
	factWriter := &fakePipelineFactWriter{}
	pipeline := biz.NewFactWritePipeline(biz.FactWritePipelineDeps{
		Writer: factWriter,
		LG:     loggateway.NewNoop(),
	})
	w := newPipelineTestWorker(t, writer, pipeline, &stubConsolidator{proposals: []biz.MemoryProposal{{
		Layer:       biz.MemoryLayerL3,
		Statement:   "用户偏好深色模式界面",
		SubjectType: "preference",
		Scope:       "user",
		Confidence:  0.93,
	}}})
	cap := &capturingWriteBack{}
	w.writeBack = cap
	if err := w.extract(context.Background(), pipelineTestRequest()); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if cap.calls != 1 {
		t.Fatalf("writeback calls=%d want 1", cap.calls)
	}
	if cap.got.SessionID != "sess-pipe-1" || len(cap.got.Facts) == 0 {
		t.Fatalf("writeback input = %+v", cap.got)
	}
	if cap.got.Facts[0].Statement != "用户偏好深色模式界面" {
		t.Fatalf("statement = %q", cap.got.Facts[0].Statement)
	}
}

func TestAutoMemoryWorker_WriteBackErrorDoesNotFailExtract(t *testing.T) {
	writer := &fakeConsolidationWriter{}
	factWriter := &fakePipelineFactWriter{}
	pipeline := biz.NewFactWritePipeline(biz.FactWritePipelineDeps{
		Writer: factWriter,
		LG:     loggateway.NewNoop(),
	})
	w := newPipelineTestWorker(t, writer, pipeline, &stubConsolidator{proposals: []biz.MemoryProposal{{
		Layer:       biz.MemoryLayerL3,
		Statement:   "用户偏好深色模式界面",
		SubjectType: "preference",
		Scope:       "user",
		Confidence:  0.93,
	}}})
	w.writeBack = &capturingWriteBack{err: errors.New("kb down")}
	if err := w.extract(context.Background(), pipelineTestRequest()); err != nil {
		t.Fatalf("extract should ignore writeback error, got %v", err)
	}
}

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	sessionsess "aranea-agents/internal/biz/session"
	"aranea-agents/internal/conf"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/pkg/loggateway"
)

type fakeConflictDetector struct {
	decision biz.MemoryConflictDecision
	calls    int
}

func (f *fakeConflictDetector) DetectConflict(_ context.Context, _, _, _, _ string) (biz.MemoryConflictDecision, error) {
	f.calls++
	return f.decision, nil
}

type fakeConflictStore struct {
	superseded  [][2]string
	batchMarked []string
}

func (f *fakeConflictStore) IncrementConflictCount(_ context.Context, _ string) (int32, error) {
	return 1, nil
}
func (f *fakeConflictStore) ListConflictingFacts(_ context.Context, _, _ string, _, _ int32) ([][]byte, int32, error) {
	return nil, 0, nil
}
func (f *fakeConflictStore) BatchIncrementConflictCounts(_ context.Context, ids []string) error {
	f.batchMarked = append(f.batchMarked, ids...)
	return nil
}
func (f *fakeConflictStore) SupersedeFact(_ context.Context, oldID, newID string) error {
	f.superseded = append(f.superseded, [2]string{oldID, newID})
	return nil
}

// idConsolidationWriter returns FactRows with deterministic ids so the worker
// can resolve supersede targets post-write.
type idConsolidationWriter struct {
	facts []biz.MemoryFactWrite
}

func (w *idConsolidationWriter) UpsertFactsAndEpisodeBatch(_ context.Context, facts []biz.MemoryFactWrite, _ *biz.EpisodeWrite) (*biz.ConsolidationResult, error) {
	w.facts = append(w.facts, facts...)
	rows := make([][]byte, len(facts))
	for i := range facts {
		b, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("new-%d", i)})
		rows[i] = b
	}
	return &biz.ConsolidationResult{FactRows: rows, FactsWritten: len(facts)}, nil
}

func newConflictTestWorker(t *testing.T, writer biz.MemoryConsolidationWriter, det biz.MemoryConflictDetector, store biz.L3ConflictStore) *AutoMemoryWorker {
	t.Helper()
	const (
		sessID  = "sess-conf-1"
		agentID = "agent-conf-1"
		userID  = "user-conf-1"
	)
	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: sessID, AgentID: agentID, UserID: userID},
		msgs: []sessionsess.ChatMessage{{
			ID: "m-conf-1", SessionID: sessID, Role: "user", ContentMarkdown: "I prefer dark mode",
		}},
	}
	sessionsUC := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop(), nil)
	agentsUC := newMemoryEnabledAgentsUC(agentID)
	q := memtrpc.NewMemoryJobQueue(&conf.Runtime{}, 4, 0, loggateway.NewNoop())
	w, err := NewAutoMemoryWorker(AutoMemoryWorkerConfig{
		RuntimeConf:      &conf.Runtime{},
		Sessions:         sessionsUC,
		Agents:           agentsUC,
		Writer:           writer,
		Consolidator:     biz.NewHeuristicConsolidator(),
		Queue:            q,
		ConflictDetector: det,
		ConflictStore:    store,
		Logger:           loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewAutoMemoryWorker: %v", err)
	}
	return w
}

func TestAutoMemoryWorker_ExtractSupersedesConflict(t *testing.T) {
	writer := &idConsolidationWriter{}
	det := &fakeConflictDetector{decision: biz.MemoryConflictDecision{
		Action: biz.ConflictActionSupersede, TargetFactID: "old-1", Score: 0.95,
	}}
	store := &fakeConflictStore{}
	w := newConflictTestWorker(t, writer, det, store)

	req := memtrpc.AutoMemoryJobRequest{SessionID: "sess-conf-1", UserID: "user-conf-1", AppName: "agent-conf-1"}
	if err := w.extract(context.Background(), req); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if det.calls == 0 {
		t.Fatal("detector should be consulted for governable proposals")
	}
	if len(store.superseded) != 1 {
		t.Fatalf("expected 1 supersede call, got %v", store.superseded)
	}
	if store.superseded[0][0] != "old-1" || store.superseded[0][1] != "new-0" {
		t.Fatalf("supersede targets = %v, want [old-1 new-0]", store.superseded[0])
	}
	if len(store.batchMarked) != 0 {
		t.Fatalf("no conflict marks expected, got %v", store.batchMarked)
	}
	if strings.Contains(writer.facts[0].MetadataJSON, "conflict_candidates") {
		t.Fatalf("supersede path must not record conflict_candidates: %s", writer.facts[0].MetadataJSON)
	}
}

func TestAutoMemoryWorker_ExtractMarksConflictCandidate(t *testing.T) {
	writer := &idConsolidationWriter{}
	det := &fakeConflictDetector{decision: biz.MemoryConflictDecision{
		Action: biz.ConflictActionMarkConflict, TargetFactID: "old-2", Score: 0.85,
	}}
	store := &fakeConflictStore{}
	w := newConflictTestWorker(t, writer, det, store)

	req := memtrpc.AutoMemoryJobRequest{SessionID: "sess-conf-1", UserID: "user-conf-1", AppName: "agent-conf-1"}
	if err := w.extract(context.Background(), req); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(store.batchMarked) != 1 || store.batchMarked[0] != "old-2" {
		t.Fatalf("batchMarked = %v, want [old-2]", store.batchMarked)
	}
	if len(store.superseded) != 0 {
		t.Fatalf("no supersede expected, got %v", store.superseded)
	}
	meta := writer.facts[0].MetadataJSON
	if !strings.Contains(meta, "conflict_candidates") || !strings.Contains(meta, "old-2") {
		t.Fatalf("metadata should record conflict_candidates with old-2: %s", meta)
	}
}

func TestAutoMemoryWorker_ExtractWithoutConflictDepsStillWrites(t *testing.T) {
	writer := &idConsolidationWriter{}
	w := newConflictTestWorker(t, writer, nil, nil)
	req := memtrpc.AutoMemoryJobRequest{SessionID: "sess-conf-1", UserID: "user-conf-1", AppName: "agent-conf-1"}
	if err := w.extract(context.Background(), req); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(writer.facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(writer.facts))
	}
}

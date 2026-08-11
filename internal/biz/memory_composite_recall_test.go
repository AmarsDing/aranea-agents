package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func TestFormatCompositeRecallLine(t *testing.T) {
	t.Parallel()
	if got := formatCompositeRecallLine(CompositeRecallStoreRow{
		Layer: "L2", Title: "Dark mode", Summary: "User prefers dark theme",
	}); got != "Dark mode: User prefers dark theme" {
		t.Fatalf("L2 line = %q", got)
	}
	if got := formatCompositeRecallLine(CompositeRecallStoreRow{
		Layer: "L3", Statement: "User name is Alice",
	}); got != "User name is Alice" {
		t.Fatalf("L3 line = %q", got)
	}
}

// ── P2-R1: layered composition path ─────────────────────────────────────

type l2RecallerStub struct {
	rows [][]byte
	got  L2RecallQuery
}

func (s *l2RecallerStub) RecallEpisodes(_ context.Context, q L2RecallQuery) ([][]byte, error) {
	s.got = q
	return s.rows, nil
}

type l3RecallerStub struct {
	rows [][]byte
	got  L3FusedRecallQuery
}

func (s *l3RecallerStub) RecallFacts(_ context.Context, _ L3RecallQuery) ([][]byte, error) {
	return nil, nil
}

func (s *l3RecallerStub) RecallFactsFused(_ context.Context, q L3FusedRecallQuery) ([][]byte, error) {
	s.got = q
	return s.rows, nil
}

func factJSON(id, stmt string, importance, total float64) []byte {
	b, _ := json.Marshal(map[string]any{
		"id": id, "statement": stmt, "importance": importance,
		"confidence": 0.9, "version": 3, "source_session_id": "sess-src",
		"scores": map[string]any{"total": total},
	})
	return b
}

func episodeJSON(title, summary string, importance, total float64) []byte {
	m := map[string]any{"title": title, "outcome_summary": summary, "importance": importance}
	if total > 0 {
		m["scores"] = map[string]any{"total": total}
	}
	b, _ := json.Marshal(m)
	return b
}

// The layered path must rank by the calibrated scores.total (not raw
// importance), propagate L3 provenance, and pass query/session through.
func TestRecallComposite_LayeredRanksByCalibratedScore(t *testing.T) {
	l2 := &l2RecallerStub{rows: [][]byte{
		episodeJSON("Ep low", "summary", 0.95, 0.20), // high importance, low calibrated total
	}}
	l3 := &l3RecallerStub{rows: [][]byte{
		factJSON("f-low", "fact low total", 0.99, 0.10),
		factJSON("f-high", "fact high total", 0.30, 0.90),
	}}
	uc := NewMemoryCompositeRecallUsecase(&compositeStoreStub{})
	uc.SetLayerRecallers(l2, l3)
	hits, err := uc.RecallComposite(context.Background(), CompositeRecallQuery{
		AgentID: "ag1", SessionID: "s1", UserID: "u1", Query: "q", Limit: 10,
	})
	if err != nil {
		t.Fatalf("RecallComposite: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("hits=%d, want 3: %+v", len(hits), hits)
	}
	if hits[0].FactID != "f-high" || hits[0].Layer != "L3" {
		t.Fatalf("hits[0]=%+v, want f-high L3 first (calibrated total 0.90)", hits[0])
	}
	if hits[0].Score != 0.90 {
		t.Fatalf("hits[0].Score=%v, want 0.90", hits[0].Score)
	}
	// provenance propagated
	if hits[0].SourceSession != "sess-src" || hits[0].Confidence != 0.9 || hits[0].Version != 3 {
		t.Fatalf("provenance lost: %+v", hits[0])
	}
	// L2 episode ranked by its calibrated total (0.20), above f-low (0.10)
	if hits[1].Layer != "L2" || hits[1].Score != 0.20 {
		t.Fatalf("hits[1]=%+v, want L2 score 0.20", hits[1])
	}
	// query/session forwarded to both recallers
	if l2.got.SessionID != "s1" || l2.got.Query != "q" || l2.got.AgentID != "ag1" {
		t.Fatalf("l2 query forward: %+v", l2.got)
	}
	if l3.got.Runtime.AgentID != "ag1" || l3.got.Runtime.UserID != "u1" || l3.got.Query != "q" {
		t.Fatalf("l3 query forward: %+v", l3.got)
	}
}

// L2 rows without annotated scores fall back to importance; the limit caps
// the merged set after score-descended sort.
func TestRecallComposite_LayeredFallbackAndLimit(t *testing.T) {
	l2 := &l2RecallerStub{rows: [][]byte{
		episodeJSON("Ep no-score", "sum", 0.50, 0), // no scores key → importance 0.50
	}}
	l3 := &l3RecallerStub{rows: [][]byte{
		factJSON("f1", "fact one", 0.1, 0.80),
		factJSON("f2", "fact two", 0.1, 0.70),
	}}
	uc := NewMemoryCompositeRecallUsecase(&compositeStoreStub{})
	uc.SetLayerRecallers(l2, l3)
	hits, err := uc.RecallComposite(context.Background(), CompositeRecallQuery{
		AgentID: "ag1", Query: "q", Limit: 2,
	})
	if err != nil {
		t.Fatalf("RecallComposite: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits=%d, want 2 (limit)", len(hits))
	}
	if hits[0].FactID != "f1" || hits[1].FactID != "f2" {
		t.Fatalf("order: %+v", hits)
	}
}

// Without layer recallers the legacy store path stays in effect.
func TestRecallComposite_LegacyStorePathPreserved(t *testing.T) {
	store := &compositeStoreStub{rows: []CompositeRecallStoreRow{
		{Layer: "L3", Statement: "legacy fact", Score: 0.7, FactID: "lf1"},
	}}
	uc := NewMemoryCompositeRecallUsecase(store)
	hits, err := uc.RecallComposite(context.Background(), CompositeRecallQuery{AgentID: "ag1", Query: "q", Limit: 5})
	if err != nil {
		t.Fatalf("RecallComposite: %v", err)
	}
	if len(hits) != 1 || hits[0].Line != "legacy fact" || hits[0].Score != 0.7 {
		t.Fatalf("legacy hits: %+v", hits)
	}
}

type compositeStoreStub struct {
	rows []CompositeRecallStoreRow
}

func (s *compositeStoreStub) CompositeSearchMemories(_ context.Context, _, _, _, _ string, _ int32) ([]CompositeRecallStoreRow, error) {
	return s.rows, nil
}

// ── P0-C: single embedding per turn across L2/L3 (2026-08-11) ───────────
//
// The layered composite path previously embedded the same query twice per
// turn (once inside L2, once inside L3 fused), each a network round-trip on
// the LLM critical path. The composite usecase now embeds once and passes
// the vector down via QueryEmbedding + EmbedAttempted.

type embedderStub struct {
	calls         int
	vec           []float32
	err           error
	deadlineDelta time.Duration
}

func (e *embedderStub) Embed(ctx context.Context, _ string) ([]float32, error) {
	e.calls++
	if d, ok := ctx.Deadline(); ok {
		e.deadlineDelta = time.Until(d)
	}
	if e.err != nil {
		return nil, e.err
	}
	return e.vec, nil
}

func TestRecallComposite_LayeredSharesSingleEmbedding(t *testing.T) {
	l2 := &l2RecallerStub{}
	l3 := &l3RecallerStub{}
	emb := &embedderStub{vec: []float32{0.1, 0.2, 0.3}}
	uc := NewMemoryCompositeRecallUsecase(&compositeStoreStub{})
	uc.SetLayerRecallers(l2, l3)
	uc.SetEmbedder(emb, loggateway.NewNoop())
	_, err := uc.RecallComposite(context.Background(), CompositeRecallQuery{
		AgentID: "ag1", Query: "q", Limit: 5,
	})
	if err != nil {
		t.Fatalf("RecallComposite: %v", err)
	}
	if emb.calls != 1 {
		t.Fatalf("embed calls=%d, want exactly 1 per turn", emb.calls)
	}
	if !l2.got.EmbedAttempted || !l3.got.EmbedAttempted {
		t.Fatalf("EmbedAttempted not propagated: l2=%+v l3=%+v", l2.got, l3.got)
	}
	if len(l2.got.QueryEmbedding) != 3 || len(l3.got.QueryEmbedding) != 3 {
		t.Fatalf("shared embedding not propagated: l2=%v l3=%v", l2.got.QueryEmbedding, l3.got.QueryEmbedding)
	}
	// Shared embed must carry a bounded deadline (LLM critical path).
	if emb.deadlineDelta <= 0 || emb.deadlineDelta > 3*time.Second+500*time.Millisecond {
		t.Fatalf("shared embed ctx deadline=%v, want ~3s timeout", emb.deadlineDelta)
	}
}

func TestRecallComposite_LayeredEmbedFailureSkipsLayerReembed(t *testing.T) {
	l2 := &l2RecallerStub{}
	l3 := &l3RecallerStub{}
	emb := &embedderStub{err: errors.New("embed down")}
	uc := NewMemoryCompositeRecallUsecase(&compositeStoreStub{})
	uc.SetLayerRecallers(l2, l3)
	uc.SetEmbedder(emb, loggateway.NewNoop())
	_, err := uc.RecallComposite(context.Background(), CompositeRecallQuery{
		AgentID: "ag1", Query: "q", Limit: 5,
	})
	if err != nil {
		t.Fatalf("RecallComposite: %v", err)
	}
	if emb.calls != 1 {
		t.Fatalf("embed calls=%d, want 1 (failure must not fan out to layers)", emb.calls)
	}
	if !l2.got.EmbedAttempted || !l3.got.EmbedAttempted {
		t.Fatal("EmbedAttempted must be true even on failure so layers skip re-embed")
	}
	if l2.got.QueryEmbedding != nil || l3.got.QueryEmbedding != nil {
		t.Fatal("failed embed must pass nil vector (layers degrade to non-vector search)")
	}
}

// ── P0-C: usecase-level preset embedding + L2 embed timeout ─────────────

type l2StoreStub struct {
	gotVec []float32
}

func (s *l2StoreStub) RecallL2Episodes(_ context.Context, _, _, _ string, vec []float32, _ int32) ([][]byte, error) {
	s.gotVec = vec
	return nil, nil
}

func TestL2Recall_PresetEmbeddingSkipsEmbed(t *testing.T) {
	store := &l2StoreStub{}
	emb := &embedderStub{vec: []float32{9, 9}}
	uc := NewMemoryL2RecallUsecase(store, emb, loggateway.NewNoop())
	preset := []float32{1, 2, 3}
	if _, err := uc.RecallEpisodes(context.Background(), L2RecallQuery{
		AgentID: "ag1", Query: "q", QueryEmbedding: preset, EmbedAttempted: true,
	}); err != nil {
		t.Fatal(err)
	}
	if emb.calls != 0 {
		t.Fatalf("embed calls=%d, want 0 when embedding preset", emb.calls)
	}
	if len(store.gotVec) != 3 {
		t.Fatalf("store vec=%v, want preset vector", store.gotVec)
	}
}

func TestL2Recall_EmbedAttemptedFailureDegradesWithoutReembed(t *testing.T) {
	store := &l2StoreStub{}
	emb := &embedderStub{vec: []float32{9, 9}}
	uc := NewMemoryL2RecallUsecase(store, emb, loggateway.NewNoop())
	if _, err := uc.RecallEpisodes(context.Background(), L2RecallQuery{
		AgentID: "ag1", Query: "q", EmbedAttempted: true, // nil vector = upstream embed failed
	}); err != nil {
		t.Fatal(err)
	}
	if emb.calls != 0 {
		t.Fatalf("embed calls=%d, want 0 (attempted upstream)", emb.calls)
	}
	if store.gotVec != nil {
		t.Fatalf("store vec=%v, want nil (non-vector degrade)", store.gotVec)
	}
}

func TestL2Recall_OwnEmbedAppliesTimeout(t *testing.T) {
	store := &l2StoreStub{}
	emb := &embedderStub{vec: []float32{1}}
	uc := NewMemoryL2RecallUsecase(store, emb, loggateway.NewNoop())
	if _, err := uc.RecallEpisodes(context.Background(), L2RecallQuery{
		AgentID: "ag1", Query: "q",
	}); err != nil {
		t.Fatal(err)
	}
	if emb.calls != 1 {
		t.Fatalf("embed calls=%d, want 1", emb.calls)
	}
	if emb.deadlineDelta <= 0 || emb.deadlineDelta > 3*time.Second+500*time.Millisecond {
		t.Fatalf("L2 embed ctx deadline=%v, want ~3s timeout (P0-C)", emb.deadlineDelta)
	}
}

type l3ScoredCaptureStub struct {
	gotVec []float32
}

func (s *l3ScoredCaptureStub) RecallL3Hits(_ context.Context, _, _, _, _ string, vec []float32, _ int32) ([]RecallHit, error) {
	s.gotVec = vec
	return nil, nil
}

func TestL3FusedRecall_PresetEmbeddingSkipsEmbed(t *testing.T) {
	scored := &l3ScoredCaptureStub{}
	emb := &embedderStub{vec: []float32{9, 9}}
	uc := NewMemoryL3RecallUsecase(recallStoreMock{}, scored, emb, loggateway.NewNoop())
	preset := []float32{1, 2, 3}
	if _, err := uc.RecallFactsFused(context.Background(), L3FusedRecallQuery{
		Runtime: MemoryRuntimeContext{AgentID: "ag1"}, Scopes: []string{"agent"},
		Query: "q", QueryEmbedding: preset, EmbedAttempted: true,
	}); err != nil {
		t.Fatal(err)
	}
	if emb.calls != 0 {
		t.Fatalf("embed calls=%d, want 0 when embedding preset", emb.calls)
	}
	if len(scored.gotVec) != 3 {
		t.Fatalf("store vec=%v, want preset vector", scored.gotVec)
	}
}

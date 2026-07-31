package data_test

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/pkg/loggateway"
)

// insertRecallFact inserts one active fact for recall scoring tests.
func insertRecallFact(t *testing.T, d *data.Data, scopeID, statement string, importance float64) {
	t.Helper()
	w := data.NewL3FactWriterAdapter(d, nil)
	_, err := w.UpsertFactRow(context.Background(), biz.FactUpsert{
		ScopeType:  "agent",
		ScopeID:    scopeID,
		AgentID:    scopeID,
		Statement:  statement,
		FactKind:   "preference",
		Importance: importance,
		Confidence: 0.9,
		SourceKind: "manual",
	})
	if err != nil {
		t.Fatalf("UpsertFactRow(%q): %v", statement, err)
	}
}

func scoresTotalFromRaw(t *testing.T, raw []byte) float64 {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	sc, ok := m["scores"].(map[string]any)
	if !ok {
		t.Fatalf("raw row missing scores object: %s", raw)
	}
	total, ok := sc["total"].(float64)
	if !ok {
		t.Fatalf("scores.total missing or not a number: %s", raw)
	}
	return total
}

// TestL3Recall_BruteForceAnnotatesScores is the Bug A regression test at the
// store level: the brute-force recall path (fact count below the 5000
// threshold) must compute the hybrid score and annotate every returned row
// with a "scores" object whose total is > 0, instead of returning bare DB
// JSON ordered by importance only.
func TestL3Recall_BruteForceAnnotatesScores(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	ctx := context.Background()
	insertRecallFact(t, d, "agent-r3a", "User likes coffee a lot", 0.9)
	insertRecallFact(t, d, "agent-r3a", "User enjoys tea", 0.9)

	reader := data.NewL3FactReaderForUser(d)
	rows, err := reader.RecallL3Facts(ctx, "agent", "agent-r3a", "", "coffee", nil, 10, 0)
	if err != nil {
		t.Fatalf("RecallL3Facts: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2", len(rows))
	}
	for _, raw := range rows {
		if total := scoresTotalFromRaw(t, raw); total <= 0 {
			t.Fatalf("scores.total=%f, want > 0", total)
		}
	}
	// The keyword-matching coffee fact must outrank the tea fact.
	first := scoresTotalFromRaw(t, rows[0])
	second := scoresTotalFromRaw(t, rows[1])
	if first < second {
		t.Fatalf("ranking wrong: first total=%f < second total=%f (coffee fact should rank first)", first, second)
	}
	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatal(err)
	}
	if stmt, _ := m["statement"].(string); stmt != "User likes coffee a lot" {
		t.Fatalf("top row statement=%q, want coffee fact", stmt)
	}
}

// TestL3Recall_BruteForceAppliesMinScore verifies the brute-force path honours
// the minScore filter (previously it ignored minScore entirely).
func TestL3Recall_BruteForceAppliesMinScore(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	ctx := context.Background()
	insertRecallFact(t, d, "agent-r3b", "User likes coffee a lot", 0.9)
	insertRecallFact(t, d, "agent-r3b", "User enjoys tea", 0.5)

	reader := data.NewL3FactReaderForUser(d)
	rows, err := reader.RecallL3Facts(ctx, "agent", "agent-r3b", "", "coffee", nil, 10, 0.55)
	if err != nil {
		t.Fatalf("RecallL3Facts: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d, want 1 (only the keyword-matching fact survives minScore=0.55)", len(rows))
	}
	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatal(err)
	}
	if stmt, _ := m["statement"].(string); stmt != "User likes coffee a lot" {
		t.Fatalf("statement=%q, want coffee fact", stmt)
	}
}

// TestL3ScoredRecallAdapter_PopulatesScoreTotal is the direct Bug A
// regression: the scored recall adapter must propagate the internally computed
// score into RecallHit.Scores.Total. Before the fix Total was always 0, so
// RecallFactsFused filtered out every hit at minScore=0.55 while use_count
// kept growing (3982 recalls, 0 injections).
func TestL3ScoredRecallAdapter_PopulatesScoreTotal(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	ctx := context.Background()
	insertRecallFact(t, d, "agent-r3c", "User likes coffee a lot", 0.9)

	adapter := data.NewL3ScoredRecallAdapter(d)
	hits, err := adapter.RecallL3Hits(ctx, "agent", "agent-r3c", "", "coffee", nil, 10)
	if err != nil {
		t.Fatalf("RecallL3Hits: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits=%d, want 1", len(hits))
	}
	if hits[0].Scores.Total <= 0 {
		t.Fatalf("Scores.Total=%f, want > 0 (Bug A: fused recall filters everything at minScore)", hits[0].Scores.Total)
	}
	if hits[0].Scores.Keyword <= 0 {
		t.Fatalf("Scores.Keyword=%f, want > 0 for exact keyword match", hits[0].Scores.Keyword)
	}
}

// TestL3FusedRecall_SurvivesMinScoreFilter is the end-to-end Bug A regression:
// with a real store + scored adapter, fused recall at the production default
// MinScoreQuery=0.55 must return the relevant fact instead of an empty list.
func TestL3FusedRecall_SurvivesMinScoreFilter(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	ctx := context.Background()
	insertRecallFact(t, d, "agent-r3d", "User likes coffee a lot", 0.9)
	insertRecallFact(t, d, "agent-r3d", "User enjoys tea", 0.5)

	uc := biz.NewMemoryL3RecallUsecase(
		data.NewL3FactReaderForUser(d),
		data.NewL3ScoredRecallAdapter(d),
		nil, // no embedder: exercises the brute-force scoring path
		loggateway.NewNoop(),
	)
	if uc == nil {
		t.Fatal("NewMemoryL3RecallUsecase returned nil")
	}
	// DEBUG: inspect adapter hits directly
	adhits, aderr := data.NewL3ScoredRecallAdapter(d).RecallL3Hits(ctx, "agent", "agent-r3d", "", "coffee", nil, 20)
	t.Logf("adapter hits=%d err=%v", len(adhits), aderr)
	for _, h := range adhits {
		t.Logf("  hit id=%s total=%f decay=%f", h.ID, h.Scores.Total, h.Scores.Decay)
	}
	rows, err := uc.RecallFactsFused(ctx, biz.L3FusedRecallQuery{
		Runtime:       biz.MemoryRuntimeContext{AgentID: "agent-r3d"},
		Query:         "coffee",
		Limit:         10,
		MinScoreQuery: 0.55,
	})
	if err != nil {
		t.Fatalf("RecallFactsFused: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("fused rows=%d, want 1 (Bug A: minScore filter dropped everything)", len(rows))
	}
	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatal(err)
	}
	if stmt, _ := m["statement"].(string); stmt != "User likes coffee a lot" {
		t.Fatalf("statement=%q, want coffee fact", stmt)
	}
}

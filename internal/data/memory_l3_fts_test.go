package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/loggateway"
)

// fakeVectorStore is a test double for vector.VectorStore that returns
// canned hits (or an error) from Search.
type fakeVectorStore struct {
	hits []vector.VectorHit
	err  error
}

func (f *fakeVectorStore) Upsert(context.Context, string, []float64, map[string]string) error {
	return nil
}

func (f *fakeVectorStore) Search(context.Context, []float64, int, float64) ([]vector.VectorHit, error) {
	return f.hits, f.err
}

func (f *fakeVectorStore) Delete(context.Context, string) error { return nil }

// setupL3FTSTestRepo builds an isolated Postgres schema with the production
// memory-chain DDL and returns a threshold-overridable l3FactRepo. The FTS
// GIN index (DDL 20261128) is intentionally NOT created: to_tsvector works
// without the index, and these tests assert recall semantics, not index
// performance.
func setupL3FTSTestRepo(t *testing.T, vs vector.VectorStore, threshold int) *l3FactRepo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	if err := EnsureSessionMemorySchema(ctx, client, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("ensure session memory schema: %v", err)
	}
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	r := newL3FactRepo(d, vs)
	r.bruteForceThreshold = threshold
	return r
}

func upsertL3Fact(t *testing.T, r *l3FactRepo, scopeID, statement string, importance float64) string {
	t.Helper()
	raw, err := r.UpsertFactRow(context.Background(), biz.FactUpsert{
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
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatalf("upserted fact missing id: %s", raw)
	}
	return id
}

func factRowID(t *testing.T, raw []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	id, _ := m["id"].(string)
	return id
}

func factRowScore(t *testing.T, raw []byte, key string) float64 {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	sc, ok := m["scores"].(map[string]any)
	if !ok {
		t.Fatalf("raw row missing scores object: %s", raw)
	}
	v, _ := sc[key].(float64)
	return v
}

// TestRRFFuseRanked covers the pure fusion function: union ordering by fused
// score, cross-list dedup, blank-ID skipping, empty inputs, and k fallback.
func TestRRFFuseRanked(t *testing.T) {
	// b appears in both lists (vec#2, fts#1) and must outrank single-list
	// entries; a (vec#1) edges out c (fts#2) on rank.
	scores, order := rrfFuseRanked(60, []string{"a", "b"}, []string{"b", "c"})
	want := []string{"b", "a", "c"}
	if len(order) != len(want) {
		t.Fatalf("order=%v, want %v", order, want)
	}
	for i, id := range want {
		if order[i] != id {
			t.Fatalf("order=%v, want %v", order, want)
		}
	}
	if !(scores["b"] > scores["a"] && scores["a"] > scores["c"]) {
		t.Fatalf("fused scores not strictly ordered: %v", scores)
	}

	// Empty inputs produce empty outputs.
	scores, order = rrfFuseRanked(60, nil, nil)
	if len(order) != 0 || len(scores) != 0 {
		t.Fatalf("empty input: order=%v scores=%v", order, scores)
	}

	// Blank / whitespace IDs are skipped.
	_, order = rrfFuseRanked(60, []string{" ", "x", ""})
	if len(order) != 1 || order[0] != "x" {
		t.Fatalf("blank IDs: order=%v", order)
	}

	// k <= 0 falls back to the default constant.
	s1, _ := rrfFuseRanked(0, []string{"a"})
	s2, _ := rrfFuseRanked(l3RRFK, []string{"a"})
	if s1["a"] != s2["a"] {
		t.Fatalf("k fallback: %f != %f", s1["a"], s2["a"])
	}
}

// TestSearchL3FTS_RanksAlphanumericTokens verifies the FTS candidate query:
// alphanumeric tokens (codes, names, IDs) match via the 'simple' tsvector
// config, scope filters isolate namespaces, and blank queries degrade to nil.
func TestSearchL3FTS_RanksAlphanumericTokens(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	ctx := context.Background()
	scope := "agent-fts-direct"
	targetID := upsertL3Fact(t, r, scope, "Deploy token ZXQ4817 belongs to Falcon project", 0.5)
	upsertL3Fact(t, r, scope, "User enjoys tea in the afternoon", 0.5)

	ids, err := r.searchL3FTS(ctx, "agent", scope, "", "ZXQ4817", 10)
	if err != nil {
		t.Fatalf("searchL3FTS: %v", err)
	}
	if len(ids) != 1 || ids[0] != targetID {
		t.Fatalf("ids=%v, want [%s]", ids, targetID)
	}

	// Blank query degrades to nil (callers continue without FTS candidates).
	ids, err = r.searchL3FTS(ctx, "agent", scope, "", "   ", 10)
	if err != nil || ids != nil {
		t.Fatalf("blank query: ids=%v err=%v, want nil/nil", ids, err)
	}

	// Scope isolation: a fact with the same token in another scope must not
	// leak into this scope's candidate list.
	upsertL3Fact(t, r, "agent-fts-other", "ZXQ4817 appears in another scope", 0.5)
	ids, err = r.searchL3FTS(ctx, "agent", scope, "", "ZXQ4817", 10)
	if err != nil {
		t.Fatalf("searchL3FTS scoped: %v", err)
	}
	if len(ids) != 1 || ids[0] != targetID {
		t.Fatalf("scope leak: ids=%v, want [%s]", ids, targetID)
	}
}

// TestL3Recall_BruteForceInjectsFTSCandidate is the P2-3 brute-force path
// regression: a keyword-strong fact with low importance falls outside the
// SQL pre-limit (importance top-60) and must be rescued by the FTS extra
// candidate injection.
func TestL3Recall_BruteForceInjectsFTSCandidate(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0) // threshold 0 → default → brute force
	ctx := context.Background()
	scope := "agent-fts-bf"
	// 70 filler facts outrank the target by importance, pushing it out of the
	// 60-row SQL pre-limit pool.
	for i := 0; i < 70; i++ {
		upsertL3Fact(t, r, scope, fmt.Sprintf("Filler fact %02d about everyday preferences", i), 0.5)
	}
	upsertL3Fact(t, r, scope, "Project codename ZXQ4817 uses delta brackets", 0.05)

	rows, err := r.RecallL3Facts(ctx, "agent", scope, "", "ZXQ4817", nil, 10, 0)
	if err != nil {
		t.Fatalf("RecallL3Facts: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no rows returned")
	}
	var m map[string]any
	if err := json.Unmarshal(rows[0], &m); err != nil {
		t.Fatal(err)
	}
	stmt, _ := m["statement"].(string)
	if !strings.Contains(stmt, "ZXQ4817") {
		t.Fatalf("top row statement=%q, want the FTS-rescued ZXQ4817 fact", stmt)
	}
	if kw := factRowScore(t, rows[0], "keyword"); kw <= 0 {
		t.Fatalf("rescued fact keyword score=%f, want > 0", kw)
	}
}

// TestL3Recall_VectorFTSFusion exercises the pgvector+FTS RRF fusion path
// (non-brute-force): the candidate pool is the RRF union of vector hits and
// FTS hits, each row carries the fused rrf score for observability, and
// vector similarity comes from the store hit map.
func TestL3Recall_VectorFTSFusion(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 1) // count(3) > 1 → non-brute-force
	ctx := context.Background()
	scope := "agent-fts-fusion"
	vecFactID := upsertL3Fact(t, r, scope, "Semantic match about evening routines", 0.5)
	ftsFactID := upsertL3Fact(t, r, scope, "Serial number QXT99 device pairing", 0.5)
	unrelatedID := upsertL3Fact(t, r, scope, "Unrelated fact about the weather", 0.5)

	r.vectorStore = &fakeVectorStore{hits: []vector.VectorHit{{ID: vecFactID, Score: 0.9}}}

	rows, err := r.RecallL3Facts(ctx, "agent", scope, "", "QXT99", []float32{0.1, 0.2, 0.3}, 10, 0)
	if err != nil {
		t.Fatalf("RecallL3Facts: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2 (vector hit + FTS hit; unrelated fact must not enter the pool)", len(rows))
	}
	got := map[string][]byte{}
	for _, raw := range rows {
		got[factRowID(t, raw)] = raw
	}
	if _, ok := got[unrelatedID]; ok {
		t.Fatal("unrelated fact entered the fused pool (neither vector nor FTS returned it)")
	}
	vecRaw, ok := got[vecFactID]
	if !ok {
		t.Fatalf("vector-hit fact %s missing from fused recall", vecFactID)
	}
	ftsRaw, ok := got[ftsFactID]
	if !ok {
		t.Fatalf("FTS-hit fact %s missing from fused recall", ftsFactID)
	}
	if v := factRowScore(t, vecRaw, "vector"); v != 0.9 {
		t.Fatalf("vector score=%f, want 0.9 from store hit map", v)
	}
	if kw := factRowScore(t, ftsRaw, "keyword"); kw <= 0 {
		t.Fatalf("FTS fact keyword score=%f, want > 0", kw)
	}
	for id, raw := range got {
		if rrf := factRowScore(t, raw, "rrf"); rrf <= 0 {
			t.Fatalf("fact %s rrf=%f, want > 0 (fused score annotated)", id, rrf)
		}
	}
}

// TestL3Recall_VectorStoreErrorDegradesToFTS verifies that a vector-store
// failure does not fail the recall: the FTS channel still supplies candidates.
func TestL3Recall_VectorStoreErrorDegradesToFTS(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 1)
	ctx := context.Background()
	scope := "agent-fts-degrade"
	ftsFactID := upsertL3Fact(t, r, scope, "Serial number QXT99 device pairing", 0.5)
	upsertL3Fact(t, r, scope, "Another fact about something else", 0.5)

	r.vectorStore = &fakeVectorStore{err: errors.New("pgvector connection refused")}

	rows, err := r.RecallL3Facts(ctx, "agent", scope, "", "QXT99", []float32{0.1, 0.2, 0.3}, 10, 0)
	if err != nil {
		t.Fatalf("RecallL3Facts must not fail on vector-store error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d, want 1 (FTS-only candidate)", len(rows))
	}
	if got := factRowID(t, rows[0]); got != ftsFactID {
		t.Fatalf("row id=%s, want FTS candidate %s", got, ftsFactID)
	}
}

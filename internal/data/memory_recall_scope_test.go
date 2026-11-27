package data

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/loggateway"
)

// fakeFilteredVectorStore extends fakeVectorStore (the global Search) with
// SearchFiltered, recording every filter it receives.
type fakeFilteredVectorStore struct {
	fakeVectorStore
	filtered []vector.VectorHit
	filters  []map[string]string
}

func (f *fakeFilteredVectorStore) SearchFiltered(_ context.Context, _ []float64, _ int, _ float64, filter map[string]string) ([]vector.VectorHit, error) {
	f.filters = append(f.filters, filter)
	return f.filtered, nil
}

var _ vector.FilteredVectorStore = (*fakeFilteredVectorStore)(nil)

// foreignHits are vector hits whose IDs exist in no scope of the test schema,
// i.e. what a global top-K over a table shared by many users returns.
func foreignHits(n int) []vector.VectorHit {
	hits := make([]vector.VectorHit, n)
	for i := range hits {
		hits[i] = vector.VectorHit{ID: fmt.Sprintf("foreign-fact-%03d", i), Score: 0.95}
	}
	return hits
}

func upsertScopedFact(t *testing.T, r *l3FactRepo, in biz.FactUpsert) string {
	t.Helper()
	if in.FactKind == "" {
		in.FactKind = "event"
	}
	in.Importance, in.Confidence, in.SourceKind = 0.5, 0.9, "manual"
	raw, err := r.UpsertFactRow(context.Background(), in)
	if err != nil {
		t.Fatalf("UpsertFactRow(%q): %v", in.Statement, err)
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

func rowIDs(t *testing.T, rows [][]byte, key string) []string {
	t.Helper()
	out := make([]string, 0, len(rows))
	for _, raw := range rows {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		v, _ := m[key].(string)
		out = append(out, v)
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestL3Recall_UserScopedVectorLegUsesFilteredSearch: a user-bound recall
// must ask the store for that user's vectors only. Before the fix the leg
// was a global top-K, so in a table shared by many users (AMC eval) the
// user's semantically matching fact was crowded out and, with no lexical
// overlap, never recalled.
func TestL3Recall_UserScopedVectorLegUsesFilteredSearch(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 1)
	const user = "user-scope-a"
	own := upsertScopedFact(t, r, biz.FactUpsert{
		ScopeType: "user", ScopeID: user, UserID: user, AgentID: user,
		Statement: "Moved to Lisbon last spring", SkipPIIRedact: true,
	})
	fvs := &fakeFilteredVectorStore{
		fakeVectorStore: fakeVectorStore{hits: foreignHits(60)},
		filtered:        []vector.VectorHit{{ID: own, Score: 0.81}},
	}
	r.vectorStore = fvs

	rows, err := r.RecallL3Facts(context.Background(), "user", user, user, "qqzzxv wvbn", []float32{1, 0, 0}, 10, 0)
	if err != nil {
		t.Fatalf("RecallL3Facts: %v", err)
	}
	if got := rowIDs(t, rows, "id"); !containsString(got, own) {
		t.Fatalf("own vector-matched fact %s not recalled; got %v", own, got)
	}
	if len(fvs.filters) != 1 || fvs.filters[0]["user_id"] != user {
		t.Fatalf("filters=%v, want one search filtered by user_id=%s", fvs.filters, user)
	}
	if fvs.search != 0 {
		t.Fatalf("global Search called %d times, want 0 for a user-bound recall", fvs.search)
	}
}

// TestL3Recall_OutOfScopeVectorHitsFallBackToRecency: when every vector hit
// lies outside the scope and no lexical leg matches, recall must fall back to
// the scope's recency pool instead of returning nothing (the old path fused
// the foreign IDs, lost them all to the SQL scope filter and returned empty).
func TestL3Recall_OutOfScopeVectorHitsFallBackToRecency(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 1)
	const user = "user-scope-b"
	own := upsertScopedFact(t, r, biz.FactUpsert{
		ScopeType: "user", ScopeID: user, UserID: user, AgentID: user,
		Statement: "Prefers window seats on long flights", SkipPIIRedact: true,
	})
	r.vectorStore = &fakeVectorStore{hits: foreignHits(60)} // no filter support

	rows, err := r.RecallL3Facts(context.Background(), "user", user, user, "qqzzxv wvbn", []float32{1, 0, 0}, 10, 0)
	if err != nil {
		t.Fatalf("RecallL3Facts: %v", err)
	}
	if got := rowIDs(t, rows, "id"); !containsString(got, own) {
		t.Fatalf("scope fact %s not recalled after foreign-only vector hits; got %v", own, got)
	}
}

// TestL2Recall_FactVectorSessionLeg: episodes have no vector rows, so the
// semantic leg maps the agent's fact hits to their source sessions. An old
// episode outside the 40-row recency pool must be reachable through its
// session's fact. Before the fix the vector path looked fact IDs up as
// episode IDs and returned nothing.
func TestL2Recall_FactVectorSessionLeg(t *testing.T) {
	r := setupL2RecallTestRepo(t)
	ctx := context.Background()
	const agent = "agent-xsess"
	insertL2Episode(t, r, "sess-old", "t-old", "Old trip planning", "Booked the ferry to Capri", 0.5)
	for i := 0; i < l2RecallCandidatePool+5; i++ {
		insertL2Episode(t, r, fmt.Sprintf("sess-filler-%02d", i), fmt.Sprintf("t-f%02d", i),
			fmt.Sprintf("Filler episode %02d", i), "Nothing notable happened", 0.5)
	}
	fact := upsertScopedFact(t, newL3FactRepo(r.data, nil), biz.FactUpsert{
		ScopeType: "agent", ScopeID: agent, AgentID: agent,
		Statement: "Booked the ferry to Capri", SourceSessionID: "sess-old",
	})
	fvs := &fakeFilteredVectorStore{filtered: []vector.VectorHit{{ID: fact, Score: 0.82}}}
	r.vectorStore = fvs

	rows, err := r.RecallL2Episodes(ctx, agent, "", "qqzzxv wvbn", []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatalf("RecallL2Episodes: %v", err)
	}
	var found bool
	for _, raw := range rows {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if m["session_id"] == "sess-old" {
			found = true
			if v := factRowScore(t, raw, "vector"); v < 0.8 {
				t.Fatalf("sess-old vector score=%f, want the fact hit score 0.82", v)
			}
		}
	}
	if !found {
		t.Fatalf("old episode not recalled through its session's fact; sessions=%v", rowIDs(t, rows, "session_id"))
	}
	if len(fvs.filters) != 1 || fvs.filters[0]["agent_id"] != agent {
		t.Fatalf("filters=%v, want one search filtered by agent_id=%s", fvs.filters, agent)
	}
}

// TestEvalStore_VectorCorrectionUsesUserScopedSearch: the eval correction
// pass must search this user's vectors only. A global top-K over a table
// shared by many users returns other users' near-duplicates; the user filter
// then drops them all and the stale value is never invalidated (AMC knowledge
// update questions keep answering the old value).
func TestEvalStore_VectorCorrectionUsesUserScopedSearch(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	fvs := &fakeFilteredVectorStore{}
	emb := newEvalIntentTestEmbedder(map[string][]float32{
		"Actually, I hate blue now": evalIntentTestAxis(evalIntentAxesTable[biz.EvalIntentCorrection], 1),
	})
	store := NewEvalMemoryStore(r.data, emb, loggateway.NewNoop()).(*evalMemoryStore)
	store.facts = newL3FactRepo(r.data, fvs)
	store.syncer = nil
	ctx := context.Background()
	const user = "u-vec-scope"

	if _, err := store.AddMessages(ctx, user, "s-vec-scope", []biz.EvalMessage{
		{Role: "user", Content: "My favorite color is blue", MessageID: "m1"},
	}); err != nil {
		t.Fatal(err)
	}
	old := evalC3FindFact(t, evalC3AllFacts(t, r, user), "favorite color is blue")
	oldID, _ := old["id"].(string)
	fvs.hits = foreignHits(evalCorrectionTopK) // global: other users' near-duplicates
	fvs.filtered = []vector.VectorHit{{ID: oldID, Score: 0.9}}
	fvs.filters = nil

	if _, err := store.AddMessages(ctx, user, "s-vec-scope", []biz.EvalMessage{
		{Role: "user", Content: "Actually, I hate blue now", MessageID: "m2"},
	}); err != nil {
		t.Fatal(err)
	}
	old = evalC3FindFact(t, evalC3AllFacts(t, r, user), "favorite color is blue")
	if vu, _ := old["valid_until"].(string); vu == "" {
		t.Fatalf("correction did not invalidate the user's own stale fact: %v", old)
	}
	var scoped bool
	for _, f := range fvs.filters {
		if f["user_id"] == user {
			scoped = true
		}
	}
	if !scoped {
		t.Fatalf("filters=%v, want a search filtered by user_id=%s", fvs.filters, user)
	}
}

// TestL2Recall_UnmappedVectorHitsKeepKeywordRecall: vector hits that map to
// no session of this agent must not empty the result; the recency/keyword
// legs still answer (the old vector path returned nothing here).
func TestL2Recall_UnmappedVectorHitsKeepKeywordRecall(t *testing.T) {
	r := setupL2RecallTestRepo(t)
	insertL2Episode(t, r, "sess-1", "t1", "Falcon project review", "Reviewed Falcon milestones", 0.5)
	r.vectorStore = &fakeVectorStore{hits: foreignHits(40)}

	rows, err := r.RecallL2Episodes(context.Background(), "agent-xsess", "", "Falcon", []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatalf("RecallL2Episodes: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no rows: unmapped vector hits emptied L2 recall")
	}
}

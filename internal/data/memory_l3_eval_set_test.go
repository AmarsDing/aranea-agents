package data

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// ──────────────────────────────────────────────────────────
// P2-4: offline evaluation set (LongMemEval five-capability subset, 50 items)
//
// The dataset lives in testdata/memory_l3_eval_set.json and is executed
// against a real Postgres schema via the production recall path (brute-force
// + FTS injection; no embedder so the run is deterministic). Every item is
// scoped as eval-<id> for isolation within one shared schema.
//
// Gate semantics (dataset min_score = 0.55, deliberately stricter than the
// 0.35 production runtime default since 2026-08-08 P0-4):
//   - extraction / multi_hop / temporal / knowledge_update: every expect_hits
//     substring must appear in some recalled statement, and every
//     expect_absent substring must appear in NONE.
//   - refusal: nothing may be recalled above min_score (abstention).
// The suite reports per-capability pass rates and fails when the overall rate
// drops below 90% — the initial baseline is 100% (recorded in
// docs/development/70-orchestration-longtask-memory.development.md §18.7).
// ──────────────────────────────────────────────────────────

type memoryEvalSeed struct {
	Statement  string  `json:"statement"`
	Details    string  `json:"details,omitempty"`
	Importance float64 `json:"importance,omitempty"`
	Kind       string  `json:"kind,omitempty"`
	AgeDays    int     `json:"age_days,omitempty"`
	Invalidate bool    `json:"invalidate,omitempty"`
}

type memoryEvalItem struct {
	ID           string           `json:"id"`
	Capability   string           `json:"capability"`
	Query        string           `json:"query"`
	Seeds        []memoryEvalSeed `json:"seed_facts"`
	ExpectHits   []string         `json:"expect_hits,omitempty"`
	ExpectAbsent []string         `json:"expect_absent,omitempty"`
	ExpectEmpty  bool             `json:"expect_empty,omitempty"`
}

type memoryEvalSet struct {
	Version  int              `json:"version"`
	Name     string           `json:"name"`
	MinScore float64          `json:"min_score"`
	Items    []memoryEvalItem `json:"items"`
}

func TestMemoryL3OfflineEvalSet(t *testing.T) {
	raw, err := os.ReadFile("testdata/memory_l3_eval_set.json")
	if err != nil {
		t.Fatalf("read eval set: %v", err)
	}
	var set memoryEvalSet
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatalf("parse eval set: %v", err)
	}
	if len(set.Items) != 50 {
		t.Fatalf("eval set size=%d, want 50", len(set.Items))
	}
	minScore := set.MinScore
	if minScore <= 0 {
		minScore = 0.55
	}

	r := setupL3FTSTestRepo(t, nil, 0)
	ctx := context.Background()

	type capStat struct{ pass, total int }
	stats := map[string]*capStat{}
	var failures []string

	for _, it := range set.Items {
		it := it
		t.Run(it.ID, func(t *testing.T) {
			stat := stats[it.Capability]
			if stat == nil {
				stat = &capStat{}
				stats[it.Capability] = stat
			}
			stat.total++
			scope := "eval-" + it.ID

			for _, s := range it.Seeds {
				imp := s.Importance
				if imp <= 0 {
					imp = 0.85
				}
				kind := s.Kind
				if kind == "" {
					kind = "preference" // durable likes stay evergreen; recency still demotes stale rows
				}
				up := biz.FactUpsert{
					ScopeType: "agent", ScopeID: scope, AgentID: scope,
					Statement: s.Statement, DetailsMarkdown: s.Details,
					FactKind: kind, Importance: imp, Confidence: 0.9,
					SourceKind: "eval",
				}
				if s.AgeDays > 0 {
					ts := time.Now().UTC().Add(-time.Duration(s.AgeDays) * 24 * time.Hour).Format(time.RFC3339)
					up.CreatedAt, up.UpdatedAt = ts, ts
				}
				rawFact, err := r.UpsertFactRow(ctx, up)
				if err != nil {
					t.Fatalf("seed %q: %v", s.Statement, err)
				}
				if s.Invalidate {
					if _, err := r.InvalidateFact(ctx, factRowID(t, rawFact)); err != nil {
						t.Fatalf("invalidate %q: %v", s.Statement, err)
					}
				}
			}

			rows, err := r.RecallL3Facts(ctx, "agent", scope, "", it.Query, nil, 10, minScore)
			if err != nil {
				t.Fatalf("recall: %v", err)
			}
			stmts := make([]string, 0, len(rows))
			for _, row := range rows {
				var m map[string]any
				if err := json.Unmarshal(row, &m); err != nil {
					t.Fatalf("unmarshal row: %v", err)
				}
				stmt, _ := m["statement"].(string)
				stmts = append(stmts, stmt)
			}
			joined := strings.Join(stmts, "\n")

			var itemErrs []string
			if it.ExpectEmpty && len(rows) != 0 {
				itemErrs = append(itemErrs, fmt.Sprintf("refusal: expected empty recall, got %d rows: %s", len(rows), joined))
			}
			for _, h := range it.ExpectHits {
				if !strings.Contains(joined, h) {
					itemErrs = append(itemErrs, fmt.Sprintf("missing expected hit %q in recall %q", h, joined))
				}
			}
			for _, a := range it.ExpectAbsent {
				if strings.Contains(joined, a) {
					itemErrs = append(itemErrs, fmt.Sprintf("stale/absent fact %q leaked into recall %q", a, joined))
				}
			}
			if len(itemErrs) > 0 {
				for _, e := range itemErrs {
					t.Error(e)
				}
				failures = append(failures, it.ID)
				return
			}
			stat.pass++
		})
	}

	// Summary table (baseline record for docs §18.7).
	caps := make([]string, 0, len(stats))
	for c := range stats {
		caps = append(caps, c)
	}
	sort.Strings(caps)
	pass, total := 0, 0
	for _, c := range caps {
		s := stats[c]
		pass += s.pass
		total += s.total
		t.Logf("capability %-16s %d/%d", c, s.pass, s.total)
	}
	rate := float64(pass) / float64(total)
	t.Logf("OVERALL %d/%d (%.1f%%), failures: %v", pass, total, rate*100, failures)
	if rate < 0.9 {
		t.Errorf("eval set pass rate %.1f%% below 90%% gate (failures: %v)", rate*100, failures)
	}
}

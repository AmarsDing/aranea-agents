package data

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupL2RecallTestRepo builds an isolated Postgres schema with the
// production memory-chain DDL and returns an l2EpisodeRepo without a vector
// store (nil → the brute-force recallL2Episodes path under test).
func setupL2RecallTestRepo(t *testing.T) *l2EpisodeRepo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	if err := EnsureSessionMemorySchema(context.Background(), client, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("ensure session memory schema: %v", err)
	}
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	return newL2EpisodeRepo(d, nil)
}

func insertL2Episode(t *testing.T, r *l2EpisodeRepo, sessionID, taskID, title, summary string, importance float64) {
	t.Helper()
	err := r.InsertL1ArchiveEpisode(context.Background(), biz.L1ArchiveEpisodeInsert{
		SessionID:      sessionID,
		AgentID:        "agent-xsess",
		TaskID:         taskID,
		TaskTitle:      title,
		OutcomeSummary: summary,
		Importance:     importance,
		Confidence:     0.9,
	})
	if err != nil {
		t.Fatalf("InsertL1ArchiveEpisode(%q): %v", title, err)
	}
}

// TestL2Recall_CrossSessionReachable is the P2-R2 regression: episodes from
// OTHER sessions of the same agent must be reachable in recall. Before the
// fix the candidate pool was SQL-filtered by session_id, so cross-session
// memories could never be recalled (the vector path was already
// cross-session; the brute-force path was not).
func TestL2Recall_CrossSessionReachable(t *testing.T) {
	r := setupL2RecallTestRepo(t)
	ctx := context.Background()
	insertL2Episode(t, r, "sess-current", "t1", "Tea preferences discussion", "User likes green tea", 0.5)
	insertL2Episode(t, r, "sess-other", "t2", "Deploy token ZXQ4817 setup", "Token belongs to Falcon project", 0.5)

	// Recall from sess-current with a query that only matches the OTHER
	// session's episode. Cross-session hit must surface.
	rows, err := r.RecallL2Episodes(ctx, "agent-xsess", "sess-current", "ZXQ4817", nil, 10)
	if err != nil {
		t.Fatalf("RecallL2Episodes: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no rows: cross-session episode unreachable (session-filtered candidate pool)")
	}
	found := false
	for _, raw := range rows {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err == nil {
			if title, _ := m["title"].(string); title == "Deploy token ZXQ4817 setup" {
				found = true
				if kw := factRowScore(t, raw, "keyword"); kw <= 0 {
					t.Fatalf("cross-session hit keyword score=%f, want > 0", kw)
				}
			}
		}
	}
	if !found {
		t.Fatal("cross-session episode 'Deploy token ZXQ4817 setup' not in recall results")
	}
}

// TestL2Recall_SameSessionBoostPreserved verifies P2-R2 keeps the continuity
// signal: with equal importance/recency/keyword match, the current-session
// episode must outscore the cross-session one (session boost weight > 0).
func TestL2Recall_SameSessionBoostPreserved(t *testing.T) {
	r := setupL2RecallTestRepo(t)
	ctx := context.Background()
	insertL2Episode(t, r, "sess-current", "t1", "Falcon project sync", "Discussed Falcon milestones", 0.5)
	insertL2Episode(t, r, "sess-other", "t2", "Falcon project review", "Reviewed Falcon milestones", 0.5)

	rows, err := r.RecallL2Episodes(ctx, "agent-xsess", "sess-current", "Falcon", nil, 10)
	if err != nil {
		t.Fatalf("RecallL2Episodes: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2 (both sessions reachable)", len(rows))
	}
	var sameScore, crossScore float64
	for _, raw := range rows {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		sess, _ := m["session_id"].(string)
		total := factRowScore(t, raw, "total")
		if sess == "sess-current" {
			sameScore = total
		} else {
			crossScore = total
		}
	}
	if sameScore <= crossScore {
		t.Fatalf("same-session total=%f must exceed cross-session total=%f (session boost)", sameScore, crossScore)
	}
}

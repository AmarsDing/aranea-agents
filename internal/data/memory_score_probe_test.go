package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestScoreDistributionProbe is a MANUAL diagnostics probe for the 2026-08-26
// domain-B regression follow-up: review the adaptive minScore parameters
// (floor 0.35, top1 ratio 0.6) against the REAL post-fix score distribution.
//
// It embeds each probe question via the local Ollama bge-m3 endpoint, loads a
// candidate pool from memory_facts with the same select shape as production,
// scores every candidate with the production scoreFactRows path, and prints
// the full ranked breakdown plus where the adaptive cut line falls.
//
// Usage:
//
//	ARANEA_SCORE_PROBE=1 \
//	ARANEA_SCORE_PROBE_DSN="postgres://postgres:123456@127.0.0.1:5432/aranea?sslmode=disable" \
//	go test ./internal/data/ -run TestScoreDistributionProbe -v -count=1
func TestScoreDistributionProbe(t *testing.T) {
	if os.Getenv("ARANEA_SCORE_PROBE") != "1" {
		t.Skip("set ARANEA_SCORE_PROBE=1 to run the score distribution probe")
	}
	dsn := strings.TrimSpace(os.Getenv("ARANEA_SCORE_PROBE_DSN"))
	if dsn == "" {
		t.Fatal("ARANEA_SCORE_PROBE_DSN is required (explicitly point at the target DB)")
	}
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open target DB: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping target DB: %v", err)
	}

	agentID := "a21265a2d8f24072fb638b50" // eval_memory_probe
	questions := []string{
		"现在的值班电话是多少？",
		"当前的巡检周期是？",
		"现在出口带宽多大？",
		"现在的变更窗口安排是什么？",
	}
	now := time.Now().UTC()
	for _, q := range questions {
		emb, err := probeEmbedOllama(q)
		if err != nil {
			t.Fatalf("embed %q: %v", q, err)
		}
		// Candidate pool: same select shape as production, agent scope + the two
		// user scopes seen in eval (default_user plant owner, user 1 ask owner).
		rows, err := db.QueryContext(ctx, sqlFactSelectSQL(true)+`
 WHERE status='active' AND deleted_at='' AND valid_until=''
   AND ((scope_type='agent' AND scope_id=$1) OR (scope_type='user' AND scope_id IN ('default_user','1')))
 ORDER BY COALESCE(NULLIF(valid_from,''), created_at) DESC LIMIT 100`, agentID)
		if err != nil {
			t.Fatalf("query candidates: %v", err)
		}
		scored := scoreFactRows(rows, tokenizeQuery(q), emb, nil, 0, now)
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate candidates: %v", err)
		}
		sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

		top1 := 0.0
		if len(scored) > 0 {
			top1 = scored[0].score
		}
		eff := biz.AdaptiveRecallMinScore(0.35, top1)
		t.Logf("=== %q candidates=%d top1=%.4f adaptiveCut=max(0.35, top1*0.6)=%.4f ===", q, len(scored), top1, eff)
		show := len(scored)
		if show > 10 {
			show = 10
		}
		for i := 0; i < show; i++ {
			s := scored[i]
			stmt := s.stmt
			if len([]rune(stmt)) > 42 {
				stmt = string([]rune(stmt)[:42])
			}
			cut := ""
			if s.score < eff {
				cut = "  <-- CUT"
			}
			t.Logf("[%02d] total=%.4f kw=%.3f vec=%.3f imp=%.3f rec=%.3f q=%.3f | %s%s",
				i+1, s.score, s.breakdown.Keyword, s.breakdown.Vector, s.breakdown.Importance,
				s.breakdown.Recency, s.breakdown.QualityScore, stmt, cut)
		}
		// Count how many would be dropped by the cut.
		dropped := 0
		for _, s := range scored {
			if s.score < eff {
				dropped++
			}
		}
		t.Logf("cut keeps %d / drops %d", len(scored)-dropped, dropped)
	}
}

// probeEmbedOllama embeds one text via the local Ollama /api/embeddings
// endpoint (bge-m3, 1024-dim), matching the production embedder config.
func probeEmbedOllama(text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]string{"model": "bge-m3", "prompt": text})
	resp, err := http.Post("http://127.0.0.1:11434/api/embeddings", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding (status %s)", resp.Status)
	}
	return out.Embedding, nil
}

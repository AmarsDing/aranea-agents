package data_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/pkg/loggateway"
)

// TestConsolidationWriter_ConflictBumpsVersionAndDedupesEpisode is the
// regression guard for two production failures found 2026-07-29:
//
//  1. The fact upsert used unqualified `version = version + 1`, which Postgres
//     rejects with 42702 (ambiguous between memory_facts.version and
//     excluded.version). Every consolidation batch rolled back; no new fact
//     had been persisted since 2026-06-13 despite successful LLM extraction.
//  2. The episode partial unique index was missing (migration version
//     collision), so ON CONFLICT(session_id,title,agent_id) WHERE
//     l1_task_id = ” raised 42P10. Re-running the same batch must upsert the
//     existing episode, never insert a duplicate.
//
// memory_episodes (with both partial unique indexes) and memory_facts are
// created from the production chain DDL by openTestDataForMemory.
func TestConsolidationWriter_ConflictBumpsVersionAndDedupesEpisode(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	writer := data.NewMemoryConsolidationWriterAdapter(d, loggateway.NewNoop())
	ctx := context.Background()

	batch := func() *biz.ConsolidationResult {
		res, err := writer.UpsertFactsAndEpisodeBatch(ctx, []biz.MemoryFactWrite{{
			ScopeType:  "agent",
			ScopeID:    "agent-test",
			AgentID:    "agent-test",
			Statement:  "用户喜欢喝咖啡",
			FactKind:   "preference",
			SourceKind: "auto_memory",
		}}, &biz.EpisodeWrite{
			SessionID: "sess-1",
			AgentID:   "agent-test",
			Title:     "会话整理：咖啡偏好",
			Outcome:   "completed",
		})
		if err != nil {
			t.Fatalf("UpsertFactsAndEpisodeBatch: %v", err)
		}
		return res
	}

	first := batch()
	if first.FactsWritten != 1 || first.FactsDeduped != 0 {
		t.Fatalf("first batch: written=%d deduped=%d, want 1/0", first.FactsWritten, first.FactsDeduped)
	}
	second := batch()
	if second.FactsDeduped != 1 {
		t.Fatalf("second batch: deduped=%d, want 1 (fingerprint conflict)", second.FactsDeduped)
	}

	db := d.RWDB().ReadDB(ctx)
	scanOne := func(query string, dest ...any) {
		t.Helper()
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if !rows.Next() {
			t.Fatalf("query returned no rows: %s", query)
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
	}

	var version, factCount int
	scanOne(`SELECT version, count(*) OVER () FROM memory_facts WHERE scope_type='agent' AND scope_id='agent-test'`, &version, &factCount)
	if factCount != 1 {
		t.Fatalf("fact rows=%d, want 1 (upsert must not duplicate)", factCount)
	}
	if version != 2 {
		t.Fatalf("version=%d, want 2 (conflict update bumps version)", version)
	}

	var episodeCount int
	scanOne(`SELECT count(*) FROM memory_episodes WHERE session_id='sess-1'`, &episodeCount)
	if episodeCount != 1 {
		t.Fatalf("episode rows=%d, want 1 (ON CONFLICT must upsert, not duplicate)", episodeCount)
	}
}

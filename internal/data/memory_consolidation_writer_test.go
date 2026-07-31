package data_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/pkg/loggateway"
)

// createEpisodesTableForConsolidation creates memory_episodes plus the two
// partial unique indexes from migration 20261118 (renumbered from 20260802),
// which the consolidation upsert targets via ON CONFLICT ... WHERE clauses.
func createEpisodesTableForConsolidation(t *testing.T, d *data.Data) {
	t.Helper()
	ctx := context.Background()
	client := d.ClientFromCtx(ctx)
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS memory_episodes (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  l1_task_id TEXT NOT NULL DEFAULT '',
  episode_kind TEXT NOT NULL DEFAULT 'task',
  title TEXT NOT NULL,
  goal TEXT NOT NULL DEFAULT '',
  outcome TEXT NOT NULL DEFAULT '',
  outcome_summary TEXT NOT NULL DEFAULT '',
  result_preview TEXT NOT NULL DEFAULT '',
  failure_reason TEXT NOT NULL DEFAULT '',
  importance REAL NOT NULL DEFAULT 0.5,
  confidence REAL NOT NULL DEFAULT 0.7,
  user_feedback TEXT NOT NULL DEFAULT '',
  critic_score REAL NOT NULL DEFAULT -1,
  span_count INTEGER NOT NULL DEFAULT 0,
  message_count INTEGER NOT NULL DEFAULT 0,
  tool_call_count INTEGER NOT NULL DEFAULT 0,
  skill_call_count INTEGER NOT NULL DEFAULT 0,
  mcp_call_count INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  total_cost_micro_usd INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  l1_snapshot_json TEXT NOT NULL DEFAULT '{}',
  key_decisions_json TEXT NOT NULL DEFAULT '[]',
  key_artifacts_json TEXT NOT NULL DEFAULT '[]',
  embedding_status TEXT NOT NULL DEFAULT 'pending',
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dim INTEGER NOT NULL DEFAULT 0,
  embedding_blob BYTEA,
  embedding_norm REAL NOT NULL DEFAULT 0,
  consolidation_status TEXT NOT NULL DEFAULT 'consolidated',
  consolidated_at TEXT NOT NULL DEFAULT '',
  consolidated_l3_count INTEGER NOT NULL DEFAULT 0,
  consolidated_l4_count INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  started_at TEXT NOT NULL DEFAULT '',
  ended_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_episodes_session_l1_task
  ON memory_episodes(session_id, l1_task_id) WHERE l1_task_id != ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_episodes_session_title_agent
  ON memory_episodes(session_id, title, agent_id) WHERE l1_task_id = ''`,
	}
	for _, s := range stmts {
		if _, err := client.ExecContext(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
}

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
func TestConsolidationWriter_ConflictBumpsVersionAndDedupesEpisode(t *testing.T) {
	d, _ := openTestDataForMemory(t)
	createEpisodesTableForConsolidation(t, d)
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

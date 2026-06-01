package sessionmemory_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
)

func openConsolidateBatchStore(t *testing.T) *sessionmemory.Store {
	return openConsolidateBatchStoreTables(t, true)
}

func openConsolidateBatchStoreNoActionLog(t *testing.T) *sessionmemory.Store {
	return openConsolidateBatchStoreTables(t, false)
}

func openConsolidateBatchStoreTables(t *testing.T, withActionLog bool) *sessionmemory.Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS memory_facts (
 id TEXT PRIMARY KEY, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL DEFAULT '',
 workspace_id TEXT NOT NULL DEFAULT '', user_id TEXT NOT NULL DEFAULT '', team_id TEXT NOT NULL DEFAULT '', agent_id TEXT NOT NULL DEFAULT '',
 statement TEXT NOT NULL, statement_normalized TEXT NOT NULL DEFAULT '', fingerprint TEXT NOT NULL DEFAULT '', details_markdown TEXT NOT NULL DEFAULT '',
 fact_kind TEXT NOT NULL DEFAULT 'fact', tags_json TEXT NOT NULL DEFAULT '[]',
 confidence REAL NOT NULL DEFAULT 0.7, importance REAL NOT NULL DEFAULT 0.5,
 use_count INTEGER NOT NULL DEFAULT 0, hit_count INTEGER NOT NULL DEFAULT 0,
 positive_feedback_count INTEGER NOT NULL DEFAULT 0, negative_feedback_count INTEGER NOT NULL DEFAULT 0, conflict_count INTEGER NOT NULL DEFAULT 0,
 source_kind TEXT NOT NULL DEFAULT 'manual', source_episode_id TEXT NOT NULL DEFAULT '', source_session_id TEXT NOT NULL DEFAULT '',
 source_message_id TEXT NOT NULL DEFAULT '', source_external TEXT NOT NULL DEFAULT '',
 version INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'active', superseded_by TEXT NOT NULL DEFAULT '',
 embedding_status TEXT NOT NULL DEFAULT 'pending', embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INTEGER NOT NULL DEFAULT 0,
 embedding_blob BLOB, embedding_norm REAL NOT NULL DEFAULT 0,
 pii_flag INTEGER NOT NULL DEFAULT 0, redacted_statement TEXT NOT NULL DEFAULT '', pii_types TEXT NOT NULL DEFAULT '',
 ttl_days INTEGER NOT NULL DEFAULT 0, decay_factor REAL NOT NULL DEFAULT 0.98, next_decay_at TEXT NOT NULL DEFAULT '',
 last_used_at TEXT NOT NULL DEFAULT '', expires_at TEXT NOT NULL DEFAULT '',
 metadata_json TEXT NOT NULL DEFAULT '{}', quality_score REAL NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '',
 UNIQUE(scope_type, scope_id, fingerprint))`,
	}
	if withActionLog {
		stmts = append(stmts, `CREATE TABLE IF NOT EXISTS memory_action_log (
 id TEXT PRIMARY KEY, action TEXT NOT NULL, target_kind TEXT NOT NULL, target_id TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '', policy_version TEXT NOT NULL DEFAULT 'consolidate_v1',
 source_event_ids_json TEXT NOT NULL DEFAULT '[]', metadata_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL)`)
	}
	stmts = append(stmts, `CREATE TABLE IF NOT EXISTS memory_episodes (
 id TEXT PRIMARY KEY, session_id TEXT NOT NULL, run_id TEXT NOT NULL DEFAULT '', team_id TEXT NOT NULL DEFAULT '',
 agent_id TEXT NOT NULL DEFAULT '', l1_task_id TEXT NOT NULL DEFAULT '', episode_kind TEXT NOT NULL DEFAULT 'task',
 title TEXT NOT NULL, goal TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL DEFAULT '', outcome_summary TEXT NOT NULL DEFAULT '',
 result_preview TEXT NOT NULL DEFAULT '', failure_reason TEXT NOT NULL DEFAULT '', importance REAL NOT NULL DEFAULT 0.5,
 confidence REAL NOT NULL DEFAULT 0.7, user_feedback TEXT NOT NULL DEFAULT '', critic_score REAL NOT NULL DEFAULT -1,
 span_count INTEGER NOT NULL DEFAULT 0, message_count INTEGER NOT NULL DEFAULT 0, tool_call_count INTEGER NOT NULL DEFAULT 0,
 skill_call_count INTEGER NOT NULL DEFAULT 0, mcp_call_count INTEGER NOT NULL DEFAULT 0, total_tokens INTEGER NOT NULL DEFAULT 0,
 total_cost_micro_usd INTEGER NOT NULL DEFAULT 0, duration_ms INTEGER NOT NULL DEFAULT 0, l1_snapshot_json TEXT NOT NULL DEFAULT '{}',
 key_decisions_json TEXT NOT NULL DEFAULT '[]', key_artifacts_json TEXT NOT NULL DEFAULT '[]', embedding_status TEXT NOT NULL DEFAULT 'pending',
 embedding_model TEXT NOT NULL DEFAULT '', embedding_dim INTEGER NOT NULL DEFAULT 0, embedding_blob BLOB, embedding_norm REAL NOT NULL DEFAULT 0,
 consolidation_status TEXT NOT NULL DEFAULT 'pending', consolidated_at TEXT NOT NULL DEFAULT '', consolidated_l3_count INTEGER NOT NULL DEFAULT 0,
 consolidated_l4_count INTEGER NOT NULL DEFAULT 0, metadata_json TEXT NOT NULL DEFAULT '{}', started_at TEXT NOT NULL DEFAULT '',
 ended_at TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL, archived_at TEXT NOT NULL DEFAULT '',
 deleted_at TEXT NOT NULL DEFAULT '')`)
	for _, stmt := range stmts {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	return sessionmemory.NewStore(client, loggateway.NewNoop())
}

func TestUpsertFactsAndEpisodeBatch_LinksProvenance(t *testing.T) {
	store := openConsolidateBatchStore(t)
	ctx := context.Background()

	result, err := store.UpsertFactsAndEpisodeBatch(ctx, []sessionmemory.MemoryFactUpsert{
		{
			ScopeType: "agent", ScopeID: "agent-1", AgentID: "agent-1",
			Statement: "I prefer tea", SourceKind: "auto_memory",
			SourceSessionID: "sess-1", SourceMessageID: "msg-1",
		},
		{
			ScopeType: "agent", ScopeID: "agent-1", AgentID: "agent-1",
			Statement: "My name is Alice", SourceKind: "auto_memory",
			SourceSessionID: "sess-1", SourceMessageID: "msg-2",
		},
	}, &sessionmemory.EpisodeInsert{
		SessionID: "sess-1", AgentID: "agent-1", Title: "Consolidation",
		OutcomeSummary: "tea; Alice", ConsolidatedL3: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FactsWritten != 2 || len(result.FactRows) != 2 || len(result.EpisodeRow) == 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	var ep map[string]any
	if err := json.Unmarshal(result.EpisodeRow, &ep); err != nil {
		t.Fatal(err)
	}
	epID, _ := ep["id"].(string)
	if epID == "" {
		t.Fatal("episode id missing")
	}

	for _, raw := range result.FactRows {
		var fact map[string]any
		if err := json.Unmarshal(raw, &fact); err != nil {
			t.Fatal(err)
		}
		if got := fact["source_episode_id"]; got != epID {
			t.Fatalf("source_episode_id=%v want %s", got, epID)
		}
	}

	rows, total, _, _, err := store.ListFactRows(ctx, "agent", "agent-1", "", "active", "", 10, 0)
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("facts total=%d rows=%d err=%v", total, len(rows), err)
	}
	eps, err := store.ListEpisodeRowsForRecall(ctx, "agent-1", "sess-1", 5)
	if err != nil || len(eps) != 1 {
		t.Fatalf("episodes=%d err=%v", len(eps), err)
	}
}

func TestUpsertFactsAndEpisodeBatch_RollsBackOnEpisodeFailure(t *testing.T) {
	store := openConsolidateBatchStore(t)
	ctx := context.Background()

	_, err := store.UpsertFactsAndEpisodeBatch(ctx, []sessionmemory.MemoryFactUpsert{
		{
			ScopeType: "agent", ScopeID: "agent-2", AgentID: "agent-2",
			Statement: "Should not persist alone", SourceKind: "auto_memory",
		},
	}, &sessionmemory.EpisodeInsert{
		SessionID: "", Title: "missing session",
	})
	if err == nil {
		t.Fatal("expected episode validation error")
	}

	rows, total, _, _, err := store.ListFactRows(ctx, "agent", "agent-2", "", "active", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("expected rollback, got total=%d", total)
	}
}

func TestUpsertFactsAndEpisodeBatch_RejectsOrphanEpisode(t *testing.T) {
	store := openConsolidateBatchStore(t)
	ctx := context.Background()

	_, err := store.UpsertFactsAndEpisodeBatch(ctx, nil, &sessionmemory.EpisodeInsert{
		SessionID: "sess-1", Title: "orphan",
	})
	if err == nil {
		t.Fatal("expected error for episode without facts")
	}
}

func TestUpsertFactsAndEpisodeBatch_NonStrictAuditFailureStillCommits(t *testing.T) {
	store := openConsolidateBatchStoreNoActionLog(t)
	store.SetPolicyEngine(biz.NewMemoryPolicyEngineStatic(store, false))
	ctx := context.Background()

	result, err := store.UpsertFactsAndEpisodeBatch(ctx, []sessionmemory.MemoryFactUpsert{
		{
			ScopeType: "agent", ScopeID: "agent-3", AgentID: "agent-3",
			Statement: "Audit may fail", SourceKind: "auto_memory",
		},
	}, &sessionmemory.EpisodeInsert{
		SessionID: "sess-3", AgentID: "agent-3", Title: "ok",
	})
	if err != nil {
		t.Fatalf("non-strict batch should commit: %v", err)
	}
	if result.FactsWritten != 1 || len(result.EpisodeRow) == 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUpsertFactsAndEpisodeBatch_StrictAuditFailureRollsBack(t *testing.T) {
	store := openConsolidateBatchStoreNoActionLog(t)
	store.SetPolicyEngine(biz.NewMemoryPolicyEngineStatic(store, true))
	ctx := context.Background()

	_, err := store.UpsertFactsAndEpisodeBatch(ctx, []sessionmemory.MemoryFactUpsert{
		{
			ScopeType: "agent", ScopeID: "agent-4", AgentID: "agent-4",
			Statement: "Should rollback", SourceKind: "auto_memory",
		},
	}, &sessionmemory.EpisodeInsert{
		SessionID: "sess-4", AgentID: "agent-4", Title: "blocked",
	})
	if err == nil {
		t.Fatal("expected strict audit failure")
	}
	rows, total, _, _, err := store.ListFactRows(ctx, "agent", "agent-4", "", "active", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("expected rollback, got total=%d", total)
	}
}

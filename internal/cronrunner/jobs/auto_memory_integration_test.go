package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	sessionsess "aranea-agents/internal/biz/session"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/sessionmemory"
	memtrpc "aranea-agents/internal/memory/trpc"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
)

func openAutoMemoryIntegrationStore(t *testing.T) *sessionmemory.Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	ctx := context.Background()
	for _, stmt := range []string{
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
 pii_flag INTEGER NOT NULL DEFAULT 0, redacted_statement TEXT NOT NULL DEFAULT '',
 ttl_days INTEGER NOT NULL DEFAULT 0, decay_factor REAL NOT NULL DEFAULT 0.98, next_decay_at TEXT NOT NULL DEFAULT '',
 last_used_at TEXT NOT NULL DEFAULT '', expires_at TEXT NOT NULL DEFAULT '',
 metadata_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 archived_at TEXT NOT NULL DEFAULT '', deleted_at TEXT NOT NULL DEFAULT '',
 UNIQUE(scope_type, scope_id, fingerprint))`,
		`CREATE TABLE IF NOT EXISTS memory_action_log (
 id TEXT PRIMARY KEY, action TEXT NOT NULL, target_kind TEXT NOT NULL, target_id TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '', policy_version TEXT NOT NULL DEFAULT 'consolidate_v1',
 source_event_ids_json TEXT NOT NULL DEFAULT '[]', metadata_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS memory_episodes (
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
 deleted_at TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	return sessionmemory.NewStore(client)
}

func TestAutoMemoryWorker_IntegrationExtractChain(t *testing.T) {
	store := openAutoMemoryIntegrationStore(t)
	ctx := context.Background()

	const (
		sessID  = "sess-int-1"
		agentID = "agent-int-1"
		userID  = "user-int-1"
		msgID   = "msg-u-1"
	)

	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: sessID, AgentID: agentID, UserID: userID},
		msgs: []sessionsess.ChatMessage{{
			ID: msgID, SessionID: sessID, Role: "user", ContentMarkdown: "I prefer dark mode",
		}},
	}
	sessionsUC := biz.NewSessionUsecase(repo, nil, nil, nil)
	q := memtrpc.NewMemoryJobQueue(4, 0)
	w := NewAutoMemoryWorker(0, sessionsUC, nil, store, nil, nil, nil, biz.NewHeuristicConsolidator(), q)

	req := memtrpc.AutoMemoryJobRequest{SessionID: sessID, UserID: userID, AppName: agentID}
	if err := w.extract(ctx, req); err != nil {
		t.Fatalf("extract: %v", err)
	}

	rows, total, _, _, err := store.ListFactRows(ctx, "agent", agentID, "", "active", "", 20, 0)
	if err != nil {
		t.Fatalf("ListFactRows: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 fact, got total=%d rows=%d", total, len(rows))
	}
	var fact map[string]any
	if err := json.Unmarshal(rows[0], &fact); err != nil {
		t.Fatal(err)
	}
	if got := fact["source_kind"]; got != "auto_memory" {
		t.Fatalf("source_kind=%v", got)
	}
	if got := fact["source_message_id"]; got != msgID {
		t.Fatalf("source_message_id=%v want %s", got, msgID)
	}

	eps, err := store.ListEpisodeRowsForRecall(ctx, agentID, sessID, 5)
	if err != nil {
		t.Fatalf("ListEpisodeRowsForRecall: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(eps))
	}
	var ep map[string]any
	if err := json.Unmarshal(eps[0], &ep); err != nil {
		t.Fatal(err)
	}
	if got, _ := ep["consolidated_l3_count"].(float64); int(got) != 1 {
		t.Fatalf("consolidated_l3_count=%v", ep["consolidated_l3_count"])
	}
}

func TestAutoMemoryWorker_DrainUsesInjectedQueue(t *testing.T) {
	store := openAutoMemoryIntegrationStore(t)
	ctx := context.Background()

	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: "sess-q-1", AgentID: "agent-q-1", UserID: "user-q-1"},
		msgs: []sessionsess.ChatMessage{{
			ID: "m1", SessionID: "sess-q-1", Role: "user", ContentMarkdown: "My name is Alice",
		}},
	}
	sessionsUC := biz.NewSessionUsecase(repo, nil, nil, nil)
	q := memtrpc.NewMemoryJobQueue(4, 0)
	w := NewAutoMemoryWorker(0, sessionsUC, nil, store, nil, nil, nil, biz.NewHeuristicConsolidator(), q)

	q.Enqueue(memtrpc.AutoMemoryJobRequest{SessionID: "sess-q-1", UserID: "user-q-1", AppName: "agent-q-1"})
	w.drain(ctx)

	rows, total, _, _, err := store.ListFactRows(ctx, "agent", "agent-q-1", "", "active", "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected fact after queue drain, got total=%d", total)
	}
}

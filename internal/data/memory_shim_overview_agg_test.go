package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// P2-1/P2-2（2026-10-18 审计）数据层契约：L0/L1 全景卡 SQL 聚合的口径必须
// 与被替代的行扫描路径一致——
//
//	L0：session(+agent) 内快照总数与今日新增（created_at >= 当日 00:00 UTC）；
//	L1：仅 status='active' 任务计数（镜像 biz 调用 status="active"），字段口径
//	    镜像 listL1FieldRows(includeInternal=false, includeExpired=false)：
//	    visibility != 'internal' 且未过期。

const overviewAggTestDDL = `
CREATE TABLE memory_l0_assembly_snapshots (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL DEFAULT '',
  span_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  context_window_tokens INTEGER NOT NULL DEFAULT 0,
  budget_tokens INTEGER NOT NULL DEFAULT 0,
  recent_window_turns INTEGER NOT NULL DEFAULT 0,
  recent_window_tokens INTEGER NOT NULL DEFAULT 0,
  summary_token_estimate INTEGER NOT NULL DEFAULT 0,
  l1_field_count INTEGER NOT NULL DEFAULT 0,
  l1_token_estimate INTEGER NOT NULL DEFAULT 0,
  l3_chunk_count INTEGER NOT NULL DEFAULT 0,
  l3_token_estimate INTEGER NOT NULL DEFAULT 0,
  l4_path_count INTEGER NOT NULL DEFAULT 0,
  l4_token_estimate INTEGER NOT NULL DEFAULT 0,
  prompt_token_estimate INTEGER NOT NULL DEFAULT 0,
  prompt_token_actual INTEGER NOT NULL DEFAULT 0,
  used_ratio REAL NOT NULL DEFAULT 0,
  truncate_strategy TEXT NOT NULL DEFAULT '',
  truncated_message_count INTEGER NOT NULL DEFAULT 0,
  summarized_turn_from INTEGER NOT NULL DEFAULT 0,
  summarized_turn_to INTEGER NOT NULL DEFAULT 0,
  segments_json TEXT NOT NULL DEFAULT '[]',
  warning_codes_json TEXT NOT NULL DEFAULT '[]',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE TABLE memory_l1_tasks (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  team_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  task_key TEXT NOT NULL DEFAULT '',
  task_title TEXT NOT NULL DEFAULT '',
  task_goal TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  schema_version INTEGER NOT NULL DEFAULT 1,
  budget_tokens INTEGER NOT NULL DEFAULT 8192,
  used_tokens INTEGER NOT NULL DEFAULT 0,
  parent_task_id TEXT NOT NULL DEFAULT '',
  shared_with_json TEXT NOT NULL DEFAULT '[]',
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  archived_at TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(session_id, task_key, agent_id)
);
CREATE TABLE memory_l1_fields (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  field_path TEXT NOT NULL,
  field_kind TEXT NOT NULL DEFAULT 'string',
  visibility TEXT NOT NULL DEFAULT 'prompt',
  pin_to_prompt INTEGER NOT NULL DEFAULT 1,
  is_required INTEGER NOT NULL DEFAULT 0,
  value_text TEXT NOT NULL DEFAULT '',
  value_json TEXT NOT NULL DEFAULT '',
  value_ref TEXT NOT NULL DEFAULT '',
  preview TEXT NOT NULL DEFAULT '',
  token_estimate INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT 'agent',
  source_ref TEXT NOT NULL DEFAULT '',
  ttl_seconds INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL DEFAULT 1,
  last_read_at TEXT NOT NULL DEFAULT '',
  read_count INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(task_id, field_path)
)`

func setupOverviewAggDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	if _, err := db.ExecContext(context.Background(), overviewAggTestDDL); err != nil {
		t.Fatalf("create overview agg tables: %v", err)
	}
	return db
}

func overviewAggData(db *sql.DB) *Data {
	return &Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
}

func insertAggL0Snapshot(t *testing.T, db *sql.DB, id, sessionID, agentID, createdAt string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO memory_l0_assembly_snapshots (id, session_id, agent_id, created_at) VALUES ($1, $2, $3, $4)`,
		id, sessionID, agentID, createdAt)
	if err != nil {
		t.Fatalf("insert l0 snapshot %s: %v", id, err)
	}
}

func insertAggL1Task(t *testing.T, db *sql.DB, id, sessionID, agentID, status, createdAt string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO memory_l1_tasks (id, session_id, agent_id, task_key, status, started_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6, $6)`,
		id, sessionID, agentID, "k-"+id, status, createdAt)
	if err != nil {
		t.Fatalf("insert l1 task %s: %v", id, err)
	}
}

func insertAggL1Field(t *testing.T, db *sql.DB, id, taskID, sessionID, visibility, expiresAt string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO memory_l1_fields (id, task_id, session_id, field_path, visibility, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, '2026-08-16T00:00:00Z', '2026-08-16T00:00:00Z')`,
		id, taskID, sessionID, "p-"+id, visibility, expiresAt)
	if err != nil {
		t.Fatalf("insert l1 field %s: %v", id, err)
	}
}

func TestAggregateL0Overview_CountsBySessionAndAgent(t *testing.T) {
	db := setupOverviewAggDB(t)
	repo := newL0SnapshotRepo(overviewAggData(db))

	now := time.Now().UTC()
	today := now.Format(time.RFC3339Nano)
	yesterday := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)
	todayStart := now.Truncate(24 * time.Hour).Format(time.RFC3339Nano)

	insertAggL0Snapshot(t, db, "s1", "sess-agg", "ag-a", today)
	insertAggL0Snapshot(t, db, "s2", "sess-agg", "ag-a", yesterday)
	insertAggL0Snapshot(t, db, "s3", "sess-agg", "ag-b", today)
	insertAggL0Snapshot(t, db, "s4", "sess-other", "ag-a", today)

	t.Run("session + agent filter", func(t *testing.T) {
		agg, err := repo.AggregateL0Overview(context.Background(), "sess-agg", "ag-a", todayStart)
		if err != nil {
			t.Fatalf("AggregateL0Overview: %v", err)
		}
		if agg.ItemCount != 2 || agg.TodayAdded != 1 {
			t.Fatalf("agg = %+v, want item=2 today=1 (s1 today + s2 yesterday)", agg)
		}
	})

	t.Run("session wide when agent empty", func(t *testing.T) {
		agg, err := repo.AggregateL0Overview(context.Background(), "sess-agg", "", todayStart)
		if err != nil {
			t.Fatalf("AggregateL0Overview: %v", err)
		}
		if agg.ItemCount != 3 || agg.TodayAdded != 2 {
			t.Fatalf("agg = %+v, want item=3 today=2 (s1+s2+s3, s1+s3 today)", agg)
		}
	})

	t.Run("empty session is zero not error", func(t *testing.T) {
		agg, err := repo.AggregateL0Overview(context.Background(), " ", "ag-a", todayStart)
		if err != nil {
			t.Fatalf("AggregateL0Overview: %v", err)
		}
		if agg.ItemCount != 0 || agg.TodayAdded != 0 {
			t.Fatalf("agg = %+v, want zero", agg)
		}
	})
}

func TestAggregateL1Overview_TaskAndFieldSemantics(t *testing.T) {
	db := setupOverviewAggDB(t)
	repo := newL1WorkingMemoryRepo(overviewAggData(db))

	now := time.Now().UTC()
	today := now.Format(time.RFC3339Nano)
	yesterday := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)
	todayStart := now.Truncate(24 * time.Hour).Format(time.RFC3339Nano)

	// t1 今日 active / t2 昨日 active / t3 paused（不计）/ t4 他 session / t5 他 agent active
	insertAggL1Task(t, db, "t1", "sess-agg", "ag-a", "active", today)
	insertAggL1Task(t, db, "t2", "sess-agg", "ag-a", "active", yesterday)
	insertAggL1Task(t, db, "t3", "sess-agg", "ag-a", "paused", today)
	insertAggL1Task(t, db, "t4", "sess-other", "ag-a", "active", today)
	insertAggL1Task(t, db, "t5", "sess-agg", "ag-b", "active", today)

	insertAggL1Field(t, db, "f1", "t1", "sess-agg", "prompt", "")                      // 计
	insertAggL1Field(t, db, "f2", "t1", "sess-agg", "internal", "")                    // internal → 弃
	insertAggL1Field(t, db, "f3", "t1", "sess-agg", "prompt", "2020-01-01T00:00:00Z")  // 已过期 → 弃
	insertAggL1Field(t, db, "f4", "t1", "sess-agg", "prompt", "2999-01-01T00:00:00Z")  // 未过期 → 计
	insertAggL1Field(t, db, "f5", "t2", "sess-agg", "prompt", "")                      // 计
	insertAggL1Field(t, db, "f6", "t3", "sess-agg", "prompt", "")                      // 宿主非 active → 弃

	t.Run("active only with field semantics", func(t *testing.T) {
		agg, err := repo.AggregateL1Overview(context.Background(), "sess-agg", "ag-a", todayStart)
		if err != nil {
			t.Fatalf("AggregateL1Overview: %v", err)
		}
		if agg.ItemCount != 2 || agg.TodayAdded != 1 {
			t.Fatalf("tasks: got item=%d today=%d, want 2/1 (t1+t2, t1 today; t3 paused excluded)", agg.ItemCount, agg.TodayAdded)
		}
		if agg.FieldCount != 3 {
			t.Fatalf("fields: got %d, want 3 (f1+f4+f5; internal/expired/paused-host excluded)", agg.FieldCount)
		}
	})

	t.Run("agent filter narrows fields too", func(t *testing.T) {
		agg, err := repo.AggregateL1Overview(context.Background(), "sess-agg", "ag-b", todayStart)
		if err != nil {
			t.Fatalf("AggregateL1Overview: %v", err)
		}
		if agg.ItemCount != 1 || agg.TodayAdded != 1 || agg.FieldCount != 0 {
			t.Fatalf("agg = %+v, want item=1 today=1 fields=0 (t5 only)", agg)
		}
	})

	t.Run("empty agent counts whole session", func(t *testing.T) {
		agg, err := repo.AggregateL1Overview(context.Background(), "sess-agg", "", todayStart)
		if err != nil {
			t.Fatalf("AggregateL1Overview: %v", err)
		}
		if agg.ItemCount != 3 || agg.TodayAdded != 2 {
			t.Fatalf("tasks: got item=%d today=%d, want 3/2 (t1+t2+t5)", agg.ItemCount, agg.TodayAdded)
		}
	})
}

package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// EnsureSourceSyncSchema —— G-38 SourceSync 四表双轨 Ensure（§G-38 ③ 双轨
// 铁律：迁移轨道 20261298–20261301 与 Ensure 轨道同 commit，列集合与 SQL
// 字节级一致）。启动幂等跑（reconcileVersionedEnsureSchemas 家族），fresh 库
// 一键建齐、存量库重跑安全。索引一并 IF NOT EXISTS（CONCURRENTLY 不适合
// Ensure 轨道——构建中断残留 INVALID 索引需另行治理，四表为空表场景全量
// 成本趋零）。
const ensureSourceSyncSchemaDDL = `
CREATE TABLE IF NOT EXISTS knowledge_source_connectors (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('shared_dir', 'notion')),
  name TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  collection_id TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '{}',
  interval_seconds INTEGER NOT NULL DEFAULT 900,
  enabled BOOLEAN NOT NULL DEFAULT false,
  sync_state TEXT NOT NULL DEFAULT 'idle' CHECK (sync_state IN ('idle', 'running', 'auth_paused', 'error')),
  cursor TEXT NOT NULL DEFAULT '',
  next_run_at TEXT NOT NULL DEFAULT '',
  last_run_at TEXT NOT NULL DEFAULT '',
  last_success_at TEXT NOT NULL DEFAULT '',
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  max_items_per_run INTEGER NOT NULL DEFAULT 500,
  max_deletes_per_run INTEGER NOT NULL DEFAULT 200,
  max_fetch_bytes_per_run BIGINT NOT NULL DEFAULT 536870912,
  run_token TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS knowledge_source_connector_items (
  connector_id TEXT NOT NULL DEFAULT '',
  external_id TEXT NOT NULL DEFAULT '',
  document_id TEXT NOT NULL DEFAULT '',
  fingerprint TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  own_lifecycle BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'missing', 'deleted')),
  miss_count INTEGER NOT NULL DEFAULT 0,
  last_seen_run_id TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  first_seen_at TEXT NOT NULL DEFAULT '',
  last_seen_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (connector_id, external_id)
);
CREATE TABLE IF NOT EXISTS knowledge_source_connector_credential (
  id TEXT PRIMARY KEY,
  connector_id TEXT NOT NULL DEFAULT '',
  credential_key TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  secret_ref TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS knowledge_source_connector_runs (
  id TEXT PRIMARY KEY,
  connector_id TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('success', 'partial', 'failed', 'skipped')),
  created INTEGER NOT NULL DEFAULT 0,
  updated INTEGER NOT NULL DEFAULT 0,
  unchanged INTEGER NOT NULL DEFAULT 0,
  fetch_failed INTEGER NOT NULL DEFAULT 0,
  deleted INTEGER NOT NULL DEFAULT 0,
  miss_unknown INTEGER NOT NULL DEFAULT 0,
  bytes_pulled BIGINT NOT NULL DEFAULT 0,
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_src_connectors_due
  ON knowledge_source_connectors (next_run_at)
  WHERE enabled AND deleted_at = '';
CREATE INDEX IF NOT EXISTS idx_src_connectors_workspace
  ON knowledge_source_connectors (workspace_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_src_connectors_collection_active
  ON knowledge_source_connectors (collection_id)
  WHERE deleted_at = '';
CREATE INDEX IF NOT EXISTS idx_src_items_missing
  ON knowledge_source_connector_items (connector_id)
  WHERE status = 'missing';
CREATE INDEX IF NOT EXISTS idx_src_items_document
  ON knowledge_source_connector_items (document_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_src_credential_active
  ON knowledge_source_connector_credential (connector_id, credential_key)
  WHERE deleted_at = '';
CREATE INDEX IF NOT EXISTS idx_src_credential_connector
  ON knowledge_source_connector_credential (connector_id);
CREATE INDEX IF NOT EXISTS idx_src_runs_conn
  ON knowledge_source_connector_runs (connector_id, started_at DESC);
`

// EnsureSourceSyncSchema 运行四表 DDL（nil-safe；多语句单 Exec 走
// 简单查询协议，channel_turn_job_schema 先例对齐）。
func EnsureSourceSyncSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	if _, err := db.ExecContext(ctx, ensureSourceSyncSchemaDDL); err != nil {
		return fmt.Errorf("ensure knowledge_source_sync schema: %w", err)
	}
	return nil
}

// sourceSyncTableColumnSet 钉阵用：四表列集合黄金表（出现新列=迁移与
// Ensure 与黄金表三处同改，缺一处即红）。顺序与写侧 INSERT 列序钉死一致。
var sourceSyncTableColumnSet = map[string][]string{
	"knowledge_source_connectors": {
		"id", "kind", "name", "workspace_id", "collection_id", "config_json",
		"interval_seconds", "enabled", "sync_state", "cursor",
		"next_run_at", "last_run_at", "last_success_at",
		"consecutive_failures", "error_code", "error_message",
		"max_items_per_run", "max_deletes_per_run", "max_fetch_bytes_per_run",
		"run_token", "created_at", "updated_at", "deleted_at",
	},
	"knowledge_source_connector_items": {
		"connector_id", "external_id", "document_id", "fingerprint",
		"content_hash", "own_lifecycle", "status", "miss_count",
		"last_seen_run_id", "error_message", "first_seen_at", "last_seen_at",
		"deleted_at",
	},
	"knowledge_source_connector_credential": {
		"id", "connector_id", "credential_key", "status", "secret_ref",
		"metadata_json", "created_at", "updated_at", "deleted_at",
	},
	"knowledge_source_connector_runs": {
		"id", "connector_id", "started_at", "finished_at", "status",
		"created", "updated", "unchanged", "fetch_failed", "deleted",
		"miss_unknown", "bytes_pulled", "error_code", "error_message",
	},
}

// sourceSyncTables 四表声明序（钉阵遍历唯一取值源）。
var sourceSyncTables = []string{
	"knowledge_source_connectors",
	"knowledge_source_connector_items",
	"knowledge_source_connector_credential",
	"knowledge_source_connector_runs",
}

// assertSourceSyncColumnSet PG 钉阵辅助：断言 information_schema 列集合与
// 黄金表字节一致（长度+逐项），返回首处失配描述（空串=全等）。
func assertSourceSyncColumnSet(ctx context.Context, db *sql.DB, table string, want []string) (string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT column_name FROM information_schema.columns
WHERE table_name = $1 AND table_schema = current_schema()
ORDER BY ordinal_position`, table)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(got) != len(want) {
		return fmt.Sprintf("%s column count %d want %d (got %s)", table, len(got), len(want), strings.Join(got, ",")), nil
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Sprintf("%s column[%d]=%s want %s", table, i, got[i], want[i]), nil
		}
	}
	return "", nil
}

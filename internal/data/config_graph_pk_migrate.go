package data

import (
	"context"
	"database/sql"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// ddlConfigGraphCompositePK 修复 20261260 的主键设计缺陷（M81 P1-3 回放实测
// 发现）：节点/边 ID 是 (type,ref)/(src,dst,type) 的确定性 uuid5——跨代不变；
// 而 generation 双代切换设计要求新旧两代共存（重建写新代，旧代保留 1 代供
// 对账，异步清理）。原 PRIMARY KEY(id) 使同 id 无法跨代共存，第二次全量重建
// 必现 23505 duplicate key（config_graph_nodes_pkey），幂等重建破产。
//
// 修复：两表主键改为 (id, generation)。真实唯一键仍是
// uq_config_graph_nodes_ref(node_type, ref_id, generation) /
// uq_config_graph_edges(src_id, dst_id, edge_type, generation)，UPSERT 冲突
// 目标不变。
//
// 幂等：
//   - Postgres：DROP CONSTRAINT IF EXISTS + ADD CONSTRAINT，重跑先删后建恒成立。
//   - SQLite：不支持 DROP CONSTRAINT，整表重建（_new 复制 → drop → rename +
//     重建索引），重跑同样成立（每次把现数据复制进全新 _new）。
func ddlConfigGraphCompositePK(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return fmt.Errorf("config graph composite pk: rawDB is nil")
	}
	var stmts []string
	if d.IsPostgres() {
		stmts = []string{
			`ALTER TABLE config_graph_nodes DROP CONSTRAINT IF EXISTS config_graph_nodes_pkey`,
			`ALTER TABLE config_graph_nodes ADD CONSTRAINT config_graph_nodes_pkey PRIMARY KEY (id, generation)`,
			`ALTER TABLE config_graph_edges DROP CONSTRAINT IF EXISTS config_graph_edges_pkey`,
			`ALTER TABLE config_graph_edges ADD CONSTRAINT config_graph_edges_pkey PRIMARY KEY (id, generation)`,
		}
	} else {
		stmts = []string{
			`CREATE TABLE IF NOT EXISTS config_graph_nodes_new (
  id TEXT NOT NULL,
  node_type TEXT NOT NULL,
  ref_id TEXT NOT NULL DEFAULT '',
  node_key TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  attrs_json TEXT NOT NULL DEFAULT '{}',
  generation INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (id, generation)
)`,
			`INSERT OR IGNORE INTO config_graph_nodes_new (id, node_type, ref_id, node_key, display_name, workspace_id, status, attrs_json, generation, created_at, updated_at)
  SELECT id, node_type, ref_id, node_key, display_name, workspace_id, status, attrs_json, generation, created_at, updated_at FROM config_graph_nodes`,
			`DROP TABLE config_graph_nodes`,
			`ALTER TABLE config_graph_nodes_new RENAME TO config_graph_nodes`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uq_config_graph_nodes_ref ON config_graph_nodes(node_type, ref_id, generation)`,
			`CREATE INDEX IF NOT EXISTS idx_config_graph_nodes_type_status ON config_graph_nodes(node_type, status, generation)`,
			`CREATE INDEX IF NOT EXISTS idx_config_graph_nodes_key ON config_graph_nodes(node_key)`,
			`CREATE INDEX IF NOT EXISTS idx_config_graph_nodes_ws ON config_graph_nodes(workspace_id)`,
			`CREATE TABLE IF NOT EXISTS config_graph_edges_new (
  id TEXT NOT NULL,
  src_id TEXT NOT NULL DEFAULT '',
  dst_id TEXT NOT NULL DEFAULT '',
  edge_type TEXT NOT NULL,
  evidence_json TEXT NOT NULL DEFAULT '{}',
  workspace_id TEXT NOT NULL DEFAULT '',
  generation INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (id, generation)
)`,
			`INSERT OR IGNORE INTO config_graph_edges_new (id, src_id, dst_id, edge_type, evidence_json, workspace_id, generation, created_at)
  SELECT id, src_id, dst_id, edge_type, evidence_json, workspace_id, generation, created_at FROM config_graph_edges`,
			`DROP TABLE config_graph_edges`,
			`ALTER TABLE config_graph_edges_new RENAME TO config_graph_edges`,
			`CREATE UNIQUE INDEX IF NOT EXISTS uq_config_graph_edges ON config_graph_edges(src_id, dst_id, edge_type, generation)`,
			`CREATE INDEX IF NOT EXISTS idx_config_graph_edges_src ON config_graph_edges(src_id, generation)`,
			`CREATE INDEX IF NOT EXISTS idx_config_graph_edges_dst ON config_graph_edges(dst_id, generation)`,
		}
	}
	// 与 executeSQLFileWithDialect 同款：逐语句独立事务（PG 出错会污染连接），
	// already-exists / undefined-object 视为幂等跳过。
	for _, stmt := range stmts {
		tx, err := rawDB.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("config graph composite pk: begin tx: %w", err)
		}
		_, err = tx.ExecContext(ctx, stmt)
		if err != nil {
			_ = tx.Rollback()
			if d.AlreadyExistsErr(err) || d.UndefinedObjectErr(err) {
				lg.Debug("config graph composite pk: statement skipped (idempotent)",
					loggateway.StepID("data.ddl_migration.config_graph_pk"),
					loggateway.Err(err))
				continue
			}
			return fmt.Errorf("config graph composite pk: %w\n---\n%s", err, stmt)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("config graph composite pk: commit: %w", err)
		}
	}
	lg.Info("config graph composite primary key applied",
		loggateway.StepID("data.ddl_migration.config_graph_pk"),
		loggateway.Str("dialect", string(d)))
	return nil
}

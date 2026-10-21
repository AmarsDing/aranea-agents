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
// 幂等：DROP CONSTRAINT IF EXISTS + ADD CONSTRAINT，重跑先删后建恒成立。
func ddlConfigGraphCompositePK(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return fmt.Errorf("config graph composite pk: rawDB is nil")
	}
	stmts := []string{
		`ALTER TABLE config_graph_nodes DROP CONSTRAINT IF EXISTS config_graph_nodes_pkey`,
		`ALTER TABLE config_graph_nodes ADD CONSTRAINT config_graph_nodes_pkey PRIMARY KEY (id, generation)`,
		`ALTER TABLE config_graph_edges DROP CONSTRAINT IF EXISTS config_graph_edges_pkey`,
		`ALTER TABLE config_graph_edges ADD CONSTRAINT config_graph_edges_pkey PRIMARY KEY (id, generation)`,
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

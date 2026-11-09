package data

import (
	"context"
	"database/sql"
)

// 评审 2026-11-09 M2:死信幂等库层兜底(部分唯一索引)的启动期 reconcile。
//
// 与迁移 sql/migrations/20261310_ceoboard_dead_letters_unreplayed_ref_uidx.sql
// 同体两条语句:先清理存量重复,再建部分唯一索引。版本化迁移只跑一次,而 DEV
// 模式(DEPLOY_ENV=dev)ent auto-migrate 带 WithDropIndex(true),会把 ent model
// 之外的索引抹掉——本函数挂在 reconcileVersionedEnsureSchemas 上,每次启动幂等
// 重建,使存量/开发库收敛到代码当前确保的索引集(与 EnsureKnowledgeSchema 等的
// 启动期 reconcile 先例对齐)。
//
// 幂等(清理谓词自幂等 + IF NOT EXISTS)、双方言(pg/sqlite 语法一致)、
// nil-safe。ceoboard_dead_letters 由 ent Schema.Create 先行建立(L1→L2 顺序),
// 表缺失按 UndefinedObjectErr 容错跳过(与执行器既有容错哲学一致)。
func EnsureCeoboardDeadLetterIdemSchema(ctx context.Context, rawDB *sql.DB, d Dialect) error {
	if rawDB == nil {
		return nil
	}
	stmts := []string{
		// ① 每组 (kind, source, ref_key) 未重放行保留 id 最大者(与 AddDeadLetter
		//    查重 ORDER BY id DESC LIMIT 1 的"最新优先"语义一致)。
		`DELETE FROM ceoboard_dead_letters
		   WHERE "ref_key" <> '' AND "replayed_at" IS NULL
		     AND "id" NOT IN (
		       SELECT MAX("id") FROM ceoboard_dead_letters
		        WHERE "ref_key" <> '' AND "replayed_at" IS NULL
		        GROUP BY "kind", "source", "ref_key"
		     )`,
		// ② 部分唯一索引:ref_key=''(无业务键可关联)与已重放行不参与幂等。
		`CREATE UNIQUE INDEX IF NOT EXISTS ceoboard_dead_letters_unreplayed_ref_uidx
		   ON ceoboard_dead_letters ("kind", "source", "ref_key")
		   WHERE "ref_key" <> '' AND "replayed_at" IS NULL`,
	}
	for _, stmt := range stmts {
		if _, err := rawDB.ExecContext(ctx, stmt); err != nil {
			if d.UndefinedObjectErr(err) {
				// 表尚未由 ent 创建(异常顺序),交由下次启动收敛。
				return nil
			}
			return err
		}
	}
	return nil
}

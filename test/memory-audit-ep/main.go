// 一次性排查脚本：核查 memory_episodes 唯一索引与 L1 归档状态（2026-07-29）
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Println("ping:", err)
		os.Exit(1)
	}

	queries := []struct{ title, q string }{
		{"== memory_episodes 实际索引 ==", `SELECT indexname, indexdef FROM pg_indexes WHERE tablename='memory_episodes' ORDER BY indexname`},
		{"== schema_migrations 碰撞版本（4 组） ==", `SELECT version, name, applied_at FROM schema_migrations WHERE version IN (20260624,20260802,20260803,20261115) ORDER BY version`},
		{"== schema_migrations 20261116~20261120 占用情况 ==", `SELECT version, name, applied_at FROM schema_migrations WHERE version BETWEEN 20261116 AND 20261120 ORDER BY version`},
		{"== schema_migrations 20260720/20260729 占用情况 ==", `SELECT version, name, applied_at FROM schema_migrations WHERE version IN (20260720,20260729) ORDER BY version`},
		{"== cascade_saga_steps 表是否存在 ==", `SELECT table_name FROM information_schema.tables WHERE table_name IN ('cascade_saga_steps','self_improvement_runs')`},
		{"== cascade_saga_steps.id 列类型 ==", `SELECT column_name, data_type FROM information_schema.columns WHERE table_name='cascade_saga_steps' AND column_name='id'`},
		{"== self_improvement observing 索引 ==", `SELECT indexname, indexdef FROM pg_indexes WHERE tablename='self_improvement_runs'`},
		{"== team_copy_ownership 待回填行数 ==", `SELECT count(*) FROM teams WHERE team_key LIKE '%-copy-%' AND kind IN ('system_builtin','ecosystem_preset','marketplace','certified')`},
		{"== cascade saga_id 列类型（验证 20260803 DDL 是否生效） ==", `SELECT table_name, column_name, data_type FROM information_schema.columns WHERE column_name = 'saga_id' ORDER BY table_name`},
		{"== monitor_trace interrupted 待回填量（验证 20261115 数据迁移是否被跳过） ==", `SELECT count(*) FROM monitor_traces WHERE status = 'running' AND updated_at < now() - interval '1 day'`},
		{"== memory_l1_tasks 归档状态 ==", `SELECT agent_id, status, count(*), count(*) FILTER (WHERE archived_at != '' AND archived_at IS NOT NULL) archived FROM memory_l1_tasks GROUP BY 1,2 ORDER BY 1`},
		{"== memory_episodes 全表 ==", `SELECT count(*) FROM memory_episodes`},
		{"== memory_l1_fields 明细 ==", `SELECT task_id, field_path, left(COALESCE(content,''),40) content, visibility, COALESCE(expires_at,'') exp FROM memory_l1_fields LIMIT 10`},
	}
	for _, it := range queries {
		fmt.Println("\n" + it.title)
		rows, err := db.Query(it.q)
		if err != nil {
			fmt.Println("  ERR:", err)
			continue
		}
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		n := 0
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				fmt.Println("  scan ERR:", err)
				break
			}
			fmt.Print("  ")
			for i, c := range cols {
				fmt.Printf("%s=%v  ", c, vals[i])
			}
			fmt.Println()
			n++
		}
		if n == 0 {
			fmt.Println("  (0 rows)")
		}
		rows.Close()
	}
}

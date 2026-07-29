// 一次性排查脚本：核查记忆 L0-L4 各表数据分布（2026-07-29 记忆中心"精灵助手使用多次但无内容"排查）
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
		{"== agent_runtime_settings 精灵助手记忆开关 ==", `SELECT agent_id, memory_enabled, l1_enabled, l2_recall_enabled, l2_episode_enabled, l3_enabled, l4_enabled, l0_snapshot_enabled, l0_snapshot_mode, l0_inject_l1, l0_inject_l3, l0_inject_l4 FROM agent_runtime_settings WHERE agent_id IN ('agent___spirit__','agent___system_admin__','agent___memory__')`},
		{"== agent_runtime_settings 全表统计 ==", `SELECT count(*) total, count(*) FILTER (WHERE memory_enabled) mem_on, count(*) FILTER (WHERE l2_episode_enabled) l2ep_on, count(*) FILTER (WHERE l4_enabled) l4_on, count(*) FILTER (WHERE l0_snapshot_enabled) l0snap_on FROM agent_runtime_settings`},
		{"== 精灵助手 4 条 facts 明细 ==", `SELECT id, left(statement,80) stmt, use_count, hit_count, source_kind, status, created_at, updated_at FROM memory_facts WHERE scope_id='agent___spirit__' ORDER BY use_count DESC`},
		{"== memory_facts 按 scope 分布 ==", `SELECT scope_type, scope_id, count(*) n, sum(hit_count) hits, sum(use_count) uses, count(*) FILTER (WHERE status='active') active FROM memory_facts GROUP BY 1,2 ORDER BY n DESC LIMIT 20`},
		{"== memory_facts 按 agent_id 分布 ==", `SELECT COALESCE(agent_id,'<null>'), count(*) n, sum(hit_count) hits FROM memory_facts GROUP BY 1 ORDER BY n DESC LIMIT 20`},
		{"== memory_facts 按 source_kind/status ==", `SELECT COALESCE(source_kind,'<null>'), COALESCE(status,'<null>'), count(*) FROM memory_facts GROUP BY 1,2 ORDER BY 3 DESC LIMIT 20`},
		{"== memory_episodes 按 agent 分布 ==", `SELECT COALESCE(agent_id,'<null>'), COALESCE(episode_kind,'<null>'), count(*) FROM memory_episodes GROUP BY 1,2 ORDER BY 3 DESC LIMIT 20`},
		{"== memory_entities 按 scope 分布 ==", `SELECT scope_type, scope_id, count(*) n, sum(use_count) uses FROM memory_entities GROUP BY 1,2 ORDER BY n DESC LIMIT 20`},
		{"== memory_relations 总量 ==", `SELECT count(*) FROM memory_relations`},
		{"== memory_l0_assembly_snapshots 按 agent 分布 ==", `SELECT COALESCE(agent_id,'<null>'), count(*) FROM memory_l0_assembly_snapshots GROUP BY 1 ORDER BY 2 DESC LIMIT 20`},
		{"== memory_l1_tasks 按 agent 分布 ==", `SELECT COALESCE(agent_id,'<null>'), COALESCE(status,'<null>'), count(*) FROM memory_l1_tasks GROUP BY 1,2 ORDER BY 3 DESC LIMIT 20`},
		{"== memory_l1_tasks 精灵助手明细 ==", `SELECT id, session_id, status, left(COALESCE(task_goal,''),60) goal, created_at, updated_at, ended_at FROM memory_l1_tasks WHERE agent_id='agent___spirit__' ORDER BY created_at DESC LIMIT 10`},
		{"== memory_l1_fields 总量 ==", `SELECT count(*) FROM memory_l1_fields`},
		{"== memory_action_log 最近10条 ==", `SELECT id, action, target, left(COALESCE(reason,''),50) reason, created_at FROM memory_action_log ORDER BY created_at DESC LIMIT 10`},
		{"== sessions 按 agent 分布(top10) ==", `SELECT COALESCE(agent_id,'<null>'), count(*) FROM sessions GROUP BY 1 ORDER BY 2 DESC LIMIT 10`},
		{"== agents 表中的精灵助手 ==", `SELECT id, agent_key, display_name, kind, status FROM agents WHERE agent_key LIKE '%spirit%' OR display_name LIKE '%精灵%' LIMIT 10`},
		{"== memory_action_log 总量/按action ==", `SELECT COALESCE(action,'<null>'), count(*) FROM memory_action_log GROUP BY 1 ORDER BY 2 DESC LIMIT 10`},
		{"== memory_job_deadletter 总量 ==", `SELECT count(*) FROM memory_job_deadletter`},
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

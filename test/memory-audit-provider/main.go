// 一次性排查脚本：核查精灵助手 LLM 提取器 provider/model 解析链（2026-07-29 记忆写入停滞排查）
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
		{"== 精灵助手 memory_worker / l0_compress provider-model 设置 ==", `SELECT agent_id, COALESCE(memory_worker_provider,'<empty>'), COALESCE(memory_worker_model,'<empty>'), COALESCE(l0_compress_provider,'<empty>'), COALESCE(l0_compress_model,'<empty>') FROM agent_runtime_settings WHERE agent_id='agent___spirit__'`},
		{"== agents 表 provider/model ==", `SELECT id, COALESCE(provider,'<empty>'), COALESCE(model,'<empty>') FROM agents WHERE id IN ('agent___spirit__','agent___system_admin__')`},
		{"== 精灵助手最近 sessions 的 default_provider/model ==", `SELECT id, COALESCE(default_provider,'<empty>'), COALESCE(default_model,'<empty>'), created_at FROM sessions WHERE agent_id='agent___spirit__' ORDER BY created_at DESC LIMIT 5`},
		{"== 最近自动记忆处理过的 sessions 归属 ==", `SELECT id, COALESCE(agent_id,'<empty>') agent, COALESCE(default_provider,'<empty>') dp, COALESCE(default_model,'<empty>') dm FROM sessions WHERE id IN ('85eff098-65b1-4c36-94fc-20c696cf48ae','85951724-213a-4de9-bfad-3611c207fd72','80d1c8c0-1455-4376-99ef-24e6fd3493ce','63a5662e-c4d3-40dd-a922-1783ae35759a','edbe2f4a-3c4d-4de5-ace0-f1b1da930a55','4fb82036-8ce4-48c1-bfe8-08094794d80f','ec86e351-88fc-4ffd-88d8-0ffce1e8af53')`},
		{"== llm_provider_models 可用模型数 ==", `SELECT count(*) total, count(*) FILTER (WHERE enabled) enabled FROM llm_provider_models`},
		{"== memory_facts 全表(确认只有4条) ==", `SELECT count(*) FROM memory_facts`},
		{"== memory_action_log 按 target_kind 分布 ==", `SELECT COALESCE(target_kind,'<null>'), COALESCE(action,'<null>'), count(*), max(created_at) latest FROM memory_action_log GROUP BY 1,2 ORDER BY 3 DESC LIMIT 15`},
		{"== 精灵助手最近 session 消息角色分布 ==", `SELECT m.role, count(*) FROM messages m JOIN sessions s ON s.id=m.session_id WHERE s.agent_id='agent___spirit__' GROUP BY 1 ORDER BY 2 DESC`},
		{"== 精灵助手最近 session 的 user 消息样例 ==", `SELECT left(m.content_markdown,120) FROM messages m JOIN sessions s ON s.id=m.session_id WHERE s.agent_id='agent___spirit__' AND m.role='user' ORDER BY m.created_at DESC LIMIT 5`},
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

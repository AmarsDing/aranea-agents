// 修正 join：rs.agent_id 关联 a.id
//go:build ignore

package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// 看一条 rs 记录的 agent_id 格式
	var sampleID string
	db.QueryRow(`SELECT agent_id FROM agent_runtime_settings LIMIT 1`).Scan(&sampleID)
	fmt.Println("sample rs.agent_id:", sampleID)

	// agents.id 格式
	var aid, akey string
	db.QueryRow(`SELECT id, agent_key FROM agents WHERE agent_key='ops_database'`).Scan(&aid, &akey)
	fmt.Println("agents.id for ops_database:", aid)

	fmt.Println("\n== ops agents tool settings (join on id) ==")
	rows, err := db.Query(`
		SELECT a.agent_key, a.display_name,
		       COALESCE(rs.tools_enabled::text,'null'),
		       COALESCE(rs.tools_profile,'null'),
		       COALESCE(rs.tools_allow_json::text,'null'),
		       COALESCE(rs.skill_load_mode,'null')
		FROM agents a
		LEFT JOIN agent_runtime_settings rs ON rs.agent_id = a.id
		WHERE a.agent_key LIKE 'ops\_%' OR a.agent_key IN ('__system_admin__','__spirit__')
		ORDER BY a.agent_key`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var k, n, en, tp, allow, slm string
		rows.Scan(&k, &n, &en, &tp, &allow, &slm)
		fmt.Printf("%-26s %-16s enabled=%-5s profile=%-12s skill_load=%s\n  allow=%s\n", k, n, en, tp, slm, trunc(allow, 300))
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

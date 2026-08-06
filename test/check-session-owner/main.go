// 一次性排查脚本：检查 sessions 表 user_id 分布及指定会话的归属
package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("\n== 目标会话 ==")
	rows, err := db.Query(`SELECT id, user_id, COALESCE(agent_id,''), created_at FROM sessions WHERE id IN ('6b56174d-e488-4335-9c6c-4d5e8341aa26','ec86e351-88fc-4ffd-88d8-0ffce1e8af53')`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var id, uid, aid, ca string
		if err := rows.Scan(&id, &uid, &aid, &ca); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("id=%s user_id=%q agent_id=%q created_at=%s\n", id, uid, aid, ca)
	}
	rows.Close()

	fmt.Println("\n== user_id 分布 ==")
	rows2, err := db.Query(`SELECT COALESCE(NULLIF(user_id,''),'<empty>') AS uid, count(*) FROM sessions GROUP BY user_id ORDER BY 2 DESC`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var uid string
		var n int
		if err := rows2.Scan(&uid, &n); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("user_id=%-10s count=%d\n", uid, n)
	}

	fmt.Println("\n== 空 user_id 会话的时间范围 ==")
	var minC, maxC sql.NullString
	if err := db.QueryRow(`SELECT min(created_at)::text, max(created_at)::text FROM sessions WHERE user_id=''`).Scan(&minC, &maxC); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("min=%v max=%v\n", minC.String, maxC.String)

	fmt.Println("\n== steps_v2 确认步骤状态 ==")
	rows3, err := db.Query(`SELECT id, kind, status, COALESCE(tool_name,'') FROM steps_v2 WHERE session_id='6b56174d-e488-4335-9c6c-4d5e8341aa26' ORDER BY id DESC LIMIT 12`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows3.Close()
	for rows3.Next() {
		var id, kind, status, tool string
		if err := rows3.Scan(&id, &kind, &status, &tool); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("id=%s kind=%s status=%s tool=%s\n", id, kind, status, tool)
	}

	fmt.Println("\n== 存量回填：空/default_user user_id → '1' ==")
	res, err := db.Exec(`UPDATE sessions SET user_id='1' WHERE user_id='' OR user_id='default_user'`)
	if err != nil {
		log.Fatal(err)
	}
	updated, _ := res.RowsAffected()
	fmt.Printf("updated %d sessions\n", updated)

	fmt.Println("\n== 回填后 user_id 分布 ==")
	rowsAfter, err := db.Query(`SELECT COALESCE(NULLIF(user_id,''),'<empty>') AS uid, count(*) FROM sessions GROUP BY user_id ORDER BY 2 DESC`)
	if err != nil {
		log.Fatal(err)
	}
	defer rowsAfter.Close()
	for rowsAfter.Next() {
		var uid string
		var n int
		if err := rowsAfter.Scan(&uid, &n); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("user_id=%-10s count=%d\n", uid, n)
	}

	fmt.Println("\n== tools 表 exec 相关工具的确认标记 ==")
	rows6, err := db.Query(`SELECT tool_key, risk_level, requires_confirmation, enabled, source FROM tools WHERE tool_key ILIKE '%exec%' OR tool_key ILIKE '%shell%' OR tool_key ILIKE '%hostexec%' OR tool_key ILIKE '%command%' ORDER BY tool_key`)
	if err != nil {
		fmt.Println("tools 查询失败:", err)
	} else {
		for rows6.Next() {
			var k, rl, src string
			var rc, en bool
			if err := rows6.Scan(&k, &rl, &rc, &en, &src); err != nil {
				log.Fatal(err)
			}
			fmt.Printf("tool_key=%s risk=%s requires_confirmation=%v enabled=%v source=%s\n", k, rl, rc, en, src)
		}
		rows6.Close()
	}
}

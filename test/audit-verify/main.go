// 一次性排查脚本：验证 audit_logs 规范化迁移结果与新审计点写入。
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
		fmt.Println("open:", err)
		return
	}
	defer db.Close()

	fmt.Println("== schema_migrations version=20260729 ==")
	rows, err := db.Query(`SELECT version, name FROM schema_migrations WHERE version = 20260729`)
	if err != nil {
		fmt.Println("migration query:", err)
	} else {
		for rows.Next() {
			var v int
			var n string
			rows.Scan(&v, &n)
			fmt.Printf("  %d %s\n", v, n)
		}
		rows.Close()
	}

	fmt.Println("== action 分布（最近 50 条） ==")
	rows, err = db.Query(`SELECT action, COUNT(*) FROM (SELECT action FROM audit_logs ORDER BY created_at DESC LIMIT 50) t GROUP BY action ORDER BY 2 DESC`)
	if err != nil {
		fmt.Println("dist query:", err)
	} else {
		for rows.Next() {
			var a string
			var c int
			rows.Scan(&a, &c)
			fmt.Printf("  %-32s %d\n", a, c)
		}
		rows.Close()
	}

	fmt.Println("== 非规范 action 残留（不含 '.' 或 resource.verb 形式） ==")
	var legacy int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action NOT LIKE '%.%'`).Scan(&legacy); err != nil {
		fmt.Println("legacy query:", err)
	}
	fmt.Println("  no-dot:", legacy)
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'agent.%' OR action LIKE 'tool.%' OR action LIKE 'mcp\_server.%' OR action LIKE 'archive.%' OR action LIKE 'skill.%'`).Scan(&legacy); err != nil {
		fmt.Println("legacy2 query:", err)
	}
	fmt.Println("  resource-first 残留:", legacy)

	fmt.Println("== detail 非 JSON 残留 ==")
	var plain int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE detail <> '' AND detail NOT LIKE '{%'`).Scan(&plain); err != nil {
		fmt.Println("plain query:", err)
	}
	fmt.Println("  plain-text detail:", plain)

	fmt.Println("== 最近 15 条 ==")
	rows, err = db.Query(`SELECT action, resource, resource_id, COALESCE(actor,''), COALESCE(ip,''), COALESCE(severity,''), left(detail,80) FROM audit_logs ORDER BY created_at DESC LIMIT 15`)
	if err != nil {
		fmt.Println("list query:", err)
		return
	}
	for rows.Next() {
		var a, r, rid, actor, ip, sev, d string
		rows.Scan(&a, &r, &rid, &actor, &ip, &sev, &d)
		fmt.Printf("  %-24s %-10s rid=%-28s actor=%-8s ip=%-13s sev=%-7s %s\n", a, r, rid, actor, ip, sev, d)
	}
	rows.Close()
}

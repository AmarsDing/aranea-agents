// 最终验证：alibabacloud 技能状态 + 版本完整性 + 运维域覆盖
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

	fmt.Println("== alibabacloud-* 最终状态 ==")
	rows, err := db.Query(`
		SELECT s.skill_key, s.status, s.enabled, s.lifecycle_status, s.filesystem_missing,
		       COALESCE(v.version,'-'), COALESCE(v.validation_status,'-')
		FROM skill s
		LEFT JOIN skill_version v ON v.skill_id = s.id AND v.published_at IS NOT NULL
		WHERE s.skill_key LIKE 'alibabacloud%'
		ORDER BY s.skill_key`)
	if err != nil {
		panic(err)
	}
	ok, bad := 0, 0
	for rows.Next() {
		var k, st, lc, ver, vs string
		var en, fm bool
		rows.Scan(&k, &st, &en, &lc, &fm, &ver, &vs)
		healthy := st == "published" && en && lc == "active" && !fm
		mark := "OK "
		if !healthy {
			mark = "BAD"
			bad++
		} else {
			ok++
		}
		fmt.Printf("[%s] %-44s %-10s enabled=%-5v fs_missing=%-5v ver=%-6s validation=%s\n", mark, k, st, en, fm, ver, vs)
	}
	rows.Close()
	fmt.Printf("\nhealthy=%d bad=%d\n", ok, bad)

	// oos-chatops-agent 核查（change_execution 域）
	fmt.Println("\n== oos / chatops 相关 ==")
	rows2, _ := db.Query(`SELECT skill_key, status, enabled FROM skill WHERE skill_key LIKE '%oos%' OR skill_key LIKE '%chatops%'`)
	found := false
	for rows2.Next() {
		var k, st string
		var en bool
		rows2.Scan(&k, &st, &en)
		fmt.Printf("%-44s %-10s enabled=%v\n", k, st, en)
		found = true
	}
	rows2.Close()
	if !found {
		fmt.Println("无记录 — ops_change_execution 域未覆盖")
	}

	// 全库统计
	var total, pub, en int
	db.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE status='published'), COUNT(*) FILTER (WHERE enabled) FROM skill WHERE lifecycle_status='active'`).Scan(&total, &pub, &en)
	fmt.Printf("\n== 全库 == total(active)=%d published=%d enabled=%d\n", total, pub, en)
}

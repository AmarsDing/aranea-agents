// 精确查询 4 个缺失 skill 的状态 + 其 import job 的冲突决策
//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	keys := []string{"brand-guidelines", "doc-coauthoring", "frontend-design", "pptx"}
	for _, k := range keys {
		var cnt int
		db.QueryRow(`SELECT COUNT(*) FROM skill WHERE skill_key=$1`, k).Scan(&cnt)
		fmt.Printf("skill '%s': %d rows\n", k, cnt)
	}

	fmt.Println("\n== jobs containing these slugs in candidates_json ==")
	rows, err := db.Query(`SELECT id, status, validation_status, message, conflict_groups_json::text, created_at, applied_at FROM skill_import_jobs WHERE candidates_json::text LIKE '%brand-guidelines%' OR candidates_json::text LIKE '%doc-coauthoring%' OR candidates_json::text LIKE '%frontend-design%' OR candidates_json::text LIKE '%"pptx"%' ORDER BY created_at DESC`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, vs, msg, cg, ca string
		var aa sql.NullString
		rows.Scan(&id, &status, &vs, &msg, &cg, &ca, &aa)
		fmt.Printf("\n--- job %s status=%s validation=%s created=%s applied=%s\n  msg=%s\n", id, status, vs, ca, aa.String, msg)
		// 提取冲突组中 recommendation / similarity
		for _, k := range keys {
			if strings.Contains(cg, k) {
				idx := strings.Index(cg, k)
				start := idx - 200
				if start < 0 {
					start = 0
				}
				end := idx + 800
				if end > len(cg) {
					end = len(cg)
				}
				fmt.Printf("  conflict[%s]: …%s…\n", k, cg[start:end])
			}
		}
		if cg == "" || cg == `{"items": []}` {
			fmt.Println("  conflict_groups: (empty)")
		}
	}
}

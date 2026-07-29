// 查 oos-chatops-agent job 被 skip 原因
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

	// 找 candidates 含 oos/chatops 的 job
	rows, err := db.Query(`SELECT id, status, candidates_json::text, conflict_groups_json::text FROM skill_import_jobs WHERE candidates_json::text LIKE '%chatops%' OR candidates_json::text LIKE '%oos%' ORDER BY created_at DESC LIMIT 3`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, st, cands, groups string
		rows.Scan(&id, &st, &cands, &groups)
		fmt.Printf("== job %s status=%s ==\ncandidates: %s\ngroups: %s\n\n", id, st, trunc(cands, 1500), trunc(groups, 800))
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

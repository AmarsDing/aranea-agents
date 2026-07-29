// 排查 3 个 apply 失败 job 的冲突 slug
//go:build ignore

package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	jobIDs := []string{"6cfe92ed3f909a929d9a4879", "3957aeef4a748146dd0a427c", "aabd61c019d4d5a97d07a15a"}
	for _, jid := range jobIDs {
		var cands, groups, msg string
		err := db.QueryRow(`SELECT candidates_json::text, conflict_groups_json::text, COALESCE(message,'') FROM skill_import_jobs WHERE id=$1`, jid).Scan(&cands, &groups, &msg)
		if err != nil {
			fmt.Printf("job %s: %v\n", jid, err)
			continue
		}
		fmt.Printf("\n== job %s msg=%s ==\n", jid, msg)
		fmt.Printf("candidates_json: %s\n", truncStr(cands, 1200))
		fmt.Printf("conflict_groups_json: %s\n", truncStr(groups, 600))
	}
}

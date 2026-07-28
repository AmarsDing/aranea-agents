// 查询批量安装进度：team_runs_v2 状态 + skill 计数
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

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM skill WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		panic(err)
	}
	fmt.Printf("skills (not deleted): %d\n", total)

	// 最近的 team_runs
	rows, err := db.Query(`SELECT id, team_id, status, created_at, updated_at FROM team_runs_v2 ORDER BY created_at DESC LIMIT 12`)
	if err != nil {
		fmt.Println("team_runs_v2 query err:", err)
	} else {
		fmt.Println("\nrecent team_runs_v2:")
		for rows.Next() {
			var id, teamID, status, created, updated string
			rows.Scan(&id, &teamID, &status, &created, &updated)
			fmt.Printf("  %s team=%s status=%s created=%s updated=%s\n", id[:8], teamID, status, created, updated)
		}
		rows.Close()
	}

	// 最近安装的 skill
	rows2, err := db.Query(`SELECT skill_key, status, created_at FROM skill ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		fmt.Println("skill query err:", err)
	} else {
		fmt.Println("\nrecent skills:")
		for rows2.Next() {
			var k, st, ca string
			rows2.Scan(&k, &st, &ca)
			fmt.Printf("  %-40s %s %s\n", k, st, ca)
		}
		rows2.Close()
	}
}

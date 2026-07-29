//go:build ignore

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
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	_, err = db.Exec(`UPDATE organizations SET name = $1, description = $2 WHERE id = $3`,
		"IT 运维行业", "直写测试：告警处理、故障诊断、日志分析。", "ae8b8503-4049-4cb2-811c-b795f5c30304")
	if err != nil {
		fmt.Fprintf(os.Stderr, "update: %v\n", err)
		os.Exit(1)
	}

	var name, desc string
	err = db.QueryRow(`SELECT name, description FROM organizations WHERE id = $1`, "ae8b8503-4049-4cb2-811c-b795f5c30304").Scan(&name, &desc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("DB-READ name=%q desc=%q\n", name, desc)
}

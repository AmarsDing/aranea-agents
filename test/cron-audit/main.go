// 一次性排查脚本：读取 cron_task / cron_task_run 实况。用完可整目录删除。
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

	fmt.Println("== cron_task ==")
	rows, err := db.Query(`SELECT id, task_key, name, status, enabled, agent_id, config_json, metadata_json FROM cron_task`)
	if err != nil {
		fmt.Println("query cron_task:", err)
		os.Exit(1)
	}
	for rows.Next() {
		var id, key, name, status, agentID, cfg, meta string
		var enabled bool
		if err := rows.Scan(&id, &key, &name, &status, &enabled, &agentID, &cfg, &meta); err != nil {
			fmt.Println("scan:", err)
			continue
		}
		fmt.Printf("id=%s key=%s name=%s status=%s enabled=%v agent=%s\n  config=%s\n  metadata=%s\n", id, key, name, status, enabled, agentID, cfg, meta)
	}
	rows.Close()

	fmt.Println("\n== cron_task_run (latest 10) ==")
	rows2, err := db.Query(`SELECT task_id, status, COALESCE(error_message,''), started_at FROM cron_task_run ORDER BY started_at DESC LIMIT 10`)
	if err != nil {
		fmt.Println("query cron_task_run:", err)
		os.Exit(1)
	}
	for rows2.Next() {
		var taskID, status, errMsg, started string
		if err := rows2.Scan(&taskID, &status, &errMsg, &started); err != nil {
			fmt.Println("scan:", err)
			continue
		}
		fmt.Printf("task=%s status=%s started=%s err=%s\n", taskID, status, started, errMsg)
	}
	rows2.Close()
}

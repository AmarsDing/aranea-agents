// 一次性维护脚本：将 __system_admin__ 的 tools_profile 重置为 "system_admin"，
// 使其与 cli_admin 工具组种子一致。用完可整目录删除。
package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	source := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	if v := os.Getenv("DATA__POSTGRES__SOURCE"); v != "" {
		source = v
	}
	db, err := sql.Open("postgres", source)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`UPDATE agent_runtime_settings SET tools_profile = 'system_admin', updated_at = $1 WHERE agent_id = 'agent___system_admin__'`, now)
	if err != nil {
		panic(err)
	}
	n, _ := res.RowsAffected()
	fmt.Printf("agent_runtime_settings updated rows=%d\n", n)

	var profile string
	if err := db.QueryRow(`SELECT tools_profile FROM agent_runtime_settings WHERE agent_id = 'agent___system_admin__'`).Scan(&profile); err != nil {
		panic(err)
	}
	fmt.Printf("tools_profile now = %q\n", profile)
}

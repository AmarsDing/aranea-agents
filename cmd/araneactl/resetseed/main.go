package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 删除 agency-pack 和 cleanup 的版本记录，让它们重新执行
	stmts := []string{
		`DELETE FROM schema_migrations WHERE version IN (20261103, 20261104)`,
		// 清除所有 organizations、非系统 agents、非系统 prompt files（与 CleanupNonSystemData 相同）
		`DELETE FROM agent_prompt_files WHERE agent_id NOT IN (SELECT id FROM agents WHERE kind = 'system_builtin' AND (agent_variant != 'dept_lead' OR agent_variant IS NULL) AND agent_key NOT LIKE 'dept-lead-%')`,
		`DELETE FROM agent_runtime_settings WHERE agent_id NOT IN (SELECT id FROM agents WHERE kind = 'system_builtin' AND (agent_variant != 'dept_lead' OR agent_variant IS NULL) AND agent_key NOT LIKE 'dept-lead-%')`,
		`DELETE FROM agents WHERE kind != 'system_builtin' OR agent_variant = 'dept_lead' OR agent_key LIKE 'dept-lead-%'`,
		`DELETE FROM teams WHERE kind != 'system_builtin'`,
		`DELETE FROM organizations`,
	}
	for _, s := range stmts {
		res, err := db.ExecContext(ctx, s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "exec failed: %s\n  err: %v\n", s, err)
			os.Exit(1)
		}
		n, _ := res.RowsAffected()
		fmt.Printf("OK (%d rows): %s\n", n, s)
	}
	fmt.Println("Reset complete. Restart the server to re-run cleanup + agency-pack import.")
}

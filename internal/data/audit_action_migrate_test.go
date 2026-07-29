package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func TestRunAuditActionNormalizeMigration(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()

	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, lg)

	if _, err := client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		action TEXT NOT NULL DEFAULT '',
		resource TEXT NOT NULL DEFAULT '',
		resource_id TEXT NOT NULL DEFAULT '',
		request_id TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		actor TEXT NOT NULL DEFAULT '',
		ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		severity TEXT NOT NULL DEFAULT '',
		metadata_json TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create audit_logs: %v", err)
	}

	rows := []struct{ id, action, detail string }{
		{"r1", "agent.create", "key=demo"},
		{"r2", "agent.delete", ""},
		{"r3", "tool.update", "key=search"},
		{"r4", "mcp_server.delete", ""},
		{"r5", "archive.session.batch", "matched=3 processed=3"},
		{"r6", "skill.filesystem.sync", "synced"},
		{"r7", "create.session", "title=chat"},        // 已规范 action + 纯文本 detail → 仅包 detail
		{"r8", "create.agent", `{"summary":"key=x"}`}, // 已规范 → 不动
	}
	for _, r := range rows {
		if _, err := client.ExecContext(ctx,
			`INSERT INTO audit_logs (id, action, resource, detail, created_at) VALUES ($1, $2, 'agent', $3, '2026-07-01T00:00:00Z')`,
			r.id, r.action, r.detail); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	if err := RunAuditActionNormalizeMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration: %v", err)
	}
	// 幂等：二次运行不报错、结果不变。
	if err := RunAuditActionNormalizeMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration second run: %v", err)
	}

	wantAction := map[string]string{
		"r1": "create.agent",
		"r2": "delete.agent",
		"r3": "update.tool",
		"r4": "delete.mcp_server",
		"r5": "archive.session",
		"r6": "sync.skill",
		"r7": "create.session",
		"r8": "create.agent",
	}
	scanStr := func(query string, args ...any) string {
		rows, err := client.QueryContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer rows.Close()
		if !rows.Next() {
			t.Fatalf("query returned no rows")
		}
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		return v
	}

	for id, want := range wantAction {
		if got := scanStr(`SELECT action FROM audit_logs WHERE id = $1`, id); got != want {
			t.Errorf("%s action = %q, want %q", id, got, want)
		}
	}

	// detail：纯文本应被包成 {"summary":...}；空串保持空；已是 JSON 的保持不变。
	wantDetail := map[string]string{
		"r1": `{"summary":"key=demo"}`,
		"r2": "",
		"r5": `{"summary":"matched=3 processed=3"}`,
		"r7": `{"summary":"title=chat"}`,
		"r8": `{"summary":"key=x"}`,
	}
	for id, want := range wantDetail {
		if got := scanStr(`SELECT detail FROM audit_logs WHERE id = $1`, id); got != want {
			t.Errorf("%s detail = %q, want %q", id, got, want)
		}
	}
}

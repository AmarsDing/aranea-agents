package data

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// M1 fix: PII scanning must happen at the unified fact-write entry so all
// three producers (auto_memory consolidation, auto_memory batch,
// immediate_fact_writer) are covered — previously only the trpc tool path
// (FactUpsert) was scanned.
func TestRedactFactWritePII_NoPII(t *testing.T) {
	got := redactFactWritePII("用户偏好深色模式", "")
	if got.piiFlag != 0 {
		t.Fatalf("piiFlag = %d, want 0", got.piiFlag)
	}
	if got.statement != "用户偏好深色模式" {
		t.Fatalf("statement changed unexpectedly: %q", got.statement)
	}
	if got.original != "" {
		t.Fatalf("original = %q, want empty", got.original)
	}
	if got.piiTypesJSON != "[]" {
		t.Fatalf("piiTypesJSON = %q, want []", got.piiTypesJSON)
	}
}

func TestRedactFactWritePII_StatementPII(t *testing.T) {
	stmt := "我的身份证号是 110101199003077777，邮箱是 zhangsan@example.com"
	got := redactFactWritePII(stmt, "")
	if got.piiFlag != 1 {
		t.Fatalf("piiFlag = %d, want 1", got.piiFlag)
	}
	if got.original != stmt {
		t.Fatalf("original not preserved: %q", got.original)
	}
	if strings.Contains(got.statement, "110101199003077777") || strings.Contains(got.statement, "zhangsan@example.com") {
		t.Fatalf("statement still contains PII: %q", got.statement)
	}
	if !strings.Contains(got.statement, "[id]") || !strings.Contains(got.statement, "[email]") {
		t.Fatalf("statement missing redaction placeholders: %q", got.statement)
	}
	var types []string
	if err := json.Unmarshal([]byte(got.piiTypesJSON), &types); err != nil {
		t.Fatalf("piiTypesJSON not valid JSON: %v", err)
	}
	joined := strings.Join(types, ",")
	if !strings.Contains(joined, "id_card") || !strings.Contains(joined, "email") {
		t.Fatalf("piiTypesJSON missing types: %q", got.piiTypesJSON)
	}
}

func TestRedactFactWritePII_DetailsPIIAlsoRedacted(t *testing.T) {
	stmt := "用户联系方式见详情"
	details := "电话 13800138000，备用邮箱 backup@example.com"
	got := redactFactWritePII(stmt, details)
	if got.piiFlag != 1 {
		t.Fatalf("piiFlag = %d, want 1 (details PII must flag the fact)", got.piiFlag)
	}
	if strings.Contains(got.details, "13800138000") || strings.Contains(got.details, "backup@example.com") {
		t.Fatalf("details still contains PII: %q", got.details)
	}
	// original must preserve the statement for ApprovePIIFact recovery.
	if got.original != stmt {
		t.Fatalf("original = %q, want statement %q", got.original, stmt)
	}
}

func TestRedactFactWritePII_EmptyStatement(t *testing.T) {
	got := redactFactWritePII("", "")
	if got.piiFlag != 0 || got.statement != "" || got.original != "" {
		t.Fatalf("unexpected result for empty input: %+v", got)
	}
}

// P0-1 (2026-08-08): the index reconciler must pick up 'pending' facts in
// addition to 'stale'/'failed'. Insert defaults embedding_status to 'pending';
// facts whose synchronous index write never ran (crash window, canary insert,
// historical rows) were stranded forever because the listing query only
// matched 'stale'/'failed'. The row JSON must also carry index_attempts so the
// reconciler's max-attempts disable guard can fire.
func TestListStaleIndexFacts_IncludesPending(t *testing.T) {
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	if err := EnsureSessionMemorySchema(ctx, client, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("ensure session memory schema: %v", err)
	}
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())

	now := time.Now().UTC().Format(time.RFC3339Nano)
	insert := func(id, embStatus string) {
		t.Helper()
		_, err := db.ExecContext(ctx,
			`INSERT INTO memory_facts (id, scope_type, scope_id, agent_id, statement, fingerprint, embedding_status, created_at, updated_at)
			 VALUES ($1, 'agent', 'agent-1', 'agent-1', $2, $1, $3, $4, $4)`,
			id, "stmt-"+id, embStatus, now)
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("f-stale", "stale")
	insert("f-failed", "failed")
	insert("f-pending", "pending")
	insert("f-fresh", "fresh")
	insert("f-disabled", "disabled")

	m := NewMemoryFactIndexMaintainerAdapter(d)
	rows, err := m.ListStaleIndexFacts(ctx, 5, 50)
	if err != nil {
		t.Fatalf("ListStaleIndexFacts: %v", err)
	}
	got := map[string]bool{}
	for _, raw := range rows {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := m["index_attempts"]; !ok {
			t.Errorf("row JSON missing index_attempts (reconciler disable guard needs it)")
		}
		id, _ := m["id"].(string)
		got[id] = true
	}
	for _, want := range []string{"f-stale", "f-failed", "f-pending"} {
		if !got[want] {
			t.Errorf("missing %s in stale index list (got %v)", want, got)
		}
	}
	for _, unwanted := range []string{"f-fresh", "f-disabled"} {
		if got[unwanted] {
			t.Errorf("unexpected %s in stale index list", unwanted)
		}
	}
}

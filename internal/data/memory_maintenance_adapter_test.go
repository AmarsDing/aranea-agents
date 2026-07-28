package data

import (
	"encoding/json"
	"strings"
	"testing"
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

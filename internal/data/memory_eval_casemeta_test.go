package data

import (
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
)

// AMC code board: commit-shaped evidence must be tagged in source metadata so
// downstream analysis (and any later repo-scoped retrieval) can segment it.
func TestEvalSourceMetadataMarksCaseKind(t *testing.T) {
	meta := evalSourceMetadata("s-1", 0, 0, nil, nil, nil,
		"2026-10-24T00:00:00Z", "", "user: fix(auth): reject expired refresh tokens")
	var m map[string]any
	if err := json.Unmarshal([]byte(meta), &m); err != nil {
		t.Fatalf("metadata not JSON: %v\n%s", err, meta)
	}
	if m["case_kind"] != "case_fix" {
		t.Fatalf("case_kind = %v, want case_fix in %s", m["case_kind"], meta)
	}
	if m["case_scope"] != "auth" {
		t.Fatalf("case_scope = %v, want auth in %s", m["case_scope"], meta)
	}
}

func TestEvalSourceMetadataLeavesProseUntouched(t *testing.T) {
	meta := evalSourceMetadata("s-1", 0, 0, nil, nil, nil,
		"2026-10-24T00:00:00Z", "", "user: 我喜欢咖啡")
	var m map[string]any
	if err := json.Unmarshal([]byte(meta), &m); err != nil {
		t.Fatalf("metadata not JSON: %v\n%s", err, meta)
	}
	if _, ok := m["case_kind"]; ok {
		t.Fatalf("prose statement must not carry case_kind: %s", meta)
	}
	if _, ok := m["case_scope"]; ok {
		t.Fatalf("prose statement must not carry case_scope: %s", meta)
	}
}

// Role handling must match the classifier exactly: an exotic role prefix is
// not stripped as a role, so the statement is not commit-shaped for either
// the fact kind or the metadata tag.
func TestEvalSourceMetadataDoesNotTagExoticRolePrefix(t *testing.T) {
	meta := evalSourceMetadata("s-1", 0, 0, nil, nil, nil,
		"2026-10-24T00:00:00Z", "", "human: fix: x")
	var m map[string]any
	if err := json.Unmarshal([]byte(meta), &m); err != nil {
		t.Fatalf("metadata not JSON: %v\n%s", err, meta)
	}
	if _, ok := m["case_kind"]; ok {
		t.Fatalf("exotic role prefix must stay untagged (classifier sees it as prose): %s", meta)
	}
	if got := biz.ClassifyEvalFactKind("human: fix: x"); biz.IsEvalCaseKind(got) {
		t.Fatalf("classifier and metadata diverge: classifier says %s", got)
	}
}

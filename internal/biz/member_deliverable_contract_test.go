package biz

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseMemberDeliverableContract_Empty(t *testing.T) {
	c, err := ParseMemberDeliverableContract("")
	if err != nil {
		t.Fatalf("empty contract should parse, got %v", err)
	}
	if c != nil {
		t.Fatalf("empty contract should yield nil, got %+v", c)
	}
}

func TestParseMemberDeliverableContract_Valid(t *testing.T) {
	raw := `{"entries":[{"topic":"design","required":true,"required_keys":["arch","api"],"schema_json":"{\"type\":\"object\"}"}]}`
	c, err := ParseMemberDeliverableContract(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c == nil || len(c.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %+v", c)
	}
	e := c.Entries[0]
	if e.Topic != "design" || !e.Required || len(e.RequiredKeys) != 2 || e.SchemaJSON == "" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if c.EntryForTopic("design") == nil {
		t.Fatal("EntryForTopic(design) should resolve")
	}
	if c.EntryForTopic("missing") != nil {
		t.Fatal("EntryForTopic(missing) should be nil")
	}
}

func TestParseMemberDeliverableContract_InvalidJSON(t *testing.T) {
	if _, err := ParseMemberDeliverableContract("{bad"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// F5 (Phase 11): inter-team deliverable contracts auto-derive MDC entries.
// topic maps 1:1 to the contract name (no re-mapping, so members and the
// spirit can never disagree on the topic name).
func TestMemberEntriesFromDeliverableContracts(t *testing.T) {
	contracts := []DeliverableContract{
		{
			Name:        "xlsx_install_result",
			Type:        "data",
			Format:      "json",
			Description: "xlsx skill 安装结果",
			SchemaJSON:  `{"type":"object","properties":{"status":{"type":"string"},"detail":{"type":"string"}},"required":["status","detail"]}`,
		},
		{
			Name:        "report",
			Type:        "document",
			Format:      "markdown",
			Description: "报告",
			// no schema → advisory only, no required keys
		},
		{
			Name:       "bad_schema",
			Type:       "data",
			Format:     "json",
			SchemaJSON: `{not json`,
		},
		{
			Name: "", // empty name skipped
			Type: "data",
		},
	}
	entries := MemberEntriesFromDeliverableContracts(contracts)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (empty name skipped), got %d: %+v", len(entries), entries)
	}
	e0 := entries[0]
	if e0.Topic != "xlsx_install_result" || !e0.Required || e0.Description != "xlsx skill 安装结果" {
		t.Fatalf("unexpected entry[0]: %+v", e0)
	}
	if e0.SchemaJSON == "" {
		t.Fatal("entry[0] schema_json should pass through")
	}
	if len(e0.RequiredKeys) != 2 || e0.RequiredKeys[0] != "status" || e0.RequiredKeys[1] != "detail" {
		t.Fatalf("entry[0] required_keys should derive from schema required, got %v", e0.RequiredKeys)
	}
	if entries[1].Topic != "report" || len(entries[1].RequiredKeys) != 0 || entries[1].SchemaJSON != "" {
		t.Fatalf("unexpected entry[1]: %+v", entries[1])
	}
	if entries[2].Topic != "bad_schema" || len(entries[2].RequiredKeys) != 0 {
		t.Fatalf("invalid schema must yield no required keys (advisory), got %+v", entries[2])
	}
}

func TestValidateMemberDeliverableEntry_NoEntry(t *testing.T) {
	c := &MemberDeliverableContract{}
	if got := c.ValidateTopicData("anything", map[string]any{"a": 1}); got != nil {
		t.Fatalf("no entry → no violations, got %v", got)
	}
}

func TestValidateMemberDeliverableEntry_RequiredKeys(t *testing.T) {
	c := &MemberDeliverableContract{Entries: []MemberDeliverableEntry{
		{Topic: "design", RequiredKeys: []string{"arch", "api"}},
	}}
	violations := c.ValidateTopicData("design", map[string]any{"arch": "x"})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %v", violations)
	}
	v := violations[0]
	if v.Kind != MemberContractViolationMissingKey || v.Topic != "design" || !strings.Contains(v.Detail, "api") {
		t.Fatalf("unexpected violation: %+v", v)
	}
	// full data passes
	if got := c.ValidateTopicData("design", map[string]any{"arch": "x", "api": "y"}); got != nil {
		t.Fatalf("expected no violations, got %v", got)
	}
}

func TestValidateMemberDeliverableEntry_Schema(t *testing.T) {
	schema := `{"type":"object","required":["arch"],"properties":{"arch":{"type":"string"}}}`
	c := &MemberDeliverableContract{Entries: []MemberDeliverableEntry{
		{Topic: "design", SchemaJSON: schema},
	}}
	violations := c.ValidateTopicData("design", map[string]any{"arch": 123})
	if len(violations) != 1 || violations[0].Kind != MemberContractViolationSchema {
		t.Fatalf("expected schema violation, got %v", violations)
	}
	if got := c.ValidateTopicData("design", map[string]any{"arch": "ok"}); got != nil {
		t.Fatalf("expected no violations, got %v", got)
	}
}

func TestValidateMemberDeliverableEntry_TopicMismatchIgnored(t *testing.T) {
	c := &MemberDeliverableContract{Entries: []MemberDeliverableEntry{
		{Topic: "design", RequiredKeys: []string{"arch"}},
	}}
	if got := c.ValidateTopicData("other", map[string]any{}); got != nil {
		t.Fatalf("unrelated topic should not be validated, got %v", got)
	}
}

func TestMemberContractViolationError_LLMActionable(t *testing.T) {
	err := &MemberContractViolationError{
		Violations: []MemberContractViolation{
			{Topic: "design", Kind: MemberContractViolationMissingKey, Detail: "缺少必需键 \"api\""},
		},
	}
	msg := err.Error()
	if !strings.Contains(msg, "design") || !strings.Contains(msg, "api") {
		t.Fatalf("error should name topic and detail, got: %s", msg)
	}
	var data map[string]any
	if jerr := json.Unmarshal([]byte(msg), &data); jerr == nil {
		t.Fatalf("error message should be human-readable text, not JSON: %s", msg)
	}
}

func TestRequiredTopicsMissing(t *testing.T) {
	c := &MemberDeliverableContract{Entries: []MemberDeliverableEntry{
		{Topic: "design", Required: true},
		{Topic: "notes"},
	}}
	missing := c.RequiredTopicsMissing(map[string]any{"notes": map[string]any{"x": 1}})
	if len(missing) != 1 || missing[0] != "design" {
		t.Fatalf("expected [design], got %v", missing)
	}
	if got := c.RequiredTopicsMissing(map[string]any{"design": map[string]any{}}); got != nil {
		t.Fatalf("expected none missing, got %v", got)
	}
}

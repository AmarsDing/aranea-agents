package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestValidatePlannerKind(t *testing.T) {
	for _, k := range []string{"", "builtin", "react", "a2ui", "BUILTIN"} {
		if err := biz.ValidatePlannerKind(k); err != nil {
			t.Fatalf("ValidatePlannerKind(%q): %v", k, err)
		}
	}
	if err := biz.ValidatePlannerKind("chain-of-thought"); err == nil {
		t.Fatal("expected error for unknown planner_kind")
	}
}

func TestValidatePlannerConfigJSON(t *testing.T) {
	if err := biz.ValidatePlannerConfigJSON("builtin", `{"reasoning_effort":"high"}`); err != nil {
		t.Fatal(err)
	}
	if err := biz.ValidatePlannerConfigJSON("builtin", "[]"); err == nil {
		t.Fatal("expected error for non-object JSON")
	}
	if err := biz.ValidatePlannerConfigJSON("react", `{"extra":1}`); err == nil {
		t.Fatal("react planner must reject non-empty config")
	}
	if err := biz.ValidatePlannerConfigJSON("builtin", `{"unknown_key":true}`); err == nil {
		t.Fatal("expected unknown field error")
	}
	if err := biz.ValidatePlannerConfigJSON("builtin", `{"thinking_enabled":"yes"}`); err == nil {
		t.Fatal("thinking_enabled must be boolean")
	}
	if err := biz.ValidatePlannerConfigJSON("a2ui", `{"instruction":"x"}`); err != nil {
		t.Fatal(err)
	}
	if err := biz.ValidatePlannerConfigJSON("", `{"reasoning_effort":"high"}`); err == nil {
		t.Fatal("expected error when planner_kind empty but config non-empty")
	}
	if err := biz.ValidatePlannerConfigJSON("", `{}`); err != nil {
		t.Fatal("empty kind with {} should be allowed")
	}
	if err := biz.ValidatePlannerConfigJSON("builtin", `{"reasoning_effort":"turbo"}`); err == nil {
		t.Fatal("expected error for invalid reasoning_effort")
	}
	if err := biz.ValidatePlannerConfigJSON("builtin", `{"reasoning_effort":"max"}`); err != nil {
		t.Fatal("max is allowed for DeepSeek-style builtin config")
	}
}

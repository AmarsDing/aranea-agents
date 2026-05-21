package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestValidateCodeExecutorType(t *testing.T) {
	for _, typ := range biz.ValidCodeExecutorTypes() {
		if err := biz.ValidateCodeExecutorType(typ); err != nil {
			t.Fatalf("expected valid %q, got %v", typ, err)
		}
	}
	if err := biz.ValidateCodeExecutorType(""); err != nil {
		t.Fatalf("empty should be valid: %v", err)
	}
	if err := biz.ValidateCodeExecutorType("jupyter"); err == nil {
		t.Fatal("expected invalid type error")
	}
}

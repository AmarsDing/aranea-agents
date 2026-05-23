package biz

import (
	"testing"
)

func TestValidateJSONSchema_Valid(t *testing.T) {
	schema := `{"type":"object","properties":{"max_content_length":{"type":"integer"}}}`
	doc := `{"max_content_length":100}`
	if err := ValidatePluginJSONSchema(schema, doc); err != nil {
		t.Fatalf("expected valid: %v", err)
	}
}

func TestValidateJSONSchema_InvalidType(t *testing.T) {
	schema := `{"type":"object","properties":{"max_content_length":{"type":"integer"}}}`
	doc := `{"max_content_length":"not-a-number"}`
	if err := ValidatePluginJSONSchema(schema, doc); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateJSONSchema_EmptySchemaSkippedByCaller(t *testing.T) {
	// ValidatePluginJSONSchema itself validates any non-empty schema; callers skip "" and "{}"
	if err := ValidatePluginJSONSchema(`{}`, `{}`); err != nil {
		t.Fatalf("empty object doc against empty schema: %v", err)
	}
}

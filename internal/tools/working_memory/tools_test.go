package working_memory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

// stubL1SchemaReader implements biz.L1SchemaReader for testing.
type stubL1SchemaReader struct {
	row []byte
	err error
}

func (s *stubL1SchemaReader) GetL1SchemaRow(_ context.Context, _ string) ([]byte, error) {
	return s.row, s.err
}

// TestValidFieldKinds verifies the 10 field_kind enum values.
func TestValidFieldKinds(t *testing.T) {
	want := []string{
		"string", "number", "boolean", "json", "reference", "markdown",
		"decision", "artifact", "progress", "constraint",
	}
	if len(biz.ValidFieldKinds) != len(want) {
		t.Fatalf("ValidFieldKinds has %d entries, want %d", len(biz.ValidFieldKinds), len(want))
	}
	for i, v := range want {
		if biz.ValidFieldKinds[i] != v {
			t.Errorf("ValidFieldKinds[%d] = %q, want %q", i, biz.ValidFieldKinds[i], v)
		}
	}
}

// TestValidateFieldAgainstSchema tests the schema validation logic.
func TestValidateFieldAgainstSchema(t *testing.T) {
	t.Run("empty_schema_allows_all", func(t *testing.T) {
		if err := validateFieldAgainstSchema(nil, "any_field", "string"); err != nil {
			t.Errorf("empty schema should allow all, got: %v", err)
		}
		if err := validateFieldAgainstSchema([]byte{}, "any_field", "string"); err != nil {
			t.Errorf("empty byte schema should allow all, got: %v", err)
		}
	})

	t.Run("no_fields_key_allows_all", func(t *testing.T) {
		schema := []byte(`{"other": "data"}`)
		if err := validateFieldAgainstSchema(schema, "any_field", "string"); err != nil {
			t.Errorf("schema without fields key should allow all, got: %v", err)
		}
	})

	t.Run("empty_fields_array_allows_all", func(t *testing.T) {
		schema := []byte(`{"fields": []}`)
		if err := validateFieldAgainstSchema(schema, "any_field", "string"); err != nil {
			t.Errorf("empty fields array should allow all, got: %v", err)
		}
	})

	t.Run("matching_field_path_allows", func(t *testing.T) {
		schema := []byte(`{"fields": [{"path": "user_name", "kind": "string"}]}`)
		if err := validateFieldAgainstSchema(schema, "user_name", "string"); err != nil {
			t.Errorf("matching field_path should be allowed, got: %v", err)
		}
	})

	t.Run("non_matching_field_path_rejects", func(t *testing.T) {
		schema := []byte(`{"fields": [{"path": "user_name", "kind": "string"}]}`)
		err := validateFieldAgainstSchema(schema, "other_field", "string")
		if err == nil {
			t.Fatal("non-matching field_path should be rejected")
		}
	})

	t.Run("invalid_json_allows_all", func(t *testing.T) {
		schema := []byte(`{invalid json}`)
		if err := validateFieldAgainstSchema(schema, "any_field", "string"); err != nil {
			t.Errorf("invalid JSON should allow all (soft constraint), got: %v", err)
		}
	})

	t.Run("multiple_fields_one_matches_allows", func(t *testing.T) {
		schema := []byte(`{"fields": [{"path": "user_name", "kind": "string"}, {"path": "task_status", "kind": "progress"}, {"path": "api_key", "kind": "string"}]}`)
		if err := validateFieldAgainstSchema(schema, "task_status", "progress"); err != nil {
			t.Errorf("one matching field in multiple should be allowed, got: %v", err)
		}
	})
}

// TestValidateFieldWithSchema tests the schema reader integration.
func TestValidateFieldWithSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_reader_allows", func(t *testing.T) {
		if err := validateFieldWithSchema(ctx, nil, "schema1", "field1", "string"); err != nil {
			t.Errorf("nil reader should allow, got: %v", err)
		}
	})

	t.Run("empty_schemaID_allows", func(t *testing.T) {
		reader := &stubL1SchemaReader{}
		if err := validateFieldWithSchema(ctx, reader, "", "field1", "string"); err != nil {
			t.Errorf("empty schemaID should allow, got: %v", err)
		}
	})

	t.Run("reader_error_allows", func(t *testing.T) {
		reader := &stubL1SchemaReader{err: errors.New("db down")}
		if err := validateFieldWithSchema(ctx, reader, "schema1", "field1", "string"); err != nil {
			t.Errorf("reader error should allow (soft constraint), got: %v", err)
		}
	})

	t.Run("valid_schema_matching_field_allows", func(t *testing.T) {
		schemaJSON, _ := json.Marshal(map[string]any{
			"fields": []any{
				map[string]any{"path": "user_name", "kind": "string"},
			},
		})
		row, _ := json.Marshal(map[string]any{"schema_json": string(schemaJSON)})
		reader := &stubL1SchemaReader{row: row}
		if err := validateFieldWithSchema(ctx, reader, "schema1", "user_name", "string"); err != nil {
			t.Errorf("matching field should be allowed, got: %v", err)
		}
	})

	t.Run("valid_schema_non_matching_field_rejects", func(t *testing.T) {
		schemaJSON, _ := json.Marshal(map[string]any{
			"fields": []any{
				map[string]any{"path": "user_name", "kind": "string"},
			},
		})
		row, _ := json.Marshal(map[string]any{"schema_json": string(schemaJSON)})
		reader := &stubL1SchemaReader{row: row}
		err := validateFieldWithSchema(ctx, reader, "schema1", "other_field", "string")
		if err == nil {
			t.Fatal("non-matching field should be rejected")
		}
	})
}

// TestContextInjection tests context key round-trips.
func TestContextInjection(t *testing.T) {
	t.Run("WithL1SchemaReader_roundtrip", func(t *testing.T) {
		ctx := context.Background()
		reader := &stubL1SchemaReader{}
		ctx = WithL1SchemaReader(ctx, reader)
		got := L1SchemaReaderFromCtx(ctx)
		if got == nil {
			t.Fatal("L1SchemaReaderFromCtx should return injected reader")
		}
	})

	t.Run("WithL1DefaultSchemaID_roundtrip", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithL1DefaultSchemaID(ctx, "schema-123")
		got := L1DefaultSchemaIDFromCtx(ctx)
		if got != "schema-123" {
			t.Errorf("L1DefaultSchemaIDFromCtx = %q, want %q", got, "schema-123")
		}
	})

	t.Run("missing_context_returns_zero_values", func(t *testing.T) {
		ctx := context.Background()
		if r := L1SchemaReaderFromCtx(ctx); r != nil {
			t.Errorf("L1SchemaReaderFromCtx on empty ctx = %v, want nil", r)
		}
		if id := L1DefaultSchemaIDFromCtx(ctx); id != "" {
			t.Errorf("L1DefaultSchemaIDFromCtx on empty ctx = %q, want empty", id)
		}
	})
}

func TestSanitizeFieldKind(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"decision", "decision"},
		{"artifact", "artifact"},
		{"progress", "progress"},
		{"constraint", "constraint"},
		{"string", "string"},
		{"number", "number"},
		{"boolean", "boolean"},
		{"json", "json"},
		{"reference", "reference"},
		{"markdown", "markdown"},
		{"", "string"},
		{"  decision  ", "decision"},
		{"unknown", "string"},
		{"DECISION", "string"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := sanitizeFieldKind(tt.input); got != tt.want {
				t.Errorf("sanitizeFieldKind(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

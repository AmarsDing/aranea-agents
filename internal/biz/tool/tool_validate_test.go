package tool

import (
	"testing"

	"aranea-agents/pkg/apierror"
)

func isAPIError(err error, domain, message string) bool {
	if err == nil {
		return false
	}
	ae, ok := apierror.From(err)
	if !ok {
		return false
	}
	return ae.Domain == domain && ae.Message == message
}

func TestValidateToolUpsert(t *testing.T) {
	tests := []struct {
		name    string
		input   ToolUpsertInput
		wantErr bool
		reason  string
		message string
	}{
		{
			name:    "missing key",
			input:   ToolUpsertInput{DisplayName: "foo"},
			wantErr: true,
			reason:  "TOOL",
			message: "tool key is required",
		},
		{
			name:    "missing display_name",
			input:   ToolUpsertInput{Key: "my_tool"},
			wantErr: true,
			reason:  "TOOL",
			message: "display name is required",
		},
		{
			name:    "invalid risk_level",
			input:   ToolUpsertInput{Key: "my_tool", DisplayName: "My Tool", RiskLevel: "extreme"},
			wantErr: true,
			reason:  "TOOL",
			message: "invalid risk_level",
		},
		{
			name:    "valid input",
			input:   ToolUpsertInput{Key: "my_tool", DisplayName: "My Tool", RiskLevel: "low", Source: "custom"},
			wantErr: false,
		},
		{
			name:    "empty source defaults",
			input:   ToolUpsertInput{Key: "my_tool", DisplayName: "My Tool"},
			wantErr: false,
		},
		{
			name:    "whitespace only key",
			input:   ToolUpsertInput{Key: "   ", DisplayName: "My Tool"},
			wantErr: true,
			reason:  "TOOL",
			message: "tool key is required",
		},
		{
			name:    "whitespace only display_name",
			input:   ToolUpsertInput{Key: "my_tool", DisplayName: "   "},
			wantErr: true,
			reason:  "TOOL",
			message: "display name is required",
		},
		{
			name:    "risk_level low",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", RiskLevel: "low"},
			wantErr: false,
		},
		{
			name:    "risk_level medium",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", RiskLevel: "medium"},
			wantErr: false,
		},
		{
			name:    "risk_level high",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", RiskLevel: "high"},
			wantErr: false,
		},
		{
			name:    "risk_level critical",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", RiskLevel: "critical"},
			wantErr: false,
		},
		{
			name:    "risk_level case insensitive",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", RiskLevel: "Low"},
			wantErr: false,
		},
		{
			name:    "invalid source",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", Source: "unknown"},
			wantErr: true,
			reason:  "TOOL",
			message: "invalid source",
		},
		{
			name:    "source case insensitive",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", Source: "Builtin"},
			wantErr: false,
		},
		{
			name:    "invalid parameters_schema_json",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", ParametersSchemaJSON: "not-json"},
			wantErr: true,
			reason:  "TOOL",
			message: "parameters_schema_json must be valid JSON",
		},
		{
			name:    "parameters_schema_json array rejected",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", ParametersSchemaJSON: `[1,2]`},
			wantErr: true,
			reason:  "TOOL",
			message: "parameters_schema_json must be a JSON object",
		},
		{
			name:    "empty risk_level allowed",
			input:   ToolUpsertInput{Key: "t", DisplayName: "T", RiskLevel: ""},
			wantErr: false,
		},
		{
			name: "valid with all json fields",
			input: ToolUpsertInput{
				Key:                  "t",
				DisplayName:          "T",
				ParametersSchemaJSON: `{"type":"object"}`,
				ResultSchemaJSON:     `{"type":"object"}`,
				ConfigSchemaJSON:     `{"type":"object"}`,
				ConfigJSON:           `{"k":"v"}`,
				DefaultConfigJSON:    `{"k":"v"}`,
				MetadataJSON:         `{"k":"v"}`,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolUpsert(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !isAPIError(err, tt.reason, tt.message) {
					t.Fatalf("expected apierror domain=%q message=%q, got %v", tt.reason, tt.message, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestAssertToolMutable(t *testing.T) {
	tests := []struct {
		name     string
		existing Tool
		input    ToolUpsertInput
		wantErr  bool
		reason   string
		message  string
	}{
		{
			name:     "custom tool allows changes",
			existing: Tool{Key: "my_tool", Source: "custom", Readonly: false},
			input:    ToolUpsertInput{Key: "my_tool_v2", Source: "external"},
			wantErr:  false,
		},
		{
			name:     "readonly builtin tool key change rejected",
			existing: Tool{Key: "read_file", Source: "builtin", Readonly: true},
			input:    ToolUpsertInput{Key: "write_file", Source: "builtin"},
			wantErr:  true,
			reason:   "TOOL",
			message:  "readonly tool key cannot change",
		},
		{
			name:     "readonly tool same key allowed",
			existing: Tool{Key: "read_file", Source: "builtin", Readonly: true},
			input:    ToolUpsertInput{Key: "read_file", Source: "builtin", DisplayName: "Read File V2"},
			wantErr:  false,
		},
		{
			name:     "builtin policy field change rejected",
			existing: Tool{Key: "read_file", Source: "builtin", Readonly: true, RequiresConfirmation: false},
			input:    ToolUpsertInput{Key: "read_file", Source: "builtin", RequiresConfirmation: true},
			wantErr:  true,
			reason:   "TOOL",
			message:  "builtin tool policy fields are read-only (requires_confirmation/supports_streaming/supports_concurrency)",
		},
		{
			name:     "readonly tool source change rejected",
			existing: Tool{Key: "read_file", Source: "builtin", Readonly: true},
			input:    ToolUpsertInput{Key: "read_file", Source: "custom"},
			wantErr:  true,
			reason:   "TOOL",
			message:  "readonly tool source cannot change",
		},
		{
			name:     "readonly tool empty source input allowed",
			existing: Tool{Key: "read_file", Source: "builtin", Readonly: true},
			input:    ToolUpsertInput{Key: "read_file", Source: "", DisplayName: "Updated"},
			wantErr:  false,
		},
		{
			name:     "readonly tool source case insensitive match",
			existing: Tool{Key: "read_file", Source: "builtin", Readonly: true},
			input:    ToolUpsertInput{Key: "read_file", Source: "Builtin"},
			wantErr:  false,
		},
		{
			name:     "readonly tool key whitespace mismatch",
			existing: Tool{Key: "read_file", Source: "builtin", Readonly: true},
			input:    ToolUpsertInput{Key: "  read_file  ", Source: "builtin"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AssertToolMutable(tt.existing, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !isAPIError(err, tt.reason, tt.message) {
					t.Fatalf("expected apierror domain=%q message=%q, got %v", tt.reason, tt.message, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestAssertToolDeletable(t *testing.T) {
	tests := []struct {
		name    string
		tool    Tool
		wantErr bool
		reason  string
		message string
	}{
		{
			name:    "builtin readonly tool cannot be deleted",
			tool:    Tool{Key: "read_file", Source: "builtin", Readonly: true},
			wantErr: true,
			reason:  "TOOL",
			message: "readonly tool cannot be deleted",
		},
		{
			name:    "custom tool can be deleted",
			tool:    Tool{Key: "my_tool", Source: "custom", Readonly: false},
			wantErr: false,
		},
		{
			name:    "mcp readonly tool cannot be deleted",
			tool:    Tool{Key: "mcp_call", Source: "mcp", Readonly: true},
			wantErr: true,
			reason:  "TOOL",
			message: "readonly tool cannot be deleted",
		},
		{
			name:    "external non-readonly tool can be deleted",
			tool:    Tool{Key: "ext_tool", Source: "external", Readonly: false},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AssertToolDeletable(tt.tool)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !isAPIError(err, tt.reason, tt.message) {
					t.Fatalf("expected apierror domain=%q message=%q, got %v", tt.reason, tt.message, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestRequireJSONObject(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		raw     string
		wantErr bool
		reason  string
		message string
	}{
		{
			name:    "empty string allowed",
			field:   "config_json",
			raw:     "",
			wantErr: false,
		},
		{
			name:    "whitespace only allowed",
			field:   "config_json",
			raw:     "   ",
			wantErr: false,
		},
		{
			name:    "valid JSON object",
			field:   "config_json",
			raw:     `{"key":"value"}`,
			wantErr: false,
		},
		{
			name:    "valid empty JSON object",
			field:   "config_json",
			raw:     `{}`,
			wantErr: false,
		},
		{
			name:    "JSON empty array rejected",
			field:   "config_json",
			raw:     `[]`,
			wantErr: true,
			reason:  "TOOL",
			message: "config_json must be a JSON object",
		},
		{
			name:    "JSON array with items rejected",
			field:   "parameters_schema_json",
			raw:     `[1,2,3]`,
			wantErr: true,
			reason:  "TOOL",
			message: "parameters_schema_json must be a JSON object",
		},
		{
			name:    "non-JSON string rejected",
			field:   "config_json",
			raw:     "not-json",
			wantErr: true,
			reason:  "TOOL",
			message: "config_json must be valid JSON",
		},
		{
			name:    "null rejected as non-object",
			field:   "config_json",
			raw:     "null",
			wantErr: true,
			reason:  "TOOL",
			message: "config_json must be a JSON object",
		},
		{
			name:    "JSON boolean rejected",
			field:   "config_json",
			raw:     "true",
			wantErr: true,
			reason:  "TOOL",
			message: "config_json must be a JSON object",
		},
		{
			name:    "JSON number rejected",
			field:   "config_json",
			raw:     "42",
			wantErr: true,
			reason:  "TOOL",
			message: "config_json must be a JSON object",
		},
		{
			name:    "JSON string rejected",
			field:   "config_json",
			raw:     `"hello"`,
			wantErr: true,
			reason:  "TOOL",
			message: "config_json must be a JSON object",
		},
		{
			name:    "nested JSON object allowed",
			field:   "config_json",
			raw:     `{"a":{"b":1}}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireJSONObject(tt.field, tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !isAPIError(err, tt.reason, tt.message) {
					t.Fatalf("expected apierror domain=%q message=%q, got %v", tt.reason, tt.message, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

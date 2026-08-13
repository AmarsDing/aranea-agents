package plugin

import (
	"testing"
)

func TestAdminPerms(t *testing.T) {
	p := AdminPerms()
	if !p.CanView {
		t.Error("CanView should be true")
	}
	if !p.CanToggle {
		t.Error("CanToggle should be true")
	}
	if !p.CanEditConfig {
		t.Error("CanEditConfig should be true")
	}
	if !p.CanViewLogs {
		t.Error("CanViewLogs should be true")
	}
}

func TestValidateJSONSchema(t *testing.T) {
	tests := []struct {
		name    string
		schema  string
		doc     string
		wantErr bool
	}{
		{
			"valid doc against schema",
			`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
			`{"name":"hello"}`,
			false,
		},
		{
			"invalid doc against schema",
			`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
			`{"age":1}`,
			true,
		},
		{
			"empty schema passes",
			"",
			`{"anything":1}`,
			false,
		},
		{
			"empty object schema passes",
			"{}",
			`{"anything":1}`,
			false,
		},
		{
			"empty doc defaults to empty object",
			`{"type":"object","properties":{"name":{"type":"string"}}}`,
			"",
			false,
		},
		{
			"type mismatch",
			`{"type":"object","properties":{"count":{"type":"integer"}}}`,
			`{"count":"not-a-number"}`,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJSONSchema(tt.schema, tt.doc)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJSONSchema() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

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

func TestNormalizeSandboxMode(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		riskLevel string
		want      SandboxMode
	}{
		{"none", "none", "", SandboxNone},
		{"none uppercase", "NONE", "", SandboxNone},
		{"none with spaces", " none ", "", SandboxNone},
		{"process", "process", "", SandboxProcess},
		{"process uppercase", "PROCESS", "", SandboxProcess},
		{"container", "container", "", SandboxContainer},
		{"container mixed case", "Container", "", SandboxContainer},
		{"unknown defaults to none", "unknown", "", SandboxNone},
		{"empty defaults to none", "", "", SandboxNone},
		{"high risk defaults to process", "", "high", SandboxProcess},
		{"critical risk defaults to process", "", "critical", SandboxProcess},
		{"high risk case insensitive", "", "HIGH", SandboxProcess},
		{"critical risk case insensitive", "", "Critical", SandboxProcess},
		{"low risk defaults to none", "", "low", SandboxNone},
		{"medium risk defaults to none", "", "medium", SandboxNone},
		{"high risk but explicit none", "none", "high", SandboxNone},
		{"high risk but explicit container", "container", "high", SandboxContainer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeSandboxMode(tt.raw, tt.riskLevel)
			if got != tt.want {
				t.Errorf("NormalizeSandboxMode(%q, %q) = %q, want %q", tt.raw, tt.riskLevel, got, tt.want)
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name   string
		policy VersionPolicy
		latest string
		want   string
	}{
		{
			"pinned takes priority",
			VersionPolicy{Pinned: "1.2.3", MinVersion: "1.0.0", MaxVersion: "2.0.0"},
			"1.9.0",
			"1.2.3",
		},
		{
			"pinned with spaces trimmed",
			VersionPolicy{Pinned: "  2.0.0  "},
			"3.0.0",
			"2.0.0",
		},
		{
			"empty pinned falls back to latest",
			VersionPolicy{Pinned: "", MinVersion: "1.0.0"},
			"1.5.0",
			"1.5.0",
		},
		{
			"whitespace pinned falls back to latest",
			VersionPolicy{Pinned: "  "},
			"1.5.0",
			"1.5.0",
		},
		{
			"latest trimmed",
			VersionPolicy{},
			"  4.0.0  ",
			"4.0.0",
		},
		{
			"both empty",
			VersionPolicy{},
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveVersion(tt.policy, tt.latest)
			if got != tt.want {
				t.Errorf("ResolveVersion(%+v, %q) = %q, want %q", tt.policy, tt.latest, got, tt.want)
			}
		})
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

package loader

import (
	"testing"
)

func TestDeriveToolsProfile(t *testing.T) {
	tests := []struct {
		name        string
		positionKey string
		want        string
	}{
		{"analyst keyword", "senior-analyst", "analyst"},
		{"research keyword", "research-lead", "analyst"},
		{"engineer keyword", "software-engineer", "general"},
		{"developer keyword", "frontend-developer", "general"},
		{"programmer keyword", "backend-programmer", "general"},
		{"coordinator keyword", "project-coordinator", "coordinator"},
		{"manager keyword", "product-manager", "coordinator"},
		{"director keyword", "creative-director", "coordinator"},
		{"writer keyword", "content-writer", "writer"},
		{"editor keyword", "chief-editor", "writer"},
		{"designer keyword", "ui-designer", "creative"},
		{"artist keyword", "3d-artist", "creative"},
		{"empty position key", "", "general"},
		{"unknown position key", "sales-rep", "general"},
		{"case insensitive", "SENIOR-ANALYST", "analyst"},
		{"mixed case", "Senior-Analyst", "analyst"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveToolsProfile(tt.positionKey)
			if got != tt.want {
				t.Errorf("deriveToolsProfile(%q) = %q, want %q", tt.positionKey, got, tt.want)
			}
		})
	}
}

func TestFillDefaults(t *testing.T) {
	t.Run("fills all empty defaults", func(t *testing.T) {
		spec := IndustrySpec{
			IndustryKey: "finance",
			Defaults:    AgentDefaults{},
			Agents:      []AgentSpec{},
		}
		fillDefaults(&spec)

		if spec.Defaults.Provider != "openrouter" {
			t.Errorf("Provider = %q, want %q", spec.Defaults.Provider, "openrouter")
		}
		if spec.Defaults.FastModel != "gpt-4.1-mini" {
			t.Errorf("FastModel = %q, want %q", spec.Defaults.FastModel, "gpt-4.1-mini")
		}
		if spec.Defaults.StrongModel != "gpt-4.1" {
			t.Errorf("StrongModel = %q, want %q", spec.Defaults.StrongModel, "gpt-4.1")
		}
		if spec.Defaults.SystemPromptMode != "file" {
			t.Errorf("SystemPromptMode = %q, want %q", spec.Defaults.SystemPromptMode, "file")
		}
		if spec.Defaults.ContextWindow != 64000 {
			t.Errorf("ContextWindow = %d, want %d", spec.Defaults.ContextWindow, 64000)
		}
		if spec.Defaults.CodeExecutor != "local" {
			t.Errorf("CodeExecutor = %q, want %q", spec.Defaults.CodeExecutor, "local")
		}
		wantDeny := []string{"workspace_exec", "filesystem", "shell", "bash"}
		if len(spec.Defaults.ToolsDeny) != len(wantDeny) {
			t.Errorf("ToolsDeny length = %d, want %d", len(spec.Defaults.ToolsDeny), len(wantDeny))
		}
		for i, v := range wantDeny {
			if spec.Defaults.ToolsDeny[i] != v {
				t.Errorf("ToolsDeny[%d] = %q, want %q", i, spec.Defaults.ToolsDeny[i], v)
			}
		}
	})

	t.Run("preserves already-set defaults", func(t *testing.T) {
		spec := IndustrySpec{
			IndustryKey: "healthcare",
			Defaults: AgentDefaults{
				Provider:         "anthropic",
				FastModel:        "claude-3-haiku",
				StrongModel:      "claude-3-opus",
				SystemPromptMode: "inline",
				ContextWindow:    128000,
				CodeExecutor:     "docker",
				ToolsDeny:        []string{"custom_deny"},
			},
			Agents: []AgentSpec{},
		}
		fillDefaults(&spec)

		if spec.Defaults.Provider != "anthropic" {
			t.Errorf("Provider = %q, want %q", spec.Defaults.Provider, "anthropic")
		}
	})

	t.Run("fills agent defaults", func(t *testing.T) {
		spec := IndustrySpec{
			IndustryKey: "retail",
			Defaults:    AgentDefaults{},
			Agents: []AgentSpec{
				{Key: "a1"},
				{Key: "a2", Variant: "special", ModelTier: "strong"},
			},
		}
		fillDefaults(&spec)

		if spec.Agents[0].Variant != "general" {
			t.Errorf("Agent[0].Variant = %q, want %q", spec.Agents[0].Variant, "general")
		}
		if spec.Agents[1].Variant != "special" {
			t.Errorf("Agent[1].Variant = %q, want %q", spec.Agents[1].Variant, "special")
		}
	})
}

func TestYamlUnmarshal(t *testing.T) {
	t.Run("parses valid YAML into IndustrySpec", func(t *testing.T) {
		yamlData := []byte(`
industry_key: fintech
defaults:
  provider: openrouter
  fast_model: gpt-4.1-mini
  strong_model: gpt-4.1
agents:
  - key: risk-analyst
    position_key: risk-analyst
    variant: risk
teams:
  - key: risk-team
    display_name: Risk Team
    mode: sequential
`)
		var spec IndustrySpec
		if err := yamlUnmarshal(yamlData, &spec); err != nil {
			t.Fatalf("yamlUnmarshal() error = %v", err)
		}
		if spec.IndustryKey != "fintech" {
			t.Errorf("IndustryKey = %q, want %q", spec.IndustryKey, "fintech")
		}
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		yamlData := []byte(`: invalid: yaml: [`)
		var spec IndustrySpec
		if err := yamlUnmarshal(yamlData, &spec); err == nil {
			t.Error("yamlUnmarshal() expected error for invalid YAML, got nil")
		}
	})
}

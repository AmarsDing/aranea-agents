package loader

import (
	"os"
	"path/filepath"
	"testing"
)

// TestYamlUnmarshal 覆盖三种 spec 的 YAML 解析路径：CompanySpec / OrganizationSpec / AgentTemplatesSpec。
// 这是 loader 包对外暴露的三个 Load* 入口的反序列化基线，回归可避免字段 tag 错位。
func TestYamlUnmarshal(t *testing.T) {
	t.Run("parses valid YAML into CompanySpec", func(t *testing.T) {
		yamlData := []byte(`
company_key: fintech
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
		var spec CompanySpec
		if err := yamlUnmarshal(yamlData, &spec); err != nil {
			t.Fatalf("yamlUnmarshal() error = %v", err)
		}
		if spec.CompanyKey != "fintech" {
			t.Errorf("CompanyKey = %q, want %q", spec.CompanyKey, "fintech")
		}
		if spec.Defaults.Provider != "openrouter" {
			t.Errorf("Provider = %q, want %q", spec.Defaults.Provider, "openrouter")
		}
		if len(spec.Agents) != 1 || spec.Agents[0].Key != "risk-analyst" {
			t.Errorf("Agents = %+v, want one agent with key risk-analyst", spec.Agents)
		}
		if len(spec.Teams) != 1 || spec.Teams[0].Key != "risk-team" {
			t.Errorf("Teams = %+v, want one team with key risk-team", spec.Teams)
		}
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		yamlData := []byte(`: invalid: yaml: [`)
		var spec CompanySpec
		if err := yamlUnmarshal(yamlData, &spec); err == nil {
			t.Error("yamlUnmarshal() expected error for invalid YAML, got nil")
		}
	})

	t.Run("parses organization spec", func(t *testing.T) {
		yamlData := []byte(`
companies:
  - key: tech
    name: Technology
    icon:
    description: Tech company
    sort_order: 1
    departments:
      - key: eng
        name: Engineering
        sort_order: 1
        positions:
          - key: swe
            name: Software Engineer
            sort_order: 1
`)
		var spec OrganizationSpec
		if err := yamlUnmarshal(yamlData, &spec); err != nil {
			t.Fatalf("yamlUnmarshal() error = %v", err)
		}
		if len(spec.Companies) != 1 {
			t.Fatalf("Companies length = %d, want 1", len(spec.Companies))
		}
		comp := spec.Companies[0]
		if comp.Key != "tech" {
			t.Errorf("Key = %q, want %q", comp.Key, "tech")
		}
		if len(comp.Departments) != 1 || comp.Departments[0].Key != "eng" {
			t.Errorf("Departments = %+v, want one dept with key eng", comp.Departments)
		}
		if len(comp.Departments[0].Positions) != 1 || comp.Departments[0].Positions[0].Key != "swe" {
			t.Errorf("Positions = %+v, want one position with key swe", comp.Departments[0].Positions)
		}
	})

	t.Run("organization spec falls back to legacy industries key", func(t *testing.T) {
		yamlData := []byte(`
industries:
  - key: legacy-industry
    name: Legacy Industry
    sort_order: 1
`)
		var spec OrganizationSpec
		if err := yamlUnmarshal(yamlData, &spec); err != nil {
			t.Fatalf("yamlUnmarshal() error = %v", err)
		}
		got := spec.ResolvedCompanies()
		if len(got) != 1 || got[0].Key != "legacy-industry" {
			t.Errorf("ResolvedCompanies = %+v, want one legacy industry", got)
		}
	})

	t.Run("parses agent templates spec", func(t *testing.T) {
		yamlData := []byte(`
templates:
  - key: general-assistant
    label: General Assistant
    icon:
    display_name: General Assistant
    provider: openrouter
    model: gpt-4.1-mini
    description: A general-purpose assistant
    sort_order: 1
`)
		var spec AgentTemplatesSpec
		if err := yamlUnmarshal(yamlData, &spec); err != nil {
			t.Fatalf("yamlUnmarshal() error = %v", err)
		}
		if len(spec.Templates) != 1 {
			t.Fatalf("Templates length = %d, want 1", len(spec.Templates))
		}
		tmpl := spec.Templates[0]
		if tmpl.Key != "general-assistant" {
			t.Errorf("Key = %q, want %q", tmpl.Key, "general-assistant")
		}
		if tmpl.Provider != "openrouter" {
			t.Errorf("Provider = %q, want %q", tmpl.Provider, "openrouter")
		}
	})
}

// TestFillDefaults 验证 CompanySpec 缺省字段的填充行为，避免默认值漂移。
func TestFillDefaults(t *testing.T) {
	t.Run("fills all empty defaults", func(t *testing.T) {
		spec := CompanySpec{
			CompanyKey: "finance",
			Defaults:   AgentDefaults{},
			Agents:     []AgentSpec{},
		}
		fillDefaults(&spec)

		if spec.Defaults.Provider != DefaultProvider {
			t.Errorf("Provider = %q, want %q", spec.Defaults.Provider, DefaultProvider)
		}
		if spec.Defaults.FastModel != DefaultFastModel {
			t.Errorf("FastModel = %q, want %q", spec.Defaults.FastModel, DefaultFastModel)
		}
		if spec.Defaults.StrongModel != DefaultStrongModel {
			t.Errorf("StrongModel = %q, want %q", spec.Defaults.StrongModel, DefaultStrongModel)
		}
		if spec.Defaults.SystemPromptMode != DefaultSystemPromptMode {
			t.Errorf("SystemPromptMode = %q, want %q", spec.Defaults.SystemPromptMode, DefaultSystemPromptMode)
		}
		if spec.Defaults.ContextWindow != DefaultContextWindow {
			t.Errorf("ContextWindow = %d, want %d", spec.Defaults.ContextWindow, DefaultContextWindow)
		}
		if spec.Defaults.CodeExecutor != DefaultCodeExecutor {
			t.Errorf("CodeExecutor = %q, want %q", spec.Defaults.CodeExecutor, DefaultCodeExecutor)
		}
		if len(spec.Defaults.ToolsDeny) != len(DefaultToolsDeny) {
			t.Fatalf("ToolsDeny length = %d, want %d", len(spec.Defaults.ToolsDeny), len(DefaultToolsDeny))
		}
		for i, v := range DefaultToolsDeny {
			if spec.Defaults.ToolsDeny[i] != v {
				t.Errorf("ToolsDeny[%d] = %q, want %q", i, spec.Defaults.ToolsDeny[i], v)
			}
		}
	})

	t.Run("preserves already-set defaults", func(t *testing.T) {
		spec := CompanySpec{
			CompanyKey: "healthcare",
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
		if spec.Defaults.FastModel != "claude-3-haiku" {
			t.Errorf("FastModel = %q, want %q", spec.Defaults.FastModel, "claude-3-haiku")
		}
		if spec.Defaults.StrongModel != "claude-3-opus" {
			t.Errorf("StrongModel = %q, want %q", spec.Defaults.StrongModel, "claude-3-opus")
		}
		if spec.Defaults.SystemPromptMode != "inline" {
			t.Errorf("SystemPromptMode = %q, want %q", spec.Defaults.SystemPromptMode, "inline")
		}
		if spec.Defaults.ContextWindow != 128000 {
			t.Errorf("ContextWindow = %d, want %d", spec.Defaults.ContextWindow, 128000)
		}
		if spec.Defaults.CodeExecutor != "docker" {
			t.Errorf("CodeExecutor = %q, want %q", spec.Defaults.CodeExecutor, "docker")
		}
		if len(spec.Defaults.ToolsDeny) != 1 || spec.Defaults.ToolsDeny[0] != "custom_deny" {
			t.Errorf("ToolsDeny = %v, want [custom_deny]", spec.Defaults.ToolsDeny)
		}
	})

	t.Run("fills agent defaults", func(t *testing.T) {
		spec := CompanySpec{
			CompanyKey: "retail",
			Defaults:   AgentDefaults{},
			Agents: []AgentSpec{
				{Key: "a1"},
				{Key: "a2", Variant: "special", ModelTier: "strong"},
			},
		}
		fillDefaults(&spec)

		if spec.Agents[0].Variant != DefaultVariant {
			t.Errorf("Agent[0].Variant = %q, want %q", spec.Agents[0].Variant, DefaultVariant)
		}
		if spec.Agents[0].ModelTier != DefaultModelTier {
			t.Errorf("Agent[0].ModelTier = %q, want %q", spec.Agents[0].ModelTier, DefaultModelTier)
		}
		if spec.Agents[0].ToolsProfile != DefaultToolsProfile {
			t.Errorf("Agent[0].ToolsProfile = %q, want %q", spec.Agents[0].ToolsProfile, DefaultToolsProfile)
		}
		if spec.Agents[1].Variant != "special" {
			t.Errorf("Agent[1].Variant = %q, want %q", spec.Agents[1].Variant, "special")
		}
		if spec.Agents[1].ModelTier != "strong" {
			t.Errorf("Agent[1].ModelTier = %q, want %q", spec.Agents[1].ModelTier, "strong")
		}
	})

	t.Run("preserves explicitly set tools profile", func(t *testing.T) {
		spec := CompanySpec{
			CompanyKey: "legal",
			Defaults:   AgentDefaults{},
			Agents: []AgentSpec{
				{Key: "a1", ToolsProfile: "coordinator"},
			},
		}
		fillDefaults(&spec)

		if spec.Agents[0].ToolsProfile != "coordinator" {
			t.Errorf("Agent[0].ToolsProfile = %q, want %q", spec.Agents[0].ToolsProfile, "coordinator")
		}
	})
}

// TestDefaultsConsistency 验证默认值常量非零且与 fillDefaults 输出一致（P8 保障）。
func TestDefaultsConsistency(t *testing.T) {
	t.Run("all default constants are non-zero", func(t *testing.T) {
		if DefaultProvider == "" {
			t.Error("DefaultProvider is empty")
		}
		if DefaultFastModel == "" {
			t.Error("DefaultFastModel is empty")
		}
		if DefaultStrongModel == "" {
			t.Error("DefaultStrongModel is empty")
		}
		if DefaultSystemPromptMode == "" {
			t.Error("DefaultSystemPromptMode is empty")
		}
		if DefaultContextWindow == 0 {
			t.Error("DefaultContextWindow is zero")
		}
		if DefaultCodeExecutor == "" {
			t.Error("DefaultCodeExecutor is empty")
		}
		if DefaultVariant == "" {
			t.Error("DefaultVariant is empty")
		}
		if DefaultModelTier == "" {
			t.Error("DefaultModelTier is empty")
		}
		if DefaultToolsProfile == "" {
			t.Error("DefaultToolsProfile is empty")
		}
		if len(DefaultToolsDeny) == 0 {
			t.Error("DefaultToolsDeny is empty")
		}
	})

	t.Run("fillDefaults output matches constants", func(t *testing.T) {
		spec := CompanySpec{
			CompanyKey: "test",
			Defaults:   AgentDefaults{},
			Agents:     []AgentSpec{{Key: "a1"}},
		}
		fillDefaults(&spec)

		if spec.Defaults.Provider != DefaultProvider {
			t.Errorf("fillDefaults Provider = %q, constant = %q", spec.Defaults.Provider, DefaultProvider)
		}
		if spec.Defaults.FastModel != DefaultFastModel {
			t.Errorf("fillDefaults FastModel = %q, constant = %q", spec.Defaults.FastModel, DefaultFastModel)
		}
		if spec.Defaults.StrongModel != DefaultStrongModel {
			t.Errorf("fillDefaults StrongModel = %q, constant = %q", spec.Defaults.StrongModel, DefaultStrongModel)
		}
		if spec.Defaults.SystemPromptMode != DefaultSystemPromptMode {
			t.Errorf("fillDefaults SystemPromptMode = %q, constant = %q", spec.Defaults.SystemPromptMode, DefaultSystemPromptMode)
		}
		if spec.Defaults.ContextWindow != DefaultContextWindow {
			t.Errorf("fillDefaults ContextWindow = %d, constant = %d", spec.Defaults.ContextWindow, DefaultContextWindow)
		}
		if spec.Defaults.CodeExecutor != DefaultCodeExecutor {
			t.Errorf("fillDefaults CodeExecutor = %q, constant = %q", spec.Defaults.CodeExecutor, DefaultCodeExecutor)
		}
		if spec.Agents[0].Variant != DefaultVariant {
			t.Errorf("fillDefaults Variant = %q, constant = %q", spec.Agents[0].Variant, DefaultVariant)
		}
		if spec.Agents[0].ModelTier != DefaultModelTier {
			t.Errorf("fillDefaults ModelTier = %q, constant = %q", spec.Agents[0].ModelTier, DefaultModelTier)
		}
		if spec.Agents[0].ToolsProfile != DefaultToolsProfile {
			t.Errorf("fillDefaults ToolsProfile = %q, constant = %q", spec.Agents[0].ToolsProfile, DefaultToolsProfile)
		}
	})
}

// TestLoadCompanySpec 走完整路径：从临时目录写入 agents.yaml，调 LoadCompanySpec，
// 验证 CompanyKey 注入 + fillDefaults 触发。覆盖生产路径 data.SeedPackIndustry
// 调用 LoadCompanySpec 后的契约。
func TestLoadCompanySpec(t *testing.T) {
	dir := t.TempDir()
	companyDir := filepath.Join(dir, "demo")
	if err := os.Mkdir(companyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yamlContent := []byte(`
defaults:
  fast_model: ""
agents: []
teams: []
`)
	if err := os.WriteFile(filepath.Join(companyDir, "agents.yaml"), yamlContent, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	spec, err := LoadCompanySpec(dir, "demo")
	if err != nil {
		t.Fatalf("LoadCompanySpec() error = %v", err)
	}
	if spec.CompanyKey != "demo" {
		t.Errorf("CompanyKey = %q, want %q (injected by loader)", spec.CompanyKey, "demo")
	}
	if spec.Defaults.Provider != DefaultProvider {
		t.Errorf("Provider = %q, want %q (filled by default)", spec.Defaults.Provider, DefaultProvider)
	}
}

func TestLoadCompanySpec_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadCompanySpec(dir, "missing")
	if err == nil {
		t.Fatal("LoadCompanySpec() expected error for missing dir, got nil")
	}
}

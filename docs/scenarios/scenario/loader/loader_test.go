package loader

import (
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
)

func TestResolveModel(t *testing.T) {
	defaults := AgentDefaults{
		Provider:    "openrouter",
		FastModel:   "gpt-4.1-mini",
		StrongModel: "gpt-4.1",
	}

	tests := []struct {
		name             string
		tier             string
		wantProvider     string
		wantModel        string
	}{
		{"strong tier returns strong model", "strong", "openrouter", "gpt-4.1"},
		{"fast tier returns fast model", "fast", "openrouter", "gpt-4.1-mini"},
		{"empty tier returns fast model", "", "openrouter", "gpt-4.1-mini"},
		{"unknown tier returns fast model", "unknown", "openrouter", "gpt-4.1-mini"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProvider, gotModel := resolveModel(defaults, tt.tier)
			if gotProvider != tt.wantProvider {
				t.Errorf("resolveModel(%q) provider = %q, want %q", tt.tier, gotProvider, tt.wantProvider)
			}
			if gotModel != tt.wantModel {
				t.Errorf("resolveModel(%q) model = %q, want %q", tt.tier, gotModel, tt.wantModel)
			}
		})
	}
}

func TestJsonStringList(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  string
	}{
		{"empty slice returns empty JSON array", []string{}, "[]"},
		{"nil slice returns empty JSON array", nil, "[]"},
		{"single item", []string{"foo"}, `["foo"]`},
		{"multiple items", []string{"a", "b", "c"}, `["a","b","c"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonStringList(tt.items)
			if got != tt.want {
				t.Errorf("jsonStringList(%v) = %q, want %q", tt.items, got, tt.want)
			}
			if len(tt.items) > 0 {
				var parsed []string
				if err := json.Unmarshal([]byte(got), &parsed); err != nil {
					t.Errorf("jsonStringList result is not valid JSON: %v", err)
				}
			}
		})
	}
}

func TestSkillRuntimeJSON(t *testing.T) {
	tests := []struct {
		name   string
		skills []string
		want   string
	}{
		{"no skills returns empty object", nil, "{}"},
		{"empty skills returns empty object", []string{}, "{}"},
		{"single skill", []string{"search"}, `{"allowed_slugs":["search"]}`},
		{"multiple skills", []string{"search", "code"}, `{"allowed_slugs":["search","code"]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skillRuntimeJSON(tt.skills...)
			if got != tt.want {
				t.Errorf("skillRuntimeJSON(%v) = %q, want %q", tt.skills, got, tt.want)
			}
			var parsed map[string]any
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Errorf("skillRuntimeJSON result is not valid JSON: %v", err)
			}
		})
	}
}

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
			CompanyKey: "finance",
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
		spec := IndustrySpec{
			CompanyKey: "retail",
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
		if spec.Agents[0].ModelTier != "fast" {
			t.Errorf("Agent[0].ModelTier = %q, want %q", spec.Agents[0].ModelTier, "fast")
		}
		if spec.Agents[0].ToolsProfile != "general" {
			t.Errorf("Agent[0].ToolsProfile = %q, want %q", spec.Agents[0].ToolsProfile, "general")
		}
		if spec.Agents[1].Variant != "special" {
			t.Errorf("Agent[1].Variant = %q, want %q", spec.Agents[1].Variant, "special")
		}
		if spec.Agents[1].ModelTier != "strong" {
			t.Errorf("Agent[1].ModelTier = %q, want %q", spec.Agents[1].ModelTier, "strong")
		}
	})

	t.Run("derives tools profile from position key when tools allow is set", func(t *testing.T) {
		spec := IndustrySpec{
			CompanyKey: "legal",
			Defaults:    AgentDefaults{},
			Agents: []AgentSpec{
				{Key: "a1", PositionKey: "data-analyst", ToolsAllow: []string{"search"}},
			},
		}
		fillDefaults(&spec)

		if spec.Agents[0].ToolsProfile != "analyst" {
			t.Errorf("Agent[0].ToolsProfile = %q, want %q", spec.Agents[0].ToolsProfile, "analyst")
		}
	})

	t.Run("preserves explicitly set tools profile", func(t *testing.T) {
		spec := IndustrySpec{
			CompanyKey: "legal",
			Defaults:    AgentDefaults{},
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

func TestYamlUnmarshal(t *testing.T) {
	t.Run("parses valid YAML into IndustrySpec", func(t *testing.T) {
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
		var spec IndustrySpec
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
		var spec IndustrySpec
		if err := yamlUnmarshal(yamlData, &spec); err == nil {
			t.Error("yamlUnmarshal() expected error for invalid YAML, got nil")
		}
	})

	t.Run("parses taxonomy spec", func(t *testing.T) {
		yamlData := []byte(`
industries:
  - key: tech
    name: Technology
    icon: 💻
    description: Tech industry
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
		var spec TaxonomySpec
		if err := yamlUnmarshal(yamlData, &spec); err != nil {
			t.Fatalf("yamlUnmarshal() error = %v", err)
		}
		if len(spec.Industries) != 1 {
			t.Fatalf("Industries length = %d, want 1", len(spec.Industries))
		}
		ind := spec.Industries[0]
		if ind.Key != "tech" {
			t.Errorf("Key = %q, want %q", ind.Key, "tech")
		}
		if len(ind.Departments) != 1 || ind.Departments[0].Key != "eng" {
			t.Errorf("Departments = %+v, want one dept with key eng", ind.Departments)
		}
		if len(ind.Departments[0].Positions) != 1 || ind.Departments[0].Positions[0].Key != "swe" {
			t.Errorf("Positions = %+v, want one position with key swe", ind.Departments[0].Positions)
		}
	})

	t.Run("parses agent templates spec", func(t *testing.T) {
		yamlData := []byte(`
templates:
  - key: general-assistant
    label: General Assistant
    icon: 🤖
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

func TestConvertGraphSpec(t *testing.T) {
	keyToID := map[string]string{
		"agent-a": "id-001",
		"agent-b": "id-002",
	}

	t.Run("converts nodes and edges", func(t *testing.T) {
		gs := &GraphSpec{
			Layout: "horizontal",
			Nodes: []GraphNodeSpec{
				{ID: "n1", Type: "agent", Label: "Agent A", AgentKey: "agent-a", Role: "worker"},
				{ID: "n2", Type: "agent", Label: "Agent B", AgentKey: "agent-b", Role: "reviewer"},
				{ID: "n3", Type: "router", Label: "Router"},
			},
			Edges: []GraphEdgeSpec{
				{ID: "e1", Source: "n1", Target: "n2"},
				{ID: "e2", Source: "n3", Target: "n1"},
			},
		}

		result := convertGraphSpec(gs, keyToID)

		if result.Version != 1 {
			t.Errorf("Version = %d, want 1", result.Version)
		}
		if result.Layout != "horizontal" {
			t.Errorf("Layout = %q, want %q", result.Layout, "horizontal")
		}
		if len(result.Nodes) != 3 {
			t.Fatalf("Nodes length = %d, want 3", len(result.Nodes))
		}
		if result.Nodes[0].AgentID != "id-001" {
			t.Errorf("Nodes[0].AgentID = %q, want %q", result.Nodes[0].AgentID, "id-001")
		}
		if result.Nodes[0].Role != "worker" {
			t.Errorf("Nodes[0].Role = %q, want %q", result.Nodes[0].Role, "worker")
		}
		if result.Nodes[2].AgentID != "" {
			t.Errorf("Nodes[2].AgentID = %q, want empty (no agent_key)", result.Nodes[2].AgentID)
		}
		if len(result.Edges) != 2 {
			t.Fatalf("Edges length = %d, want 2", len(result.Edges))
		}
		if result.Edges[0].Source != "n1" || result.Edges[0].Target != "n2" {
			t.Errorf("Edges[0] = %+v, want Source=n1 Target=n2", result.Edges[0])
		}
	})

	t.Run("missing agent key maps to empty AgentID", func(t *testing.T) {
		gs := &GraphSpec{
			Nodes: []GraphNodeSpec{
				{ID: "n1", Type: "agent", Label: "Unknown", AgentKey: "nonexistent"},
			},
		}
		result := convertGraphSpec(gs, keyToID)
		if result.Nodes[0].AgentID != "" {
			t.Errorf("AgentID = %q, want empty for missing key", result.Nodes[0].AgentID)
		}
	})

	t.Run("empty graph spec", func(t *testing.T) {
		gs := &GraphSpec{}
		result := convertGraphSpec(gs, keyToID)
		if len(result.Nodes) != 0 {
			t.Errorf("Nodes length = %d, want 0", len(result.Nodes))
		}
		if len(result.Edges) != 0 {
			t.Errorf("Edges length = %d, want 0", len(result.Edges))
		}
	})
}

func TestBuildBizTeam(t *testing.T) {
	spec := &IndustrySpec{CompanyKey: "fintech"}
	keyToID := map[string]string{
		"analyst": "id-a",
		"writer":  "id-b",
	}

	t.Run("builds team with members and graph", func(t *testing.T) {
		ts := &TeamSpec{
			Key:            "risk-team",
			DisplayName:    "Risk Analysis Team",
			Mode:           "sequential",
			Description:    "Analyzes risk",
			MaxConcurrency: 2,
			TimeoutSeconds: 300,
			IntentAnchorKey: "analyst",
			SynthesizerKey:  "writer",
			Members: []TeamMemberSpec{
				{AgentKey: "analyst", Role: "worker", Name: "Analyst", SortOrder: 1, TaskPrompt: "Analyze risk"},
				{AgentKey: "writer", Role: "reviewer", Name: "Writer", SortOrder: 2, TaskPrompt: "Write report"},
			},
			Graph: &GraphSpec{
				Layout: "horizontal",
				Nodes: []GraphNodeSpec{
					{ID: "n1", Type: "agent", AgentKey: "analyst", Role: "worker"},
				},
				Edges: []GraphEdgeSpec{
					{ID: "e1", Source: "n1", Target: "n2"},
				},
			},
		}

		team, err := BuildBizTeamFromSpec(spec, ts, keyToID)
		if err != nil {
			t.Fatalf("BuildBizTeamFromSpec() error = %v", err)
		}
		if team.TeamKey != "risk-team" {
			t.Errorf("TeamKey = %q, want %q", team.TeamKey, "risk-team")
		}
		if team.DisplayName != "Risk Analysis Team" {
			t.Errorf("DisplayName = %q, want %q", team.DisplayName, "Risk Analysis Team")
		}
		if team.Status != "active" {
			t.Errorf("Status = %q, want %q", team.Status, "active")
		}
		if team.DepartmentID != "fintech" {
			t.Errorf("DepartmentID = %q, want %q", team.DepartmentID, "fintech")
		}
		if team.DefinitionJSON == "" {
			t.Error("DefinitionJSON is empty, want non-empty")
		}
	})

	t.Run("builds team with critic loop", func(t *testing.T) {
		ts := &TeamSpec{
			Key:  "critic-team",
			Mode: "sequential",
			CriticLoop: &CriticLoopSpec{
				MaxIterations:  3,
				ScoreThreshold: 0.8,
			},
			Members: []TeamMemberSpec{
				{AgentKey: "analyst", Role: "worker", Name: "Analyst"},
			},
		}

		team, err := BuildBizTeamFromSpec(spec, ts, keyToID)
		if err != nil {
			t.Fatalf("BuildBizTeamFromSpec() error = %v", err)
		}

		var ospec biz.OrchestrationSpec
		if err := json.Unmarshal([]byte(team.DefinitionJSON), &ospec); err != nil {
			t.Fatalf("unmarshal DefinitionJSON: %v", err)
		}
		if ospec.CriticLoop == nil {
			t.Fatal("CriticLoop is nil, want non-nil")
		}
		if ospec.CriticLoop.MaxIterations != 3 {
			t.Errorf("CriticLoop.MaxIterations = %d, want 3", ospec.CriticLoop.MaxIterations)
		}
		if ospec.CriticLoop.ScoreThreshold != 0.8 {
			t.Errorf("CriticLoop.ScoreThreshold = %f, want 0.8", ospec.CriticLoop.ScoreThreshold)
		}
	})

	t.Run("returns error for missing agent key", func(t *testing.T) {
		ts := &TeamSpec{
			Key: "bad-team",
			Members: []TeamMemberSpec{
				{AgentKey: "nonexistent", Role: "worker", Name: "Ghost"},
			},
		}

		_, err := BuildBizTeamFromSpec(spec, ts, keyToID)
		if err == nil {
			t.Error("BuildBizTeamFromSpec() expected error for missing agent key, got nil")
		}
	})
}

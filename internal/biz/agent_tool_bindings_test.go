package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type bindingsAgentReader struct {
	AgentReader
	agents []Agent
}

func (r *bindingsAgentReader) SearchAgents(context.Context, AgentListQuery) (AgentListResult, error) {
	return AgentListResult{Items: r.agents, Total: len(r.agents)}, nil
}

type bindingsSettingsRepo struct {
	byAgent map[string]AgentRuntimeSettings
}

func (r *bindingsSettingsRepo) GetAgentRuntimeSettings(_ context.Context, id string) (AgentRuntimeSettings, error) {
	if s, ok := r.byAgent[id]; ok {
		return s, nil
	}
	return AgentRuntimeSettings{}, ErrNotFound
}

func (r *bindingsSettingsRepo) ListAgentRuntimeSettings(context.Context) (map[string]AgentRuntimeSettings, error) {
	return r.byAgent, nil
}

func (r *bindingsSettingsRepo) UpsertAgentRuntimeSettings(_ context.Context, v AgentRuntimeSettings) (AgentRuntimeSettings, error) {
	return v, nil
}

type bindingsToolReader struct {
	ToolRegistryReader
	tool      Tool
	catalog   []Tool
	overrides []ToolAgentOverride
}

func (r *bindingsToolReader) GetTool(context.Context, string) (Tool, error) { return r.tool, nil }

func (r *bindingsToolReader) SearchTools(context.Context, ToolListQuery) (ToolListResult, error) {
	return ToolListResult{Items: r.catalog, Total: len(r.catalog)}, nil
}

func (r *bindingsToolReader) ListToolAgentOverrides(context.Context, string) ([]ToolAgentOverride, error) {
	return r.overrides, nil
}

func newBindingsUsecase(reader AgentReader, settings AgentRuntimeSettingsRepo, tools ToolRegistryReader) *AgentUsecase {
	return NewAgentUsecase(AgentUsecaseDeps{
		Reader:   reader,
		Settings: settings,
		Tools:    tools,
		Lg:       loggateway.NewNoop(),
	})
}

func bindingByAgentID(items []ToolAgentBinding, id string) *ToolAgentBinding {
	for i := range items {
		if items[i].AgentID == id {
			return &items[i]
		}
	}
	return nil
}

// Agents without a settings row must fall back to defaults (coding profile,
// tools enabled) instead of being treated as tools-disabled.
func TestGetToolAgentBindings_defaultsWhenSettingsRowMissing(t *testing.T) {
	reader := &bindingsAgentReader{agents: []Agent{
		{ID: "a1", AgentKey: "k1", DisplayName: "Agent One", Status: "active"},
	}}
	settings := &bindingsSettingsRepo{byAgent: map[string]AgentRuntimeSettings{}}
	tool := Tool{Key: "read_file", Enabled: true, Category: "filesystem", Source: "builtin"}
	tools := &bindingsToolReader{tool: tool, catalog: []Tool{tool}}

	uc := newBindingsUsecase(reader, settings, tools)
	items, err := uc.GetToolAgentBindings(context.Background(), "read_file", "")
	if err != nil {
		t.Fatalf("GetToolAgentBindings: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 binding, got %d", len(items))
	}
	b := items[0]
	if b.AgentKey != "k1" || b.AgentName != "Agent One" || b.AgentStatus != "active" {
		t.Fatalf("agent metadata not carried through: %+v", b)
	}
	if !b.ToolsEnabled {
		t.Fatal("default settings must have ToolsEnabled=true")
	}
	if b.Profile != "coding" {
		t.Fatalf("default profile = %q, want coding", b.Profile)
	}
	if b.State != "allowed" {
		t.Fatalf("read_file under coding profile = %q (%s), want allowed", b.State, b.Reason)
	}
}

func TestGetToolAgentBindings_toolsDisabledAgentDenied(t *testing.T) {
	reader := &bindingsAgentReader{agents: []Agent{{ID: "a1", AgentKey: "k1"}}}
	s := DefaultAgentRuntimeSettings()
	s.AgentID = "a1"
	s.ToolsEnabled = false
	settings := &bindingsSettingsRepo{byAgent: map[string]AgentRuntimeSettings{"a1": s}}
	tool := Tool{Key: "read_file", Enabled: true, Category: "filesystem", Source: "builtin"}
	tools := &bindingsToolReader{tool: tool, catalog: []Tool{tool}}

	uc := newBindingsUsecase(reader, settings, tools)
	items, err := uc.GetToolAgentBindings(context.Background(), "read_file", "")
	if err != nil {
		t.Fatalf("GetToolAgentBindings: %v", err)
	}
	b := items[0]
	if b.State != "denied" || b.Reason != "agent_tools_disabled" {
		t.Fatalf("want denied/agent_tools_disabled, got %q/%q", b.State, b.Reason)
	}
	if b.ToolsEnabled {
		t.Fatal("ToolsEnabled must mirror settings row")
	}
}

func TestGetToolAgentBindings_overrideDenyWins(t *testing.T) {
	reader := &bindingsAgentReader{agents: []Agent{{ID: "a1", AgentKey: "k1"}}}
	s := DefaultAgentRuntimeSettings()
	s.AgentID = "a1"
	s.ToolsProfile = "full"
	settings := &bindingsSettingsRepo{byAgent: map[string]AgentRuntimeSettings{"a1": s}}
	tool := Tool{Key: "read_file", Enabled: true, Category: "filesystem", Source: "builtin"}
	tools := &bindingsToolReader{
		tool:    tool,
		catalog: []Tool{tool},
		overrides: []ToolAgentOverride{
			{ToolKey: "read_file", AgentID: "a1", Mode: "deny"},
		},
	}

	uc := newBindingsUsecase(reader, settings, tools)
	items, err := uc.GetToolAgentBindings(context.Background(), "read_file", "")
	if err != nil {
		t.Fatalf("GetToolAgentBindings: %v", err)
	}
	b := items[0]
	if b.State != "denied" || b.Reason != "override_deny" {
		t.Fatalf("want denied/override_deny, got %q/%q", b.State, b.Reason)
	}
	if b.OverrideMode != "deny" {
		t.Fatalf("OverrideMode = %q, want deny", b.OverrideMode)
	}
}

func TestGetToolAgentBindings_overrideAllowEnablesCatalogDisabledTool(t *testing.T) {
	reader := &bindingsAgentReader{agents: []Agent{{ID: "a1", AgentKey: "k1"}}}
	// No settings row → defaults (coding profile).
	settings := &bindingsSettingsRepo{byAgent: map[string]AgentRuntimeSettings{}}
	// Catalog-disabled + not named by coding profile → globally denied, but an
	// allow override must flip it on.
	tool := Tool{Key: "shell_exec", Enabled: false, Category: "runtime", Source: "builtin"}
	tools := &bindingsToolReader{
		tool:    tool,
		catalog: []Tool{tool},
		overrides: []ToolAgentOverride{
			{ToolKey: "shell_exec", AgentID: "a1", Mode: "allow"},
		},
	}

	uc := newBindingsUsecase(reader, settings, tools)
	items, err := uc.GetToolAgentBindings(context.Background(), "shell_exec", "")
	if err != nil {
		t.Fatalf("GetToolAgentBindings: %v", err)
	}
	b := items[0]
	if b.State != "allowed" || b.Reason != "override_allow" {
		t.Fatalf("want allowed/override_allow, got %q/%q", b.State, b.Reason)
	}
	if b.OverrideMode != "allow" {
		t.Fatalf("OverrideMode = %q, want allow", b.OverrideMode)
	}
}

func TestGetToolAgentBindings_requiresToolID(t *testing.T) {
	uc := newBindingsUsecase(&bindingsAgentReader{}, &bindingsSettingsRepo{}, &bindingsToolReader{})
	if _, err := uc.GetToolAgentBindings(context.Background(), "  ", ""); err == nil {
		t.Fatal("expected error for blank tool id")
	}
}

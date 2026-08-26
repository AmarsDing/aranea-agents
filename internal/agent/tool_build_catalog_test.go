package agent

import (
	"context"
	"errors"
	"slices"
	"testing"

	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
)

// fakeToolLookup implements biz.TeamToolLookup with call counters, proving
// the agent-build path batch-loads catalog rows instead of looping GetTool
// (the N+1 regression this file guards).
type fakeToolLookup struct {
	entries        []biztool.ToolCatalogEntry
	entriesErr     error
	overrides      []biz.ToolAgentOverride
	overridesErr   error
	entriesCalls   int
	entriesKeys    []string
	overridesCalls int
	getToolCalls   int
}

func (f *fakeToolLookup) GetTool(_ context.Context, _ string) (biztool.Tool, error) {
	f.getToolCalls++
	return biztool.Tool{}, errors.New("GetTool must not be called on the build path")
}

func (f *fakeToolLookup) ListToolCatalogEntries(_ context.Context, keys []string) ([]biztool.ToolCatalogEntry, error) {
	f.entriesCalls++
	f.entriesKeys = append([]string(nil), keys...)
	return f.entries, f.entriesErr
}

func (f *fakeToolLookup) ListToolAgentOverridesByAgent(_ context.Context, _ string) ([]biztool.ToolAgentOverride, error) {
	f.overridesCalls++
	return f.overrides, f.overridesErr
}

func (f *fakeToolLookup) RecordToolInvocationParams(_ context.Context, _ biztool.ToolInvocationParamWrite) error {
	return nil
}

func (f *fakeToolLookup) RecordToolInvocation(_ context.Context, _ biztool.ToolInvocationWrite) error {
	return nil
}

func (f *fakeToolLookup) RecordToolInvocationAudit(_ context.Context, _ biztool.ToolInvocationAuditWrite) error {
	return nil
}

func (f *fakeToolLookup) HasToolGrant(_ context.Context, _, _ string) bool { return false }

func (f *fakeToolLookup) GrantTool(_ context.Context, _, _, _ string) error { return nil }

func (f *fakeToolLookup) ListEnabledParamRulesForGate(_ context.Context, _ string) ([]biztool.ToolParamRule, error) {
	return nil, nil
}

func TestLoadToolBuildCatalog_BatchLoadsOnce(t *testing.T) {
	fk := &fakeToolLookup{
		entries: []biztool.ToolCatalogEntry{
			{Key: "web_fetch", ConfigJSON: `{"ua":"x"}`, RequiresConfirmation: true},
			{Key: "read_file", DefaultConfigJSON: `{"max":"10"}`},
		},
		overrides: []biz.ToolAgentOverride{{ToolKey: "read_file", ConfigOverrideJSON: `{"max":"20"}`}},
	}
	deps := TRPCBuilderDeps{}
	deps.ToolUC = fk
	eff := map[string]bool{"web_fetch": true, "read_file": true, "disabled_tool": false}

	c := loadToolBuildCatalog(context.Background(), "agent-1", eff, deps)
	if c == nil {
		t.Fatal("catalog nil")
	}
	if fk.entriesCalls != 1 || fk.overridesCalls != 1 {
		t.Fatalf("entriesCalls=%d overridesCalls=%d, want 1/1", fk.entriesCalls, fk.overridesCalls)
	}
	if fk.getToolCalls != 0 {
		t.Fatalf("GetTool called %d times on build path (N+1 regression)", fk.getToolCalls)
	}
	// Only enabled keys, sorted for deterministic SQL.
	if want := []string{"read_file", "web_fetch"}; !slices.Equal(fk.entriesKeys, want) {
		t.Fatalf("entries keys = %v, want %v", fk.entriesKeys, want)
	}
	if len(c.entries) != 2 || len(c.overrides) != 1 {
		t.Fatalf("entries=%d overrides=%d, want 2/1", len(c.entries), len(c.overrides))
	}
}

func TestLoadToolBuildCatalog_SkipsWhenNothingToLoad(t *testing.T) {
	fk := &fakeToolLookup{}
	deps := TRPCBuilderDeps{}
	deps.ToolUC = fk
	if c := loadToolBuildCatalog(context.Background(), "agent-1", nil, deps); c != nil {
		t.Fatal("nil eff must yield nil catalog")
	}
	if c := loadToolBuildCatalog(context.Background(), "agent-1", map[string]bool{"x": false}, deps); c != nil {
		t.Fatal("all-disabled eff must yield nil catalog")
	}
	if c := loadToolBuildCatalog(context.Background(), "", map[string]bool{"x": true}, deps); c != nil {
		t.Fatal("empty agentID must yield nil catalog")
	}
	depsNoUC := TRPCBuilderDeps{}
	if c := loadToolBuildCatalog(context.Background(), "agent-1", map[string]bool{"x": true}, depsNoUC); c != nil {
		t.Fatal("nil ToolUC must yield nil catalog")
	}
	if fk.entriesCalls != 0 || fk.overridesCalls != 0 {
		t.Fatal("no queries expected when catalog loading is skipped")
	}
}

func TestLoadToolBuildCatalog_LoadFailuresFailClosed(t *testing.T) {
	fk := &fakeToolLookup{
		entriesErr:   errors.New("db down"),
		overridesErr: errors.New("db down"),
	}
	deps := TRPCBuilderDeps{}
	deps.ToolUC = fk
	eff := map[string]bool{"a": true}

	c := loadToolBuildCatalog(context.Background(), "agent-1", eff, deps)
	if c == nil {
		t.Fatal("catalog must still be returned so the gate can fail closed")
	}
	if cat := c.confirmCatalog(eff); !cat["a"].requiresConfirm {
		t.Fatalf("batch+override failure must fail closed, got %v", cat)
	}
	if merged := c.mergedConfigMaps(eff); len(merged) != 0 {
		t.Fatalf("merged = %v, want empty on load failure", merged)
	}
}

func TestToolBuildCatalog_MergedConfigMaps(t *testing.T) {
	c := &toolBuildCatalog{
		entries: map[string]biztool.ToolCatalogEntry{
			// config_json wins over default_config_json on overlapping keys.
			"web_fetch": {Key: "web_fetch", ConfigJSON: `{"ua":"x"}`, DefaultConfigJSON: `{"ua":"d"}`},
			// default_config_json fills keys missing from config_json; override wins on the same key.
			"read_file": {Key: "read_file", DefaultConfigJSON: `{"max":"10"}`},
			// empty config_json ("{}") must not suppress fallback keys (BUG-2).
			"save_file": {Key: "save_file", ConfigJSON: `{}`, DefaultConfigJSON: `{"enc":"utf8"}`},
		},
		overrides: map[string]biz.ToolAgentOverride{
			"read_file": {ToolKey: "read_file", ConfigOverrideJSON: `{"max":"20"}`},
		},
	}
	eff := map[string]bool{"web_fetch": true, "read_file": true, "save_file": true, "deleted_tool": true, "off": false}

	merged := c.mergedConfigMaps(eff)
	if got := merged["web_fetch"]; got["ua"] != "x" {
		t.Fatalf("web_fetch merged = %v, want ua=x (config_json wins)", got)
	}
	if got := merged["read_file"]; got["max"] != "20" {
		t.Fatalf("read_file merged = %v, want max=20 (override wins)", got)
	}
	if got := merged["save_file"]; got["enc"] != "utf8" {
		t.Fatalf("save_file merged = %v, want enc=utf8 (fallback fills gap)", got)
	}
	if _, ok := merged["deleted_tool"]; ok {
		t.Fatal("missing entry must be skipped (fail-soft for configs)")
	}
	if _, ok := merged["off"]; ok {
		t.Fatal("disabled tool must be skipped")
	}

	var nilCatalog *toolBuildCatalog
	if got := nilCatalog.mergedConfigMaps(eff); got != nil {
		t.Fatalf("nil catalog merged = %v, want nil", got)
	}
}

func TestToolBuildCatalog_ConfirmCatalog(t *testing.T) {
	eff := map[string]bool{"a": true, "b": true, "c": true, "d": true, "off": false}
	c := &toolBuildCatalog{
		entries: map[string]biztool.ToolCatalogEntry{
			"a": {Key: "a", RequiresConfirmation: true},
			"b": {Key: "b"},
			"c": {Key: "c"},
			// "d" missing (deleted mid-build) → fail-closed below.
		},
		overrides: map[string]biz.ToolAgentOverride{
			"c": {ToolKey: "c", RequiresConfirmation: true},
		},
	}
	cat := c.confirmCatalog(eff)
	if !cat["a"].requiresConfirm {
		t.Error("a: entry requires_confirmation must gate")
	}
	if _, ok := cat["b"]; ok {
		t.Error("b: no flag anywhere must stay ungated (absent from map)")
	}
	if !cat["c"].requiresConfirm {
		t.Error("c: override requires_confirmation must gate")
	}
	if !cat["d"].requiresConfirm {
		t.Error("d: missing tool row must fail closed")
	}
	if _, ok := cat["off"]; ok {
		t.Error("off: disabled tool must not be gated")
	}

	// Overrides query failure → fail closed for every enabled tool.
	failed := &toolBuildCatalog{
		entries:         map[string]biztool.ToolCatalogEntry{"a": {Key: "a"}},
		overridesFailed: true,
	}
	cat = failed.confirmCatalog(map[string]bool{"a": true, "b": true})
	if len(cat) != 2 || !cat["a"].requiresConfirm || !cat["b"].requiresConfirm {
		t.Fatalf("overrides failure must fail closed for all enabled tools, got %v", cat)
	}

	var nilCatalog *toolBuildCatalog
	if got := nilCatalog.confirmCatalog(eff); got != nil {
		t.Fatalf("nil catalog confirm = %v, want nil", got)
	}
}

// TestBuildToolConfirmGate_UsesProvidedCatalog proves the gate consumes the
// pre-loaded catalog snapshot without touching ToolUC (no DB on gate build).
func TestBuildToolConfirmGate_UsesProvidedCatalog(t *testing.T) {
	gate := buildToolConfirmGate(context.Background(), biz.Agent{ID: "a1"}, TRPCBuilderDeps{},
		map[string]confirmCatalogEntry{"bash": {requiresConfirm: true}})
	if gate == nil {
		t.Fatal("gate nil")
	}
	if !gate.catalogCheck("bash") {
		t.Fatal("provided catalog must gate bash")
	}
	if gate.catalogCheck("read_file") {
		t.Fatal("read_file must stay ungated")
	}

	// No catalog + no plugin → no gate.
	if g := buildToolConfirmGate(context.Background(), biz.Agent{ID: "a1"}, TRPCBuilderDeps{}, nil); g != nil {
		t.Fatalf("empty catalog without plugin must yield nil gate, got %+v", g)
	}
}

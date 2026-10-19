package biz

import (
	"context"
	stderrors "errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// leanCatalogToolReader implements the optional lean-catalog capability and
// fails loudly if the fallback SearchTools path is taken instead.
type leanCatalogToolReader struct {
	ToolRegistryReader // embedded nil — any unimplemented method panics if reached
	leanItems          []Tool
	leanCalls          int
	searchCalls        int
}

func (r *leanCatalogToolReader) ListAllToolsForEffective(context.Context) ([]Tool, error) {
	r.leanCalls++
	return r.leanItems, nil
}

func (r *leanCatalogToolReader) SearchTools(context.Context, ToolListQuery) (ToolListResult, error) {
	r.searchCalls++
	return ToolListResult{}, stderrors.New("SearchTools must not run on the runtime effective path")
}

func (r *leanCatalogToolReader) ListToolAgentOverridesByAgent(context.Context, string) ([]ToolAgentOverride, error) {
	return nil, nil
}

// searchOnlyToolReader lacks the lean capability (legacy fake shape) — the
// effective computation must fall back to SearchTools Items for it.
type searchOnlyToolReader struct {
	ToolRegistryReader
	items       []Tool
	searchCalls int
}

func (r *searchOnlyToolReader) SearchTools(context.Context, ToolListQuery) (ToolListResult, error) {
	r.searchCalls++
	return ToolListResult{Items: r.items, Total: len(r.items)}, nil
}

func (r *searchOnlyToolReader) ListToolAgentOverridesByAgent(context.Context, string) ([]ToolAgentOverride, error) {
	return nil, nil
}

func effectiveToolsFixture(tools ToolRegistryReader) *AgentUsecase {
	return NewAgentUsecase(AgentUsecaseDeps{
		Reader:   &policyAgentReader{},
		Settings: &policySettingsRepo{byAgent: map[string]AgentRuntimeSettings{}},
		Tools:    tools,
		Lg:       loggateway.NewNoop(),
	})
}

// P3-1 (2026-10-19): the runtime effective path must use the lean single-scan
// catalog (no COUNT / SUMMARY / stats aggregates on SearchTools), with Items
// parity — buildAgentEffectiveTools sees exactly the same tool rows.
func TestGetEffectiveTools_PrefersLeanCatalog(t *testing.T) {
	catalog := []Tool{
		{ID: "t1", Key: "read_file", DisplayName: "Read File", Category: "fs", Enabled: true},
		{ID: "t2", Key: "write_file", DisplayName: "Write File", Category: "fs", Enabled: false},
	}
	reader := &leanCatalogToolReader{leanItems: catalog}
	uc := effectiveToolsFixture(reader)

	eff, err := uc.GetEffectiveTools(context.Background(), "agent-a")
	if err != nil {
		t.Fatalf("GetEffectiveTools: %v", err)
	}
	if reader.leanCalls != 1 {
		t.Fatalf("lean catalog calls=%d want exactly 1", reader.leanCalls)
	}
	if reader.searchCalls != 0 {
		t.Fatalf("SearchTools must not run on the runtime effective path, got %d calls", reader.searchCalls)
	}
	if len(eff.Items) < len(catalog) {
		t.Fatalf("effective items must cover the catalog (plus synthetics), got %#v", eff.Items)
	}
	byKey := map[string]EffectiveAgentTool{}
	for _, it := range eff.Items {
		byKey[it.ToolKey] = it
	}
	// Items parity: every catalog row reaches the effective computation with
	// its key/display/category carried through unchanged.
	for _, c := range catalog {
		it, ok := byKey[c.Key]
		if !ok {
			t.Fatalf("catalog key %q missing from effective items", c.Key)
		}
		if it.DisplayName != c.DisplayName || it.Category != c.Category {
			t.Fatalf("catalog row %#v degraded in effective items: %#v", c, it)
		}
	}
}

// Legacy fakes without the lean capability keep the SearchTools Items fallback.
func TestGetEffectiveTools_FallsBackToSearchToolsItems(t *testing.T) {
	catalog := []Tool{
		{ID: "t1", Key: "read_file", DisplayName: "Read File", Category: "fs", Enabled: true},
	}
	reader := &searchOnlyToolReader{items: catalog}
	uc := effectiveToolsFixture(reader)

	eff, err := uc.GetEffectiveTools(context.Background(), "agent-a")
	if err != nil {
		t.Fatalf("GetEffectiveTools: %v", err)
	}
	if reader.searchCalls != 1 {
		t.Fatalf("SearchTools fallback calls=%d want 1", reader.searchCalls)
	}
	found := false
	for _, it := range eff.Items {
		if it.ToolKey == "read_file" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fallback must yield SearchTools Items, got %#v", eff.Items)
	}
}

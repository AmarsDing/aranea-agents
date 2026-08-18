package deferred

import (
	"strings"
	"testing"
)

func TestRenderCatalogCue_Basic(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", Description: "Fetch and read web page content", Category: "web"},
		{Name: "shell_exec", Description: "Execute shell commands", Category: "runtime"},
		{Name: "browser_navigate", Description: "Navigate browser to URL", Category: "browser"},
		{Name: "browser_click", Description: "Click element in browser", Category: "browser"},
	}
	cue := RenderCatalogCue(catalog)

	if cue == "" {
		t.Fatal("expected non-empty cue")
	}
	// 应包含所有工具名
	for _, entry := range catalog {
		if !strings.Contains(cue, entry.Name) {
			t.Errorf("cue missing tool %q", entry.Name)
		}
	}
	// 应包含类别分组
	if !strings.Contains(cue, "web") {
		t.Error("cue missing category 'web'")
	}
	if !strings.Contains(cue, "runtime") {
		t.Error("cue missing category 'runtime'")
	}
	if !strings.Contains(cue, "browser") {
		t.Error("cue missing category 'browser'")
	}
	// 应包含 tool_load 提示
	if !strings.Contains(cue, "tool_load") {
		t.Error("cue missing tool_load hint")
	}
}

func TestRenderCatalogCue_EmptyCatalog(t *testing.T) {
	cue := RenderCatalogCue(nil)
	if cue != "" {
		t.Errorf("expected empty cue for nil catalog, got %q", cue)
	}
	cue = RenderCatalogCue([]DeferredToolEntry{})
	if cue != "" {
		t.Errorf("expected empty cue for empty catalog, got %q", cue)
	}
}

func TestRenderCatalogCue_Deterministic(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "b_tool", Description: "B tool", Category: "cat2"},
		{Name: "a_tool", Description: "A tool", Category: "cat1"},
		{Name: "c_tool", Description: "C tool", Category: "cat1"},
	}
	cue1 := RenderCatalogCue(catalog)
	cue2 := RenderCatalogCue(catalog)
	if cue1 != cue2 {
		t.Error("cue not deterministic across calls")
	}
}

func TestRenderCatalogCue_SortedByCategory(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "z_tool", Description: "Z tool", Category: "zeta"},
		{Name: "a_tool", Description: "A tool", Category: "alpha"},
		{Name: "m_tool", Description: "M tool", Category: "beta"},
	}
	cue := RenderCatalogCue(catalog)

	// alpha 应在 beta 之前，beta 应在 zeta 之前
	alphaIdx := strings.Index(cue, "alpha")
	betaIdx := strings.Index(cue, "beta")
	zetaIdx := strings.Index(cue, "zeta")
	if alphaIdx >= betaIdx || betaIdx >= zetaIdx {
		t.Errorf("categories not sorted: alpha=%d, beta=%d, zeta=%d", alphaIdx, betaIdx, zetaIdx)
	}
}

func TestRenderCatalogCue_SortedByNameWithinCategory(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "zebra", Description: "Z tool", Category: "cat"},
		{Name: "alpha", Description: "A tool", Category: "cat"},
		{Name: "middle", Description: "M tool", Category: "cat"},
	}
	cue := RenderCatalogCue(catalog)

	alphaIdx := strings.Index(cue, "alpha")
	middleIdx := strings.Index(cue, "middle")
	zebraIdx := strings.Index(cue, "zebra")
	if alphaIdx >= middleIdx || middleIdx >= zebraIdx {
		t.Errorf("tools not sorted within category: alpha=%d, middle=%d, zebra=%d", alphaIdx, middleIdx, zebraIdx)
	}
}

func TestRenderCatalogCue_NoCategory(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "tool_a", Description: "Tool A"},
		{Name: "tool_b", Description: "Tool B"},
	}
	cue := RenderCatalogCue(catalog)
	if cue == "" {
		t.Fatal("expected non-empty cue")
	}
	if !strings.Contains(cue, "tool_a") {
		t.Error("cue missing tool_a")
	}
	if !strings.Contains(cue, "tool_b") {
		t.Error("cue missing tool_b")
	}
}

func TestRenderCatalogCue_ContainsInstructions(t *testing.T) {
	catalog := []DeferredToolEntry{
		{Name: "web_fetch", Description: "Fetch web content", Category: "web"},
	}
	cue := RenderCatalogCue(catalog)

	if !strings.Contains(cue, "tool_load") {
		t.Error("cue missing tool_load instruction")
	}
}

func TestRenderCatalogCue_CompactsLongDescription(t *testing.T) {
	long := strings.Repeat("工具说明很长", 30) + "。后面的句子不应出现。"
	cue := RenderCatalogCue([]DeferredToolEntry{
		{Name: "memory_add", Description: long, Category: "memory"},
	})
	if strings.Contains(cue, "后面的句子不应出现") {
		t.Fatal("catalog cue must drop sentences after the first")
	}
	if strings.Contains(cue, long) {
		t.Fatal("catalog cue must not keep the raw long description")
	}
	if !strings.Contains(cue, "memory_add") {
		t.Fatal("catalog cue missing tool name")
	}
}

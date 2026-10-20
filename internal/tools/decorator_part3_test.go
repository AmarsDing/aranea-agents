package tools

import (
	"testing"
)

func TestBudgetOverrideForTool_PrefixedName(t *testing.T) {

	for _, name := range []string{"bw_browser_take_screenshot", "playwright_browser_snapshot"} {
		if b := budgetOverrideForTool(name); b == nil {
			t.Errorf("expected override for prefixed tool %q", name)
		}
	}
}

func TestBudgetOverrideForTool_NonMatchingSuffix(t *testing.T) {

	if b := budgetOverrideForTool("browser_take_screenshot_extra"); b != nil {
		t.Errorf("expected nil for non-matching suffix, got %+v", b)
	}
}

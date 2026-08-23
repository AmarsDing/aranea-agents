package data

import (
	"path/filepath"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestLoadSpiritPromptMarkdown_OnlyCoreFiles(t *testing.T) {
	files, err := loadSpiritPromptMarkdown(filepath.Join("..", "scenario"), loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files, want IDENTITY/CAPABILITIES/DECISION", len(files))
	}
	got := map[string]bool{}
	for _, f := range files {
		got[f.name] = true
		if f.body == "" {
			t.Fatalf("%s empty", f.name)
		}
	}
	for _, name := range []string{"IDENTITY.md", "CAPABILITIES.md", "DECISION.md"} {
		if !got[name] {
			t.Fatalf("missing %s", name)
		}
	}
	for _, banned := range []string{"company_lead.md", "dept_lead.md", "orchestrator.md"} {
		if got[banned] {
			t.Fatalf("spirit must not mount %s", banned)
		}
	}
}

func TestIsSpiritPromptMarkdown(t *testing.T) {
	if !isSpiritPromptMarkdown("IDENTITY.md") {
		t.Fatal("IDENTITY.md must be allowed")
	}
	if isSpiritPromptMarkdown("company_lead.md") || isSpiritPromptMarkdown("orchestrator.md") {
		t.Fatal("governance prompts must stay off spirit")
	}
}

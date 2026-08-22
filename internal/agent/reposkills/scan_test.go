package reposkills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScan_TrustedAgentsSkills(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	skillDir := filepath.Join(root, ".agents", "skills", "xlsx-review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: XLSX Review\ndescription: Review spreadsheets before send.\n---\n\n# body\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Scan(root, []string{root})
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1: %+v", len(got), got)
	}
	if got[0].Slug != "xlsx-review" || !strings.Contains(got[0].Description, "spreadsheets") {
		t.Fatalf("entry = %+v", got[0])
	}
	cue := FormatCue(got)
	if !strings.Contains(cue, "$xlsx-review") || !strings.Contains(cue, "<workspace_skills>") {
		t.Fatalf("cue = %s", cue)
	}
}

func TestScan_CwdOverridesRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "skills", "shared", "SKILL.md"), []byte("---\ndescription: from root\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(root, "pkg")
	if err := os.MkdirAll(filepath.Join(cwd, ".codex", "skills", "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".codex", "skills", "shared", "SKILL.md"), []byte("---\ndescription: from cwd\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Scan(cwd, []string{root})
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	if got[0].Description != "from cwd" {
		t.Fatalf("cwd should win, got %q", got[0].Description)
	}
}

func TestScan_UntrustedSkipped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "skills", "x", "SKILL.md"), []byte("---\ndescription: secret\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Scan(root, []string{t.TempDir()}); len(got) != 0 {
		t.Fatalf("untrusted scan = %+v", got)
	}
}

func TestFormatCue_Empty(t *testing.T) {
	t.Parallel()
	if FormatCue(nil) != "" {
		t.Fatal("empty entries must yield empty cue")
	}
}

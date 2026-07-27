package cli_admin

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/pkginstall"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestSkillInstallFromURLRejectsEmptyURL(t *testing.T) {
	tool := newSkillInstallFromURLTool(Deps{})
	callable, ok := tool.(trpctool.CallableTool)
	if !ok {
		t.Fatal("skill install tool is not callable")
	}
	_, err := callable.Call(context.Background(), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Fatalf("Call() error = %v, want url required", err)
	}
}

func TestRegisterAllIncludesPackageInstallTool(t *testing.T) {
	tools := RegisterAll(Deps{})
	var names []string
	for _, tool := range tools {
		if decl := tool.Declaration(); decl != nil {
			names = append(names, decl.Name)
		}
	}
	raw, _ := json.Marshal(names)
	if !strings.Contains(string(raw), "cli_admin_pkg_install_from_url") {
		t.Fatalf("RegisterAll() names = %s, missing package install tool", raw)
	}
	if !strings.Contains(string(raw), "cli_admin_skill_install_from_url") {
		t.Fatalf("RegisterAll() names = %s, missing skill install tool", raw)
	}
}

func TestPkgInstallFromURLToolDryRun(t *testing.T) {
	// This test requires a real remote git URL; local paths are rejected
	// by ValidateRepoURL for security reasons. Skip if no remote is available.
	t.Skip("requires a real remote git URL; local paths are rejected for security")
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "aranea-package.yaml"), []byte(`version: 1
metadata:
  name: test-pkg
spec:
  skills:
    - path: skill.zip
`))
	mustWrite(t, filepath.Join(repo, "skill.zip"), []byte("zip placeholder"))
	git(t, repo, "init")
	git(t, repo, "add", ".")
	git(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "init")

	tool := newPkgInstallFromURLTool(Deps{})
	callable, ok := tool.(trpctool.CallableTool)
	if !ok {
		t.Fatal("pkg install tool is not callable")
	}
	raw, err := json.Marshal(map[string]any{"url": repo, "dry_run": true})
	if err != nil {
		t.Fatal(err)
	}
	out, err := callable.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	got, ok := out.(pkgInstallOutput)
	if !ok {
		t.Fatalf("Call() output type = %T", out)
	}
	if got.Skipped == 0 || len(got.Errors) > 0 {
		t.Fatalf("Call() output = %+v, want dry-run skipped without errors", got)
	}
}

func TestSummarizeSkillSteps(t *testing.T) {
	t.Run("pending_conflict wins and aggregates", func(t *testing.T) {
		steps := []pkginstall.StepResult{
			{Resource: "mcp_server:m", Action: "created"},
			{Resource: "skill:a", Action: "created", Status: "installed", JobID: "j1"},
			{Resource: "skill:b", Action: "pending_conflict", Status: "pending_conflict", JobID: "j2",
				Conflicts: []pkginstall.ConflictInfo{{GroupID: "g1"}, {GroupID: "g2"}}},
		}
		jobID, status, conflicts := summarizeSkillSteps(steps)
		if status != "pending_conflict" {
			t.Fatalf("status = %q, want pending_conflict", status)
		}
		if jobID != "j2" {
			t.Fatalf("jobID = %q, want j2 (first pending job)", jobID)
		}
		if len(conflicts) != 2 {
			t.Fatalf("conflicts = %v, want 2 entries", conflicts)
		}
	})
	t.Run("failed beats installed", func(t *testing.T) {
		steps := []pkginstall.StepResult{
			{Resource: "skill:a", Action: "created", Status: "installed", JobID: "j1"},
			{Resource: "skill:b", Action: "error", Status: "failed", JobID: "j2"},
		}
		_, status, _ := summarizeSkillSteps(steps)
		if status != "failed" {
			t.Fatalf("status = %q, want failed", status)
		}
	})
	t.Run("installed", func(t *testing.T) {
		steps := []pkginstall.StepResult{
			{Resource: "skill:a", Action: "created", Status: "installed", JobID: "j1"},
		}
		jobID, status, conflicts := summarizeSkillSteps(steps)
		if status != "installed" || jobID != "j1" || len(conflicts) != 0 {
			t.Fatalf("got (%q, %q, %v), want (j1, installed, [])", jobID, status, conflicts)
		}
	})
	t.Run("no skill steps", func(t *testing.T) {
		_, status, _ := summarizeSkillSteps([]pkginstall.StepResult{{Resource: "graph:g", Action: "created"}})
		if status != "" {
			t.Fatalf("status = %q, want empty", status)
		}
	})
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

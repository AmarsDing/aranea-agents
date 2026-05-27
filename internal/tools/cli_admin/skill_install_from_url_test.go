package cli_admin

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

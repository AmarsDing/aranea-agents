package fleet

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 一键安装目录里的命令优先于 PATH；agents.json 能覆盖内置项、追加新工具。
func TestCLIRootPresetsAndLookup(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARANEA_FLEET_CLI_ROOT", root)
	if err := os.MkdirAll(filepath.Join(root, "npm"), 0o755); err != nil {
		t.Fatal(err)
	}
	name := "grok"
	if os.PathSeparator == '\\' {
		name += ".cmd"
	}
	if err := os.WriteFile(filepath.Join(root, "npm", name), []byte("@echo off\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"claude", "codex", "gemini", "grok", "cursor", "trae", "codebuddy", "doubao", "qoder"} {
		if p, ok := findPreset(id); !ok || p.Install == "" {
			t.Fatalf("%s should be installable", id)
		}
	}
	extra := `[{"id":"codex","label":"改名","command":"codex-x"},{"id":"newcli","command":"newcli","install":"npm:new-cli"},{"id":"shell","command":"evil"}]`
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(extra), 0o644); err != nil {
		t.Fatal(err)
	}
	got := map[string]AgentPreset{}
	list := ListPresets()
	for _, p := range list {
		got[p.ID] = p
	}
	if p := got["grok"]; !p.Available || !strings.HasPrefix(p.ResolvedPath, root) {
		t.Fatalf("grok should resolve under CLI root: %+v", p)
	}
	if got["codex"].Label != "改名" || got["codex"].Command != "codex-x" {
		t.Fatalf("agents.json should override codex: %+v", got["codex"])
	}
	if got["newcli"].Label != "newcli" || got["newcli"].Install != "npm:new-cli" {
		t.Fatalf("agents.json should append newcli: %+v", got["newcli"])
	}
	if last := list[len(list)-1]; last.ID != "shell" || last.Command == "evil" {
		t.Fatalf("shell must stay last and not be overridable: %+v", last)
	}
	t.Setenv("NPM_CONFIG_PREFIX", `C:\elsewhere`) // 用户环境里的设置不能漏进终端
	u := &Usecase{opts: Options{CLIDir: `C:\fleetctl`}}
	var path string
	prefixes := 0
	env := strings.Join(u.terminalEnv("t1", "tok"), "\n")
	for _, kv := range strings.Split(env, "\n") {
		if strings.EqualFold(kv[:min(5, len(kv))], "PATH=") {
			path = kv
		}
		if strings.HasPrefix(strings.ToLower(kv), "npm_config_prefix=") {
			prefixes++
		}
	}
	if !strings.Contains(path, filepath.Join(root, "npm")) || !strings.Contains(path, filepath.Join(root, "trae")) {
		t.Fatalf("terminal PATH should include install dirs: %s", path)
	}
	// CLI 自更新（npm i -g）要落回 CLI 根目录，Claude 自动更新关掉
	if prefixes != 1 || !strings.Contains(env, "npm_config_prefix="+filepath.Join(root, "npm")) || !strings.Contains(env, "DISABLE_AUTOUPDATER=1") {
		t.Fatalf("terminal env should pin npm prefix to CLI root and disable Claude auto-update:\n%s", env)
	}
}

func TestUnzipRejectsEscapingPaths(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "bad.zip")
	f, _ := os.Create(zp)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("../escape.txt")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	_ = f.Close()
	if err := unzip(zp, filepath.Join(dir, "out")); err == nil {
		t.Fatal("zip-slip path must be rejected")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err == nil {
		t.Fatal("file escaped the target dir")
	}
}

func TestApprovalPromptsFromCAO(t *testing.T) {
	for _, s := range []string{
		"Do you want to proceed?\n❯ 1. Yes\nEsc to cancel · Tab to amend",
		"Command Approval Required\n[a] Accept  [d] Decline",
		"Yes, proceed\nNo, reject (type to add feedback)",
		"↑/↓ to navigate · Enter to select",
		// 2026-11-20 实测屏幕末尾
		"● 1. Trust folder (demo-repo)\n2. Trust parent folder (x)\n3. Don't trust",
		"› 1. Trust and continue\n2. Back\nenter continue · esc back",
		"2. Exit the application\n↑/↓\nnavigate\n·\nEnter\nselect",
		"Press any key to log in...",
		"请使用豆包移动端扫码并在手机上确认；按 Ctrl+C 可取消",
		"(Use ↑↓ to select, Enter to login · Esc to quit)",
		"Run this command?\nNot in allowlist: echo\n→ Run (once) (y)\nSkip & tell the agent what to do instead (esc or n)",
	} {
		if !matchNeedsInput(strings.ToLower(s)) {
			t.Fatalf("should be needs-input: %q", s)
		}
	}
}

package agentbridge

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultACPLaunch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key     string
		command string
		args    []string
		ok      bool
	}{
		{"codebuddy", "codebuddy", []string{"--acp"}, true},
		{"CodeBuddy", "codebuddy", []string{"--acp"}, true},
		// M4 §B：workbuddy 键归一复用 codebuddy 适配器（ADR-88 D3）。
		{"workbuddy", "codebuddy", []string{"--acp"}, true},
		{"work_buddy", "codebuddy", []string{"--acp"}, true},
		{"tencent_workbuddy", "codebuddy", []string{"--acp"}, true},
		{"codex", "npx", []string{"-y", "@agentclientprotocol/codex-acp"}, true},
		{"unknown", "", nil, false},
	}
	for _, tc := range cases {
		cmd, args, ok := DefaultACPLaunch(tc.key)
		if ok != tc.ok || cmd != tc.command {
			t.Errorf("DefaultACPLaunch(%q) = (%q,%v,%v), want (%q,%v,%v)",
				tc.key, cmd, args, ok, tc.command, tc.args, tc.ok)
		}
		if ok && len(tc.args) > 0 && (len(args) != len(tc.args) || args[0] != tc.args[0]) {
			t.Errorf("DefaultACPLaunch(%q) args = %v, want %v", tc.key, args, tc.args)
		}
	}
}

// TestDefaultACPLaunch_Claude 兼容探测（P3-1）：新 bin 优先、旧 bin 回退、
// 均缺失给新名（探针 L1 引导）。
func TestDefaultACPLaunch_Claude(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"claude_code", "claude-code", "claudecode"} {
		cmd, args, ok := DefaultACPLaunch(key)
		if !ok || len(args) != 0 {
			t.Fatalf("DefaultACPLaunch(%q) = (%q,%v,%v)", key, cmd, args, ok)
		}
		if cmd != claudeACPCommandNew && cmd != claudeACPCommandLegacy {
			t.Fatalf("claude command = %q, want new/legacy bin", cmd)
		}
		// 环境无两 bin 时必须回新名（探针按此诊断「未安装」）。
		if _, errNew := exec.LookPath(claudeACPCommandNew); errNew != nil {
			if _, errLegacy := exec.LookPath(claudeACPCommandLegacy); errLegacy != nil {
				if cmd != claudeACPCommandNew {
					t.Fatalf("both bins missing: command = %q, want %q", cmd, claudeACPCommandNew)
				}
			}
		}
	}
}

// TestDefaultACPLaunch_Shims 自研 shim：bin/ 绝对路径（随宿主 cwd 漂移故破例）。
func TestDefaultACPLaunch_Shims(t *testing.T) {
	t.Parallel()
	shimSuffix := ""
	if runtime.GOOS == "windows" {
		shimSuffix = ".exe"
	}
	cases := []struct {
		key string
		bin string
	}{
		{"deepseek", shimDeepseekBin},
		{"deep_seek", shimDeepseekBin},
		{"deepseek_harness", shimDeepseekBin},
		{"grok", shimGrokBin},
		{"grok_bot", shimGrokBin},
		{"xai_grok", shimGrokBin},
	}
	for _, tc := range cases {
		cmd, args, ok := DefaultACPLaunch(tc.key)
		if !ok || len(args) != 0 {
			t.Fatalf("DefaultACPLaunch(%q) = (%q,%v,%v)", tc.key, cmd, args, ok)
		}
		want := tc.bin + shimSuffix
		if !strings.HasSuffix(cmd, string(filepath.Separator)+want) && cmd != want {
			t.Fatalf("DefaultACPLaunch(%q) command = %q, want …/%s", tc.key, cmd, want)
		}
	}
}

func TestApplyDefaultLaunch(t *testing.T) {
	t.Parallel()
	ag := &CodingAgent{AgentKey: "codebuddy"}
	ApplyDefaultLaunch(ag)
	if ag.Command != "codebuddy" || len(ag.Args) != 1 || ag.Args[0] != "--acp" {
		t.Fatalf("codebuddy default = %+v", ag)
	}
	keep := &CodingAgent{AgentKey: "codebuddy", Command: "custom", Args: []string{"--flag"}}
	ApplyDefaultLaunch(keep)
	if keep.Command != "custom" || keep.Args[0] != "--flag" {
		t.Fatalf("explicit argv must be kept, got %+v", keep)
	}
	// M4：workbuddy 注册省略 argv 时归一 codebuddy 预设。
	wb := &CodingAgent{AgentKey: "workbuddy"}
	ApplyDefaultLaunch(wb)
	if wb.Command != "codebuddy" || len(wb.Args) != 1 || wb.Args[0] != "--acp" {
		t.Fatalf("workbuddy default = %+v", wb)
	}
}

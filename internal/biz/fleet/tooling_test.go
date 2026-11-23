package fleet

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestUpdateDue(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if !updateDue("", now) || !updateDue("bad", now) {
		t.Fatal("unknown last update must be due")
	}
	if updateDue(now.Add(-2*time.Hour).Format(time.RFC3339), now) {
		t.Fatal("2h ago must not be due")
	}
	if !updateDue(now.Add(-25*time.Hour).Format(time.RFC3339), now) {
		t.Fatal("25h ago must be due")
	}
}

// 用户改设置不能抹掉安装流程记下的版本与时间。
func TestSetAgentSettingKeepsVersion(t *testing.T) {
	u := newTestUsecase(t, t.TempDir())
	ctx := context.Background()
	_ = putDoc(ctx, u.store, kindAgentSetting, "codex", AgentSetting{ID: "codex", Mode: PermManual, Version: "codex-cli 1.0", UpdatedAt: "2026-11-21T00:00:00Z"})
	if _, err := u.SetAgentSetting(ctx, AgentSetting{ID: "codex", Mode: PermAutoEdit, AutoUpdate: true}); err != nil {
		t.Fatal(err)
	}
	got := u.GetAgentSetting(ctx, "codex")
	if got.Version != "codex-cli 1.0" || got.UpdatedAt == "" || !got.AutoUpdate || got.Mode != PermAutoEdit {
		t.Fatalf("setting = %+v", got)
	}
}

// 连通测试走真实子进程：.cmd 垫片 + 带空格的提示原样送达；回复 OK 判能对话，未登录判要登录。
func TestAgentSmokeTest(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only (.cmd shim)")
	}
	root := t.TempDir()
	t.Setenv("ARANEA_FLEET_CLI_ROOT", root)
	_ = os.MkdirAll(filepath.Join(root, "npm"), 0o755)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, "npm", name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 收到的参数原样回显，再回 OK
	write("okcli.cmd", "@echo off\r\necho args=[%*]\r\necho OK\r\n")
	write("nologin.cmd", "@echo off\r\necho Not logged in · Please run /login\r\nexit /b 1\r\n")
	// Gemini 没配认证时的原话
	write("noauth.cmd", "@echo off\r\necho Please set an Auth method in your settings.json or specify GEMINI_API_KEY\r\nexit /b 41\r\n")
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(`[
		{"id":"okcli","command":"okcli","test_args":["-p","{prompt}"]},
		{"id":"nologin","command":"nologin","test_args":["-p","{prompt}"]},
		{"id":"noauth","command":"noauth","test_args":["-p","{prompt}"]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	u := newTestUsecase(t, t.TempDir())
	wait := func(id string) (string, string, TestResult) {
		if err := u.TestAgent(id); err != nil {
			t.Fatal(err)
		}
		var st, log string
		var res TestResult
		waitFor(t, "smoke test "+id, 30*time.Second, func() bool {
			st, log, res = TestStatus(id)
			return st != InstallRunning
		})
		return st, log, res
	}
	st, log, res := wait("okcli")
	if st != InstallOK || !res.OK || res.Verdict != "能对话" || AgentReadiness("okcli") != ReadyOK {
		t.Fatalf("ok case: state=%s res=%+v log=%s", st, res, log)
	}
	if !strings.Contains(log, `args=[-p "`+testPrompt+`"]`) {
		t.Fatalf("prompt with spaces not delivered intact:\n%s", log)
	}
	st, _, res = wait("nologin")
	if st != InstallFailed || res.OK || !strings.Contains(res.Verdict, "登录") || AgentReadiness("nologin") != ReadyNeedLogin {
		t.Fatalf("login case: state=%s res=%+v", st, res)
	}
	if _, _, res = wait("noauth"); !strings.Contains(res.Verdict, "登录") || AgentReadiness("noauth") != ReadyNeedLogin {
		t.Fatalf("auth-method case: res=%+v", res)
	}
}

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		local, remote string
		want          bool
	}{
		{"2.1.330 (Claude Code)", "2.1.333", true},
		{"codex-cli 0.171.0", "0.171.0", false},
		{"grok 1.0.89 (28be5bb1bf65)", "1.0.100", true},
		{"2.1.330", "2.1.4", false},
		{"2026.11.19-f005e9d", "2026.11.20-ab12cd3", true},
		{"2026.11.19-f005e9d", "2026.11.19-0a0a0a0", true}, // 同一天的新构建
		{"trae-cli version 0.121.3", "0.216.0", true},
		{"未记录", "1.0.0", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.local, c.remote); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.local, c.remote, got)
		}
	}
}

func TestPickVersion(t *testing.T) {
	trae := "trae-cli version 0.121.3\nbuild date: 2026-10-09T09:02:03Z\ncommit date: 2026-10-09T08:15:05Z\n"
	if got := pickVersion(trae); got != "trae-cli version 0.121.3" {
		t.Fatalf("trae: %q", got)
	}
	if got := pickVersion("npm warn deprecated x\n2.1.330 (Claude Code)\n"); got != "2.1.330 (Claude Code)" {
		t.Fatalf("claude: %q", got)
	}
}

package fleet

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPickLoginURLAndCode(t *testing.T) {
	// codex 先打印本机回调地址，再打印真正的登录页
	text := "Starting local login server on http://localhost:1455.\n" +
		"If your browser did not open, navigate to this URL to authenticate:\n" +
		"https://auth.example.test/oauth/authorize?client_id=x&state=y.\n" +
		"Enter the code ABCD-EFGH when asked"
	if got := pickLoginURL(text, false); got != "https://auth.example.test/oauth/authorize?client_id=x&state=y" {
		t.Fatalf("url: %q", got)
	}
	if got := pickDeviceCode(text); got != "ABCD-EFGH" {
		t.Fatalf("code: %q", got)
	}
	if got := pickLoginURL("Web UI on http://127.0.0.1:7788/ ready", true); got != "http://127.0.0.1:7788/" {
		t.Fatalf("first-run web UI url: %q", got)
	}
	if got := pickLoginURL("callback http://127.0.0.1:9999/cb only", false); got != "" {
		t.Fatalf("loopback-only output must give no url, got %q", got)
	}
	if got := pickDeviceCode("build ABCD-EFGH finished"); got != "" {
		t.Fatalf("no code mentioned, got %q", got)
	}
	if got := readQR(`QR saved to C:\Windows\System32\secret.png`); got != "" {
		t.Fatal("png outside temp dir must not be read")
	}
}

type fakeVault struct {
	mu   sync.Mutex
	next int64
	m    map[int64]string
}

func (v *fakeVault) Save(_ context.Context, prev int64, _, _, secret string) (int64, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if prev == 0 {
		v.next++
		prev = v.next
	}
	v.m[prev] = secret
	return prev, "ws1", nil
}

func (v *fakeVault) Reveal(_ context.Context, _ string, id int64) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.m[id], nil
}

func (v *fakeVault) Delete(_ context.Context, _ string, id int64) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.m, id)
	return nil
}

// 假工具：login 打印登录页与设备码后退出，status 报账号，-p 只认指定的 Key。
const fakeAuthCmd = "@echo off\r\n" +
	"if \"%1\"==\"login\" (\r\n" +
	"  echo Listening on http://localhost:1455/cb\r\n" +
	"  echo Visit https://example.test/oauth/authorize?x=1 and enter code WXYZ-1234\r\n" +
	"  exit /b 0\r\n)\r\n" +
	"if \"%1\"==\"status\" (\r\n  echo Logged in as dev@example.test\r\n  exit /b 0\r\n)\r\n" +
	"if \"%FAKE_KEY%\"==\"good-key-123456\" (echo OK) else (echo Error: invalid x-api-key & exit /b 1)\r\n"

func TestToolKeyAndLogin(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only (.cmd shim)")
	}
	root := t.TempDir()
	t.Setenv("ARANEA_FLEET_CLI_ROOT", root)
	_ = os.MkdirAll(filepath.Join(root, "npm"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "npm", "fakeauth.cmd"), []byte(fakeAuthCmd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(`[{"id":"fakeauth","command":"fakeauth",
		"test_args":["-p","{prompt}"],"login_args":["login"],"status_args":["status"],"key_env":"FAKE_KEY","email_flag":"--email"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	v := &fakeVault{m: map[int64]string{}}
	SetKeyVault(v)
	u := newTestUsecase(t, t.TempDir())
	p, _ := findPreset("fakeauth")
	ctx := context.Background()
	verify := func(key string) TestResult {
		if err := u.SaveToolKey(ctx, "fakeauth", key, ""); err != nil {
			t.Fatal(err)
		}
		var res TestResult
		waitFor(t, "key verify", 30*time.Second, func() bool {
			var st string
			st, _, res = TestStatus("fakeauth")
			return st != InstallRunning
		})
		return res
	}

	if res := verify("bad-key-000000"); res.OK || !strings.Contains(res.Verdict, "没有保存") || u.GetAgentSetting(ctx, "fakeauth").KeyCredID != 0 {
		t.Fatalf("bad key must not be saved: %+v", res)
	}
	if AgentReadiness("fakeauth") == ReadyNeedLogin {
		t.Fatal("a rejected candidate key must not change the tool's login status")
	}
	if res := verify("good-key-123456"); !res.OK {
		t.Fatalf("good key: %+v", res)
	}
	set := u.GetAgentSetting(ctx, "fakeauth")
	if set.KeyCredID == 0 || set.KeyHint != "…3456" || v.m[set.KeyCredID] != "good-key-123456" {
		t.Fatalf("saved setting: %+v vault=%v", set, v.m)
	}
	if env := u.toolKeyEnv(p); len(env) != 1 || env[0] != "FAKE_KEY=good-key-123456" {
		t.Fatalf("inject: %v", env)
	}
	// 存下的 Key 之后的连通测试自动带上
	if err := u.TestAgent("fakeauth"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "test with saved key", 30*time.Second, func() bool { st, _, _ := TestStatus("fakeauth"); return st == InstallOK })
	if err := u.DeleteToolKey(ctx, "fakeauth"); err != nil || len(v.m) != 0 || u.GetAgentSetting(ctx, "fakeauth").KeyCredID != 0 {
		t.Fatalf("delete: err=%v vault=%v", err, v.m)
	}

	if _, err := u.StartToolLogin("fakeauth", ""); err != nil {
		t.Fatal(err)
	}
	var st LoginState
	waitFor(t, "login done", 30*time.Second, func() bool { st = u.ToolLoginState("fakeauth"); return st.Done })
	if !st.OK || st.Account != "dev@example.test" || st.URL != "https://example.test/oauth/authorize?x=1" || st.Code != "WXYZ-1234" {
		t.Fatalf("login state: %+v", st)
	}
	if AgentReadiness("fakeauth") != ReadyOK || u.GetAgentSetting(ctx, "fakeauth").Account != "dev@example.test" {
		t.Fatal("login must mark ready and keep the account")
	}
	if _, err := u.StartToolLogin("fakeauth", "bad email"); err == nil {
		t.Fatal("bad email must be rejected before anything starts")
	}
}

func TestToolEnvForACP(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARANEA_FLEET_CLI_ROOT", root)
	v := &fakeVault{m: map[int64]string{}}
	SetKeyVault(v)
	u := newTestUsecase(t, t.TempDir())
	ctx := context.Background()
	set := u.GetAgentSetting(ctx, "deepseek")
	id, _, _ := v.Save(ctx, 0, "", "", "sk-test-0001")
	set.KeyCredID, set.KeyWorkspace = id, "ws1"
	if err := putDoc(ctx, u.store, kindAgentSetting, set.ID, set); err != nil {
		t.Fatal(err)
	}
	got := u.ToolEnvForACP("dsh", map[string]string{"OTHER": "1"})
	if got["DEEPSEEK_API_KEY"] != "sk-test-0001" || got["OTHER"] != "1" {
		t.Fatalf("merged env: %v", got)
	}
	if got := u.ToolEnvForACP("dsh", map[string]string{"DEEPSEEK_API_KEY": "own"}); got["DEEPSEEK_API_KEY"] != "own" {
		t.Fatalf("registration env must win: %v", got)
	}
	if got := u.ToolEnvForACP("claude", map[string]string{"A": "1"}); len(got) != 1 {
		t.Fatalf("non-ACP tool untouched: %v", got)
	}
}

// 假工具：login --with-trae-pat 只认指定的令牌与企业域名，status 报账号。
const fakePATCmd = "@echo off\r\n" +
	"if \"%1\"==\"login\" if \"%2\"==\"--with-trae-pat\" (\r\n" +
	"  if \"%FAKE_PAT%\"==\"good-pat-123456\" if \"%FAKE_HOST%\"==\"https://api.example.test\" (echo Login succeeded & exit /b 0)\r\n" +
	"  echo Error: invalid personal access token & exit /b 1\r\n)\r\n" +
	"if \"%1\"==\"login\" if \"%2\"==\"status\" (echo Logged in as dev@example.test & exit /b 0)\r\n" +
	"exit /b 1\r\n"

func TestSaveToolKeyWithTokenLogin(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only (.cmd shim)")
	}
	root := t.TempDir()
	t.Setenv("ARANEA_FLEET_CLI_ROOT", root)
	_ = os.MkdirAll(filepath.Join(root, "npm"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "npm", "fakepat.cmd"), []byte(fakePATCmd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(`[{"id":"fakepat","command":"fakepat","test_args":["-p","{prompt}"]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	defaultAuth["fakepat"] = authSpec{status: []string{"login", "status"}, keyEnv: "FAKE_PAT", keyName: "访问令牌",
		hostEnv: "FAKE_HOST", keyLogin: []string{"login", "--with-trae-pat"}}
	t.Cleanup(func() { delete(defaultAuth, "fakepat") })
	v := &fakeVault{m: map[int64]string{}}
	SetKeyVault(v)
	u := newTestUsecase(t, t.TempDir())
	ctx := context.Background()
	save := func(key, host string) TestResult {
		if err := u.SaveToolKey(ctx, "fakepat", key, host); err != nil {
			t.Fatal(err)
		}
		var res TestResult
		waitFor(t, "token login", 30*time.Second, func() bool {
			var st string
			st, _, res = TestStatus("fakepat")
			return st != InstallRunning
		})
		return res
	}
	if err := u.SaveToolKey(ctx, "fakepat", "good-pat-123456", "http://api.example.test"); err == nil {
		t.Fatal("a non-https host must be rejected before anything runs")
	}
	if res := save("bad-pat-000000", "https://api.example.test"); res.OK || res.Verdict != "访问令牌 无效或没有权限，没有保存" {
		t.Fatalf("bad token: %+v", res)
	}
	if res := save("good-pat-123456", "https://api.example.test/"); !res.OK {
		t.Fatalf("good token: %+v", res)
	}
	set := u.GetAgentSetting(ctx, "fakepat")
	if set.Host != "https://api.example.test" || set.Account != "dev@example.test" || set.KeyHint != "…3456" {
		t.Fatalf("saved setting: %+v", set)
	}
	p, _ := findPreset("fakepat")
	if env := strings.Join(u.toolKeyEnv(p), " "); env != "FAKE_PAT=good-pat-123456 FAKE_HOST=https://api.example.test" {
		t.Fatalf("inject: %s", env)
	}
}

package fleet

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz/agentbridge"
)

// 摘要不被 TUI 重绘挤掉：重复的界面框行只留最后一次，转圈行和快捷键提示行去掉，框线剥掉，回复留下。
func TestTranscriptTail(t *testing.T) {
	redraw := "→ Add a follow-up\nctrl+c to stop\n⠰⠳ Working 109 tokens\n"
	text := strings.Repeat(redraw, 30) + "BRIDGE-DONE\n" + strings.Repeat(redraw, 30)
	got := transcriptTail(text, 5)
	if !strings.Contains(got, "BRIDGE-DONE") || strings.Contains(got, "Working") || strings.Contains(got, "ctrl+c to stop") {
		t.Fatalf("transcriptTail = %q", got)
	}
	// Grok 的回复画在框里，底下是输入框和快捷键行（2026-11-21 实测）
	framed := "╭────────────╮\n│ ARANEA-OK  │\n╰────────────╯\n┌──────┐\n│ >    │\n└──────┘\nGrok 4.7 · always-approve ─┘\nShift+Tab:mode  │  Ctrl+x:shortcuts\n"
	if got := transcriptTail(framed, 5); got != "ARANEA-OK\nGrok 4.7 · always-approve" {
		t.Fatalf("framed transcriptTail = %q", got)
	}
}

func TestLaunchArgvModesAndSettings(t *testing.T) {
	p := AgentPreset{ID: "x", ResolvedPath: "/bin/x", FullAccessFlag: "--yolo", AutoEditFlag: "--permission-mode acceptEdits"}
	if got := strings.Join(launchArgv(p, PermManual, []string{"-m", "m1"}), " "); got != "/bin/x -m m1" {
		t.Fatalf("manual: %s", got)
	}
	if got := strings.Join(launchArgv(p, PermAutoEdit, nil), " "); got != "/bin/x --permission-mode acceptEdits" {
		t.Fatalf("auto_edit: %s", got)
	}
	if got := strings.Join(launchArgv(p, PermFull, nil), " "); got != "/bin/x --yolo" {
		t.Fatalf("full: %s", got)
	}
	if launchMode(sessionSpec{FullAccess: true}, AgentSetting{Mode: PermManual}) != PermFull ||
		launchMode(sessionSpec{Mode: PermAutoEdit}, AgentSetting{Mode: PermFull}) != PermAutoEdit ||
		launchMode(sessionSpec{}, AgentSetting{Mode: PermAutoEdit}) != PermAutoEdit {
		t.Fatal("launchMode precedence wrong")
	}

	u := newTestUsecase(t, t.TempDir())
	ctx := context.Background()
	if got := u.GetAgentSetting(ctx, "claude"); got.Mode != PermManual {
		t.Fatalf("default mode = %q", got.Mode)
	}
	if _, err := u.SetAgentSetting(ctx, AgentSetting{ID: "claude", Mode: PermAutoEdit, Args: []string{"--model", "opus"}}); err != nil {
		t.Fatal(err)
	}
	if got := u.GetAgentSetting(ctx, "claude"); got.Mode != PermAutoEdit || strings.Join(got.Args, " ") != "--model opus" {
		t.Fatalf("setting not saved: %+v", got)
	}
	for _, bad := range []AgentSetting{
		{ID: "claude", Mode: "root"},
		{ID: "amp", Mode: PermFull},                      // 没有原生参数的档位
		{ID: "codex", Args: []string{"-m", "x & del *"}}, // shell 元字符
		{ID: "codex", Args: []string{"-c", "a=\"b\""}},   // 引号
		{ID: "nope", Mode: PermManual},                   // 不存在的工具
	} {
		if _, err := u.SetAgentSetting(ctx, bad); err == nil {
			t.Fatalf("should reject %+v", bad)
		}
	}
	// 上手状态落库：清掉内存后从库里读回
	u.recordReadiness("codex", ReadyOK)
	setReadiness("codex", "")
	u.loadReadiness()
	if AgentReadiness("codex") != ReadyOK {
		t.Fatalf("readiness not restored from store: %q", AgentReadiness("codex"))
	}
	for screen, want := range map[string]string{
		"Press any key to log in...":                                ReadyNeedLogin,
		"请使用豆包移动端扫码并在手机上确认":                                         ReadyNeedLogin,
		"[a] Trust this workspace\n[q] Quit":                        ReadyNeedTrust,
		"› 1. Trust and continue":                                   ReadyNeedTrust,
		"Run this command?\nNot in allowlist: echo\nRun (once) (y)": "",
	} {
		if got := classifyWaiting(screen); got != want {
			t.Fatalf("classifyWaiting(%q) = %q, want %q", screen, got, want)
		}
	}
}

// recHandler 记录进度，按 decision 作答。
type recHandler struct {
	mu       sync.Mutex
	updates  []string
	asked    []string
	decision string
}

func (h *recHandler) OnUpdate(kind, text string) {
	h.mu.Lock()
	h.updates = append(h.updates, kind+":"+text)
	h.mu.Unlock()
}

func (h *recHandler) OnPermission(_ context.Context, title string, opts []agentbridge.PermissionOption) (string, error) {
	h.mu.Lock()
	h.asked = append(h.asked, title)
	h.mu.Unlock()
	if len(opts) != 2 {
		return "", os.ErrInvalid
	}
	return h.decision, nil
}

// 假 CLI：收到提示后「干活」一会儿，弹 y/n 确认框；按 y 打 DONE-APPROVED，按 n / Esc 打 DONE-REJECTED。
const fakeCLI = `
process.stdin.setRawMode && process.stdin.setRawMode(true);
let stage = 0, buf = '';
const work = (then) => { let i = 0; const t = setInterval(() => { process.stdout.write('thinking '.repeat(24) + '\r\n'); if (++i >= 5) { clearInterval(t); then(); } }, 200); };
process.stdout.write('fake cli ready\r\n> ');
process.stdin.on('data', (d) => {
  const s = d.toString();
  if (stage === 0) { buf += s; if (buf.includes('\r')) { stage = 1; work(() => process.stdout.write('\r\nDo you want to proceed? (y/n)\r\n')); } }
  else if (stage === 1) {
    if (s.includes('y')) { stage = 2; work(() => process.stdout.write('\r\nDONE-APPROVED\r\n> ')); }
    else if (s.includes('n') || s.includes('\x1b')) { stage = 2; work(() => process.stdout.write('\r\nDONE-REJECTED\r\n> ')); }
  }
});
`

// 编码工具派活走运行台终端：提示送达、确认框交给人、按人的决定按键、做完返回屏幕摘要、进度持续上报。
func TestBridgeSessionApproveAndReject(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only (.cmd shim)")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	root := t.TempDir()
	t.Setenv("ARANEA_FLEET_CLI_ROOT", root)
	script := filepath.Join(root, "fakecli.js")
	if err := os.WriteFile(script, []byte(fakeCLI), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(root, "npm"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "npm", "fakecli.cmd"), []byte("@\""+node+"\" \""+script+"\" %*\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(`[{"id":"fakecli","label":"假 CLI","command":"fakecli","approve_key":"y"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	bridgePoll = 200 * time.Millisecond
	t.Cleanup(func() { bridgePoll = time.Second })

	repo := t.TempDir()
	u := newTestUsecase(t, repo)
	run := func(decision string) (string, *recHandler, error) {
		b, err := u.NewBridgeSession(&agentbridge.CodingAgent{Command: BridgeCommandPrefix + "fakecli"})
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()
		h := &recHandler{decision: decision}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		out, err := b.Prompt(ctx, repo, "do the thing", h)
		return out, h, err
	}
	var wg sync.WaitGroup
	results := map[string][3]any{}
	var mu sync.Mutex
	for _, d := range []string{"approve", "reject"} {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			out, h, err := run(d)
			mu.Lock()
			results[d] = [3]any{out, h, err}
			mu.Unlock()
		}(d)
	}
	wg.Wait()
	for d, want := range map[string]string{"approve": "DONE-APPROVED", "reject": "DONE-REJECTED"} {
		out, h, err := results[d][0].(string), results[d][1].(*recHandler), results[d][2]
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		if !strings.Contains(out, want) {
			t.Fatalf("%s: summary missing %s:\n%s", d, want, out)
		}
		if len(h.asked) != 1 || !strings.Contains(strings.ToLower(h.asked[0]), "do you want to proceed") {
			t.Fatalf("%s: permission asked %d times: %q", d, len(h.asked), h.asked)
		}
		if len(h.updates) == 0 || !strings.Contains(h.updates[0], "运行台") {
			t.Fatalf("%s: no progress updates: %q", d, h.updates)
		}
	}
	// 每次派活一个终端，做完进程被结束，标签和输出留着
	terms := u.ListTerminals("")
	if len(terms) != 2 {
		t.Fatalf("want 2 terminals, got %d", len(terms))
	}
	waitFor(t, "bridge terminals exit after Close", 15*time.Second, func() bool {
		for _, ti := range u.ListTerminals("") {
			if ti.Status != StatusExited && ti.Status != StatusFailed {
				return false
			}
		}
		return true
	})
	if _, err := u.NewBridgeSession(&agentbridge.CodingAgent{Command: BridgeCommandPrefix + "fakecli", Args: []string{"a|b"}}); err == nil {
		t.Fatal("args with shell metacharacters must be rejected")
	}
}

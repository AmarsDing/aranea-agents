package officecli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	artifactbiz "aranea-agents/internal/biz/artifact"
)

// ---------- 路径围栏 ----------

func TestJailPath(t *testing.T) {
	t.Run("相对路径放行", func(t *testing.T) {
		got, err := jailPath("docs/report.docx")
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join("docs", "report.docx") {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("当前目录文件", func(t *testing.T) {
		if _, err := jailPath("deck.pptx"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("拒绝空路径", func(t *testing.T) {
		if _, err := jailPath("  "); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("拒绝点点逃逸", func(t *testing.T) {
		for _, p := range []string{"../secret.docx", "a/../../b.docx", ".."} {
			if _, err := jailPath(p); err == nil {
				t.Fatalf("expected error for %q", p)
			}
		}
	})
	t.Run("拒绝绝对路径", func(t *testing.T) {
		abs := "/etc/passwd"
		if runtime.GOOS == "windows" {
			abs = `C:\Windows\system32\config.docx`
		}
		if _, err := jailPath(abs); err == nil {
			t.Fatalf("expected error for %q", abs)
		}
	})
	t.Run("拒绝卷名相对路径", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("卷名语义仅 Windows")
		}
		if _, err := jailPath(`C:secret.docx`); err == nil {
			t.Fatal("expected error for volume-relative path")
		}
	})
}

// ---------- 动词白名单与参数校验 ----------

func TestBuildReadArgv(t *testing.T) {
	t.Run("放行只读动词", func(t *testing.T) {
		for _, args := range [][]string{
			{"view", "report.docx", "outline"},
			{"get", "deck.pptx", "/slide[1]", "--depth", "1", "--json"},
			{"query", "data.xlsx", "cell[value>5000]"},
			{"validate", "report.docx"},
			{"dump", "report.docx"},
			{"help"},
			{"help", "docx", "paragraph"},
		} {
			if _, err := buildReadArgv(args); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		}
	})
	t.Run("拒绝写动词", func(t *testing.T) {
		for _, verb := range []string{"create", "add", "set", "remove", "save", "open", "close"} {
			if _, err := buildReadArgv([]string{verb, "a.docx"}); err == nil {
				t.Fatalf("verb %q must be rejected", verb)
			}
		}
	})
	t.Run("拒绝系统命令", func(t *testing.T) {
		for _, verb := range []string{"watch", "install", "mcp", "unwatch", "mark"} {
			if _, err := buildReadArgv([]string{verb, "a.docx"}); err == nil {
				t.Fatalf("verb %q must be rejected", verb)
			}
		}
	})
	t.Run("拒绝输出重定向与浏览器", func(t *testing.T) {
		for _, args := range [][]string{
			{"view", "a.docx", "html", "-o", "x.html"},
			{"view", "a.docx", "html", "--output", "x.html"},
			{"view", "a.docx", "html", "--browser"},
		} {
			if _, err := buildReadArgv(args); err == nil {
				t.Fatalf("%v must be rejected", args)
			}
		}
	})
	t.Run("拒绝越界与错位文件参数", func(t *testing.T) {
		if _, err := buildReadArgv([]string{"view", "../a.docx", "outline"}); err == nil {
			t.Fatal("expected jail error")
		}
		if _, err := buildReadArgv([]string{"view", "--json"}); err == nil {
			t.Fatal("expected flag-like file rejection")
		}
		if _, err := buildReadArgv([]string{"view"}); err == nil {
			t.Fatal("expected missing file error")
		}
	})
}

func TestBuildWriteArgv(t *testing.T) {
	t.Run("放行写动词", func(t *testing.T) {
		for _, args := range [][]string{
			{"create", "deck.pptx"},
			{"add", "deck.pptx", "/", "--type", "slide", "--prop", "title=Q4 Report"},
			{"set", "doc.docx", "/", "--find", "draft", "--replace", "final"},
			{"remove", "deck.pptx", "/slide[2]"},
			{"save", "deck.pptx"},
			{"open", "deck.pptx"},
			{"close", "deck.pptx"},
		} {
			if _, err := buildWriteArgv(args); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		}
	})
	t.Run("拒绝只读动词", func(t *testing.T) {
		for _, verb := range []string{"view", "get", "query", "validate", "help"} {
			if _, err := buildWriteArgv([]string{verb, "a.docx"}); err == nil {
				t.Fatalf("verb %q must be rejected", verb)
			}
		}
	})
	t.Run("拒绝空参数", func(t *testing.T) {
		if _, err := buildWriteArgv(nil); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestBuildRenderArgv(t *testing.T) {
	t.Run("组装输出名与 MIME", func(t *testing.T) {
		argv, out, mime, err := buildRenderArgv(renderInput{File: "docs/deck.pptx", Mode: "screenshot"})
		if err != nil {
			t.Fatal(err)
		}
		if argv[0] != "view" || argv[2] != "screenshot" || argv[3] != "-o" {
			t.Fatalf("unexpected argv %v", argv)
		}
		if !strings.HasPrefix(out, "deck-") || !strings.HasSuffix(out, ".png") {
			t.Fatalf("unexpected out name %q", out)
		}
		if filepath.Dir(out) != "." {
			t.Fatalf("out name must not carry dir: %q", out)
		}
		if mime != "image/png" {
			t.Fatalf("mime %q", mime)
		}
	})
	t.Run("全部模式", func(t *testing.T) {
		for mode, want := range map[string]string{"html": ".html", "screenshot": ".png", "svg": ".svg", "pdf": ".pdf"} {
			_, out, _, err := buildRenderArgv(renderInput{File: "a.docx", Mode: mode})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(out, want) {
				t.Fatalf("mode %s → %q, want suffix %s", mode, out, want)
			}
		}
	})
	t.Run("拒绝非法模式与越界文件", func(t *testing.T) {
		if _, _, _, err := buildRenderArgv(renderInput{File: "a.docx", Mode: "watch"}); err == nil {
			t.Fatal("expected mode error")
		}
		if _, _, _, err := buildRenderArgv(renderInput{File: "../a.docx", Mode: "html"}); err == nil {
			t.Fatal("expected jail error")
		}
	})
	t.Run("extra_args 禁止输出重定向", func(t *testing.T) {
		for _, extra := range [][]string{{"-o", "x.png"}, {"--output=x.png"}, {"--browser"}} {
			if _, _, _, err := buildRenderArgv(renderInput{File: "a.pptx", Mode: "html", ExtraArgs: extra}); err == nil {
				t.Fatalf("%v must be rejected", extra)
			}
		}
		if _, _, _, err := buildRenderArgv(renderInput{File: "a.pptx", Mode: "screenshot", ExtraArgs: []string{"--grid", "3"}}); err != nil {
			t.Fatal(err)
		}
	})
}

// ---------- exec 行为（helper 进程假冒 officecli） ----------

// TestHelperProcess 不是测试：作为子进程模拟 officecli 行为。
// 约定：FAKE_OFFICECLI_EXIT 控制退出码；argv 含 "view" 且带 -o 时写出渲染文件。
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	var args []string
	for i, a := range os.Args {
		if a == "--" && i+1 < len(os.Args) {
			args = os.Args[i+1:]
			break
		}
	}
	for i, a := range args {
		if a == "-o" && i+1 < len(args) && os.Getenv("FAKE_OFFICECLI_NOFILE") != "1" {
			_ = os.WriteFile(args[i+1], []byte("fake-render-bytes"), 0o644)
		}
	}
	os.Stdout.WriteString("stdout:")
	os.Stdout.WriteString(strings.Join(args, " "))
	os.Stdout.WriteString(" flush=" + os.Getenv("OFFICECLI_RESIDENT_FLUSH"))
	os.Stderr.WriteString("stderr-line")
	code := 0
	if v := os.Getenv("FAKE_OFFICECLI_EXIT"); v == "1" {
		code = 1
	}
	os.Exit(code)
}

// withFakeBin 把 execCommand seam 切到 helper 进程，返回恢复函数。
func withFakeBin(t *testing.T, exitCode string) func() {
	t.Helper()
	orig := execCommand
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	t.Setenv("FAKE_OFFICECLI_EXIT", exitCode)
	execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		argv := append([]string{"-test.run=TestHelperProcess", "--"}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], argv...)
		cmd.Env = os.Environ()
		return cmd
	}
	return func() { execCommand = orig }
}

func execResultMap(t *testing.T, res execResult) map[string]any {
	t.Helper()
	m, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("result not a map: %T", res.Result)
	}
	return m
}

func TestExecSuccess(t *testing.T) {
	defer withFakeBin(t, "0")()
	cfg := Config{Bin: os.Args[0], Timeout: 10 * time.Second}
	res := cfg.exec(context.Background(), t.TempDir(), []string{"view", "a.docx", "outline"})
	m := execResultMap(t, res)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("expected ok, got %v", m)
	}
	if !strings.Contains(m["stdout"].(string), "view a.docx outline") {
		t.Fatalf("stdout missing argv echo: %q", m["stdout"])
	}
	if m["exit_code"].(int) != 0 {
		t.Fatalf("exit_code %v", m["exit_code"])
	}
}

func TestExecNonZeroExit(t *testing.T) {
	defer withFakeBin(t, "1")()
	cfg := Config{Bin: os.Args[0], Timeout: 10 * time.Second}
	res := cfg.exec(context.Background(), t.TempDir(), []string{"get", "a.docx", "/nope"})
	m := execResultMap(t, res)
	if ok, _ := m["ok"].(bool); ok {
		t.Fatal("expected ok=false on non-zero exit")
	}
	if m["stderr"].(string) == "" {
		t.Fatal("stderr must be captured as diagnostic evidence")
	}
}

func TestExecBinaryMissing(t *testing.T) {
	cfg := Config{Bin: filepath.Join(t.TempDir(), "no-such-officecli-bin"), Timeout: time.Second}
	res := cfg.exec(context.Background(), t.TempDir(), []string{"help"})
	m := execResultMap(t, res)
	if ok, _ := m["ok"].(bool); ok {
		t.Fatal("expected ok=false when binary missing")
	}
	if !strings.Contains(m["error"].(string), "ARANEA_OFFICECLI_BIN") {
		t.Fatalf("error must hint install/config: %q", m["error"])
	}
}

// TestExecForcesResidentFlushEach 回归：officecli 默认驻留内存+空闲落盘，Agent
// 多进程调用模型下驻留进程可能被杀导致编辑丢失（2026-08-15 实测）；子进程必须
// 带 OFFICECLI_RESIDENT_FLUSH=each。用户显式设置时尊重用户值。
func TestExecForcesResidentFlushEach(t *testing.T) {
	defer withFakeBin(t, "0")()
	cfg := Config{Bin: os.Args[0], Timeout: 10 * time.Second}
	m := execResultMap(t, cfg.exec(context.Background(), t.TempDir(), []string{"add", "a.pptx", "/", "--type", "slide"}))
	if !strings.Contains(m["stdout"].(string), "flush=each") {
		t.Fatalf("child env must force OFFICECLI_RESIDENT_FLUSH=each: %q", m["stdout"])
	}

	t.Setenv("OFFICECLI_RESIDENT_FLUSH", "off")
	m = execResultMap(t, cfg.exec(context.Background(), t.TempDir(), []string{"view", "a.pptx", "outline"}))
	if !strings.Contains(m["stdout"].(string), "flush=off") {
		t.Fatalf("user-set OFFICECLI_RESIDENT_FLUSH must be respected: %q", m["stdout"])
	}
}

// ---------- render 制品落盘 ----------

type fakeSaver struct {
	saved   bool
	saveErr error
}

func (f *fakeSaver) Save(_ context.Context, _, name, mime string, data []byte) (artifactbiz.Artifact, error) {
	if f.saveErr != nil {
		return artifactbiz.Artifact{}, f.saveErr
	}
	f.saved = true
	if name == "" || mime == "" || len(data) == 0 {
		return artifactbiz.Artifact{}, errors.New("bad save args")
	}
	return artifactbiz.Artifact{ID: "art-1"}, nil
}

func callRender(t *testing.T, saver artifactbiz.Saver, in renderInput) map[string]any {
	t.Helper()
	tool := newRenderTool(Config{Bin: os.Args[0], Timeout: 10 * time.Second}, t.TempDir(), saver)
	raw, err := tool.Call(context.Background(), mustJSON(t, in))
	if err != nil {
		t.Fatal(err)
	}
	res, ok := raw.(execResult)
	if !ok {
		t.Fatalf("unexpected result type %T", raw)
	}
	return execResultMap(t, res)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRenderPersistsArtifactDegradesWithoutSession(t *testing.T) {
	defer withFakeBin(t, "0")()
	saver := &fakeSaver{}
	// 无会话上下文：制品跳过落盘但 ok 保持 true，文件路径返回。
	m := callRender(t, saver, renderInput{File: "deck.pptx", Mode: "screenshot"})
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("expected ok, got %v", m)
	}
	if saver.saved {
		t.Fatal("saver must not be called without session ctx")
	}
	if m["file"].(string) == "" || !strings.Contains(m["note"].(string), "会话 ID") {
		t.Fatalf("expected workspace-file fallback note, got %v", m)
	}
	if !strings.HasSuffix(m["file"].(string), ".png") {
		t.Fatalf("file %q", m["file"])
	}
}

func TestRenderNilSaver(t *testing.T) {
	defer withFakeBin(t, "0")()
	m := callRender(t, nil, renderInput{File: "a.docx", Mode: "html"})
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("expected ok, got %v", m)
	}
	if !strings.Contains(m["note"].(string), "制品服务不可用") {
		t.Fatalf("got %v", m)
	}
}

func TestRenderPropagatesExecFailure(t *testing.T) {
	defer withFakeBin(t, "1")()
	m := callRender(t, &fakeSaver{}, renderInput{File: "deck.pptx", Mode: "screenshot"})
	if ok, _ := m["ok"].(bool); ok {
		t.Fatal("expected ok=false")
	}
	if _, exists := m["artifact_url"]; exists {
		t.Fatal("failed render must not produce artifact")
	}
}

// TestRenderSVGFallsBackToStdout 回归：officecli svg 模式忽略 -o、产物只写
// stdout（2026-08-15 实测）——输出文件缺失时必须回退取 stdout，不得判失败。
func TestRenderSVGFallsBackToStdout(t *testing.T) {
	defer withFakeBin(t, "0")()
	t.Setenv("FAKE_OFFICECLI_NOFILE", "1") // helper 不写 -o 文件，模拟 svg 行为
	m := callRender(t, nil, renderInput{File: "deck.pptx", Mode: "svg"})
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("svg stdout fallback must stay ok, got %v", m)
	}
	if m["source"].(string) != "stdout" {
		t.Fatalf("expected source=stdout, got %v", m)
	}
	if size, _ := m["size_bytes"].(int); size == 0 {
		t.Fatal("size_bytes must reflect stdout payload")
	}
}

// ---------- EnabledTools 白名单 ----------

func TestEnabledTools(t *testing.T) {
	cfg := Config{Bin: "officecli"}
	dir := t.TempDir()

	t.Run("按 eff 过滤", func(t *testing.T) {
		got := EnabledTools(map[string]bool{ToolRead: true, ToolRender: true}, cfg, dir, nil)
		if len(got) != 2 {
			t.Fatalf("want 2 tools, got %d", len(got))
		}
		seen := map[string]bool{}
		for _, tool := range got {
			seen[tool.Declaration().Name] = true
		}
		if !seen[ToolRead] || !seen[ToolRender] || seen[ToolWrite] {
			t.Fatalf("unexpected set %v", seen)
		}
	})
	t.Run("空 eff 或空目录不挂载", func(t *testing.T) {
		if got := EnabledTools(nil, cfg, dir, nil); got != nil {
			t.Fatal("nil eff must mount nothing")
		}
		if got := EnabledTools(map[string]bool{ToolRead: true}, cfg, "", nil); got != nil {
			t.Fatal("empty dir must fail-closed")
		}
	})
	t.Run("AnyEnabled", func(t *testing.T) {
		if !AnyEnabled(map[string]bool{ToolWrite: true}) {
			t.Fatal("expected true")
		}
		if AnyEnabled(map[string]bool{"other": true}) {
			t.Fatal("expected false")
		}
	})
}

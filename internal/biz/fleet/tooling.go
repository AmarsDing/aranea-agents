package fleet

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/safego"
)

// 编码工具页用的三件事：登录终端、连通测试、版本与自动更新。都在「工具工作区」（<CLI 根>\.workspace）里跑，
// 不需要先登记项目。

// ToolWorkspaceID 是工具工作区的终端所属 worktree id（不落库，不出现在项目列表）。
const ToolWorkspaceID = "tools"

const testPrompt = "Reply with exactly OK and nothing else"

// defaultTestArgs 是各家非交互模式（打印一次回复就退出）的参数，2026-11-20 实测或取自各家文档。
var defaultTestArgs = map[string][]string{
	"claude":    {"-p", "{prompt}"},
	"codex":     {"exec", "--skip-git-repo-check", "{prompt}"},
	"gemini":    {"-p", "{prompt}"},
	"grok":      {"-p", "{prompt}"},
	"cursor":    {"-p", "--trust", "{prompt}"},
	"trae":      {"exec", "{prompt}"},
	"codebuddy": {"-p", "{prompt}"},
	"doubao":    {"-p", "{prompt}"},
	"qoder":     {"-p", "{prompt}"},
	"qwen":      {"-p", "{prompt}"},
	"copilot":   {"-p", "{prompt}"},
	"opencode":  {"run", "{prompt}"},
	"deepseek":  {"--profile", "headless", "{prompt}"},
}

func toolWorkspace() (Workspace, error) {
	root := CLIRoot()
	if root == "" {
		return Workspace{}, apierror.FailedPrecondition(domain, "未配置 ARANEA_FLEET_CLI_ROOT")
	}
	dir := filepath.Join(root, ".workspace")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Workspace{}, err
	}
	return Workspace{ID: ToolWorkspaceID, Name: "工具工作区", Path: dir, Kind: KindPrimary, Status: WorkspaceReady}, nil
}

// ---------- 连通测试 ----------

// TestResult 是最近一次连通测试的结论。
type TestResult struct {
	OK         bool   `json:"ok"`
	Verdict    string `json:"verdict"`
	Reply      string `json:"reply"`
	DurationMS int64  `json:"duration_ms"`
}

type testJob struct {
	installJob // 复用状态与日志缓冲
	result     TestResult
}

var (
	testMu   sync.Mutex
	testJobs = map[string]*testJob{}
)

// TestStatus 返回最近一次连通测试的状态、日志与结论；没测过返回空。
func TestStatus(id string) (state, log string, res TestResult) {
	testMu.Lock()
	j := testJobs[id]
	testMu.Unlock()
	if j == nil {
		return "", "", TestResult{}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state, string(j.log), j.result
}

// TestAgent 在后台做一次连通测试：非交互模式真实发一句话，看回复、耗时，判断要不要登录、额度是否用完。
func (u *Usecase) TestAgent(id string) error {
	return u.startTest(id, false, func(p AgentPreset, set AgentSetting, dir string, j *installJob) TestResult {
		return u.runSmokeTest(p, set, dir, j, nil)
	}, nil)
}

// testRunner 是一次测试的具体做法（真实对话，或验证候选 Key）。
type testRunner func(p AgentPreset, set AgentSetting, dir string, j *installJob) TestResult

// startTest 起一次后台测试，进度与结论走 TestStatus。candidate = 试的是候选凭据（新 API Key），
// 不改工具当前的登录状态；after 在出结论后、标记完成前调用，可改写结论（如保存 Key 失败）。
func (u *Usecase) startTest(id string, candidate bool, run testRunner, after func(TestResult) TestResult) error {
	p, ok := findPreset(id)
	if !ok || p.ID == "shell" {
		return apierror.BadRequest(domain, "unknown agent: "+id)
	}
	if !p.Available {
		return apierror.FailedPrecondition(domain, p.Label+" 还没安装")
	}
	if len(p.TestArgs) == 0 {
		return apierror.FailedPrecondition(domain, p.Label+" 没有非交互模式，不能做连通测试；可点「登录」在终端里试")
	}
	w, err := toolWorkspace()
	if err != nil {
		return err
	}
	testMu.Lock()
	if j := testJobs[id]; j != nil && j.state == InstallRunning {
		testMu.Unlock()
		return apierror.FailedPrecondition(domain, p.Label+" 正在测试")
	}
	j := &testJob{installJob: installJob{state: InstallRunning}}
	testJobs[id] = j
	testMu.Unlock()

	set := u.GetAgentSetting(context.Background(), id)
	safego.Go(u.ctx, "fleet-test-"+id, func() {
		res := run(p, set, w.Path, &j.installJob)
		switch {
		case candidate:
		case res.OK:
			u.recordReadiness(id, ReadyOK)
		case strings.Contains(res.Verdict, "登录"):
			u.recordReadiness(id, ReadyNeedLogin)
		}
		if after != nil {
			res = after(res)
		}
		j.mu.Lock()
		j.result = res
		j.state = InstallOK
		if !res.OK {
			j.state = InstallFailed
		}
		j.mu.Unlock()
	})
	return nil
}

func (u *Usecase) runSmokeTest(p AgentPreset, set AgentSetting, dir string, j *installJob, extraEnv []string) TestResult {
	test := make([]string, 0, len(p.TestArgs))
	for _, a := range p.TestArgs {
		test = append(test, strings.ReplaceAll(a, "{prompt}", testPrompt))
	}
	// 子命令（codex exec、opencode run）在前，模型与附加参数跟在它后面才对子命令生效
	lead := 0
	if len(p.TestArgs) > 0 && !strings.HasPrefix(p.TestArgs[0], "-") && !strings.Contains(p.TestArgs[0], "{prompt}") {
		lead = 1
	}
	args := append(append(append([]string(nil), test[:lead]...), withModel(p, set.Model, set.Effort, set.Args)...), test[lead:]...)
	j.logf("$ %s %s", p.Command, strings.Join(args, " "))
	j.logf("（在 %s 里运行，最长 120 秒）", dir)
	ctx, cancel := context.WithTimeout(u.ctx, 120*time.Second)
	defer cancel()
	var buf strings.Builder
	start := time.Now()
	env := append(append(u.terminalEnv("tooltest", ""), u.toolKeyEnv(p)...), extraEnv...)
	err := runTool(ctx, p, args, dir, env, io.MultiWriter(j, &buf))
	res := TestResult{DurationMS: time.Since(start).Milliseconds()}
	out := StripANSI(buf.String())
	res.Reply = lastLines(out, 6)
	low := strings.ToLower(out)
	// 额度与网络问题先判：不少工具遇到 429 / 断网会静默重试到超时，只报超时看不出原因
	switch {
	case err == nil && strings.Contains(out, "OK"):
		res.OK, res.Verdict = true, "能对话"
	case quotaHit(low):
		res.Verdict = "模型服务拒绝：额度用完或被限流"
	case containsAny(low, "high demand", "overloaded", "status 503", "\"code\":503", "status: 503", "529"):
		res.Verdict = "模型服务繁忙（厂商临时过载）：换个模型或稍后再试"
	case containsAny(low, "etimedout", "econnreset", "econnrefused", "enotfound", "eai_again", "fetch failed", "socket hang up"):
		res.Verdict = "连不上模型服务（检查网络或代理）：" + failLine(out, "")
	case ctx.Err() == context.DeadlineExceeded:
		res.Verdict = "超时：120 秒内没有回复（模型服务慢或卡在交互界面）"
	case classifyWaiting(out) == ReadyNeedLogin || containsAny(low, "not logged in", "/login", "auth method", "not authenticated", "api_key", "api key", "x-api-key", "unauthorized", "authentication_error"):
		res.Verdict = "要登录：点「登录」完成登录（或配好 API Key）后再测"
	case strings.Contains(low, "no longer supported"):
		res.Verdict = "厂商已停用这种登录方式：" + failLine(out, "")
	case err != nil:
		res.Verdict = "失败：" + failLine(out, err.Error())
	default:
		res.Verdict = "回复里没有 OK，可能没按要求回答"
	}
	j.logf("── %s（%.1f 秒）", res.Verdict, float64(res.DurationMS)/1000)
	return res
}

// failLine 取输出里最能说明原因的一行（含 error / not supported 等的最后一行），没有就用 fallback。
func failLine(out, fallback string) string {
	lines := nonEmptyLines(out)
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.ToLower(lines[i]); containsAny(l, "error", "not supported", "unsupported", "failed", "invalid", "denied", "not found", "no longer") {
			if r := []rune(lines[i]); len(r) > 200 {
				return string(r[:200]) + "…"
			}
			return lines[i]
		}
	}
	return fallback
}

// quotaHit：输出里有额度用完 / 限流的迹象（s 已转小写）。
func quotaHit(s string) bool {
	return containsAny(s, "429", "rate limit", "quota", "credit", "resource_exhausted", "resource exhausted")
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func lastLines(s string, n int) string {
	lines := nonEmptyLines(s)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// runTool 非交互地跑一次工具。Windows 上 npm 的 .cmd 垫片要经 cmd.exe，且带空格的路径与参数
// 必须用 /s 原样命令行，否则 cmd 会剥掉引号；参数只来自内置表与已校验的工具设置，不含 shell 元字符。
func runTool(ctx context.Context, p AgentPreset, args []string, dir string, env []string, out io.Writer) error {
	var cmd *exec.Cmd
	lower := strings.ToLower(p.ResolvedPath)
	if runtime.GOOS == "windows" && (strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat")) {
		quoted := make([]string, 0, len(args)+1)
		quoted = append(quoted, `"`+p.ResolvedPath+`"`)
		for _, a := range args {
			if strings.ContainsAny(a, " \t") {
				a = `"` + a + `"`
			}
			quoted = append(quoted, a)
		}
		cmd = exec.CommandContext(ctx, comspec())
		setRawCmdLine(cmd, `/d /s /c "`+strings.Join(quoted, " ")+`"`)
	} else {
		cmd = exec.CommandContext(ctx, p.ResolvedPath, args...)
	}
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = 5 * time.Second
	return cmd.Run()
}

// ---------- 版本与自动更新 ----------

// recordInstalled 在安装成功后记下版本与时间（版本取 --version 第一行）。
func (u *Usecase) recordInstalled(id string) {
	p, ok := findPreset(id)
	if !ok || !p.Available {
		return
	}
	s := u.GetAgentSetting(context.Background(), id)
	s.Version, s.UpdatedAt = u.toolVersion(p), nowString()
	_ = putDoc(context.Background(), u.store, kindAgentSetting, id, s)
}

func (u *Usecase) toolVersion(p AgentPreset) string {
	ctx, cancel := context.WithTimeout(u.ctx, 30*time.Second)
	defer cancel()
	var buf strings.Builder
	_ = runTool(ctx, p, []string{"--version"}, os.TempDir(), u.terminalEnv("toolver", ""), &buf)
	ver := pickVersion(StripANSI(buf.String()))
	if r := []rune(ver); len(r) > 60 {
		ver = string(r[:60])
	}
	return ver
}

var versionRe = regexp.MustCompile(`\d+\.\d+`)

// pickVersion 取 --version 输出里第一行像版本号的（TRAE 会多打几行构建信息）；都不像就取第一行。
func pickVersion(out string) string {
	lines := nonEmptyLines(out)
	for _, l := range lines {
		if versionRe.MatchString(l) {
			return strings.TrimSpace(l)
		}
	}
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

// backfillVersions 给早先装好、没记版本的工具补一次版本号（只补版本，不动更新时间）。
func (u *Usecase) backfillVersions() {
	for _, p := range ListPresets() {
		if p.ID == "shell" || !p.Available {
			continue
		}
		if s := u.GetAgentSetting(u.ctx, p.ID); !versionRe.MatchString(s.Version) {
			s.Version = u.toolVersion(p)
			_ = putDoc(u.ctx, u.store, kindAgentSetting, p.ID, s)
		}
	}
}

// autoUpdateLoop 每小时看一次：开了自动更新、距上次安装满 24 小时的工具依次重装到最新版。
func (u *Usecase) autoUpdateLoop() {
	u.backfillVersions()
	u.refreshLoginStatus()
	u.checkUpdatesDue()
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-u.ctx.Done():
			return
		case <-tick.C:
		}
		u.refreshLoginStatus()
		u.checkUpdatesDue()
		for _, p := range ListPresets() {
			if p.ID == "shell" || !p.Available || p.Install == "" {
				continue
			}
			s := u.GetAgentSetting(u.ctx, p.ID)
			if !s.AutoUpdate || !updateDue(s.UpdatedAt, time.Now()) {
				continue
			}
			if err := u.InstallAgent(p.ID); err != nil {
				continue
			}
			for st, _ := InstallStatus(p.ID); st == InstallRunning; st, _ = InstallStatus(p.ID) {
				select {
				case <-u.ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
	}
}

func updateDue(updatedAt string, now time.Time) bool {
	t, err := time.Parse(time.RFC3339, updatedAt)
	return err != nil || now.Sub(t) >= 24*time.Hour
}

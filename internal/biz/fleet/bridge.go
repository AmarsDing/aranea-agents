package fleet

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/pkg/apierror"
)

// BridgeCommandPrefix 标记「在运行台终端里执行」的编码 agent：agentbridge 里 command 登记为 fleet:<工具 id>。
// 这样编码工具页、coding_* 工具（aranea 的 Agent、Graph 的 Agent 节点）、工作台「派给 AI」
// 都能把活派给运行台里的九家 CLI，执行过程同时在运行台终端里实时可见。
const BridgeCommandPrefix = "fleet:"

// IsBridgeCommand 报告 agentbridge 登记的命令是不是运行台终端。
func IsBridgeCommand(cmd string) bool {
	return strings.HasPrefix(strings.TrimSpace(cmd), BridgeCommandPrefix)
}

var bridgeOptions = []agentbridge.PermissionOption{
	{OptionID: "approve", Name: "批准", Kind: "allow_once"},
	{OptionID: "reject", Name: "拒绝", Kind: "reject_once"},
}

// BridgeSession 实现 agentbridge.ACPSession：一次派发 = 运行台里一个终端。
type BridgeSession struct {
	u      *Usecase
	preset AgentPreset
	mode   string
	args   []string

	// effective 是这次派活实际生效的档位（派活指定的，否则是工具设置里的）
	effective string
	// seen 是提示发出时、每次确认时屏幕上已有的行（启动画面、确认菜单）：最后的回复不要它们
	seen map[string]bool
	// promptHead 是提示的开头：屏幕上回显提示的行不算回复
	promptHead string

	mu        sync.Mutex
	term      string
	cancelled bool
}

var _ agentbridge.ACPSession = (*BridgeSession)(nil)

// NewBridgeSession 按编码 agent 的登记建会话。autonomy：full = 完全放权，workspace = 自动改文件，其余用工具默认设置。
func (u *Usecase) NewBridgeSession(agent *agentbridge.CodingAgent) (*BridgeSession, error) {
	id := strings.TrimPrefix(strings.TrimSpace(agent.Command), BridgeCommandPrefix)
	p, ok := findPreset(id)
	if !ok || p.ID == "shell" {
		return nil, apierror.BadRequest(domain, "运行台没有这个工具："+id)
	}
	if !p.Available {
		return nil, apierror.FailedPrecondition(domain, p.Label+" 未安装，可在运行台「CLI 工具」里一键安装")
	}
	if err := validateArgs(agent.Args); err != nil {
		return nil, err
	}
	b := &BridgeSession{u: u, preset: p}
	switch agent.DefaultAutonomy {
	case "full":
		if p.FullAccessFlag != "" {
			b.mode = PermFull
		}
	case "workspace":
		if p.AutoEditFlag != "" {
			b.mode = PermAutoEdit
		}
	}
	if len(agent.Args) > 0 {
		b.args = agent.Args
	}
	return b, nil
}

// 平台自己的信号（不算 agent 的一轮进展）：启动说明、画面快照。
const (
	bridgeNoteKind   = "_aranea/note"
	bridgeScreenKind = "_aranea/screen"
)

// Prompt 在 cwd 对应的运行台 worktree 里起终端、送提示，阻塞到这一轮做完；确认框经 h.OnPermission 交给人。
func (b *BridgeSession) Prompt(ctx context.Context, cwd, prompt string, h agentbridge.EventHandler) (string, error) {
	w, err := b.u.workspaceByPath(ctx, cwd)
	if err != nil {
		return "", err
	}
	if r := []rune(strings.Join(strings.Fields(prompt), " ")); len(r) > 24 {
		b.promptHead = string(r[:24])
	} else {
		b.promptHead = string(r)
	}
	// 实际生效的档位：派活时指定的，否则是工具设置里的（与 startTerminal 的 launchMode 同口径）
	b.effective = b.mode
	if b.effective == "" {
		b.effective = b.u.GetAgentSetting(ctx, b.preset.ID).Mode
	}
	// 目录是用户自己登记的工作目录：目录信任框自动过，不再当成一次确认来问人
	info, err := b.u.startTerminal(w, sessionSpec{WorkspaceID: w.ID, Agent: b.preset.ID, Prompt: prompt, Mode: b.mode, Args: b.args,
		TrustKey: b.preset.TrustKey, Title: "编码任务 · " + truncate(prompt, 24)})
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	b.term = info.ID
	b.mu.Unlock()
	h.OnUpdate(bridgeNoteKind, fmt.Sprintf("在运行台「%s」里启动 %s（%s）", w.Name, b.preset.Label, w.Path))
	out, err := b.watch(ctx, info.ID, h)
	if err == nil && strings.TrimSpace(out) != "" {
		// 做完时把终端里的最终输出当作它的回复
		h.OnUpdate("agent_message_chunk", out)
	}
	return out, err
}

// bridgePoll 是 watch 的轮询间隔（测试里调小）。
var bridgePoll = time.Second

func (b *BridgeSession) watch(ctx context.Context, id string, h agentbridge.EventHandler) (string, error) {
	var lastExcerpt string
	var lastUpdate, lastBeat time.Time
	snapped := false
	tick := time.NewTicker(bridgePoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-tick.C:
		}
		if b.isCancelled() {
			return "", errors.New("已取消")
		}
		s, ok := b.u.Session(id)
		if !ok {
			return "", errors.New("运行台终端已被关闭")
		}
		ti := s.Info()
		now := time.Now()
		// 画面快照只用来刷新「最近一句」：TUI 的框线、转圈每次重绘都不一样，当成回复会把对话刷成乱码
		if ex := transcriptTail(s.ReadText(6), 3); ex != "" && ex != lastExcerpt && now.Sub(lastUpdate) >= 3*bridgePoll {
			h.OnUpdate(bridgeScreenKind, ex)
			lastExcerpt, lastUpdate, lastBeat = ex, now, now
		} else if now.Sub(lastBeat) >= 10*bridgePoll {
			h.OnUpdate("_aranea/heartbeat", "")
			lastBeat = now
		}
		// 工具自己的弹窗（订阅 / 额度提示，带「Don't show again」）：它不会自己走开，派活也不会有结果，直接说清楚
		if blocked := strings.ToLower(strings.Join(strings.Fields(s.ReadText(20)), " ")); strings.Contains(blocked, "show again") {
			return "", fmt.Errorf("%s 停在它自己的提示框上（订阅 / 额度提示，带「Don't show again」），不会继续做。在运行台里打开这个终端处理一次后再派活", b.preset.Label)
		}
		// 提示发出、开始干活时，屏幕上的启动画面记下来：最后的回复不要它
		if !snapped && ti.Status == StatusWorking {
			b.remember(s.ReadText(400))
			snapped = true
		}
		switch ti.Status {
		case StatusExited, StatusFailed:
			return "", fmt.Errorf("%s 已退出（%d）：%s", b.preset.Label, ti.ExitCode, s.ReadText(8))
		case StatusDone:
			return b.answer(s.ReadText(400)), nil
		case StatusNeedsInput:
			if err := b.ask(ctx, s, h); err != nil {
				return "", err
			}
		}
	}
}

// ask 把确认框交给人，按人的决定按键，再等确认框消失（免得同一个框问两次）。
func (b *BridgeSession) ask(ctx context.Context, s *Session, h agentbridge.EventHandler) error {
	screen := s.ReadText(14)
	kind := classifyWaiting(screen)
	if kind == ReadyNeedLogin {
		return fmt.Errorf("%s 需要先登录（%s）。在运行台「CLI 工具」里打开它完成登录后再派活", b.preset.Label, b.preset.LoginHint)
	}
	opt := ""
	switch {
	case kind == ReadyNeedTrust:
		// 用户登记的工作目录：信任框不问人
		opt = "approve"
	case b.effective == PermFull:
		// 工具设置成「完全放权」：它仍弹出的确认框按用户的设置直接允许
		opt = "approve"
		h.OnUpdate(bridgeNoteKind, "已按「完全放权」自动确认："+transcriptTail(screen, 2))
	default:
		var err error
		opt, err = h.OnPermission(ctx, b.preset.Label+" 请求确认：\n"+transcriptTail(screen, 10), bridgeOptions)
		if err != nil {
			return err
		}
	}
	key := orDefault(b.preset.RejectKey, "\x1b")
	if opt == "approve" {
		key = orDefault(b.preset.ApproveKey, "\r")
		if kind == ReadyNeedTrust && b.preset.TrustKey != "" {
			key = b.preset.TrustKey
		}
	}
	// 确认菜单也记下来：答过之后它还留在屏幕上方，不算回复
	b.remember(s.ReadText(400))
	if err := s.Write([]byte(key)); err != nil {
		return err
	}
	for i := 0; i < 15; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(bridgePoll):
		}
		if s.Info().Status != StatusNeedsInput {
			return nil
		}
	}
	return nil
}

// transcriptTail 把终端回放压成摘要：TUI 每次重绘都会重复输入框、提示、转圈行，
// 每行只留最后一次出现、去掉转圈行，再取末 n 行——agent 的回复因此不被界面框挤掉。
func transcriptTail(text string, n int) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = contentOf(l)
	}
	last := map[string]int{}
	for i, t := range lines {
		last[t] = i
	}
	var out []string
	for i, t := range lines {
		if t == "" || last[t] != i || strings.IndexFunc(t, isSpinner) >= 0 || isUIChrome(t) {
			continue
		}
		out = append(out, t)
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return strings.Join(out, "\n")
}

// remember 记下屏幕上已有的行（按去掉框线后的文字）。
func (b *BridgeSession) remember(text string) {
	if b.seen == nil {
		b.seen = map[string]bool{}
	}
	for _, l := range strings.Split(text, "\n") {
		if t := contentOf(l); t != "" {
			b.seen[t] = true
		}
	}
}

// answer 是做完时的回复：屏幕最后一段里，提示发出、确认之后才出现的行；一行都没有就退回整段尾部。
func (b *BridgeSession) answer(text string) string {
	all := transcriptTail(text, 30)
	if len(b.seen) == 0 {
		return all
	}
	var fresh []string
	for _, l := range strings.Split(all, "\n") {
		if l != "" && !b.seen[l] && (b.promptHead == "" || !strings.Contains(l, b.promptHead)) {
			fresh = append(fresh, l)
		}
	}
	if len(fresh) == 0 {
		return all
	}
	return strings.Join(fresh, "\n")
}

// contentOf 去掉一行两头的框线（TUI 把回复、输入框画在 │ … │ 里），只留里面的字；整行都是框线的返回空。
func contentOf(line string) string {
	t := strings.TrimFunc(strings.TrimSpace(line), func(r rune) bool { return isBoxRune(r) || r == ' ' })
	if strings.IndexFunc(t, func(r rune) bool { return !isBoxRune(r) && r != ' ' }) < 0 {
		return ""
	}
	// 空的输入框只剩一个提示符
	if t == ">" || t == "❯" || t == "›" {
		return ""
	}
	return t
}

func isBoxRune(r rune) bool { return (r >= 0x2500 && r <= 0x257F) || r == '|' }

// uiChrome 是各家 TUI 每屏都画的提示行（快捷键、模式、模型切换、小贴士），不是 agent 的回复。
var uiChrome = []string{
	"shift+tab", "ctrl+", "esc to", "? for shortcuts", "/model to change", "to interrupt",
	"to toggle", "↵ send", "tip:",
	// 想 / 答的过程行和空输入框的占位字（Grok、Codex、CodeBuddy、豆包，2026-11-21 实测）
	"thinking…", "thinking...", "responding…", "thought for", "worked for", "to do anything", "for the next task",
}

// progressLine 是「Waking… (2s · streaming)」「Responding… (0s • esc…)」这类进度行（CodeBuddy、豆包，2026-11-21 实测）。
var progressLine = regexp.MustCompile(`…\s*\(\d+s\b`)

func isUIChrome(line string) bool {
	// 一个字母都没有的行是转圈、计数器的碎片（「/ 2 2」「◦ 3」「8.1」）
	if strings.IndexFunc(line, unicode.IsLetter) < 0 {
		return true
	}
	l := strings.ToLower(line)
	for _, p := range uiChrome {
		if strings.Contains(l, p) {
			return true
		}
	}
	return progressLine.MatchString(line)
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func (b *BridgeSession) isCancelled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cancelled
}

func (b *BridgeSession) session() (*Session, bool) {
	b.mu.Lock()
	id := b.term
	b.mu.Unlock()
	if id == "" {
		return nil, false
	}
	return b.u.Session(id)
}

// Cancel 先按 Ctrl+C 请 agent 停下，Prompt 随即以「已取消」返回。
func (b *BridgeSession) Cancel(context.Context) error {
	b.mu.Lock()
	b.cancelled = true
	b.mu.Unlock()
	if s, ok := b.session(); ok {
		return s.Write([]byte{0x03})
	}
	return nil
}

// Kill 结束终端进程（标签和输出留在运行台里可回看）。
func (b *BridgeSession) Kill(context.Context) error {
	if s, ok := b.session(); ok {
		s.Kill()
	}
	return nil
}

// Ping 终端还在跑就算活着。
func (b *BridgeSession) Ping(context.Context) error {
	s, ok := b.session()
	if !ok {
		return nil // 还没起终端（Prompt 之前）
	}
	if st := s.Info().Status; st == StatusExited || st == StatusFailed {
		return errors.New("终端已退出")
	}
	return nil
}

// ProcessPID 返回终端进程 PID。
func (b *BridgeSession) ProcessPID() int {
	if s, ok := b.session(); ok {
		return s.Info().PID
	}
	return 0
}

// Close 结束进程，免得每次派活都留下一个空闲的 CLI 进程；终端输出留在运行台。
func (b *BridgeSession) Close() error { return b.Kill(context.Background()) }

// workspaceByPath 找路径与 cwd 相同的 worktree（含主检出）。编码工具与运行台共用项目登记，主检出总能找到。
func (u *Usecase) workspaceByPath(ctx context.Context, cwd string) (Workspace, error) {
	ws, err := u.ListWorkspaces(ctx, "", false)
	if err != nil {
		return Workspace{}, err
	}
	want := filepath.Clean(cwd)
	for _, w := range ws {
		if strings.EqualFold(filepath.Clean(w.Path), want) && w.Status == WorkspaceReady {
			return w, nil
		}
	}
	return Workspace{}, apierror.NotFound(domain, "运行台里没有这个目录的项目："+cwd)
}

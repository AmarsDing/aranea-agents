package fleet

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// Prompt 在 cwd 对应的运行台 worktree 里起终端、送提示，阻塞到这一轮做完；确认框经 h.OnPermission 交给人。
func (b *BridgeSession) Prompt(ctx context.Context, cwd, prompt string, h agentbridge.EventHandler) (string, error) {
	w, err := b.u.workspaceByPath(ctx, cwd)
	if err != nil {
		return "", err
	}
	info, err := b.u.startTerminal(w, sessionSpec{WorkspaceID: w.ID, Agent: b.preset.ID, Prompt: prompt, Mode: b.mode, Args: b.args,
		Title: "编码任务 · " + truncate(prompt, 24)})
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	b.term = info.ID
	b.mu.Unlock()
	h.OnUpdate("agent_message_chunk", fmt.Sprintf("已在运行台「%s」里启动 %s，可在运行台看实时终端", w.Name, b.preset.Label))
	return b.watch(ctx, info.ID, h)
}

// bridgePoll 是 watch 的轮询间隔（测试里调小）。
var bridgePoll = time.Second

func (b *BridgeSession) watch(ctx context.Context, id string, h agentbridge.EventHandler) (string, error) {
	var lastExcerpt string
	var lastUpdate, lastBeat time.Time
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
		if ex := s.ReadText(6); ex != lastExcerpt && now.Sub(lastUpdate) >= 3*bridgePoll {
			h.OnUpdate("agent_message_chunk", ex)
			lastExcerpt, lastUpdate, lastBeat = ex, now, now
		} else if now.Sub(lastBeat) >= 10*bridgePoll {
			h.OnUpdate("_aranea/heartbeat", "")
			lastBeat = now
		}
		switch ti.Status {
		case StatusExited, StatusFailed:
			return "", fmt.Errorf("%s 已退出（%d）：%s", b.preset.Label, ti.ExitCode, s.ReadText(8))
		case StatusDone:
			return transcriptTail(s.ReadText(400), 30), nil
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
	opt, err := h.OnPermission(ctx, b.preset.Label+" 请求确认：\n"+screen, bridgeOptions)
	if err != nil {
		return err
	}
	key := orDefault(b.preset.RejectKey, "\x1b")
	if opt == "approve" {
		key = orDefault(b.preset.ApproveKey, "\r")
		if kind == ReadyNeedTrust && b.preset.TrustKey != "" {
			key = b.preset.TrustKey
		}
	}
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
	last := map[string]int{}
	for i, l := range lines {
		last[strings.TrimSpace(l)] = i
	}
	var out []string
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || last[t] != i || strings.IndexFunc(t, isSpinner) >= 0 {
			continue
		}
		out = append(out, t)
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return strings.Join(out, "\n")
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

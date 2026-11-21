package service

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/agentbridge/acp"
	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/internal/biz/fleet"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/safego"
)

// acpSummaryLimit 结果摘要截断上限（设计 §12：入库前截断 4000 字符）。
const acpSummaryLimit = 4000

// selfLaunchCommand 是注册表 command 字段的内置 shim 哨兵："@self" 解析为
// 宿主进程自身可执行文件。配合 args=["acp-shim","--base-url=…",…] 复用
// admin 二进制的通用 shim 子命令，实现 OpenAI 兼容工具零新增二进制接入。
const selfLaunchCommand = "@self"

// resolveLaunchCommand 把 "@self" 解析为宿主可执行文件绝对路径；裸命令名先在运行台
// 一键安装目录里找（如装在 D:\...\CLI\npm 的 claude-agent-acp），找不到再原样返回交 PATH
// 解析。解析失败回退原值（交 spawn/探针路径给出诊断）。
func resolveLaunchCommand(command string) string {
	if c := strings.TrimSpace(command); c != selfLaunchCommand {
		if c != "" && !strings.ContainsAny(c, `/\`) {
			if p, ok := fleet.LookInCLIRoot(c); ok {
				return p
			}
		}
		return command
	}
	exe, err := os.Executable()
	if err != nil {
		return command
	}
	return exe
}

// acpSessionFactory 是 agentbridge.SessionFactory 的生产实现：
// 按 agent 注册信息 spawn ACP 子进程并完成 initialize 握手；command 为 fleet:<工具 id> 的
// 编码 agent 改在运行台终端里执行（见 fleet.BridgeSession）。
type acpSessionFactory struct{}

// fleetBridge 是运行台用例，由 provideFleetUsecase 装配时登记。不进 wire 图：会话工厂先于运行台构造，
// 且 cmd/admin 的 wire 生成当前因既有漂移失败（2026-11-20 登记），改依赖关系就得手改 wire_gen。
var fleetBridge atomic.Pointer[fleet.Usecase]

// SetFleetBridge 登记运行台用例，fleet:<工具 id> 的编码 agent 由它执行。
func SetFleetBridge(u *fleet.Usecase) { fleetBridge.Store(u) }

// NewACPSessionFactory 构造 ACP 会话工厂（Wire 装配用）。
func NewACPSessionFactory() agentbridge.SessionFactory {
	return acpSessionFactory{}
}

// Spawn 启动子进程 + initialize 握手。会话（session/new）延迟到
// Prompt 时建立——cwd 在派发解析项目后才确定。
func (acpSessionFactory) Spawn(ctx context.Context, agent *agentbridge.CodingAgent) (agentbridge.ACPSession, error) {
	if fleet.IsBridgeCommand(agent.Command) {
		fl := fleetBridge.Load()
		if fl == nil {
			return nil, apierror.Unavailable(apierror.DomainAgentBridge, "运行台不可用（数据层未就绪）")
		}
		return fl.NewBridgeSession(agent)
	}
	client, err := acp.Start(ctx, acp.SpawnOptions{
		Command: resolveLaunchCommand(agent.Command),
		Args:    agent.Args,
		Env:     agent.Env,
	}, nil)
	if err != nil {
		return nil, err
	}
	return &acpSession{client: client}, nil
}

// acpSession 包装 ACP client，实现 agentbridge.ACPSession。
type acpSession struct {
	client *acp.Client

	mu     sync.Mutex
	sessID string // session/new 返回的 ACP 会话 ID（Prompt 时建立）
}

// Prompt 建立 ACP 会话（cwd）并阻塞执行至 agent 完成，返回结果摘要
// （累积 agent_message_chunk 文本，截断 4000 字符）。流式更新与审批经 h 转发。
func (s *acpSession) Prompt(ctx context.Context, cwd, prompt string, h agentbridge.EventHandler) (string, error) {
	// handler 为 nil（不应发生：usecase 已兜底 discardEvents）时静默执行。
	// 先于 session/new 注入，避免会话就绪期间的 update 丢失。
	var collector *acpEventCollector
	if h != nil {
		collector = &acpEventCollector{h: h}
		s.client.SetHandler(collector)
	}

	sessID, err := s.client.NewSession(ctx, cwd)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.sessID = sessID
	s.mu.Unlock()

	stopReason, err := s.client.Prompt(ctx, sessID, prompt)
	if err != nil {
		return "", err
	}
	summary := ""
	if collector != nil {
		summary = collector.summary()
	}
	if stopReason == acp.StopReasonCancelled {
		return summary, context.Canceled
	}
	return summary, nil
}

// Cancel 通知 agent 取消当前 prompt（未建立会话时 no-op）。
func (s *acpSession) Cancel(context.Context) error {
	s.mu.Lock()
	sessID := s.sessID
	s.mu.Unlock()
	if sessID == "" {
		return nil
	}
	return s.client.Cancel(sessID)
}

// Close 关闭连接并终止子进程。幂等。
func (s *acpSession) Close() error {
	s.client.Close()
	return nil
}

// Kill M4（设计「活跃性监管」§D）：cancel 宽限期后强制终止整个进程组
// （Windows taskkill /T /F，Unix 负 pgid）。语义=「动作发出即成功」：
// 底层 Process.Kill 等待退出无超时保证，故异步发出并以 ctx 兜底，不允许永久阻塞调用方。
func (s *acpSession) Kill(ctx context.Context) error {
	done := make(chan struct{})
	safego.GoBackground("agentbridge.session.kill", func() {
		s.client.Proc().Kill()
		close(done)
	})
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Ping M4（设计「活跃性监管」§A L-protocol）：`_aranea/ping` 探测透传。
// 任何合法 JSON-RPC 响应（含 -32601 Method not found）即 pong。
func (s *acpSession) Ping(ctx context.Context) error {
	return s.client.Ping(ctx)
}

// ProcessPID M4 P0-2b：子进程 PID（Windows 为进程组根），任务行持久化/重启收割用。
func (s *acpSession) ProcessPID() int { return s.client.Proc().PID() }

// acpEventCollector 将 acp.SessionHandler 适配到 agentbridge.EventHandler，
// 同时累积 agent_message_chunk 文本作为结果摘要。
type acpEventCollector struct {
	h agentbridge.EventHandler

	mu   sync.Mutex
	text strings.Builder
}

// OnUpdate 实现 acp.SessionHandler（在 conn 读循环同步调用，必须快速返回）。
func (c *acpEventCollector) OnUpdate(_ context.Context, n acp.SessionNotification) {
	kind := n.Update.Kind
	text := ""
	if n.Update.Content != nil {
		text = n.Update.Content.Text
	}
	// 03 P0：tool_call 步骤需要标题（「正在修改 xxx.ts」的看板摘要来源）——
	// ACP SessionUpdate.Title 与 Content.Text 分离，这里合成「标题\n详情」，
	// 下游事件持久化拆回首行作 title（service splitFirstLine）。
	if kind == "tool_call" && n.Update.Title != "" {
		if text != "" {
			text = n.Update.Title + "\n" + text
		} else {
			text = n.Update.Title
		}
	}
	if kind == "agent_message_chunk" && text != "" {
		c.mu.Lock()
		// 上限截断：防止超长输出撑爆内存（摘要语义，非全文转录）。
		if c.text.Len() < acpSummaryLimit {
			c.text.WriteString(text)
		}
		c.mu.Unlock()
	}
	c.h.OnUpdate(kind, text)
}

// OnPermission 实现 acp.SessionHandler（独立 goroutine 调用，可阻塞）。
// M1 透传到 service 注入的 handler（默认取首选项放行，M2 起中继确认卡片）。
func (c *acpEventCollector) OnPermission(ctx context.Context, req acp.PermissionRequestParams) (acp.PermissionResult, error) {
	opts := make([]agentbridge.PermissionOption, 0, len(req.Options))
	for _, o := range req.Options {
		opts = append(opts, agentbridge.PermissionOption{
			OptionID: o.OptionID,
			Name:     o.Name,
			Kind:     o.Kind,
		})
	}
	optionID, err := c.h.OnPermission(ctx, req.ToolCall.Title, opts)
	if err != nil {
		return acp.PermissionResult{Outcome: acp.PermissionOutcome{Outcome: acp.PermissionOutcomeCancelled}}, nil
	}
	return acp.PermissionResult{Outcome: acp.PermissionOutcome{
		Outcome:  acp.PermissionOutcomeSelected,
		OptionID: optionID,
	}}, nil
}

// summary 返回累积的摘要文本（截断 4000 字符）。
func (c *acpEventCollector) summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.text.String()
	if len(s) > acpSummaryLimit {
		s = s[:acpSummaryLimit]
	}
	return strings.TrimSpace(s)
}

// --- interface guards ---

var (
	_ agentbridge.ACPSession     = (*acpSession)(nil)
	_ agentbridge.SessionFactory = acpSessionFactory{}
	_ acp.SessionHandler         = (*acpEventCollector)(nil)
)

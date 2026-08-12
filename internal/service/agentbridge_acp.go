package service

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/agentbridge/acp"
	"aranea-agents/internal/biz/agentbridge"
)

// acpSummaryLimit 结果摘要截断上限（设计 §12：入库前截断 4000 字符）。
const acpSummaryLimit = 4000

// acpSessionFactory 是 agentbridge.SessionFactory 的生产实现：
// 按 agent 注册信息 spawn ACP 子进程并完成 initialize 握手。
type acpSessionFactory struct{}

// NewACPSessionFactory 构造 ACP 会话工厂（Wire 装配用）。
func NewACPSessionFactory() agentbridge.SessionFactory {
	return acpSessionFactory{}
}

// Spawn 启动子进程 + initialize 握手。会话（session/new）延迟到
// Prompt 时建立——cwd 在派发解析项目后才确定。
func (acpSessionFactory) Spawn(ctx context.Context, agent *agentbridge.CodingAgent) (agentbridge.ACPSession, error) {
	client, err := acp.Start(ctx, acp.SpawnOptions{
		Command: agent.Command,
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
	_ agentbridge.ACPSession      = (*acpSession)(nil)
	_ agentbridge.SessionFactory  = acpSessionFactory{}
	_ acp.SessionHandler          = (*acpEventCollector)(nil)
)

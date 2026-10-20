package acpshim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"aranea-agents/internal/agentbridge/acp"
	"aranea-agents/pkg/loggateway"
)

// permission.go — 工具调用 → session/request_permission 经纪人（设计 §D-2/§E）。
//
//	权限诚实（AC-10）：title = 工具名 + 逐字 JSON 参数，禁模型散文；
//	option kinds 提供 allow_once/allow_always/reject_once/reject_always 全集；
//	阻塞等宿主 outcome；超时/失联/拒绝 → error（fail-closed，loop 中止）。

// Permission optionID 约定（宿主 always 缓存语义键）。
const (
	OptionAllowOnce    = "allow_once"
	OptionAllowAlways  = "allow_always"
	OptionRejectOnce   = "reject_once"
	OptionRejectAlways = "reject_always"
)

// permissionOptions 四 kinds 全集（AC-10 契约断言点）。
var permissionOptions = []acp.PermissionOption{
	{OptionID: OptionAllowOnce, Name: "允许一次", Kind: OptionAllowOnce},
	{OptionID: OptionAllowAlways, Name: "总是允许", Kind: OptionAllowAlways},
	{OptionID: OptionRejectOnce, Name: "拒绝一次", Kind: OptionRejectOnce},
	{OptionID: OptionRejectAlways, Name: "总是拒绝", Kind: OptionRejectAlways},
}

// ErrApprovalRejected 宿主否决（含 reject_once / reject_always / outcome=cancelled）。
var ErrApprovalRejected = errors.New("acpshim: approval rejected by host")

// Broker 合成 session/request_permission 并阻塞等待结论。
// allow_always / reject_always 结论做任务域内存缓存（Q1 终值：
// 进程生命周期 = 任务域，不落库；同时留风险记录——由宿主侧审批审计承载）。
type Broker struct {
	lg loggateway.Logger

	connMu sync.RWMutex
	conn   *Conn // Serve 时迟绑定（BindConn）

	timeout time.Duration // 宿主审批超时（默认 5min；语义=等人不是等 agent）
	guard   *PermissionGuard

	mu           sync.Mutex
	alwaysAllow  map[string]bool
	alwaysReject map[string]bool
}

// NewBroker 构造经纪人。timeout<=0 取 5min；guard 为 nil 时不限速（测试便捷，
// 生产装配 cmd/acp-shim-* 必须显式传）。
func NewBroker(lg loggateway.Logger, timeout time.Duration, guard *PermissionGuard) *Broker {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Broker{
		lg:           lg.With(loggateway.Str("component", "acpshim.broker")),
		timeout:      timeout,
		guard:        guard,
		alwaysAllow:  make(map[string]bool),
		alwaysReject: make(map[string]bool),
	}
}

// BindConn 迟绑定连接（Server.Serve 装配；Server 先于 Conn 构造）。
func (b *Broker) BindConn(c *Conn) {
	b.connMu.Lock()
	b.conn = c
	b.connMu.Unlock()
}

// AlwaysAllowed 命中 allow_always 任务域缓存（Q1：任务域内不再问宿主）。
func (b *Broker) AlwaysAllowed(tool string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.alwaysAllow[tool]
}

// Request 合成一次权限请求并阻塞等待宿主结论。
// 允许 → nil；拒绝/取消/超时/失联/速率失控 → error。
func (b *Broker) Request(ctx context.Context, sessionID string, call ToolCallRequest) error {
	b.mu.Lock()
	if b.alwaysReject[call.Name] {
		b.mu.Unlock()
		return fmt.Errorf("%w (reject_always cached)", ErrApprovalRejected)
	}
	b.mu.Unlock()

	if b.guard != nil {
		if err := b.guard.Enter(); err != nil {
			return err // 速率/挂起越限：fail-closed
		}
		defer b.guard.Leave()
	}

	b.connMu.RLock()
	conn := b.conn
	b.connMu.RUnlock()
	if conn == nil {
		return errors.New("acpshim: broker not bound to a connection")
	}

	rawInput := json.RawMessage(call.Arguments)
	if len(rawInput) == 0 {
		rawInput = json.RawMessage("{}")
	}
	params := acp.PermissionRequestParams{
		SessionID: sessionID,
		ToolCall: acp.ToolCallInfo{
			ToolCallID: call.ID,
			// AC-10：标题 = 工具名 + 逐字 JSON 参数，无模型散文。
			Title:    call.Name + " " + string(rawInput),
			Kind:     kindForTool(call.Name),
			RawInput: rawInput,
		},
		Options: permissionOptions,
	}

	reqCtx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	b.lg.Info("acpshim: request permission",
		loggateway.SessionID(sessionID), loggateway.Str("tool", call.Name))

	res, err := conn.Call(reqCtx, acp.MethodRequestPermission, params)
	if err != nil {
		// 超时（reqCtx）/宿主失联（conn 关闭）/宿主侧取消（错误帧）——一律 fail-closed。
		return fmt.Errorf("acpshim: permission call: %w", err)
	}
	var pr acp.PermissionResult
	if err := json.Unmarshal(res, &pr); err != nil {
		return fmt.Errorf("acpshim: permission result decode: %w", err)
	}

	switch {
	case pr.Outcome.Outcome == acp.PermissionOutcomeSelected && pr.Outcome.OptionID == OptionAllowOnce:
		return nil
	case pr.Outcome.Outcome == acp.PermissionOutcomeSelected && pr.Outcome.OptionID == OptionAllowAlways:
		b.mu.Lock()
		b.alwaysAllow[call.Name] = true
		b.mu.Unlock()
		return nil
	case pr.Outcome.Outcome == acp.PermissionOutcomeSelected && pr.Outcome.OptionID == OptionRejectAlways:
		b.mu.Lock()
		b.alwaysReject[call.Name] = true
		b.mu.Unlock()
		return fmt.Errorf("%w (reject_always)", ErrApprovalRejected)
	default:
		// reject_once / outcome=cancelled / 未知组合：一律拒绝。
		return fmt.Errorf("%w (outcome=%s option=%s)", ErrApprovalRejected, pr.Outcome.Outcome, pr.Outcome.OptionID)
	}
}

// kindForTool 映射 ACP kind（read/edit/execute）供宿主 UI 分级渲染。
func kindForTool(name string) string {
	switch name {
	case "fs_read", "fs_list":
		return "read"
	case "fs_write":
		return "edit"
	default:
		return "execute"
	}
}

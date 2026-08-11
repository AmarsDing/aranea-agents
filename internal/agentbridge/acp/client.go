package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// SessionHandler 接收 agent 的流式事件与审批请求（业务层实现）。
// OnUpdate 必须快速返回（在 conn 读循环中同步调用）；
// OnPermission 可阻塞（独立 goroutine 中调用），但应有超时。
type SessionHandler interface {
	OnUpdate(ctx context.Context, n SessionNotification)
	OnPermission(ctx context.Context, req PermissionRequestParams) (PermissionResult, error)
}

// Client 是一个 ACP 客户端，绑定一个 agent 子进程。
// 生命周期：Start → NewSession → Prompt/Cancel → Close。
type Client struct {
	proc *Process
	conn *Conn
	caps AgentCapabilities
}

// Start 启动子进程并完成 initialize 握手（校验协议版本）。
func Start(ctx context.Context, opt SpawnOptions, h SessionHandler) (*Client, error) {
	proc, err := Spawn(ctx, opt)
	if err != nil {
		return nil, err
	}
	c := &Client{proc: proc}
	c.conn = NewConn(proc.Stdout(), proc.Stdin(),
		func(ctx context.Context, req PermissionRequestParams) (PermissionResult, error) {
			if h == nil {
				return PermissionResult{Outcome: PermissionOutcome{Outcome: PermissionOutcomeCancelled}}, nil
			}
			return h.OnPermission(ctx, req)
		},
		func(ctx context.Context, n SessionNotification) {
			if h != nil {
				h.OnUpdate(ctx, n)
			}
		},
	)

	res, err := c.conn.Call(ctx, MethodInitialize, InitializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientCapabilities: ClientCapabilities{
			FS:       FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
	})
	if err != nil {
		proc.Kill()
		return nil, fmt.Errorf("acp: initialize: %w", err)
	}
	var ir InitializeResult
	if err := json.Unmarshal(res, &ir); err != nil {
		proc.Kill()
		return nil, fmt.Errorf("acp: initialize result: %w", err)
	}
	if ir.ProtocolVersion != ProtocolVersion {
		proc.Kill()
		return nil, fmt.Errorf("acp: unsupported protocol version %d (client=%d)", ir.ProtocolVersion, ProtocolVersion)
	}
	c.caps = ir.AgentCapabilities
	return c, nil
}

// Proc 暴露底层子进程（监控/恢复用）。
func (c *Client) Proc() *Process { return c.proc }

// NewSession 建立 ACP 会话，cwd 为 agent 工作目录。
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	res, err := c.conn.Call(ctx, MethodSessionNew, NewSessionParams{CWD: cwd, MCPServers: []any{}})
	if err != nil {
		return "", fmt.Errorf("acp: session/new: %w", err)
	}
	var r NewSessionResult
	if err := json.Unmarshal(res, &r); err != nil {
		return "", fmt.Errorf("acp: session/new result: %w", err)
	}
	if r.SessionID == "" {
		return "", errors.New("acp: empty sessionId")
	}
	return r.SessionID, nil
}

// Prompt 发送任务并阻塞至 agent 完成，返回 stopReason。
// 流式更新与审批经 handler 回调。
func (c *Client) Prompt(ctx context.Context, sessionID, text string) (string, error) {
	res, err := c.conn.Call(ctx, MethodSessionPrompt, PromptParams{
		SessionID: sessionID,
		Prompt:    []ContentBlock{{Type: "text", Text: text}},
	})
	if err != nil {
		return "", fmt.Errorf("acp: session/prompt: %w", err)
	}
	var r PromptResult
	if err := json.Unmarshal(res, &r); err != nil {
		return "", fmt.Errorf("acp: session/prompt result: %w", err)
	}
	return r.StopReason, nil
}

// Cancel 通知 agent 取消当前 prompt。
func (c *Client) Cancel(sessionID string) error {
	return c.conn.Notify(MethodSessionCancel, CancelNotificationParams{SessionID: sessionID})
}

// Close 关闭连接并终止子进程。幂等。
func (c *Client) Close() {
	_ = c.conn.Close()
	c.proc.Kill()
}

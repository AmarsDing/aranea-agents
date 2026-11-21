package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
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
		tail := proc.StderrTail()
		proc.Kill()
		return nil, fmt.Errorf("acp: initialize: %w%s", err, stderrHint(tail))
	}
	var ir InitializeResult
	if err := json.Unmarshal(res, &ir); err != nil {
		tail := proc.StderrTail()
		proc.Kill()
		return nil, fmt.Errorf("acp: initialize result: %w%s", err, stderrHint(tail))
	}
	if ir.ProtocolVersion != ProtocolVersion {
		tail := proc.StderrTail()
		proc.Kill()
		return nil, fmt.Errorf("acp: unsupported protocol version %d (client=%d)%s", ir.ProtocolVersion, ProtocolVersion, stderrHint(tail))
	}
	c.caps = ir.AgentCapabilities
	return c, nil
}

// stderrHint 把子进程 stderr 尾部附加到握手失败错误（此前仅探针路径可见，
// 派发路径失败时 LLM/用户拿不到任何诊断）。取尾部 2048 rune 防超长输出撑爆消息。
//
// 输出保证是合法 UTF-8（评审 2026-11-09 D 项）：error 文本最终写入 PG TEXT 列，
// 非法字节序列会被拒收（SQLSTATE 22021）。Windows 下子进程 stderr 常是 GBK，
// 先归一编码再按 rune 边界截断——此前的裸字节截断既可能在多字节序列中间切刀，
// 也可能把整段非法字节带进 error。
func stderrHint(tail string) string {
	tail = stderrToUTF8(tail)
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return ""
	}
	rs := []rune(tail)
	const maxRunes = 2048
	if len(rs) > maxRunes {
		tail = "…" + string(rs[len(rs)-maxRunes:])
	}
	return "（stderr 尾部: " + tail + "）"
}

// stderrToUTF8 把子进程 stderr 尾部归一为合法 UTF-8。合法输入原样返回；
// 非法输入按 GB18030（GBK 超集，对应 Windows zh-CN 控制台代码页 CP936）整段
// 试探解码（非法/截断字节会被解码器替换为 U+FFFD），与逐字节替换兜底串比较
// 替换符数量取更优者——真实 GBK 文本整段还原为零替换，纯乱码则退回
// ToValidUTF8 保住散落的 ASCII 片段。
func stderrToUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	decoded, err := simplifiedchinese.GB18030.NewDecoder().String(s)
	fallback := strings.ToValidUTF8(s, "�")
	if err == nil || strings.Count(decoded, "�") <= strings.Count(fallback, "�") {
		return decoded
	}
	return fallback
}

// Proc 暴露底层子进程（监控/恢复用）。
func (c *Client) Proc() *Process { return c.proc }

// SetHandler 运行期注入会话级 handler（Start 时 cwd 未定，handler 未知）。
// 必须在 Prompt 前调用；nil 恢复为丢弃入站消息。
func (c *Client) SetHandler(h SessionHandler) {
	if h == nil {
		c.conn.SetHandlers(nil, nil)
		return
	}
	c.conn.SetHandlers(
		func(ctx context.Context, req PermissionRequestParams) (PermissionResult, error) {
			return h.OnPermission(ctx, req)
		},
		func(ctx context.Context, n SessionNotification) {
			h.OnUpdate(ctx, n)
		},
	)
}

// NewSession 建立 ACP 会话，cwd 为 agent 工作目录。
func (c *Client) NewSession(ctx context.Context, cwd string) (string, error) {
	res, err := c.conn.Call(ctx, MethodSessionNew, NewSessionParams{CWD: cwd, MCPServers: []any{}})
	if err != nil {
		// 适配器在建会话时退出最常见（缺登录、配置错）：带上 stderr 尾部，否则只看得到「管道已关闭」
		return "", fmt.Errorf("acp: session/new: %w%s", err, stderrHint(c.proc.StderrTail()))
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

// Ping M4 活跃性监管 `_aranea/ping` 探测（P3-3）：向 agent 发私有 JSON-RPC
// 方法。**任何合法 JSON-RPC 响应（含 Method not found 错误）都证明对端事件
// 循环存活**——原生 adapter 不认识该方法会回 -32601，同样计为 pong。
// 仅传输层失败（EOF/写失败/ctx 超时）返回错误。
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.conn.Call(ctx, MethodAraneaPing, struct{}{})
	if err == nil {
		return nil
	}
	var rerr *rpcError
	if errors.As(err, &rerr) {
		return nil // 协议层活答（方法不存在等）= 存活
	}
	return err
}

// StderrTail 返回子进程 stderr 尾部（≤32KiB，S3 诊断材料）。
func (c *Client) StderrTail() string { return c.proc.StderrTail() }

// Close 关闭连接并终止子进程。幂等。
func (c *Client) Close() {
	_ = c.conn.Close()
	c.proc.Kill()
}

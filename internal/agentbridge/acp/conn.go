package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"aranea-agents/pkg/safego"
)

// RequestHandler 处理 agent 发来的 session/request_permission，需返回审批结论。
type RequestHandler func(ctx context.Context, req PermissionRequestParams) (PermissionResult, error)

// NotifyHandler 处理 agent 发来的 session/update 通知。
type NotifyHandler func(ctx context.Context, n SessionNotification)

// ErrClosed 表示连接已关闭。
var ErrClosed = errors.New("acp: connection closed")

// rpcError 是 JSON-RPC 错误对象。
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("acp rpc error %d: %s", e.Code, e.Message) }

// frame 是线上通用帧（请求/响应/通知共用结构解析）。
type frame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// pendingCall 挂起一次出站调用。
type pendingCall struct {
	ch chan callResult
}

type callResult struct {
	result json.RawMessage
	err    error
}

// Conn 是一条 ACP NDJSON 连接。读循环单 goroutine，写互斥保护。
// 用法：NewConn → Call/Notify → Close。
type Conn struct {
	r io.Reader
	w io.Writer

	writeMu sync.Mutex

	nextID  atomic.Int64
	pendMu  sync.Mutex
	pending map[string]*pendingCall

	onReq    RequestHandler
	onNotify NotifyHandler

	done     chan struct{}
	closeOnce sync.Once
	readErr  error // 读循环终结错误（EOF 等），用于 pending 失败信息

	// handlerMu 保护 onReq/onNotify 的运行期切换（SetHandler）：
	// 会话级 handler 在 Start 后、Prompt 前由 Client.SetHandler 注入。
	handlerMu sync.RWMutex
}

// NewConn 创建连接并启动读循环。onReq/onNotify 可为 nil（丢弃对应入站消息）。
func NewConn(r io.Reader, w io.Writer, onReq RequestHandler, onNotify NotifyHandler) *Conn {
	c := &Conn{
		r:       r,
		w:       w,
		pending: make(map[string]*pendingCall),
		onReq:   onReq,
		onNotify: onNotify,
		done:    make(chan struct{}),
	}
	// 75 review Y2：读循环经 safego 托管，panic 可恢复并上报 hook
	safego.GoBackground("acp.conn.readLoop", c.readLoop)
	return c
}

// SetHandlers 运行期替换入站 handler（nil 表示丢弃对应入站消息）。
// 会话级 handler 在 Start 时未知（cwd 未确定），由 Client.SetHandler 在
// Prompt 前注入；读循环经 handlerMu 读取，保证切换安全。
func (c *Conn) SetHandlers(onReq RequestHandler, onNotify NotifyHandler) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onReq = onReq
	c.onNotify = onNotify
}

func (c *Conn) getHandlers() (RequestHandler, NotifyHandler) {
	c.handlerMu.RLock()
	defer c.handlerMu.RUnlock()
	return c.onReq, c.onNotify
}

// Call 发起一次 JSON-RPC 调用并等待响应。响应 result 原样返回。
func (c *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1) - 1 // 首个 id 为 0
	idBytes, _ := json.Marshal(id)

	pc := &pendingCall{ch: make(chan callResult, 1)}
	key := string(idBytes)
	c.pendMu.Lock()
	select {
	case <-c.done:
		c.pendMu.Unlock()
		return nil, ErrClosed
	default:
	}
	c.pending[key] = pc
	c.pendMu.Unlock()
	defer func() {
		c.pendMu.Lock()
		delete(c.pending, key)
		c.pendMu.Unlock()
	}()

	if err := c.writeFrame(frame{JSONRPC: "2.0", ID: idBytes, Method: method, Params: mustMarshal(params)}); err != nil {
		return nil, err
	}

	select {
	case res := <-pc.ch:
		return res.result, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		if c.readErr != nil {
			return nil, c.readErr
		}
		return nil, ErrClosed
	}
}

// Notify 发送一条无 id 通知。
func (c *Conn) Notify(method string, params any) error {
	return c.writeFrame(frame{JSONRPC: "2.0", Method: method, Params: mustMarshal(params)})
}

// Close 关闭连接，所有挂起调用失败返回。幂等。
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		c.pendMu.Lock()
		for k, pc := range c.pending {
			pc.ch <- callResult{err: ErrClosed}
			delete(c.pending, k)
		}
		c.pendMu.Unlock()
	})
	return nil
}

func (c *Conn) writeFrame(f frame) error {
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("acp: marshal frame: %w", err)
	}
	data = append(data, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.done:
		return ErrClosed
	default:
	}
	if _, err := c.w.Write(data); err != nil {
		return fmt.Errorf("acp: write frame: %w", err)
	}
	return nil
}

func (c *Conn) readLoop() {
	br := bufio.NewReaderSize(c.r, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			c.dispatch(bytes_TrimSpace(line))
		}
		if err != nil {
			if err != io.EOF {
				c.readErr = fmt.Errorf("acp: read: %w", err)
			}
			c.Close()
			return
		}
	}
}

// dispatch 按帧形态路由：响应 → pending；入站请求 → onReq 并回写响应；通知 → onNotify。
func (c *Conn) dispatch(line []byte) {
	if len(line) == 0 {
		return
	}
	var f frame
	if err := json.Unmarshal(line, &f); err != nil {
		return // 畸形行跳过，不断流
	}

	_, onNotify := c.getHandlers()
	onReq, _ := c.getHandlers()
	switch {
	case f.Method == MethodSessionUpdate && len(f.ID) == 0:
		if onNotify == nil {
			return
		}
		var n SessionNotification
		if err := json.Unmarshal(f.Params, &n); err != nil {
			return
		}
		onNotify(context.Background(), n)

	case f.Method == MethodRequestPermission && len(f.ID) > 0:
		var req PermissionRequestParams
		if err := json.Unmarshal(f.Params, &req); err != nil {
			c.respondError(f.ID, -32602, "invalid params")
			return
		}
		if onReq == nil {
			c.respondError(f.ID, -32601, "permission not supported")
			return
		}
		// 审批可能阻塞数分钟（等用户操作），必须异步，否则读循环停摆、
		// 后续 session/update 全部积压在管道里。
		id := f.ID
		safego.GoBackground("acp.conn.permission", func() {
			result, err := onReq(context.Background(), req)
			if err != nil {
				c.respondError(id, -32603, err.Error())
				return
			}
			_ = c.writeFrame(frame{JSONRPC: "2.0", ID: id, Result: mustMarshal(result)})
		})

	case f.Method == "" && len(f.ID) > 0:
		key := normalizeID(f.ID)
		c.pendMu.Lock()
		pc, ok := c.pending[key]
		c.pendMu.Unlock()
		if !ok {
			return // 迟到/未知响应
		}
		if f.Error != nil {
			pc.ch <- callResult{err: f.Error}
			return
		}
		pc.ch <- callResult{result: f.Result}
	}
}

func (c *Conn) respondError(id json.RawMessage, code int, msg string) {
	_ = c.writeFrame(frame{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

// normalizeID 统一 id 键形（数字/字符串兼容：JSON 数字原样，字符串去引号比较）。
func normalizeID(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		return s // 字符串 id 原样（带引号）作键，写入时亦原样回显
	}
	return s
}

func mustMarshal(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

func bytes_TrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

// Package acpshim 实现 ACP（Agent Client Protocol）服务端子集：
// 以 JSON-RPC 2.0 over stdio NDJSON 回应宿主（Client）调用，
// 并主动发出 session/update 通知与 session/request_permission 请求。
// 与本仓库 acp 包（客户端）对偶，协议版本 1。
// M4 设计 §C：本包为 acp-shim-deepseek / acp-shim-grok 的共用框架。
package acpshim

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

// ErrClosed 表示连接已关闭。
var ErrClosed = errors.New("acpshim: connection closed")

// 常用 JSON-RPC 2.0 协议错误码。
const (
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
)

// RPCError 让 handler 返回带 code 的协议错误；conn 按 code/message 回写错误帧。
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("acpshim rpc error %d: %s", e.Code, e.Message) }

// RequestRouter 分发入站请求与通知，由 server.go 实现。
type RequestRouter interface {
	// HandleRequest 处理入站请求（带 id）。返回 result（可为 nil → 回写 "result":null）。
	// 返回 *RPCError 按其 code/message 回写；其余错误按 -32603 回写。
	// 在独立 goroutine 中调用，允许长事务阻塞（session/prompt 全生命周期）。
	HandleRequest(ctx context.Context, method string, params json.RawMessage) (any, error)
	// HandleNotification 处理入站通知（无 id）。在读循环内同步调用，必须快速返回。
	HandleNotification(ctx context.Context, method string, params json.RawMessage)
}

// frame 是线上通用帧（请求/响应/通知共用结构解析），与 acp 包同形。
type frame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// pendingCall 挂起一次出站调用（session/request_permission）。
type pendingCall struct {
	ch chan callResult
}

type callResult struct {
	result json.RawMessage
	err    error
}

// Conn 是一条 ACP 服务端 NDJSON 连接。读循环单 goroutine，写互斥保护。
// 用法：NewConn（读循环自动启动）→ Call/Notify → Close。
type Conn struct {
	r io.Reader
	w io.Writer

	writeMu sync.Mutex

	nextID  atomic.Int64
	pendMu  sync.Mutex
	pending map[string]*pendingCall

	router RequestRouter

	done      chan struct{}
	closeOnce sync.Once
	readErr   error // 读循环终结错误（EOF 以外），用于 pending 失败信息
}

// NewConn 创建连接并启动读循环。router 不得为 nil。
func NewConn(r io.Reader, w io.Writer, router RequestRouter) *Conn {
	c := &Conn{
		r:       r,
		w:       w,
		pending: make(map[string]*pendingCall),
		router:  router,
		done:    make(chan struct{}),
	}
	// 与 acp 包一致：读循环经 safego 托管，panic 可恢复并上报 hook
	safego.GoBackground("acpshim.conn.readLoop", c.readLoop)
	return c
}

// Done 在连接关闭（对端 EOF / 读错误 / 显式 Close）时关闭。
// 长事务 handler（session/prompt）必须监听并尽快中止在途工作（宿主失联即停）。
func (c *Conn) Done() <-chan struct{} { return c.done }

// Call 发起一次出站调用并等待宿主响应（session/request_permission 的 outcome）。
// 响应 result 原样返回；宿主回错误帧时以 *RPCError 返回。
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

// Notify 发送一条无 id 通知（session/update / 显式心跳）。
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
		return fmt.Errorf("acpshim: marshal frame: %w", err)
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
		return fmt.Errorf("acpshim: write frame: %w", err)
	}
	return nil
}

func (c *Conn) readLoop() {
	br := bufio.NewReaderSize(c.r, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			c.dispatch(bytesTrimSpace(line))
		}
		if err != nil {
			if err != io.EOF {
				c.readErr = fmt.Errorf("acpshim: read: %w", err)
			}
			c.Close()
			return
		}
	}
}

// dispatch 按帧形态路由：入站请求 → router（独立 goroutine + 回写响应）；
// 入站通知 → router；响应 → pending。畸形行跳过，不断流。
func (c *Conn) dispatch(line []byte) {
	if len(line) == 0 {
		return
	}
	var f frame
	if err := json.Unmarshal(line, &f); err != nil {
		return // 畸形行跳过，不断流（与 acp 客户端行为对偶）
	}

	switch {
	case f.Method != "" && len(f.ID) > 0:
		// 入站请求：独立 goroutine——session/prompt 是全生命周期长事务，
		// 期间读循环必须继续吃 session/cancel 等通知。
		id := f.ID
		method := f.Method
		params := f.Params
		safego.GoBackground("acpshim.conn.request."+method, func() {
			result, err := c.router.HandleRequest(context.Background(), method, params)
			if err != nil {
				var re *RPCError
				if errors.As(err, &re) {
					c.respondError(id, re.Code, re.Message)
					return
				}
				c.respondError(id, CodeInternal, err.Error())
				return
			}
			res := mustMarshal(result)
			if res == nil {
				res = json.RawMessage("null")
			}
			_ = c.writeFrame(frame{JSONRPC: "2.0", ID: id, Result: res})
		})

	case f.Method != "":
		c.router.HandleNotification(context.Background(), f.Method, f.Params)

	case len(f.ID) > 0:
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
	_ = c.writeFrame(frame{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: msg}})
}

// normalizeID 统一 id 键形（数字/字符串兼容：JSON 数字原样，字符串带引号原样）。
func normalizeID(raw json.RawMessage) string {
	return strings.TrimSpace(string(raw))
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

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

// client.go CDP（Computer-use Device Protocol）stdio JSON-RPC 2.0 低层客户端。
// 职责：帧编解码、请求/响应按 id 多路复用、超时与错误码映射、关闭语义。
// 对 biz 层不可见，仅经 Gateway 暴露。
package computeruse

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
	"time"

	"aranea-agents/pkg/loggateway"
)

// CDP 错误码（设计文档 §2.4）映射的 sentinel error，供上层 errors.Is 判定。
var (
	// ErrElementNotFound 元素未找到 / ref 过期（-32001）。
	ErrElementNotFound = errors.New("computeruse: 元素未找到或 ref 已过期")
	// ErrNotInteractable 目标窗口失焦 / 元素不可交互（-32002）。
	ErrNotInteractable = errors.New("computeruse: 目标失焦或元素不可交互")
	// ErrInjectionDenied OS 级注入被拒绝（-32003）。
	ErrInjectionDenied = errors.New("computeruse: 输入注入被系统拒绝")
	// ErrInternal sidecar 内部错误（-32004）。
	ErrInternal = errors.New("computeruse: sidecar 内部错误")
	// ErrClosed client 已关闭（Close 后或底层流 EOF）。
	ErrClosed = errors.New("computeruse: sidecar 连接已关闭")
)

// CDP 错误码常量。
const (
	codeElementNotFound = -32001
	codeNotInteractable = -32002
	codeInjectionDenied = -32003
	codeInternal        = -32004
)

// 默认超时：普通方法 10s，动作类（action.*）30s。
const (
	defaultCallTimeout  = 10 * time.Second
	defaultActionTimout = 30 * time.Second
)

// rpcRequest / rpcResponse / rpcError 为 CDP 线格式（§2.1）。
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client 对 sidecar 子进程 stdio 的 JSON-RPC 客户端。
// 每行一个 JSON 帧；写由 writeMu 串行化，读由独立 goroutine 分发。
type Client struct {
	w  io.Writer
	lg loggateway.Logger

	nextID atomic.Int64

	writeMu sync.Mutex

	mu      sync.Mutex
	pending map[int64]chan rpcResponse
	closed  bool
	closeCh chan struct{}

	closeOnce sync.Once
	wg        sync.WaitGroup
}

// NewClient 创建客户端并启动 stdout 读循环。w 为 sidecar stdin，r 为 sidecar stdout。
func NewClient(w io.Writer, r io.Reader, lg loggateway.Logger) *Client {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	c := &Client{
		w:       w,
		lg:      lg.With(loggateway.Domain("computeruse")),
		pending: make(map[int64]chan rpcResponse),
		closeCh: make(chan struct{}),
	}
	c.wg.Add(1)
	go c.readLoop(r)
	return c
}

// Call 发起一次 RPC 并等待响应。默认超时 10s；action.* 方法 30s。
// 调用方 ctx 的更早截止/取消优先生效。
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	timeout := defaultCallTimeout
	if strings.HasPrefix(method, "action.") {
		timeout = defaultActionTimout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var rawParams json.RawMessage
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("computeruse: 参数序列化失败: %w", err)
		}
		rawParams = raw
	}

	id := c.nextID.Add(1)
	ch := make(chan rpcResponse, 1)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	c.pending[id] = ch
	c.mu.Unlock()

	frame, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: rawParams})
	if err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("computeruse: 帧序列化失败: %w", err)
	}

	c.writeMu.Lock()
	_, werr := c.w.Write(append(frame, '\n'))
	c.writeMu.Unlock()
	if werr != nil {
		c.removePending(id)
		return nil, fmt.Errorf("computeruse: 写入 sidecar 失败: %w", werr)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, mapRPCError(resp.Error)
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.removePending(id)
		return nil, ctx.Err()
	}
}

// Close 幂等关闭：所有 in-flight 请求收到 ErrClosed，读循环退出。
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.failAll(ErrClosed)
		if wc, ok := c.w.(io.Closer); ok {
			wc.Close()
		}
	})
	return nil
}

// readLoop stdout 读循环：逐行解析响应帧并分发；EOF/解析错误时 fail 所有 pending 并关闭。
func (c *Client) readLoop(r io.Reader) {
	defer c.wg.Done()
	defer func() {
		if rec := recover(); rec != nil {
			c.lg.Error("sidecar 读循环 panic",
				loggateway.StepID("computeruse.sidecar.readloop"),
				loggateway.Any("panic", rec))
			c.failAll(ErrClosed)
		}
	}()

	scanner := bufio.NewScanner(r)
	// 截图 base64 内联，帧可能很大（§2.1），放宽到 64MB
	scanner.Buffer(make([]byte, 0, 256*1024), 64*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			c.lg.Error("sidecar 响应帧解析失败，关闭连接",
				loggateway.StepID("computeruse.sidecar.readloop"),
				loggateway.Err(err))
			c.failAll(ErrClosed)
			return
		}
		c.dispatch(resp)
	}
	// EOF 或读错误：sidecar 已退出/流已断
	if err := scanner.Err(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		c.lg.Warn("sidecar stdout 读取出错",
			loggateway.StepID("computeruse.sidecar.readloop"),
			loggateway.Err(err))
	}
	c.failAll(ErrClosed)
}

// dispatch 按 id 把响应投递给等待中的调用。
func (c *Client) dispatch(resp rpcResponse) {
	c.mu.Lock()
	ch, ok := c.pending[resp.ID]
	if ok {
		delete(c.pending, resp.ID)
	}
	c.mu.Unlock()
	if ok {
		ch <- resp
	}
}

// removePending 移除等待项（超时/写失败路径）。
func (c *Client) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// failAll 标记关闭并让所有 pending 调用返回 err。
func (c *Client) failAll(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.closeCh)
	for id, ch := range c.pending {
		// 复用 error 帧通道语义：code=0 表示本地连接关闭错误
		ch <- rpcResponse{ID: id, Error: &rpcError{Code: 0, Message: err.Error()}}
		delete(c.pending, id)
	}
}

// mapRPCError 把 CDP 错误码映射为 sentinel error。
func mapRPCError(e *rpcError) error {
	if e.Code == 0 {
		// failAll 注入的连接关闭错误
		if e.Message == ErrClosed.Error() {
			return ErrClosed
		}
		return errors.New(e.Message)
	}
	var sentinel error
	switch e.Code {
	case codeElementNotFound:
		sentinel = ErrElementNotFound
	case codeNotInteractable:
		sentinel = ErrNotInteractable
	case codeInjectionDenied:
		sentinel = ErrInjectionDenied
	case codeInternal:
		sentinel = ErrInternal
	default:
		return fmt.Errorf("computeruse: sidecar 错误 %d: %s", e.Code, e.Message)
	}
	return fmt.Errorf("%w: %s", sentinel, e.Message)
}

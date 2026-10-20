package acpshim

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRouter 记录入站方法并按需编程响应。
type fakeRouter struct {
	mu            sync.Mutex
	requests      []string
	notifications []string
	handleRequest func(ctx context.Context, method string, params json.RawMessage) (any, error)
	onNotify      func(method string, params json.RawMessage)
}

func (f *fakeRouter) HandleRequest(ctx context.Context, method string, params json.RawMessage) (any, error) {
	f.mu.Lock()
	f.requests = append(f.requests, method)
	h := f.handleRequest
	f.mu.Unlock()
	if h != nil {
		return h(ctx, method, params)
	}
	return map[string]any{"ok": true}, nil
}

func (f *fakeRouter) HandleNotification(_ context.Context, method string, params json.RawMessage) {
	f.mu.Lock()
	f.notifications = append(f.notifications, method)
	cb := f.onNotify
	f.mu.Unlock()
	if cb != nil {
		cb(method, params)
	}
}

func (f *fakeRouter) gotRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *fakeRouter) gotNotifications() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.notifications...)
}

// testPipe 搭一对双向管道并返回 conn 与宿主侧端点。
func testPipe(t *testing.T, router RequestRouter) (*Conn, io.WriteCloser, *bufio.Reader) {
	t.Helper()
	shimR, hostW := io.Pipe() // shim 读 ← 宿主写
	hostR, shimW := io.Pipe() // 宿主读 ← shim 写
	c := NewConn(shimR, shimW, router)
	return c, hostW, bufio.NewReaderSize(hostR, 64*1024)
}

// readFrame 从宿主侧读取一帧（5s 超时）。
func readFrame(t *testing.T, br *bufio.Reader) frame {
	t.Helper()
	type res struct {
		line []byte
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		l, err := br.ReadBytes('\n')
		ch <- res{l, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("read frame: %v", r.err)
		}
		var f frame
		if err := json.Unmarshal(bytesTrimSpace(r.line), &f); err != nil {
			t.Fatalf("unmarshal frame %q: %v", string(r.line), err)
		}
		return f
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for frame")
		return frame{}
	}
}

// writeLine 宿主侧写一行原始文本。
func writeLine(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Fatalf("write line: %v", err)
	}
}

// waitFor 轮询条件直至满足或超时（读循环异步分发用）。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// TestConn_CallResolvesViaPendingTable：出站调用入挂起表，宿主响应按 id 路由回来。
func TestConn_CallResolvesViaPendingTable(t *testing.T) {
	c, hostW, hostR := testPipe(t, &fakeRouter{})
	defer c.Close()

	type callRes struct {
		result json.RawMessage
		err    error
	}
	resCh := make(chan callRes, 1)
	go func() {
		r, err := c.Call(context.Background(), "session/request_permission", map[string]any{"sessionId": "s1"})
		resCh <- callRes{r, err}
	}()

	// 宿主读到请求帧：方法对、id=0（首发）。
	req := readFrame(t, hostR)
	if req.Method != "session/request_permission" {
		t.Fatalf("method = %q, want session/request_permission", req.Method)
	}
	if string(req.ID) != "0" {
		t.Fatalf("id = %s, want 0", string(req.ID))
	}

	// 宿主回响应 → Call 解除。
	writeLine(t, hostW, `{"jsonrpc":"2.0","id":0,"result":{"outcome":{"outcome":"selected","optionId":"allow_once"}}}`)
	select {
	case r := <-resCh:
		if r.err != nil {
			t.Fatalf("Call error: %v", r.err)
		}
		var got struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		}
		if err := json.Unmarshal(r.result, &got); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if got.Outcome.OptionID != "allow_once" {
			t.Fatalf("optionId = %q, want allow_once", got.Outcome.OptionID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Call did not resolve")
	}
}

// TestConn_CallErrorFrame：宿主回错误帧 → *RPCError 带出 code。
func TestConn_CallErrorFrame(t *testing.T) {
	c, hostW, hostR := testPipe(t, &fakeRouter{})
	defer c.Close()

	resCh := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "session/request_permission", nil)
		resCh <- err
	}()
	req := readFrame(t, hostR)
	writeLine(t, hostW, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"error":{"code":-32603,"message":"host exploded"}}`)

	select {
	case err := <-resCh:
		var re *RPCError
		if !errors.As(err, &re) {
			t.Fatalf("error type = %T, want *RPCError", err)
		}
		if re.Code != -32603 || !strings.Contains(re.Message, "host exploded") {
			t.Fatalf("RPCError = %+v", re)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Call did not fail")
	}
}

// TestConn_MalformedLineTolerated：畸形行跳过不断流，后续正常帧照常分发。
func TestConn_MalformedLineTolerated(t *testing.T) {
	fr := &fakeRouter{}
	c, hostW, hostR := testPipe(t, fr)
	defer c.Close()

	writeLine(t, hostW, `this is not json`)
	writeLine(t, hostW, `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"s9"}}`)
	writeLine(t, hostW, `{"broken":`)
	writeLine(t, hostW, `{"jsonrpc":"2.0","id":41,"method":"initialize","params":{"protocolVersion":1}}`)

	// initialize 应得到正常响应（id 逐字回显）。
	resp := readFrame(t, hostR)
	if string(resp.ID) != "41" {
		t.Fatalf("response id = %s, want 41", string(resp.ID))
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error frame: %+v", resp.Error)
	}
	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil || result["ok"] != true {
		t.Fatalf("result = %s, err = %v", string(resp.Result), err)
	}

	waitFor(t, "cancel notification", func() bool {
		for _, m := range fr.gotNotifications() {
			if m == "session/cancel" {
				return true
			}
		}
		return false
	})
}

// TestConn_StringIDEchoedVerbatim：字符串 id 逐字回显（含引号键形一致）。
func TestConn_StringIDEchoedVerbatim(t *testing.T) {
	c, hostW, hostR := testPipe(t, &fakeRouter{})
	defer c.Close()

	writeLine(t, hostW, `{"jsonrpc":"2.0","id":"abc-123","method":"session/new","params":{"cwd":"F:/x","mcpServers":[]}}`)
	resp := readFrame(t, hostR)
	if string(resp.ID) != `"abc-123"` {
		t.Fatalf("response id = %s, want %q", string(resp.ID), `"abc-123"`)
	}
}

// TestConn_PendingFailsOnEOF：挂起调用在连接 EOF 后以 ErrClosed 失败。
func TestConn_PendingFailsOnEOF(t *testing.T) {
	c, hostW, hostR := testPipe(t, &fakeRouter{})

	resCh := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "session/request_permission", nil)
		resCh <- err
	}()
	_ = readFrame(t, hostR) // 确认请求已发出

	// 宿主侧关闭写端 → shim 读循环 EOF → 挂起调用失败。
	hostW.Close()
	select {
	case err := <-resCh:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("error = %v, want ErrClosed", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending Call not failed on EOF")
	}
}

// TestConn_UnknownResponseIgnored：未知 id 的响应不致乱，后续调用照常。
func TestConn_UnknownResponseIgnored(t *testing.T) {
	c, hostW, hostR := testPipe(t, &fakeRouter{})
	defer c.Close()

	writeLine(t, hostW, `{"jsonrpc":"2.0","id":999,"result":{"stray":true}}`)

	resCh := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "session/request_permission", nil)
		resCh <- err
	}()
	req := readFrame(t, hostR)
	writeLine(t, hostW, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"result":{}}`)
	select {
	case err := <-resCh:
		if err != nil {
			t.Fatalf("Call error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Call did not resolve after stray response")
	}
}

// TestConn_HandlerRPCErrorPassthrough：handler 返回 *RPCError 按 code 回写。
func TestConn_HandlerRPCErrorPassthrough(t *testing.T) {
	fr := &fakeRouter{}
	fr.handleRequest = func(_ context.Context, method string, _ json.RawMessage) (any, error) {
		if method == "session/bogus" {
			return nil, &RPCError{Code: CodeMethodNotFound, Message: "unknown method"}
		}
		return "fine", nil
	}
	c, hostW, hostR := testPipe(t, fr)
	defer c.Close()

	writeLine(t, hostW, `{"jsonrpc":"2.0","id":7,"method":"session/bogus","params":{}}`)
	resp := readFrame(t, hostR)
	if resp.Error == nil || resp.Error.Code != CodeMethodNotFound {
		t.Fatalf("error = %+v, want code %d", resp.Error, CodeMethodNotFound)
	}
}

// TestConn_WriteSerialization：并发 Notify 产物逐行可解析（写互斥有效）。
func TestConn_WriteSerialization(t *testing.T) {
	c, _, hostR := testPipe(t, &fakeRouter{})
	defer c.Close()

	const n = 32
	// io.Pipe 为同步管道：必须先起消费者再发写，否则首个抢到写锁的
	// goroutine 阻塞在 Write 等不到读端（wg.Wait 永不返回）。
	lines := make(chan string, n)
	go func() {
		for i := 0; i < n; i++ {
			l, err := hostR.ReadString('\n')
			if err != nil {
				return
			}
			lines <- l
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = c.Notify("session/update", map[string]any{"sessionId": "s", "seq": i})
		}(i)
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		select {
		case l := <-lines:
			var f frame
			if err := json.Unmarshal([]byte(l), &f); err != nil {
				t.Fatalf("line %d not a complete frame: %v (%q)", i, err, l)
			}
			if f.Method != "session/update" {
				t.Fatalf("line %d method = %q", i, f.Method)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("only got %d/%d frames", i, n)
		}
	}
}

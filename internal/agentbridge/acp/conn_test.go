package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// pipeConn 构造一对内存管道连接的 Conn（client 端写 requestCh，从 responseCh 读）。
type pipeConn struct {
	client *Conn
	in     *bytes.Buffer // client 写出的字节流
	out    *bytes.Buffer // 喂给 client 的字节流
	mu     sync.Mutex
}

func newPipeConn(t *testing.T, onReq RequestHandler, onNotify NotifyHandler) *pipeConn {
	t.Helper()
	p := &pipeConn{in: &bytes.Buffer{}, out: &bytes.Buffer{}}
	p.client = NewConn(p, p, onReq, onNotify)
	return p
}

func (p *pipeConn) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for p.out.Len() == 0 {
		p.mu.Unlock()
		time.Sleep(time.Millisecond)
		p.mu.Lock()
	}
	return p.out.Read(b)
}

func (p *pipeConn) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.in.Write(b)
}

func (p *pipeConn) feed(t *testing.T, line string) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.out.WriteString(line + "\n")
}

func (p *pipeConn) written(t *testing.T) []string {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	s := strings.TrimSpace(p.in.String())
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestConnCallWritesRequestFrame(t *testing.T) {
	pc := newPipeConn(t, nil, nil)
	defer pc.client.Close()

	go func() {
		time.Sleep(20 * time.Millisecond)
		pc.feed(t, `{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":1}}`)
	}()

	res, err := pc.client.Call(context.Background(), MethodInitialize, map[string]any{"protocolVersion": ProtocolVersion})
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	var ir InitializeResult
	if err := json.Unmarshal(res, &ir); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ir.ProtocolVersion != 1 {
		t.Fatalf("want protocolVersion 1, got %d", ir.ProtocolVersion)
	}

	lines := pc.written(t)
	if len(lines) != 1 {
		t.Fatalf("want 1 frame written, got %d: %v", len(lines), lines)
	}
	var frame map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &frame); err != nil {
		t.Fatalf("frame not json: %v", err)
	}
	if frame["jsonrpc"] != "2.0" || frame["method"] != MethodInitialize {
		t.Fatalf("bad frame: %v", frame)
	}
	if _, ok := frame["id"]; !ok {
		t.Fatalf("request frame must carry id: %v", frame)
	}
	if _, ok := frame["params"]; !ok {
		t.Fatalf("request frame must carry params: %v", frame)
	}
}

func TestConnCallErrorResponse(t *testing.T) {
	pc := newPipeConn(t, nil, nil)
	defer pc.client.Close()

	go func() {
		time.Sleep(20 * time.Millisecond)
		pc.feed(t, `{"jsonrpc":"2.0","id":0,"error":{"code":-32602,"message":"invalid params"}}`)
	}()

	_, err := pc.client.Call(context.Background(), MethodSessionNew, map[string]any{"cwd": `F:\x`})
	if err == nil {
		t.Fatal("want error from error response")
	}
	if !strings.Contains(err.Error(), "invalid params") {
		t.Fatalf("error should carry remote message, got %v", err)
	}
}

func TestConnConcurrentCallsRoutedByID(t *testing.T) {
	pc := newPipeConn(t, nil, nil)
	defer pc.client.Close()

	go func() {
		time.Sleep(30 * time.Millisecond)
		// 乱序返回：后发的 id=1 先回
		pc.feed(t, `{"jsonrpc":"2.0","id":1,"result":{"sessionId":"s-2"}}`)
		pc.feed(t, `{"jsonrpc":"2.0","id":0,"result":{"sessionId":"s-1"}}`)
	}()

	ctx := context.Background()
	type out struct {
		id  string
		err error
	}
	ch := make(chan out, 2)
	for range 2 {
		go func() {
			res, err := pc.client.Call(ctx, MethodSessionNew, map[string]any{"cwd": "."})
			if err != nil {
				ch <- out{err: err}
				return
			}
			var r NewSessionResult
			_ = json.Unmarshal(res, &r)
			ch <- out{id: r.SessionID}
		}()
	}
	got := map[string]bool{}
	for range 2 {
		o := <-ch
		if o.err != nil {
			t.Fatalf("call error: %v", o.err)
		}
		got[o.id] = true
	}
	if !got["s-1"] || !got["s-2"] {
		t.Fatalf("responses must be routed to correct callers, got %v", got)
	}
}

func TestConnIncomingNotificationDispatched(t *testing.T) {
	notifyCh := make(chan SessionNotification, 1)
	pc := newPipeConn(t, nil, func(_ context.Context, n SessionNotification) {
		notifyCh <- n
	})
	defer pc.client.Close()

	pc.feed(t, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hello"}}}}`)

	select {
	case n := <-notifyCh:
		if n.SessionID != "s-1" {
			t.Fatalf("want sessionId s-1, got %s", n.SessionID)
		}
		if n.Update.Kind != "agent_message_chunk" {
			t.Fatalf("want kind agent_message_chunk, got %s", n.Update.Kind)
		}
		if n.Update.Content == nil || n.Update.Content.Text != "hello" {
			t.Fatalf("want text content hello, got %+v", n.Update.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notification not dispatched")
	}
}

func TestConnIncomingPermissionRequestAnswered(t *testing.T) {
	pc := newPipeConn(t, func(_ context.Context, req PermissionRequestParams) (PermissionResult, error) {
		if req.SessionID != "s-9" {
			t.Errorf("want sessionId s-9, got %s", req.SessionID)
		}
		if req.ToolCall.Title != "go test ./..." {
			t.Errorf("want toolCall title, got %s", req.ToolCall.Title)
		}
		if len(req.Options) != 2 || req.Options[0].OptionID != "allow" {
			t.Errorf("bad options: %+v", req.Options)
		}
		return PermissionResult{Outcome: PermissionOutcome{Outcome: "selected", OptionID: "allow"}}, nil
	}, nil)
	defer pc.client.Close()

	pc.feed(t, `{"jsonrpc":"2.0","id":7,"method":"session/request_permission","params":{"sessionId":"s-9","toolCall":{"toolCallId":"tc1","title":"go test ./...","kind":"execute"},"options":[{"optionId":"allow","name":"允许","kind":"allow_once"},{"optionId":"deny","name":"拒绝","kind":"reject_once"}]}}`)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lines := pc.written(t)
		if len(lines) > 0 {
			var frame map[string]any
			if err := json.Unmarshal([]byte(lines[0]), &frame); err != nil {
				t.Fatalf("bad frame: %v", err)
			}
			if frame["id"].(float64) != 7 {
				t.Fatalf("response must echo request id 7, got %v", frame["id"])
			}
			result, ok := frame["result"].(map[string]any)
			if !ok {
				t.Fatalf("want result in response, got %v", frame)
			}
			outcome := result["outcome"].(map[string]any)
			if outcome["outcome"] != "selected" || outcome["optionId"] != "allow" {
				t.Fatalf("bad outcome: %v", outcome)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no response frame written for incoming permission request")
}

func TestConnMalformedLineSkipped(t *testing.T) {
	notifyCh := make(chan SessionNotification, 1)
	pc := newPipeConn(t, nil, func(_ context.Context, n SessionNotification) { notifyCh <- n })
	defer pc.client.Close()

	pc.feed(t, `{not json`)
	pc.feed(t, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s-1","update":{"sessionUpdate":"plan"}}}`)

	select {
	case n := <-notifyCh:
		if n.Update.Kind != "plan" {
			t.Fatalf("want plan, got %s", n.Update.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream must survive malformed line")
	}
}

func TestConnCloseFailsPendingCalls(t *testing.T) {
	pc := newPipeConn(t, nil, nil)

	done := make(chan error, 1)
	go func() {
		_, err := pc.client.Call(context.Background(), MethodSessionPrompt, nil)
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)
	pc.client.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("pending call must fail on close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending call not released on close")
	}
}

func TestConnCallContextCancel(t *testing.T) {
	pc := newPipeConn(t, nil, nil)
	defer pc.client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := pc.client.Call(ctx, MethodSessionPrompt, nil)
	if err == nil {
		t.Fatal("want ctx timeout error")
	}
}

// 确保 bufio 不被帧实现误用的静态守卫（占位引用）。
var _ = bufio.NewReader

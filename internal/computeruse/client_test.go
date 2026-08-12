package computeruse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// fakeSidecar 用 io.Pipe 模拟 sidecar 的 stdio 帧流。
// handler 返回 (result, rpcErr)；handler 阻塞则模拟 sidecar 不响应。
type fakeSidecar struct {
	client   *Client
	requests chan rpcRequest // 收到的请求（已解析）
	closeW   io.Closer       // sidecar → client 方向的写端（关闭即模拟 EOF）
}

// handler 签名的返回：result 任意可序列化对象；rpcErr 非 nil 时作为 error 帧返回。
type sidecarHandler func(req rpcRequest) (result any, rpcErr *rpcError)

func newFakeSidecar(t *testing.T, handler sidecarHandler) *fakeSidecar {
	t.Helper()
	reqR, reqW := io.Pipe()   // client → sidecar
	respR, respW := io.Pipe() // sidecar → client

	c := NewClient(reqW, respR, loggateway.NewNoop())
	fs := &fakeSidecar{client: c, requests: make(chan rpcRequest, 64), closeW: respW}

	go func() {
		scanner := bufio.NewScanner(reqR)
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			var req rpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				return
			}
			fs.requests <- req
			result, rpcErr := handler(req)
			resp := rpcResponse{ID: req.ID, Error: rpcErr}
			if rpcErr == nil {
				raw, err := json.Marshal(result)
				if err != nil {
					return
				}
				resp.Result = raw
			}
			line, err := json.Marshal(resp)
			if err != nil {
				return
			}
			if _, err := respW.Write(append(line, '\n')); err != nil {
				return
			}
		}
	}()
	return fs
}

// TestClientCallSuccess 基本请求/响应往返。
func TestClientCallSuccess(t *testing.T) {
	fs := newFakeSidecar(t, func(req rpcRequest) (any, *rpcError) {
		if req.Method != "device.ping" {
			return nil, &rpcError{Code: -32601, Message: "unknown method"}
		}
		return map[string]any{"ok": true}, nil
	})
	defer fs.client.Close()

	raw, err := fs.client.Call(context.Background(), "device.ping", nil)
	if err != nil {
		t.Fatalf("Call err = %v", err)
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || !out.OK {
		t.Fatalf("result = %s, unmarshal err = %v", raw, err)
	}

	req := <-fs.requests
	if req.JSONRPC != "2.0" || req.ID != 1 {
		t.Errorf("request frame = %+v, want jsonrpc=2.0 id=1", req)
	}
}

// TestClientCallParams 参数序列化进帧。
func TestClientCallParams(t *testing.T) {
	fs := newFakeSidecar(t, func(req rpcRequest) (any, *rpcError) {
		return map[string]any{"echo": string(req.Params)}, nil
	})
	defer fs.client.Close()

	raw, err := fs.client.Call(context.Background(), "action.key", map[string]any{"combo": "ctrl+s"})
	if err != nil {
		t.Fatalf("Call err = %v", err)
	}
	var out struct {
		Echo string `json:"echo"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal err = %v", err)
	}
	if out.Echo != `{"combo":"ctrl+s"}` {
		t.Errorf("echo = %s", out.Echo)
	}
}

// TestClientCallErrorMapping 错误码映射为 Go sentinel error。
func TestClientCallErrorMapping(t *testing.T) {
	cases := []struct {
		code int
		want error
	}{
		{-32001, ErrElementNotFound},
		{-32002, ErrNotInteractable},
		{-32003, ErrInjectionDenied},
		{-32004, ErrInternal},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			fs := newFakeSidecar(t, func(req rpcRequest) (any, *rpcError) {
				return nil, &rpcError{Code: tc.code, Message: "boom"}
			})
			defer fs.client.Close()

			_, err := fs.client.Call(context.Background(), "action.invoke", nil)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want errors.Is %v", err, tc.want)
			}
		})
	}
}

// TestClientCallContextCancel 调用方 ctx 取消时返回，不阻塞。
func TestClientCallContextCancel(t *testing.T) {
	block := make(chan struct{})
	fs := newFakeSidecar(t, func(req rpcRequest) (any, *rpcError) {
		<-block // 永不响应
		return nil, nil
	})
	defer close(block)
	defer fs.client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := fs.client.Call(ctx, "device.info", nil)
	if err == nil {
		t.Fatal("expect timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
}

// TestClientConcurrentMux 并发请求按 id 正确多路复用（响应乱序返回）。
func TestClientConcurrentMux(t *testing.T) {
	reqR, reqW := io.Pipe()
	respR, respW := io.Pipe()
	c := NewClient(reqW, respR, loggateway.NewNoop())
	defer c.Close()

	const n = 16
	var writeMu sync.Mutex
	// sidecar 侧：每个请求独立 goroutine，id 越大延迟越小 → 响应乱序到达
	go func() {
		scanner := bufio.NewScanner(reqR)
		for scanner.Scan() {
			var req rpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				return
			}
			go func(req rpcRequest) {
				time.Sleep(time.Duration(n-req.ID) * 10 * time.Millisecond)
				result, _ := json.Marshal(map[string]any{"id": req.ID})
				line, _ := json.Marshal(rpcResponse{ID: req.ID, Result: result})
				writeMu.Lock()
				respW.Write(append(line, '\n'))
				writeMu.Unlock()
			}(req)
		}
	}()

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw, err := c.Call(context.Background(), "device.ping", nil)
			if err != nil {
				errs[i] = err
				return
			}
			var out struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				errs[i] = err
				return
			}
			// 请求按启动顺序分配 id（串行启动保证单调）
			if out.ID < 1 || out.ID > n {
				errs[i] = fmt.Errorf("unexpected response id %d", out.ID)
			}
		}(i)
		// 串行启动，保证 id 分配顺序确定
		time.Sleep(2 * time.Millisecond)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// TestClientCloseFailsPending Close 幂等；in-flight 请求收到 ErrClosed。
func TestClientCloseFailsPending(t *testing.T) {
	block := make(chan struct{})
	fs := newFakeSidecar(t, func(req rpcRequest) (any, *rpcError) {
		<-block
		return nil, nil
	})
	defer close(block)

	done := make(chan error, 1)
	go func() {
		_, err := fs.client.Call(context.Background(), "device.info", nil)
		done <- err
	}()

	// 等请求发出
	<-fs.requests
	if err := fs.client.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("pending err = %v, want ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending Call not unblocked by Close")
	}

	// 幂等
	if err := fs.client.Close(); err != nil {
		t.Errorf("second Close err = %v", err)
	}
	// 关闭后新请求直接失败
	if _, err := fs.client.Call(context.Background(), "device.ping", nil); !errors.Is(err, ErrClosed) {
		t.Errorf("post-close Call err = %v, want ErrClosed", err)
	}
}

// TestClientEOFFailsPending stdout EOF 时 fail 所有 pending。
func TestClientEOFFailsPending(t *testing.T) {
	block := make(chan struct{})
	fs := newFakeSidecar(t, func(req rpcRequest) (any, *rpcError) {
		<-block
		return nil, nil
	})
	defer close(block)
	defer fs.client.Close()

	done := make(chan error, 1)
	go func() {
		_, err := fs.client.Call(context.Background(), "device.info", nil)
		done <- err
	}()
	<-fs.requests
	// sidecar 崩溃：关闭响应方向
	fs.closeW.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expect error after EOF")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending Call not failed by EOF")
	}
}

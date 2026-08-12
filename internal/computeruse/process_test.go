package computeruse

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// fakeStarter 注入式进程启动器：用 io.Pipe 模拟 sidecar stdio，不真起进程。
// 返回 starter 与启动计数（看门狗重启断言用）。
func fakeStarter(handler sidecarHandler) (starterFunc, *int32) {
	var count int32
	st := func(_ context.Context) (*processHandle, error) {
		atomic.AddInt32(&count, 1)
		stdinR, stdinW := io.Pipe()
		stdoutR, stdoutW := io.Pipe()
		exited := make(chan struct{})
		var writeMu sync.Mutex
		go func() {
			defer close(exited)
			defer stdoutW.Close()
			scanner := bufio.NewScanner(stdinR)
			scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
			for scanner.Scan() {
				var req rpcRequest
				if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
					return
				}
				// 每请求独立 goroutine：handler 挂起时不阻塞后续请求读取（模拟真实僵死只影响响应）
				go func(req rpcRequest) {
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
					writeMu.Lock()
					defer writeMu.Unlock()
					stdoutW.Write(append(line, '\n'))
				}(req)
			}
		}()
		return &processHandle{
			stdin:  stdinW,
			stdout: stdoutR,
			wait:   func() error { <-exited; return nil },
			kill: func() error {
				stdinW.Close()
				stdoutR.Close()
				return nil
			},
		}, nil
	}
	return st, &count
}

// newTestManager 测试用 Manager：fake starter + 加速心跳参数。
func newTestManager(t *testing.T, handler sidecarHandler) (*Manager, *int32) {
	t.Helper()
	st, count := fakeStarter(handler)
	m := NewManager("fake-sidecar.exe", loggateway.NewNoop())
	m.starter = st
	m.pingInterval = 30 * time.Millisecond
	m.pingTimeout = 50 * time.Millisecond
	m.missThreshold = 3
	m.stopGrace = 200 * time.Millisecond
	return m, count
}

// TestManagerEnsureRunningIdempotent EnsureRunning 幂等：多次调用只拉起一次。
func TestManagerEnsureRunningIdempotent(t *testing.T) {
	m, count := newTestManager(t, func(req rpcRequest) (any, *rpcError) {
		return map[string]any{"ok": true}, nil
	})
	defer m.Stop(context.Background())

	if err := m.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning err = %v", err)
	}
	if err := m.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("second EnsureRunning err = %v", err)
	}
	if got := atomic.LoadInt32(count); got != 1 {
		t.Errorf("starter 调用 %d 次, want 1", got)
	}
	if m.Client() == nil {
		t.Error("Client() = nil after EnsureRunning")
	}
}

// TestManagerClientCall 通过 Manager 拿到的 Client 能完成 RPC 往返。
func TestManagerClientCall(t *testing.T) {
	m, _ := newTestManager(t, func(req rpcRequest) (any, *rpcError) {
		return map[string]any{"ok": true}, nil
	})
	defer m.Stop(context.Background())

	if err := m.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning err = %v", err)
	}
	raw, err := m.Client().Call(context.Background(), "device.ping", nil)
	if err != nil {
		t.Fatalf("Call err = %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Errorf("result = %s", raw)
	}
}

// TestManagerStopGraceful 优雅停止：关闭 stdin 后进程自行退出，无需 Kill。
func TestManagerStopGraceful(t *testing.T) {
	m, _ := newTestManager(t, func(req rpcRequest) (any, *rpcError) {
		return map[string]any{"ok": true}, nil
	})
	if err := m.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning err = %v", err)
	}
	c := m.Client()
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop err = %v", err)
	}
	// Stop 后 client 已关闭
	if _, err := c.Call(context.Background(), "device.ping", nil); err == nil {
		t.Error("expect error after Stop")
	}
	// Stop 幂等
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("second Stop err = %v", err)
	}
}

// TestManagerWatchdogRestart 心跳连续超时 → 看门狗 kill 并自动重启。
func TestManagerWatchdogRestart(t *testing.T) {
	hang := make(chan struct{})
	defer close(hang)
	var startCount int32
	m, _ := newTestManager(t, nil)
	st, count := fakeStarter(func(req rpcRequest) (any, *rpcError) {
		// 首个实例：ping 挂起模拟僵死；后续实例正常响应
		if req.Method == "device.ping" && atomic.LoadInt32(&startCount) == 1 {
			<-hang
			return nil, nil
		}
		return map[string]any{"ok": true}, nil
	})
	m.starter = func(ctx context.Context) (*processHandle, error) {
		atomic.AddInt32(&startCount, 1)
		return st(ctx)
	}

	if err := m.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning err = %v", err)
	}
	defer m.Stop(context.Background())

	// 3 次心跳超时（30ms 间隔 + 50ms 超时）≈ 150ms 内应触发重启，轮询等待
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(count) >= 2 {
			return // 重启成功
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("看门狗未触发重启，starter 调用 %d 次", atomic.LoadInt32(count))
}

// TestManagerWatchdogSkipsWhileInFlight 75 review F3：sidecar 单线程顺序执行，
// 长动作在途期间无法应答 ping——看门狗此时 ping 必超时，误判僵死杀进程（假僵死）。
// 修复：有在途请求时跳过本轮 ping 且不计 miss；在途请求自身 10s/30s 超时兜底。
func TestManagerWatchdogSkipsWhileInFlight(t *testing.T) {
	block := make(chan struct{})
	var execMu sync.Mutex // 串行化执行，模拟真实单线程 sidecar
	m, count := newTestManager(t, func(req rpcRequest) (any, *rpcError) {
		execMu.Lock()
		defer execMu.Unlock()
		if req.Method == "action.invoke" {
			<-block // 长动作挂起期间 ping 得不到响应
			return map[string]any{"ok": true}, nil
		}
		return map[string]any{"ok": true}, nil
	})
	if err := m.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning err = %v", err)
	}
	defer m.Stop(context.Background())

	actDone := make(chan error, 1)
	go func() {
		_, err := m.Client().Call(context.Background(), "action.invoke", nil)
		actDone <- err
	}()

	// 等待远超 3 次心跳周期（3×(30+50)=240ms）；期间 ping 被挂起的动作阻塞。
	time.Sleep(600 * time.Millisecond)
	if got := atomic.LoadInt32(count); got != 1 {
		t.Fatalf("长动作在途期间看门狗误判僵死重启，starter 调用 %d 次, want 1", got)
	}

	close(block) // 动作放行，应正常完成
	select {
	case err := <-actDone:
		if err != nil {
			t.Errorf("action call err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("action call did not return after unblock")
	}
	if got := atomic.LoadInt32(count); got != 1 {
		t.Errorf("动作完成后仍发生重启，starter 调用 %d 次, want 1", got)
	}
}

package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- 阻塞回归测试（conn 层） ----------

func TestConnSlowPermissionDoesNotBlockUpdates(t *testing.T) {
	notifyCh := make(chan SessionNotification, 4)
	permEntered := make(chan struct{})
	releasePerm := make(chan struct{})

	pc := newPipeConn(t, func(_ context.Context, _ PermissionRequestParams) (PermissionResult, error) {
		close(permEntered)
		<-releasePerm // 卡住审批处理器
		return PermissionResult{Outcome: PermissionOutcome{Outcome: "cancelled"}}, nil
	}, func(_ context.Context, n SessionNotification) {
		notifyCh <- n
	})
	defer pc.client.Close()

	pc.feed(t, `{"jsonrpc":"2.0","id":5,"method":"session/request_permission","params":{"sessionId":"s","toolCall":{"toolCallId":"t","title":"x"},"options":[{"optionId":"a","name":"a","kind":"allow_once"}]}}`)
	<-permEntered
	// 审批卡住期间，update 通知必须仍然可达
	pc.feed(t, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"plan"}}}`)

	select {
	case n := <-notifyCh:
		if n.Update.Kind != "plan" {
			t.Fatalf("want plan, got %s", n.Update.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read loop blocked by slow permission handler")
	}
	close(releasePerm)
}

// ---------- fake ACP agent 子进程 ----------

// TestFakeACPAgent 是 fake agent 子进程替身（GO_WANT_ACP_FAKE=1 时生效）。
// 行为脚本见各 case 注释。
func TestFakeACPAgent(t *testing.T) {
	if os.Getenv("GO_WANT_ACP_FAKE") != "1" {
		return
	}
	runFakeAgent()
}

// runFakeAgent 在子进程内执行 ACP agent 行为。
func runFakeAgent() {
	rd := bufio.NewReader(os.Stdin)
	writeFrame := func(f map[string]any) {
		data, _ := json.Marshal(f)
		fmt.Println(string(data))
	}
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			os.Exit(0) // stdin 关闭 = 宿主退出
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var f map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			continue
		}
		var method string
		_ = json.Unmarshal(f["method"], &method)
		var id json.RawMessage
		if raw, ok := f["id"]; ok {
			id = raw
		}

		switch method {
		case "initialize":
			writeFrame(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": map[string]any{
				"protocolVersion":   1,
				"agentCapabilities": map[string]any{"loadSession": false},
			}})
		case "session/new":
			writeFrame(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": map[string]any{
				"sessionId": "fake-sess-1",
			}})
		case "session/prompt":
			// 1) 三段流式文本
			for _, chunk := range []string{"Hel", "lo ", "world"} {
				writeFrame(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": "fake-sess-1",
					"update":    map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": chunk}},
				}})
			}
			// 2) 发起一次权限请求并等待响应
			writeFrame(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "session/request_permission", "params": map[string]any{
				"sessionId": "fake-sess-1",
				"toolCall":  map[string]any{"toolCallId": "tc-1", "title": "go test ./...", "kind": "execute"},
				"options": []map[string]any{
					{"optionId": "allow", "name": "允许", "kind": "allow_once"},
					{"optionId": "deny", "name": "拒绝", "kind": "reject_once"},
				},
			}})
			// 同步读权限响应（下一帧必须是 id=99 的响应）
			respLine, err := rd.ReadString('\n')
			if err != nil {
				os.Exit(1)
			}
			granted := strings.Contains(respLine, `"optionId":"allow"`)
			mark := "permission-denied"
			if granted {
				mark = "permission-ok"
			}
			writeFrame(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": "fake-sess-1",
				"update":    map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": mark}},
			}})
			// 3) prompt 完成
			writeFrame(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": map[string]any{
				"stopReason": "end_turn",
			}})
		case "session/cancel":
			// 通知，无需响应
		}
	}
}

func fakeAgentSpawn(t *testing.T) SpawnOptions {
	t.Helper()
	return SpawnOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestFakeACPAgent$"},
		Env:     map[string]string{"GO_WANT_ACP_FAKE": "1"},
	}
}

// ---------- Client 集成测试（真实子进程） ----------

type recordHandler struct {
	mu       sync.Mutex
	texts    []string
	onPerm   func(ctx context.Context, req PermissionRequestParams) (PermissionResult, error)
}

func (h *recordHandler) OnUpdate(_ context.Context, n SessionNotification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n.Update.Kind == "agent_message_chunk" && n.Update.Content != nil {
		h.texts = append(h.texts, n.Update.Content.Text)
	}
}

func (h *recordHandler) OnPermission(ctx context.Context, req PermissionRequestParams) (PermissionResult, error) {
	return h.onPerm(ctx, req)
}

func (h *recordHandler) joined() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.texts, "")
}

func TestClientEndToEndWithFakeAgent(t *testing.T) {
	h := &recordHandler{onPerm: func(_ context.Context, req PermissionRequestParams) (PermissionResult, error) {
		if req.ToolCall.Title != "go test ./..." {
			t.Errorf("want toolCall title, got %s", req.ToolCall.Title)
		}
		return PermissionResult{Outcome: PermissionOutcome{Outcome: "selected", OptionID: "allow"}}, nil
	}}

	c, err := Start(context.Background(), fakeAgentSpawn(t), h)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	sessID, err := c.NewSession(context.Background(), `F:\aranea-agents`)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if sessID != "fake-sess-1" {
		t.Fatalf("want fake-sess-1, got %s", sessID)
	}

	stop, err := c.Prompt(context.Background(), sessID, "跑一下测试")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if stop != StopReasonEndTurn {
		t.Fatalf("want end_turn, got %s", stop)
	}

	// 流式文本 + 审批后标记，顺序拼接
	want := "Hello worldpermission-ok"
	if got := h.joined(); got != want {
		t.Fatalf("want joined %q, got %q", want, got)
	}

	if err := c.Cancel(sessID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

func TestClientProtocolVersionMismatch(t *testing.T) {
	// 用 echo helper 冒充 agent：它不会回 initialize，换成回畸形版本
	// 简化：直接用 fake agent 但注入错误版本——通过 env 切换
	opt := SpawnOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestFakeACPAgentBadVersion$"},
		Env:     map[string]string{"GO_WANT_ACP_FAKE_BADVER": "1"},
	}
	_, err := Start(context.Background(), opt, &recordHandler{})
	if err == nil {
		t.Fatal("want protocol version mismatch error")
	}
	if !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("error should mention protocol, got %v", err)
	}
}

// TestFakeACPAgentBadVersion 返回 protocolVersion=999 的 fake agent。
func TestFakeACPAgentBadVersion(t *testing.T) {
	if os.Getenv("GO_WANT_ACP_FAKE_BADVER") != "1" {
		return
	}
	rd := bufio.NewReader(os.Stdin)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			os.Exit(0)
		}
		if !strings.Contains(line, "initialize") {
			continue
		}
		var f map[string]json.RawMessage
		_ = json.Unmarshal([]byte(strings.TrimSpace(line)), &f)
		fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":999,"agentCapabilities":{}}}`+"\n", f["id"])
	}
}

func TestClientCloseKillsProcess(t *testing.T) {
	c, err := Start(context.Background(), fakeAgentSpawn(t), &recordHandler{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	c.Close()
	// Close 后进程必须已终止：Done 关闭
	select {
	case <-c.Proc().Done():
	case <-time.After(3 * time.Second):
		t.Fatal("process not killed after Close")
	}
}

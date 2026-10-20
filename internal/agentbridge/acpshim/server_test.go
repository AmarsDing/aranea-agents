package acpshim

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/agentbridge/acp"
)

// ---------------------------------------------------------------------------
// 内存 transport 基建（P1-2 验收：内存 transport 单测）
// ---------------------------------------------------------------------------

// hostHarness 模拟宿主侧：读 shim 出帧、写宿主入帧。
type hostHarness struct {
	t  *testing.T
	w  io.WriteCloser
	br *bufio.Reader
}

func newHostHarness(t *testing.T, srv *Server) *hostHarness {
	t.Helper()
	shimR, hostW := io.Pipe() // shim 读 ← 宿主写
	hostR, shimW := io.Pipe() // 宿主读 ← shim 写
	done := make(chan struct{})
	go func() { _ = srv.Serve(context.Background(), shimR, shimW); close(done) }()
	t.Cleanup(func() { hostW.Close(); <-done })
	return &hostHarness{t: t, w: hostW, br: bufio.NewReaderSize(hostR, 64*1024)}
}

func (h *hostHarness) sendRaw(line string) {
	h.t.Helper()
	if _, err := io.WriteString(h.w, line+"\n"); err != nil {
		h.t.Fatalf("send: %v", err)
	}
}

func (h *hostHarness) readFrame() frame {
	h.t.Helper()
	type res struct {
		line []byte
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		l, err := h.br.ReadBytes('\n')
		ch <- res{l, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			h.t.Fatalf("read frame: %v", r.err)
		}
		var f frame
		if err := json.Unmarshal(bytesTrimSpace(r.line), &f); err != nil {
			h.t.Fatalf("unmarshal %q: %v", string(r.line), err)
		}
		return f
	case <-time.After(5 * time.Second):
		h.t.Fatal("timeout waiting for frame")
		return frame{}
	}
}

// readUntilResponse 读帧直至拿到目标 id 的响应帧；途经通知攒下返回。
func (h *hostHarness) readUntilResponse(id string) (frame, []frame) {
	h.t.Helper()
	var notes []frame
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f := h.readFrame()
		if f.Method == "" && string(f.ID) == id {
			return f, notes
		}
		notes = append(notes, f)
	}
	h.t.Fatalf("no response for id %s", id)
	return frame{}, nil
}

// ---------------------------------------------------------------------------
// 测试替身
// ---------------------------------------------------------------------------

// fakeLLM 按脚本产出增量并记录每轮入参。
type fakeLLM struct {
	mu     sync.Mutex
	calls  [][]ChatMessage
	script func(call int, msgs []ChatMessage) []ChatDelta
}

func (f *fakeLLM) Chat(ctx context.Context, req ChatRequest) (<-chan ChatDelta, error) {
	f.mu.Lock()
	call := len(f.calls)
	f.calls = append(f.calls, append([]ChatMessage(nil), req.Messages...))
	script := f.script
	f.mu.Unlock()
	out := make(chan ChatDelta, 8)
	go func() {
		defer close(out)
		for _, d := range script(call, req.Messages) {
			select {
			case out <- d:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (f *fakeLLM) recorded() [][]ChatMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]ChatMessage(nil), f.calls...)
}

// fakeTools 记录执行并按名编程只读位。
type fakeTools struct {
	mu       sync.Mutex
	execs    []string
	readOnly map[string]bool
	result   string
}

func (f *fakeTools) Defs() []ToolDef { return []ToolDef{{Name: "fs_write", ParametersJSON: json.RawMessage(`{}`)}} }

func (f *fakeTools) ReadOnly(name string) bool { return f.readOnly[name] }

func (f *fakeTools) Execute(_ context.Context, cwd, name string, args json.RawMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execs = append(f.execs, name+"@"+cwd+" "+string(args))
	return f.result, nil
}

// fakeApprover 记录请求并按错误编程。
type fakeApprover struct {
	mu   sync.Mutex
	reqs []ToolCallRequest
	err  error
}

func (f *fakeApprover) AlwaysAllowed(tool string) bool { return false }

func (f *fakeApprover) Request(_ context.Context, sessionID string, call ToolCallRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, call)
	return f.err
}

// mustServer 装配测试 Server（心跳默认关）。
func mustServer(t *testing.T, llm LLMClient, tools ToolExecutor, appr Approver) *Server {
	t.Helper()
	idx := 0
	srv, err := NewServer(Config{
		Provider: "test",
		Model:    "m-test",
		LLM:      llm,
		Tools:    tools,
		Approver: appr,
		NewSessionID: func() string {
			idx++
			return "sess-" + strings.Repeat("x", idx)
		},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

// newSession 完成 initialize + session/new 握手并回 sessionId。
func newSession(t *testing.T, h *hostHarness) string {
	t.Helper()
	h.sendRaw(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":false,"writeTextFile":false},"terminal":false}}}`)
	resp, _ := h.readUntilResponse("1")
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	var init acp.InitializeResult
	if err := json.Unmarshal(resp.Result, &init); err != nil {
		t.Fatalf("initialize result: %v", err)
	}
	if init.ProtocolVersion != acp.ProtocolVersion {
		t.Fatalf("protocolVersion = %d, want %d", init.ProtocolVersion, acp.ProtocolVersion)
	}
	if init.AgentCapabilities.LoadSession {
		t.Fatal("agentCapabilities.loadSession must be false（如实不虚报）")
	}

	h.sendRaw(`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"F:/proj","mcpServers":[]}}`)
	resp2, _ := h.readUntilResponse("2")
	var ns acp.NewSessionResult
	if err := json.Unmarshal(resp2.Result, &ns); err != nil {
		t.Fatalf("session/new result: %v", err)
	}
	if ns.SessionID == "" {
		t.Fatal("empty sessionId")
	}
	return ns.SessionID
}

// ---------------------------------------------------------------------------
// P1-2 生命周期分发
// ---------------------------------------------------------------------------

func TestServer_InitializeAndSessionNew(t *testing.T) {
	llm := &fakeLLM{script: func(int, []ChatMessage) []ChatDelta { return nil }}
	h := newHostHarness(t, mustServer(t, llm, &fakeTools{}, &fakeApprover{}))
	sid := newSession(t, h)

	// _aranea/ping 显式应答（M4 活跃性监管：任何合法响应都算 pong）。
	h.sendRaw(`{"jsonrpc":"2.0","id":9,"method":"_aranea/ping","params":{}}`)
	resp, _ := h.readUntilResponse("9")
	if resp.Error != nil {
		t.Fatalf("ping error: %+v", resp.Error)
	}

	// 未知方法 → -32601。
	h.sendRaw(`{"jsonrpc":"2.0","id":10,"method":"session/load","params":{"sessionId":"` + sid + `"}}`)
	resp2, _ := h.readUntilResponse("10")
	if resp2.Error == nil || resp2.Error.Code != CodeMethodNotFound {
		t.Fatalf("session/load error = %+v, want -32601", resp2.Error)
	}
}

// TestServer_PromptTextOnly：无工具一轮收 → end_turn + 文本 chunk 更新。
func TestServer_PromptTextOnly(t *testing.T) {
	llm := &fakeLLM{script: func(call int, msgs []ChatMessage) []ChatDelta {
		return []ChatDelta{{Text: "你好，"}, {Text: "世界"}, {Done: true, Usage: &Usage{PromptTokens: 3, CompletionTokens: 5}}}
	}}
	h := newHostHarness(t, mustServer(t, llm, &fakeTools{}, &fakeApprover{}))
	sid := newSession(t, h)

	h.sendRaw(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sid + `","prompt":[{"type":"text","text":"打个招呼"}]}}`)
	resp, notes := h.readUntilResponse("3")
	var pr acp.PromptResult
	if err := json.Unmarshal(resp.Result, &pr); err != nil {
		t.Fatalf("prompt result: %v", err)
	}
	if pr.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want end_turn", pr.StopReason)
	}

	var chunks []string
	for _, n := range notes {
		if n.Method != acp.MethodSessionUpdate {
			continue
		}
		var sn acp.SessionNotification
		if err := json.Unmarshal(n.Params, &sn); err != nil {
			t.Fatalf("update params: %v", err)
		}
		if sn.Update.Kind == "agent_message_chunk" && sn.Update.Content != nil {
			chunks = append(chunks, sn.Update.Content.Text)
		}
	}
	if got := strings.Join(chunks, ""); got != "你好，世界" {
		t.Fatalf("chunks = %q, want 你好，世界", got)
	}

	// 历史应含 user+assistant 两条。
	calls := llm.recorded()
	if len(calls) != 1 {
		t.Fatalf("llm calls = %d", len(calls))
	}
	if calls[0][len(calls[0])-1].Content != "打个招呼" {
		t.Fatalf("last msg = %q", calls[0][len(calls[0])-1].Content)
	}
}

// TestServer_PromptToolRound：一轮工具往返——审批（fake 放行）→ 执行 → 回历史 → 本轮收尾。
func TestServer_PromptToolRound(t *testing.T) {
	llm := &fakeLLM{script: func(call int, msgs []ChatMessage) []ChatDelta {
		if call == 0 {
			return []ChatDelta{{Done: true, ToolCalls: []ToolCallRequest{
				{Index: 0, ID: "tc1", Name: "fs_write", Arguments: `{"path":"a.go","content":"package a"}`},
			}}}
		}
		return []ChatDelta{{Text: "写好了"}, {Done: true}}
	}}
	tools := &fakeTools{readOnly: map[string]bool{}, result: "written 9 bytes"}
	appr := &fakeApprover{}
	h := newHostHarness(t, mustServer(t, llm, tools, appr))
	sid := newSession(t, h)

	h.sendRaw(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sid + `","prompt":[{"type":"text","text":"写个文件"}]}}`)
	resp, notes := h.readUntilResponse("3")
	var pr acp.PromptResult
	_ = json.Unmarshal(resp.Result, &pr)
	if pr.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("stopReason = %q", pr.StopReason)
	}

	// 审批被问过且参数逐字（AC-10 数据源）。
	appr.mu.Lock()
	if len(appr.reqs) != 1 || appr.reqs[0].Name != "fs_write" || appr.reqs[0].Arguments != `{"path":"a.go","content":"package a"}` {
		t.Fatalf("approver reqs = %+v", appr.reqs)
	}
	appr.mu.Unlock()

	// 工具在锁定 cwd 下执行。
	tools.mu.Lock()
	if len(tools.execs) != 1 || !strings.HasPrefix(tools.execs[0], "fs_write@F:/proj ") {
		t.Fatalf("execs = %+v", tools.execs)
	}
	tools.mu.Unlock()

	// tool_call 更新发出。
	var sawTool bool
	for _, n := range notes {
		var sn acp.SessionNotification
		if json.Unmarshal(n.Params, &sn) == nil && sn.Update.Kind == "tool_call" && sn.Update.Title == "fs_write" {
			sawTool = true
		}
	}
	if !sawTool {
		t.Fatal("no tool_call update")
	}

	// 第二轮 LLM 历史：assistant(tool_calls) + tool 结果已入。
	calls := llm.recorded()
	if len(calls) != 2 {
		t.Fatalf("llm calls = %d, want 2", len(calls))
	}
	second := calls[1]
	var sawAssistantCalls, sawToolResult bool
	for _, m := range second {
		if m.Role == "assistant" && len(m.ToolCalls) == 1 {
			sawAssistantCalls = true
		}
		if m.Role == "tool" && m.ToolCallID == "tc1" && m.Content == "written 9 bytes" {
			sawToolResult = true
		}
	}
	if !sawAssistantCalls || !sawToolResult {
		t.Fatalf("second round history missing tool exchange: %+v", second)
	}
}

// TestServer_PromptRejected：审批否决 → stopReason=refused（fail-closed 中止 loop）。
func TestServer_PromptRejected(t *testing.T) {
	llm := &fakeLLM{script: func(int, []ChatMessage) []ChatDelta {
		return []ChatDelta{{Done: true, ToolCalls: []ToolCallRequest{{Index: 0, ID: "tc1", Name: "shell_exec", Arguments: `{"command":"rm -rf /"}`}}}}
	}}
	appr := &fakeApprover{err: errors.New("user said no")}
	h := newHostHarness(t, mustServer(t, llm, &fakeTools{}, appr))
	sid := newSession(t, h)

	h.sendRaw(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sid + `","prompt":[{"type":"text","text":"删库"}]}}`)
	resp, _ := h.readUntilResponse("3")
	var pr acp.PromptResult
	_ = json.Unmarshal(resp.Result, &pr)
	if pr.StopReason != StopReasonRefused {
		t.Fatalf("stopReason = %q, want refused", pr.StopReason)
	}
	if len(llm.recorded()) != 1 {
		t.Fatalf("llm calls = %d, want 1（中止后不得再走第二轮）", len(llm.recorded()))
	}
}

// TestServer_CancelDuringPrompt：session/cancel 即到即收（设计 §D-4）。
func TestServer_CancelDuringPrompt(t *testing.T) {
	// 阻塞型 LLM：通道不落任何增量、直到 ctx 关。
	blocking := &blockingLLM{}
	h := newHostHarness(t, mustServer(t, blocking, &fakeTools{}, &fakeApprover{}))
	sid := newSession(t, h)

	h.sendRaw(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sid + `","prompt":[{"type":"text","text":"长跑"}]}}`)
	time.Sleep(50 * time.Millisecond) // 等 prompt 进入流内
	h.sendRaw(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"` + sid + `"}}`)

	resp, _ := h.readUntilResponse("3")
	var pr acp.PromptResult
	_ = json.Unmarshal(resp.Result, &pr)
	if pr.StopReason != acp.StopReasonCancelled {
		t.Fatalf("stopReason = %q, want cancelled", pr.StopReason)
	}
}

// blockingLLM 通道不落增量，ctx 关即收。
type blockingLLM struct{}

func (b *blockingLLM) Chat(ctx context.Context, _ ChatRequest) (<-chan ChatDelta, error) {
	out := make(chan ChatDelta)
	go func() {
		<-ctx.Done()
		close(out)
	}()
	return out, nil
}

// TestServer_StdoutPurity：全流程出帧逐行可解析为 JSON-RPC（P1-7 stdout 纯度守卫）。
func TestServer_StdoutPurity(t *testing.T) {
	llm := &fakeLLM{script: func(call int, msgs []ChatMessage) []ChatDelta {
		if call == 0 {
			return []ChatDelta{{Text: "干"}, {Done: true, ToolCalls: []ToolCallRequest{{Index: 0, ID: "t1", Name: "fs_read", Arguments: `{"path":"x"}`}}}}
		}
		return []ChatDelta{{Text: "完"}, {Done: true}}
	}}
	tools := &fakeTools{readOnly: map[string]bool{"fs_read": true}, result: "内容"}
	h := newHostHarness(t, mustServer(t, llm, tools, &fakeApprover{}))
	sid := newSession(t, h)

	h.sendRaw(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sid + `","prompt":[{"type":"text","text":"走一遍"}]}}`)
	resp, notes := h.readUntilResponse("3")
	if resp.Error != nil {
		t.Fatalf("prompt error: %+v", resp.Error)
	}
	// readFrame 内部已做 json.Unmarshal(frame) 断言——任何非协议帧在读帧即 fatal。
	// 再显式断言 jsonrpc 版本字段全齐。
	for _, n := range notes {
		if n.JSONRPC != "2.0" {
			t.Fatalf("frame jsonrpc = %q", n.JSONRPC)
		}
	}
}

// TestServer_Heartbeat：工作期显式心跳（P1-7；_meta.aranea/heartbeat 负载）。
func TestServer_Heartbeat(t *testing.T) {
	llm := &slowLLM{delay: 150 * time.Millisecond}
	idx := 0
	srv, err := NewServer(Config{
		Provider: "test", Model: "m", LLM: llm, Tools: &fakeTools{}, Approver: &fakeApprover{},
		HeartbeatInterval: 30 * time.Millisecond,
		NewSessionID:      func() string { idx++; return "hb-sess" },
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	h := newHostHarness(t, srv)
	sid := newSession(t, h)

	h.sendRaw(`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"` + sid + `","prompt":[{"type":"text","text":"慢活"}]}}`)
	resp, notes := h.readUntilResponse("3")
	if resp.Error != nil {
		t.Fatalf("prompt error: %+v", resp.Error)
	}
	var heartbeats int
	for _, n := range notes {
		var sn acp.SessionNotification
		if json.Unmarshal(n.Params, &sn) != nil {
			continue
		}
		if sn.Update.Kind != heartbeatUpdateKind {
			continue
		}
		heartbeats++
		var meta map[string]map[string]any
		if err := json.Unmarshal(sn.Update.Meta, &meta); err != nil {
			t.Fatalf("heartbeat meta: %v", err)
		}
		hb := meta["aranea/heartbeat"]
		if hb == nil {
			t.Fatal("heartbeat meta missing aranea/heartbeat key")
		}
		if _, ok := hb["elapsed_ms"]; !ok {
			t.Fatalf("heartbeat meta keys = %v, want elapsed_ms", hb)
		}
	}
	if heartbeats == 0 {
		t.Fatal("no heartbeat updates during 150ms prompt with 30ms interval")
	}
}

// slowLLM 延迟后出一帧文本收尾（撑开心跳窗口）。
type slowLLM struct{ delay time.Duration }

func (s *slowLLM) Chat(ctx context.Context, _ ChatRequest) (<-chan ChatDelta, error) {
	out := make(chan ChatDelta, 2)
	go func() {
		defer close(out)
		select {
		case <-time.After(s.delay):
			out <- ChatDelta{Text: "好了"}
			out <- ChatDelta{Done: true}
		case <-ctx.Done():
		}
	}()
	return out, nil
}

// ---------------------------------------------------------------------------
// P1-3 权限经纪人（AC-10：标题逐字参数 + kinds 全集 + always 任务域缓存）
// ---------------------------------------------------------------------------

func TestPermissionBroker_TitleVerbatimAndKinds(t *testing.T) {
	fr := &fakeRouter{}
	c, hostW, hostR := testPipe(t, fr)
	defer c.Close()

	broker := NewBroker(nil, time.Minute, nil)
	broker.BindConn(c)

	args := `{"command":"go test ./..."}`
	resCh := make(chan error, 1)
	go func() {
		resCh <- broker.Request(context.Background(), "s1", ToolCallRequest{ID: "tc9", Name: "shell_exec", Arguments: args})
	}()

	req := readFrame(t, hostR)
	if req.Method != acp.MethodRequestPermission {
		t.Fatalf("method = %q", req.Method)
	}
	var params acp.PermissionRequestParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("params: %v", err)
	}
	// AC-10：标题 = 工具名 + 逐字 JSON 参数，无模型散文。
	wantTitle := "shell_exec " + args
	if params.ToolCall.Title != wantTitle {
		t.Fatalf("title = %q, want %q", params.ToolCall.Title, wantTitle)
	}
	if params.ToolCall.Kind != "execute" {
		t.Fatalf("kind = %q, want execute", params.ToolCall.Kind)
	}
	if string(params.ToolCall.RawInput) != args {
		t.Fatalf("rawInput = %s, want verbatim %s", string(params.ToolCall.RawInput), args)
	}
	// kinds 全集断言。
	gotKinds := map[string]bool{}
	for _, o := range params.Options {
		gotKinds[o.Kind] = true
		if o.OptionID != o.Kind {
			t.Fatalf("option %s: optionId != kind（宿主缓存语义键须一致）", o.OptionID)
		}
	}
	for _, k := range []string{OptionAllowOnce, OptionAllowAlways, OptionRejectOnce, OptionRejectAlways} {
		if !gotKinds[k] {
			t.Fatalf("missing option kind %s", k)
		}
	}

	// allow_once → 放行且不缓存。
	writeLine(t, hostW, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"result":{"outcome":{"outcome":"selected","optionId":"allow_once"}}}`)
	if err := <-resCh; err != nil {
		t.Fatalf("Request: %v", err)
	}
	if broker.AlwaysAllowed("shell_exec") {
		t.Fatal("allow_once must not populate always cache")
	}
}

// TestPermissionBroker_AlwaysCached：allow_always 入任务域缓存（Q1），后续免问宿主。
func TestPermissionBroker_AlwaysCached(t *testing.T) {
	fr := &fakeRouter{}
	c, hostW, hostR := testPipe(t, fr)
	defer c.Close()

	broker := NewBroker(nil, time.Minute, nil)
	broker.BindConn(c)

	resCh := make(chan error, 1)
	go func() {
		resCh <- broker.Request(context.Background(), "s1", ToolCallRequest{ID: "t1", Name: "fs_write", Arguments: `{"path":"a"}`})
	}()
	req := readFrame(t, hostR)
	writeLine(t, hostW, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"result":{"outcome":{"outcome":"selected","optionId":"allow_always"}}}`)
	if err := <-resCh; err != nil {
		t.Fatalf("Request: %v", err)
	}
	if !broker.AlwaysAllowed("fs_write") {
		t.Fatal("allow_always must populate task-scoped cache")
	}
}

// TestPermissionBroker_HostErrorFailsClosed：宿主错误帧（取消时 rejectPending 下发 -32603）→ error。
func TestPermissionBroker_HostErrorFailsClosed(t *testing.T) {
	fr := &fakeRouter{}
	c, hostW, hostR := testPipe(t, fr)
	defer c.Close()

	broker := NewBroker(nil, time.Minute, nil)
	broker.BindConn(c)

	resCh := make(chan error, 1)
	go func() {
		resCh <- broker.Request(context.Background(), "s1", ToolCallRequest{ID: "t1", Name: "fs_write", Arguments: `{}`})
	}()
	req := readFrame(t, hostR)
	// 宿主取消任务时下发错误帧（M4 P0-3 S5 语义）。
	writeLine(t, hostW, `{"jsonrpc":"2.0","id":`+string(req.ID)+`,"error":{"code":-32603,"message":"task cancelled"}}`)
	if err := <-resCh; err == nil {
		t.Fatal("host error must fail closed")
	}
}

// ---------------------------------------------------------------------------
// P1-4 守卫 + P1-5 工具逃逸
// ---------------------------------------------------------------------------

func TestCheckConfig_UnattendedRejected(t *testing.T) {
	_, err := NewServer(Config{
		Unattended: true,
		LLM:        &fakeLLM{},
		Tools:      &fakeTools{},
		Approver:   &fakeApprover{},
	})
	if !errors.Is(err, ErrUnattendedForbidden) {
		t.Fatalf("err = %v, want ErrUnattendedForbidden", err)
	}
}

func TestGuard_RateLimitAndPendingCap(t *testing.T) {
	g := NewPermissionGuard(3, time.Minute, 1)
	now := time.Now()
	g.SetClock(func() time.Time { return now })

	if err := g.Enter(); err != nil {
		t.Fatalf("first Enter: %v", err)
	}
	// 挂起 cap=1：未 Leave 再 Enter → 拒。
	if err := g.Enter(); !errors.Is(err, ErrPermissionPendingCap) {
		t.Fatalf("second Enter = %v, want pending cap", err)
	}
	g.Leave()
	if err := g.Enter(); err != nil {
		t.Fatalf("third Enter: %v", err)
	}
	g.Leave()
	if err := g.Enter(); err != nil {
		t.Fatalf("fourth Enter: %v", err)
	}
	g.Leave()
	// 窗口内第 4 次 → 速率拒（max 3/min）。
	if err := g.Enter(); !errors.Is(err, ErrPermissionStorm) {
		t.Fatalf("rate Enter = %v, want storm", err)
	}
	// 窗口滚过后恢复。
	now = now.Add(2 * time.Minute)
	if err := g.Enter(); err != nil {
		t.Fatalf("after window Enter: %v", err)
	}
	g.Leave()
}

func TestTools_EscapeRejected(t *testing.T) {
	root := t.TempDir()
	reg := NewRegistry(nil, 0)
	ctx := context.Background()

	// 相对逃逸
	if _, err := reg.Execute(ctx, root, "fs_read", json.RawMessage(`{"path":"../outside.txt"}`)); err == nil {
		t.Fatal("../ escape must be rejected")
	} else {
		var esc *ToolEscapeError
		if !errors.As(err, &esc) {
			t.Fatalf("err = %T %v, want ToolEscapeError", err, err)
		}
	}
	// 绝对路径逃逸
	if _, err := reg.Execute(ctx, root, "fs_read", json.RawMessage(`{"path":"`+strings.ReplaceAll(t.TempDir(), `\`, `/`)+`/x.txt"}`)); err == nil {
		t.Fatal("absolute escape must be rejected")
	}
	// 内部路径放行
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := reg.Execute(ctx, root, "fs_read", json.RawMessage(`{"path":"ok.txt"}`))
	if err != nil || out != "hi" {
		t.Fatalf("inside read = %q, %v", out, err)
	}
}

func TestTools_ReadOnlyPolicy(t *testing.T) {
	reg := NewRegistry(nil, 0)
	if !reg.ReadOnly("fs_read") || !reg.ReadOnly("fs_list") {
		t.Fatal("读类必须免批（Q2）")
	}
	if reg.ReadOnly("fs_write") || reg.ReadOnly("shell_exec") {
		t.Fatal("写与执行必须强制批（Q2）")
	}
}

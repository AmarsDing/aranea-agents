// e2e_shim_deepseek_test.go M4 P2-2（AC-9 全链路断言）：
// fake OpenAI HTTP server ← openaicompat ← acpshim（io.Pipe 内存连接）← acp.Conn ← usecase。
// 链路：dispatch→running→awaiting_approval（审批中继）→running→done；
// 断言：上游请求头（Authorization/Accept）、工具回合（tool_call→tool 结果回投→final）、
// 权限调用序（perm 夹在 round1/round2 之间）、摘要汇聚、日志/落库无密钥明文（P2-3）。
// 生产 cmd/acp-shim-deepseek 与此唯一差别是 stdio 来源（os.Stdin/os.Stdout vs io.Pipe）。
package agentbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/agentbridge/acp"
	"aranea-agents/internal/agentbridge/acpshim"
	"aranea-agents/internal/agentbridge/acpshim/openaicompat"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"go.uber.org/zap/zapcore"
)

// ---------- 事件序记录（权限调用序断言） ----------

type orderLog struct {
	mu     sync.Mutex
	events []string
}

func (l *orderLog) add(ev string) {
	l.mu.Lock()
	l.events = append(l.events, ev)
	l.mu.Unlock()
}

func (l *orderLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// ---------- 日志泄漏面捕获（P2-3：日志无明文断言） ----------

type captureLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *captureLogger) record(level, msg string, fields ...loggateway.Field) {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}
	var b strings.Builder
	b.WriteString(level + " " + msg)
	for k, v := range enc.Fields {
		fmt.Fprintf(&b, " %s=%v", k, v)
	}
	l.mu.Lock()
	l.lines = append(l.lines, b.String())
	l.mu.Unlock()
}

func (l *captureLogger) Debug(msg string, fields ...loggateway.Field) {
	l.record("debug", msg, fields...)
}
func (l *captureLogger) Info(msg string, fields ...loggateway.Field) {
	l.record("info", msg, fields...)
}
func (l *captureLogger) Warn(msg string, fields ...loggateway.Field) {
	l.record("warn", msg, fields...)
}
func (l *captureLogger) Error(msg string, fields ...loggateway.Field) {
	l.record("error", msg, fields...)
}
func (l *captureLogger) With(_ ...loggateway.Field) loggateway.Logger { return l }

func (l *captureLogger) leaks(secret string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, line := range l.lines {
		if strings.Contains(line, secret) {
			out = append(out, line)
		}
	}
	return out
}

// ---------- fake OpenAI 上游（流式 SSE；两回合脚本） ----------

type fakeOpenAIMsg struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
	ToolCalls  []struct {
		ID string `json:"id"`
	} `json:"tool_calls"`
}

type fakeOpenAIRound struct {
	auth     string
	accept   string
	stream   bool
	hasTools bool
	messages []fakeOpenAIMsg
}

type fakeOpenAI struct {
	t     *testing.T
	order *orderLog

	mu     sync.Mutex
	rounds []fakeOpenAIRound
}

func (f *fakeOpenAI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/chat/completions" || r.Method != http.MethodPost {
		http.Error(w, "bad path", http.StatusNotFound)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Model    string            `json:"model"`
		Messages []fakeOpenAIMsg   `json:"messages"`
		Tools    []json.RawMessage `json:"tools"`
		Stream   bool              `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	round := len(f.rounds)
	f.rounds = append(f.rounds, fakeOpenAIRound{
		auth:     r.Header.Get("Authorization"),
		accept:   r.Header.Get("Accept"),
		stream:   req.Stream,
		hasTools: len(req.Tools) > 0,
		messages: req.Messages,
	})
	f.mu.Unlock()
	f.order.add(fmt.Sprintf("round%d", round+1))

	w.Header().Set("Content-Type", "text/event-stream")
	writeChunk := func(delta map[string]any, finish any, usage *map[string]int) {
		chunk := map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}}
		if usage != nil {
			chunk["usage"] = *usage
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	usage := &map[string]int{"prompt_tokens": 10, "completion_tokens": 5}
	if round == 0 {
		// 次回合脚本：shell_exec 工具调用（写/执行类 → 必经宿主审批）。
		writeChunk(map[string]any{"tool_calls": []any{map[string]any{
			"index": 0, "id": "call_1", "type": "function",
			"function": map[string]any{"name": "shell_exec", "arguments": `{"command":"echo hi"}`},
		}}}, nil, nil)
		writeChunk(map[string]any{}, "tool_calls", usage)
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	// 末回合：纯文本收尾。
	writeChunk(map[string]any{"content": "DONE-MARKER"}, nil, nil)
	writeChunk(map[string]any{}, "stop", usage)
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func (f *fakeOpenAI) requests() []fakeOpenAIRound {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeOpenAIRound(nil), f.rounds...)
}

// ---------- shim 进程内会话（io.Pipe；等价 cmd/acp-shim-deepseek 装配） ----------

type shimFactory struct {
	baseURL string
	apiKey  string
	lg      loggateway.Logger
}

func (f *shimFactory) Spawn(ctx context.Context, _ *CodingAgent) (ACPSession, error) {
	llm := openaicompat.New(f.baseURL, f.apiKey, openaicompat.ProviderDeepseek, nil, f.lg)
	tools := acpshim.NewRegistry(f.lg, 2*time.Second)
	broker := acpshim.NewBroker(f.lg, 5*time.Second, acpshim.NewPermissionGuard(0, 0, 0))
	srv, err := acpshim.NewServer(acpshim.Config{
		Provider: "deepseek", Model: "deepseek-chat",
		LLM: llm, Tools: tools, Approver: broker,
		HeartbeatInterval: 0, // 确定性：关闭显式心跳（心跳过滤断言见 P3-3/liveness_test）
		LG:                f.lg,
	})
	if err != nil {
		return nil, err
	}

	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()
	serveCtx, cancelServe := context.WithCancel(context.Background())
	s := &shimPipeSession{cancelServe: cancelServe}
	s.conn = acp.NewConn(s2cR, c2sW, s.onPermission, s.onUpdate)
	s.serveDone = make(chan struct{})
	safego.GoBackground("e2e.shim.serve", func() {
		defer close(s.serveDone)
		_ = srv.Serve(serveCtx, c2sR, s2cW)
	})

	// initialize 握手（与 acp.Start 相同语义，仅不经真实子进程）。
	res, err := s.conn.Call(ctx, acp.MethodInitialize, acp.InitializeParams{
		ProtocolVersion:    acp.ProtocolVersion,
		ClientCapabilities: acp.ClientCapabilities{FS: acp.FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false}, Terminal: false},
	})
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("acp: initialize: %w", err)
	}
	var ir acp.InitializeResult
	if err := json.Unmarshal(res, &ir); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("acp: initialize result: %w", err)
	}
	if ir.ProtocolVersion != acp.ProtocolVersion {
		_ = s.Close()
		return nil, fmt.Errorf("acp: unsupported protocol version %d", ir.ProtocolVersion)
	}
	return s, nil
}

type shimPipeSession struct {
	conn        *acp.Conn
	cancelServe context.CancelFunc
	serveDone   chan struct{}

	mu      sync.Mutex
	handler EventHandler
	sessID  string
	summary strings.Builder
}

func (s *shimPipeSession) onPermission(ctx context.Context, req acp.PermissionRequestParams) (acp.PermissionResult, error) {
	s.mu.Lock()
	h := s.handler
	s.mu.Unlock()
	if h == nil {
		return acp.PermissionResult{Outcome: acp.PermissionOutcome{Outcome: acp.PermissionOutcomeCancelled}}, nil
	}
	opts := make([]PermissionOption, 0, len(req.Options))
	for _, o := range req.Options {
		opts = append(opts, PermissionOption{OptionID: o.OptionID, Name: o.Name, Kind: o.Kind})
	}
	id, err := h.OnPermission(ctx, req.ToolCall.Title, opts)
	if err != nil {
		return acp.PermissionResult{}, err
	}
	return acp.PermissionResult{Outcome: acp.PermissionOutcome{Outcome: acp.PermissionOutcomeSelected, OptionID: id}}, nil
}

func (s *shimPipeSession) onUpdate(_ context.Context, n acp.SessionNotification) {
	kind := n.Update.Kind
	text := ""
	if n.Update.Content != nil {
		text = n.Update.Content.Text
	}
	s.mu.Lock()
	if kind == "agent_message_chunk" && text != "" {
		s.summary.WriteString(text)
	}
	h := s.handler
	s.mu.Unlock()
	if h != nil {
		h.OnUpdate(kind, text)
	}
}

func (s *shimPipeSession) Prompt(ctx context.Context, cwd, prompt string, h EventHandler) (string, error) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()

	res, err := s.conn.Call(ctx, acp.MethodSessionNew, acp.NewSessionParams{CWD: cwd, MCPServers: []any{}})
	if err != nil {
		return "", fmt.Errorf("acp: session/new: %w", err)
	}
	var nsr acp.NewSessionResult
	if err := json.Unmarshal(res, &nsr); err != nil || nsr.SessionID == "" {
		return "", fmt.Errorf("acp: session/new result: %v", err)
	}
	s.mu.Lock()
	s.sessID = nsr.SessionID
	s.mu.Unlock()

	res, err = s.conn.Call(ctx, acp.MethodSessionPrompt, acp.PromptParams{
		SessionID: nsr.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("acp: session/prompt: %w", err)
	}
	var pr acp.PromptResult
	if err := json.Unmarshal(res, &pr); err != nil {
		return "", fmt.Errorf("acp: session/prompt result: %w", err)
	}
	s.mu.Lock()
	summary := s.summary.String()
	s.mu.Unlock()
	if pr.StopReason == acp.StopReasonCancelled {
		return summary, context.Canceled
	}
	return summary, nil
}

func (s *shimPipeSession) Cancel(context.Context) error {
	s.mu.Lock()
	id := s.sessID
	s.mu.Unlock()
	if id == "" {
		return nil
	}
	return s.conn.Notify(acp.MethodSessionCancel, acp.CancelNotificationParams{SessionID: id})
}

func (s *shimPipeSession) Kill(context.Context) error { return s.Close() }

// Ping 内存管道版：shim 显式应答 `_aranea/ping`（server.HandleRequest 分支）。
func (s *shimPipeSession) Ping(ctx context.Context) error {
	_, err := s.conn.Call(ctx, acp.MethodAraneaPing, struct{}{})
	return err
}

// ProcessPID 进程内 shim 无独立子进程（仅满足端口；生产 acpSession 返回真 PID）。
func (s *shimPipeSession) ProcessPID() int { return 0 }

func (s *shimPipeSession) Close() error {
	_ = s.conn.Close()
	s.cancelServe()
	return nil
}

// ---------- 审批中继 handler（模拟 service 的 Mark/Resume 链路） ----------

type e2eApprovalHandler struct {
	t        *testing.T
	uc       *AgentBridgeUsecase
	order    *orderLog
	taskIDCh chan string
}

func (h *e2eApprovalHandler) OnUpdate(_, _ string) {}

func (h *e2eApprovalHandler) OnPermission(ctx context.Context, title string, opts []PermissionOption) (string, error) {
	taskID := <-h.taskIDCh
	h.order.add("perm")
	// AC-10：审批标题 = 工具名 + 逐字 JSON 参数。
	if !strings.HasPrefix(title, "shell_exec ") || !strings.Contains(title, `"command"`) {
		h.t.Errorf("permission title = %q, want 工具名+逐字参数", title)
	}
	allow := ""
	for _, o := range opts {
		if o.OptionID == "allow_once" {
			allow = o.OptionID
		}
	}
	if allow == "" {
		return "", fmt.Errorf("no allow_once option in %v", opts)
	}
	// 审批中继：running→awaiting_approval→running。
	if err := h.uc.MarkAwaitingApproval(taskID); err != nil {
		return "", fmt.Errorf("mark awaiting: %w", err)
	}
	task, err := h.uc.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if task.Status != StatusAwaitingApproval {
		return "", fmt.Errorf("status = %s, want awaiting_approval", task.Status)
	}
	if err := h.uc.ResumeFromApproval(taskID); err != nil {
		return "", fmt.Errorf("resume: %w", err)
	}
	return allow, nil
}

// ---------- P2-2 全链路（AC-9） ----------

func TestE2E_DeepseekShimFullChain(t *testing.T) {
	order := &orderLog{}
	fake := &fakeOpenAI{t: t, order: order}
	upstream := httptest.NewServer(fake)
	defer upstream.Close()

	apiKey := "test-deepseek-key-明文哨兵"
	lg := &captureLogger{}

	e := newTestEnv()
	e.seedAgent("deepseek", true)
	e.seedProject("proj", t.TempDir())
	uc := NewAgentBridgeUsecase(UsecaseDeps{
		Agents:   e.agents,
		Projects: e.projects,
		Tasks:    e.tasks,
		Sessions: &shimFactory{baseURL: upstream.URL, apiKey: apiKey, lg: lg},
		Logger:   lg,
	})
	h := &e2eApprovalHandler{t: t, uc: uc, order: order, taskIDCh: make(chan string, 1)}

	res, err := uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "deepseek", ProjectQuery: "proj",
		Prompt: "运行 echo", Handler: h,
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	h.taskIDCh <- res.Task.ID

	// 等终态（done via run goroutine）。
	var final *CodingTask
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		task, gerr := uc.GetTask(context.Background(), res.Task.ID)
		if gerr != nil {
			t.Fatalf("get: %v", gerr)
		}
		if task.Status.IsTerminal() {
			final = task
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if final == nil {
		t.Fatal("timeout waiting task terminal")
	}
	if final.Status != StatusDone {
		t.Fatalf("status = %s, want done (error=%q)", final.Status, final.Error)
	}
	if !strings.Contains(final.Summary, "DONE-MARKER") {
		t.Errorf("summary = %q, want DONE-MARKER（第二回合文本汇聚）", final.Summary)
	}

	// 权限调用序断言：round1（工具调用下达）→ perm（宿主审批）→ round2（工具结果回投）。
	if got, want := order.snapshot(), []string{"round1", "perm", "round2"}; !equalStrings(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}

	// 上游请求头 + 工具回合断言。
	rnds := fake.requests()
	if len(rnds) != 2 {
		t.Fatalf("upstream rounds = %d, want 2", len(rnds))
	}
	for i, r := range rnds {
		if r.auth != "Bearer "+apiKey {
			t.Errorf("round %d Authorization = %q, want Bearer <注入密钥>", i+1, r.auth)
		}
		if !strings.Contains(r.accept, "text/event-stream") {
			t.Errorf("round %d Accept = %q, want text/event-stream", i+1, r.accept)
		}
		if !r.stream {
			t.Errorf("round %d stream=false, want 流式", i+1)
		}
		if !r.hasTools {
			t.Errorf("round %d 缺 tools 声明", i+1)
		}
	}
	toolIdx := -1
	for i, m := range rnds[1].messages {
		if m.Role == "tool" {
			toolIdx = i
		}
	}
	if toolIdx < 0 {
		t.Fatalf("round 2 缺 role=tool 消息：%+v", rnds[1].messages)
	}
	if m := rnds[1].messages[toolIdx]; m.ToolCallID != "call_1" {
		t.Errorf("tool 消息 tool_call_id = %q, want call_1", m.ToolCallID)
	} else if !strings.Contains(m.Content, "hi") {
		t.Errorf("tool 结果 = %q, want 含 echo 输出", m.Content)
	}

	// P2-3：日志泄露面断言——密钥只应出现在上游 Authorization 头，任何日志行不得含明文。
	if leaks := lg.leaks(apiKey); len(leaks) > 0 {
		t.Fatalf("api key leaked into logs: %v", leaks)
	}
	// P2-3：落库泄露面断言——任务行可见字段不得含密钥明文。
	for field, v := range map[string]string{
		"prompt": final.Prompt, "summary": final.Summary, "error": final.Error, "last_excerpt": final.LastExcerpt,
	} {
		if strings.Contains(v, apiKey) {
			t.Errorf("task.%s leaks api key: %q", field, v)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

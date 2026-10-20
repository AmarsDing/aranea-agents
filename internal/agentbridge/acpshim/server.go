package acpshim

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/agentbridge/acp"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// StopReasonRefused 是审批拒绝/中止时的 prompt 结束原因（ACP 规范取值；
// acp 包只常量化了本系统客户端用到的两种，此处补齐）。
const StopReasonRefused = "refused"

// heartbeatUpdateKind 显式心跳的 sessionUpdate 类别（M4 设计 §A：
// 宿主对任何 session/update 都重置静默计时；不认识的 kind 不入文本 UI）。
const heartbeatUpdateKind = "_aranea/heartbeat"

// ---------------------------------------------------------------------------
// 厂商中立的对话抽象（openaicompat.Client 实现 LLMClient；
// vendor 差异收敛在 openaicompat 的 provider 分支，禁止侵入本层——设计 §F）
// ---------------------------------------------------------------------------

// ChatMessage 是 OpenAI 兼容消息（历史串联 + 出站）。
type ChatMessage struct {
	Role       string           `json:"role"` // system / user / assistant / tool
	Content    string           `json:"content"`
	ToolCalls  []ToolCallRequest `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// ToolCallRequest 是一次工具调用（Arguments 为逐字 JSON 字符串，
// 权限标题逐字引用——AC-10 禁模型散文）。
type ToolCallRequest struct {
	Index     int    `json:"index"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef 是工具声明（OpenAI function schema 形态）。
type ToolDef struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	ParametersJSON json.RawMessage `json:"parameters"`
}

// Usage 是 token 计数（可得时；心跳负载 tokens_in/out 数据源）。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ChatDelta 是一帧流式增量。ToolCalls 只在 Done 帧携带聚合完成的全集
// （流式增量按 index 拼接由 LLMClient 内部完成——P1-6 fixture golden 断言点）。
type ChatDelta struct {
	Text         string           // assistant 文本增量（原样转发 chunk）
	Reasoning    string           // reasoning_content 增量（deepseek-reasoner；loop 默认丢弃不断流——fixture README 决策）
	ToolCalls    []ToolCallRequest
	Done         bool
	FinishReason string
	Usage        *Usage
	Err          error // 非 nil 即上游失败，消费方中止（fail-closed）
}

// ChatRequest 是一轮对话请求。
type ChatRequest struct {
	Model    string
	Messages []ChatMessage
	Tools    []ToolDef
}

// LLMClient 抽象 vendor 大模型上游。
type LLMClient interface {
	// Chat 发起一轮对话，返回增量通道（关闭即本轮结束）。
	// 实现负责把 tool_calls 流式增量聚合到 Done 帧。
	Chat(ctx context.Context, req ChatRequest) (<-chan ChatDelta, error)
}

// ToolExecutor 由 tools_local.go 的 Registry 实现。
type ToolExecutor interface {
	Defs() []ToolDef
	// ReadOnly 免批判定（Q2 终值：读类免批/写与执行强制批）。
	ReadOnly(name string) bool
	// Execute 在锁定的 cwd 下执行工具，返回给模型的文本结果。
	Execute(ctx context.Context, cwd, name string, args json.RawMessage) (string, error)
}

// Approver 由 permission.go 的 Broker 实现（合成 session/request_permission）。
type Approver interface {
	// AlwaysAllowed 命中 allow_always 任务域缓存（Q1）时免再问宿主。
	AlwaysAllowed(tool string) bool
	// Request 阻塞等待宿主结论；允许返回 nil。
	// 拒绝/超时/失联/速率失控 → 返回 error（fail-closed，loop 中止）。
	Request(ctx context.Context, sessionID string, call ToolCallRequest) error
}

// ---------------------------------------------------------------------------
// Server：initialize / session/new / session/prompt / session/cancel 分发
// ---------------------------------------------------------------------------

// Config 装配 Server。LLM/Tools/Approver 缺一不可（fail-closed，见 guard.go）。
type Config struct {
	Provider     string // 仅日志标识（"deepseek"/"grok"）
	Model        string // 经 CodingAgent.Args/--model 透传，不写死（设计 §F）
	SystemPrompt string

	LLM      LLMClient
	Tools    ToolExecutor
	Approver Approver

	// HeartbeatInterval 显式心跳周期（M4 §A；0 = 关闭，测试友好）。
	HeartbeatInterval time.Duration
	// MaxRounds 单次 prompt 工具往返上限（失控模型保护；<=0 取 32）。
	MaxRounds int
	// Unattended 「无人值守」位：shim 无此档位，true 即启动拒绝
	// （与审批中继互斥，防静默降权——P1-4 启动校验）。
	Unattended bool

	// NewSessionID 会话 id 工厂（nil 用随机 hex；测试注入确定性 id）。
	NewSessionID func() string

	LG loggateway.Logger // 构造注入（红线 #16）；nil → NewNoop()
}

// Server 实现 RequestRouter，驱动会话生命周期与 agent loop。
type Server struct {
	cfg Config
	lg  loggateway.Logger

	conn *Conn

	sessMu   sync.Mutex
	sessions map[string]*sessionState
}

type sessionState struct {
	id  string
	cwd string

	mu      sync.Mutex
	history []ChatMessage

	currentTool string // 心跳负载：正在执行的工具名
	tokens      struct {
		in  int
		out int
	}

	promptMu   sync.Mutex // 单 prompt 槽位
	cancelCh   chan struct{}
	cancelOnce *sync.Once
}

// NewServer 装配并做启动校验（fail-closed）。
func NewServer(cfg Config) (*Server, error) {
	if err := CheckConfig(&cfg); err != nil {
		return nil, err
	}
	if cfg.NewSessionID == nil {
		cfg.NewSessionID = randomSessionID
	}
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 32
	}
	lg := cfg.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg = lg.With(loggateway.Str("provider", cfg.Provider), loggateway.Str("model", cfg.Model))
	cfg.LG = lg
	return &Server{cfg: cfg, lg: lg, sessions: make(map[string]*sessionState)}, nil
}

// Serve 在给定流上跑 ACP 服务端直到连接关闭或 ctx 取消。
// main.go 以 os.Stdin/os.Stdout 调用；测试以内存管道调用。
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	c := NewConn(r, w, s)
	s.conn = c
	// Approver 装配时 conn 未知（NewServer 先于 Serve），此处补链：
	// Broker 以 ConnProvider 迟绑定（permission.go）。
	if b, ok := s.cfg.Approver.(interface{ BindConn(*Conn) }); ok {
		b.BindConn(c)
	}
	select {
	case <-c.Done():
	case <-ctx.Done():
		c.Close()
	}
	return nil
}

// Conn 暴露当前连接（心跳/测试用；Serve 前为 nil）。
func (s *Server) Conn() *Conn { return s.conn }

// HandleRequest 分发入站请求（conn 在独立 goroutine 中调用）。
func (s *Server) HandleRequest(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case acp.MethodInitialize:
		// 如实回报：protocolVersion=1，不虚报能力（设计 §D-6）。
		return acp.InitializeResult{
			ProtocolVersion:   acp.ProtocolVersion,
			AgentCapabilities: acp.AgentCapabilities{LoadSession: false},
		}, nil
	case acp.MethodSessionNew:
		return s.handleSessionNew(params)
	case acp.MethodSessionPrompt:
		return s.handlePrompt(ctx, params)
	case acp.MethodAraneaPing:
		// M4 活跃性监管：显式应答（任何合法响应都算 pong，空结果最省）。
		return struct{}{}, nil
	default:
		return nil, &RPCError{Code: CodeMethodNotFound, Message: "acpshim: unknown method " + method}
	}
}

// HandleNotification 分发入站通知（读循环内同步调用，必须快返回）。
func (s *Server) HandleNotification(_ context.Context, method string, params json.RawMessage) {
	if method != acp.MethodSessionCancel {
		return
	}
	var p acp.CancelNotificationParams
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	s.sessMu.Lock()
	sess := s.sessions[p.SessionID]
	s.sessMu.Unlock()
	if sess == nil {
		return
	}
	sess.mu.Lock()
	ch, once := sess.cancelCh, sess.cancelOnce
	sess.mu.Unlock()
	if once != nil {
		once.Do(func() { close(ch) })
	}
	s.lg.Info("acpshim: session cancel received", loggateway.SessionID(p.SessionID))
}

func (s *Server) handleSessionNew(params json.RawMessage) (any, error) {
	var p acp.NewSessionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: CodeInvalidParams, Message: "acpshim: invalid session/new params"}
	}
	if strings.TrimSpace(p.CWD) == "" {
		return nil, &RPCError{Code: CodeInvalidParams, Message: "acpshim: cwd required"}
	}
	sess := &sessionState{id: s.cfg.NewSessionID(), cwd: p.CWD}
	if s.cfg.SystemPrompt != "" {
		sess.history = append(sess.history, ChatMessage{Role: "system", Content: s.cfg.SystemPrompt})
	}
	s.sessMu.Lock()
	s.sessions[sess.id] = sess
	s.sessMu.Unlock()
	s.lg.Info("acpshim: session created", loggateway.SessionID(sess.id), loggateway.Str("cwd", p.CWD))
	return acp.NewSessionResult{SessionID: sess.id}, nil
}

func (s *Server) handlePrompt(ctx context.Context, params json.RawMessage) (any, error) {
	var p acp.PromptParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &RPCError{Code: CodeInvalidParams, Message: "acpshim: invalid session/prompt params"}
	}
	s.sessMu.Lock()
	sess := s.sessions[p.SessionID]
	s.sessMu.Unlock()
	if sess == nil {
		return nil, &RPCError{Code: CodeInvalidParams, Message: "acpshim: unknown sessionId"}
	}
	if !sess.promptMu.TryLock() {
		return nil, &RPCError{Code: CodeInternal, Message: "acpshim: prompt already in flight"}
	}
	defer sess.promptMu.Unlock()

	text := promptText(p.Prompt)
	sess.mu.Lock()
	sess.history = append(sess.history, ChatMessage{Role: "user", Content: text})
	sess.cancelCh = make(chan struct{})
	sess.cancelOnce = &sync.Once{}
	cancelCh := sess.cancelCh
	sess.mu.Unlock()

	// 派生 ctx：宿主断连（conn.Done）或 session/cancel 即撤。
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	safego.GoBackground("acpshim.prompt.cancelWatch", func() {
		select {
		case <-loopCtx.Done():
		case <-cancelCh:
			cancel()
		case <-s.conn.Done():
			cancel()
		}
	})

	stopHeartbeat := s.startHeartbeat(loopCtx, sess)
	defer close(stopHeartbeat)

	stop, err := s.runLoop(loopCtx, sess)
	if err != nil {
		if errors.Is(err, errPromptCancelled) {
			return acp.PromptResult{StopReason: acp.StopReasonCancelled}, nil
		}
		return nil, err
	}
	return acp.PromptResult{StopReason: stop}, nil
}

// errPromptCancelled 标记取消路径（区分上游错误帧）。
var errPromptCancelled = errors.New("acpshim: prompt cancelled")

// runLoop 执行「模型 → 工具 → 模型」往返直至无工具调用。
// 返回 stopReason（end_turn / refused）或 error。
func (s *Server) runLoop(ctx context.Context, sess *sessionState) (string, error) {
	for round := 0; round < s.cfg.MaxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return "", errPromptCancelled
		}
		sess.mu.Lock()
		history := append([]ChatMessage(nil), sess.history...)
		sess.mu.Unlock()

		stream, err := s.cfg.LLM.Chat(ctx, ChatRequest{
			Model:    s.cfg.Model,
			Messages: history,
			Tools:    s.cfg.Tools.Defs(),
		})
		if err != nil {
			if ctx.Err() != nil {
				return "", errPromptCancelled
			}
			return "", fmt.Errorf("acpshim: llm chat: %w", err)
		}

		final, err := s.consumeStream(ctx, sess.id, stream)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, errPromptCancelled) {
				return "", errPromptCancelled
			}
			return "", err
		}

		// 无工具调用 → 收尾。
		if len(final.ToolCalls) == 0 {
			sess.mu.Lock()
			sess.history = append(sess.history, ChatMessage{Role: "assistant", Content: final.assistantText})
			sess.mu.Unlock()
			s.trackUsage(sess, final.Usage)
			return acp.StopReasonEndTurn, nil
		}

		// 有工具调用：先把 assistant 消息（含 tool_calls）入历史。
		sess.mu.Lock()
		sess.history = append(sess.history, ChatMessage{
			Role:      "assistant",
			Content:   final.assistantText,
			ToolCalls: final.ToolCalls,
		})
		sess.mu.Unlock()

		for _, call := range final.ToolCalls {
			if err := ctx.Err(); err != nil {
				return "", errPromptCancelled
			}
			s.setCurrentTool(sess, call.Name)
			result, err := s.runTool(ctx, sess, call)
			s.setCurrentTool(sess, "")
			if err != nil {
				if errors.Is(err, errPromptCancelled) || ctx.Err() != nil {
					return "", errPromptCancelled
				}
				if errors.Is(err, errToolRejected) {
					// 审批拒绝/超时/速率失控（fail-closed）：中止 loop（设计 §E 与 M2 语义一致）。
					s.lg.Warn("acpshim: tool rejected, aborting loop",
						loggateway.SessionID(sess.id), loggateway.Str("tool", call.Name), loggateway.Err(err))
					return StopReasonRefused, nil
				}
				// 工具执行失败：结果入历史交给模型自纠，不炸 loop。
				result = "error: " + err.Error()
			}
			sess.mu.Lock()
			sess.history = append(sess.history, ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
			sess.mu.Unlock()
		}
	}
	s.lg.Warn("acpshim: max rounds exceeded", loggateway.SessionID(sess.id), loggateway.Int("max_rounds", s.cfg.MaxRounds))
	return "", fmt.Errorf("acpshim: max tool rounds (%d) exceeded", s.cfg.MaxRounds)
}

// errToolRejected 标记审批拒绝路径（loop 中止为 refused）。
var errToolRejected = errors.New("acpshim: tool call rejected")

// streamFinal 是一轮流的聚合产物。
type streamFinal struct {
	assistantText string
	ToolCalls     []ToolCallRequest
	Usage         *Usage
}

// consumeStream 消费增量：文本 chunk 即转 session/update；reasoning 默认丢弃不断流
// （fixture README 决策）；Done 帧取聚合 tool_calls。
func (s *Server) consumeStream(ctx context.Context, sessionID string, stream <-chan ChatDelta) (streamFinal, error) {
	var out streamFinal
	var sb strings.Builder
	for {
		select {
		case <-ctx.Done():
			out.assistantText = sb.String()
			return out, errPromptCancelled
		case d, ok := <-stream:
			if !ok {
				out.assistantText = sb.String()
				if ctx.Err() != nil {
					// 通道随 ctx 撤而关闭时不得谎称 end_turn（取消竞态）。
					return out, errPromptCancelled
				}
				return out, nil
			}
			if d.Err != nil {
				out.assistantText = sb.String()
				if ctx.Err() != nil {
					return out, errPromptCancelled
				}
				return out, fmt.Errorf("acpshim: llm stream: %w", d.Err)
			}
			if d.Text != "" {
				sb.WriteString(d.Text)
				s.notifyUpdate(sessionID, acp.SessionUpdate{
					Kind:    "agent_message_chunk",
					Content: &acp.ContentBlock{Type: "text", Text: d.Text},
				})
			}
			if d.Done {
				out.ToolCalls = d.ToolCalls
				out.Usage = d.Usage
			}
		}
	}
	//nolint:unreachable // 循环内已返回；保留出口提示阅读
}

// runTool 审批（读免批/写执行强制批，Q2）→ 执行。
func (s *Server) runTool(ctx context.Context, sess *sessionState, call ToolCallRequest) (string, error) {
	s.notifyUpdate(sess.id, acp.SessionUpdate{
		Kind:       "tool_call",
		Title:      call.Name,
		ToolCallID: call.ID,
	})
	if !s.cfg.Tools.ReadOnly(call.Name) && !s.cfg.Approver.AlwaysAllowed(call.Name) {
		if err := s.cfg.Approver.Request(ctx, sess.id, call); err != nil {
			return "", fmt.Errorf("%w: %v", errToolRejected, err)
		}
	}
	return s.cfg.Tools.Execute(ctx, sess.cwd, call.Name, json.RawMessage(call.Arguments))
}

// notifyUpdate 发 session/update；连接已关（宿主走了）则静默丢。
func (s *Server) notifyUpdate(sessionID string, u acp.SessionUpdate) {
	if s.conn == nil {
		return
	}
	_ = s.conn.Notify(acp.MethodSessionUpdate, acp.SessionNotification{SessionID: sessionID, Update: u})
}

// ---------------------------------------------------------------------------
// 显式心跳（P1-7；M4 §A：工作期间每 heartbeat_interval 发一条，无新内容也发）
// ---------------------------------------------------------------------------

func (s *Server) startHeartbeat(ctx context.Context, sess *sessionState) chan struct{} {
	stop := make(chan struct{})
	if s.cfg.HeartbeatInterval <= 0 {
		return stop
	}
	started := time.Now()
	safego.GoBackground("acpshim.heartbeat", func() {
		ticker := time.NewTicker(s.cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				sess.mu.Lock()
				tool := sess.currentTool
				toks := sess.tokens
				sess.mu.Unlock()
				meta, _ := json.Marshal(map[string]any{
					"aranea/heartbeat": map[string]any{
						"provider":     s.cfg.Provider,
						"current_tool": tool,
						"elapsed_ms":   time.Since(started).Milliseconds(),
						"tokens_in":    toks.in,
						"tokens_out":   toks.out,
					},
				})
				s.notifyUpdate(sess.id, acp.SessionUpdate{
					Kind: heartbeatUpdateKind,
					Meta: meta,
				})
			}
		}
	})
	return stop
}

func (s *Server) setCurrentTool(sess *sessionState, name string) {
	sess.mu.Lock()
	sess.currentTool = name
	sess.mu.Unlock()
}

func (s *Server) trackUsage(sess *sessionState, u *Usage) {
	if u == nil {
		return
	}
	sess.mu.Lock()
	sess.tokens.in += u.PromptTokens
	sess.tokens.out += u.CompletionTokens
	sess.mu.Unlock()
}

// promptText 拼合 prompt 文本块（只用 text 类型，其余忽略）。
func promptText(blocks []acp.ContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

func randomSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("s-%d", time.Now().UnixNano())
	}
	return "s-" + hex.EncodeToString(b[:])
}

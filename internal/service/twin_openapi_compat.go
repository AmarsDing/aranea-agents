// Package service — twin_openapi_compat.go
//
// TwinOpenAPICompatService 是业务系统（twinmonitor / HotCraftFlow AGT /
// NovaTrader harness 等）对接 aranea 的机器面 OpenAPI 兼容门面：将消费方
// 期望的 /api/v1/* 契约映射到 aranea 内部 Usecase 能力（agents/graphs/runs/
// memory/quota/metrics），并实现 biz.GraphRunEventSink 把图执行生命周期
// 事件以 HMAC 签名 webhook 回传消费方，形成「监控 → 分析 → 执行 → 回写」闭环。
//
// 鉴权（P0 多调用方改造，2026-11-02）：独立于 JWT 用户体系，使用机器对机器
// Bearer token——单 token 兼容面（环境变量 ARANEA_TWINOPENAPI_TOKEN，
// caller 恒为 "twinmonitor"）+ 多调用方注册表（ARANEA_TWINOPENAPI_CALLERS
// JSON，token→name/workspace_id 绑定）。guard 认证后 caller 入请求上下文，
// 各 handler 按租户门禁（读面=共享+自有；写面=仅自有）。
// 未配置任何 token 时门面禁用，路由不注册；路由前缀 /api/v1/ 下的门面子树
// 在 server 层注册为 noAuth（由本服务自校验），不影响 /api/v1/admin/* 既有
// JWT 保护。
//
// 安全闸：
//   - webhook_url 提交闸 + 拨号闸（twin_openapi_webhook.go）；
//   - 幂等键持久化（twin_openapi_idempotency 表，biz.TwinIdempotencyStore）。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/loggateway"
)

// TwinOpenAPITokenEnv 是门面机器 token 的环境变量名（存量单 token 兼容面）。
const TwinOpenAPITokenEnv = "ARANEA_TWINOPENAPI_TOKEN"

// twinRunSub 单次执行的 webhook 订阅（内存态；进程重启后由消费方
// 兜底轮询 GET /api/v1/runs/{id} 补齐终态）。
type twinRunSub struct {
	WebhookURL string
	Secret     string
	GraphID    string
	CreatedAt  time.Time
	// Nodes 缓存图定义节点元数据（node_id → name/type），供事件载荷填充。
	Nodes map[string]twinNodeMeta
	// NodeStart 记录节点开始时间戳用于 duration_ms 计算。
	NodeStart map[string]time.Time
	StartedAt time.Time
	// FinalOutput 运行级最终输出（OnRunOutput 暂存，run.completed 携带）。
	FinalOutput string
}

type twinNodeMeta struct {
	Name string
	Type string
}

// twinSecurity 门面安全面（caller 注册表 + webhook 白名单 + 幂等存储）。
// 独立成组以守 AS-COG-01 struct 字段上限。
type twinSecurity struct {
	callers []*twinCaller
	webhook *webhookAllowPolicy
	idem    biz.TwinIdempotencyStore
}

// TwinOpenAPICompatService 机器面 OpenAPI 兼容门面。
// 同时实现 biz.GraphRunEventSink（构造时自注册到 GraphExecutionUsecase）。
type TwinOpenAPICompatService struct {
	agents     *biz.AgentUsecase
	graphs     *biz.GraphUsecase
	memory     *biz.MemoryAdminUsecase
	usageUC    *usage.Usecase
	monitorUC  *monitor.Usecase
	llmModels  *biz.LlmProviderModelUsecase
	mfpDecider *biz.MemoryFactPendingDecider
	lg         loggateway.Logger
	sec        *twinSecurity
	mux        *http.ServeMux
	startAt    time.Time
	hc         *http.Client

	mu      sync.Mutex
	subs    map[string]*twinRunSub
	idempot map[string]string // idempotency_key → run_id（进程内热缓存；权威存储为 sec.idem）
}

// NewTwinOpenAPICompatService 创建门面并向 GraphExecutionUsecase 注册事件水槽。
// caller 注册表取自环境（见 twin_openapi_callers.go）；无任何 token 时
// Enabled() 返回 false。idem 为幂等键持久化端口（nil 时退化为进程内缓存，
// 仅测试/装配缺省场景；生产经 wire 注入）。
func NewTwinOpenAPICompatService(
	agents *biz.AgentUsecase,
	graphs *biz.GraphUsecase,
	memory *biz.MemoryAdminUsecase,
	usageUC *usage.Usecase,
	monitorUC *monitor.Usecase,
	llmModels *biz.LlmProviderModelUsecase,
	mfpDecider *biz.MemoryFactPendingDecider,
	idem biz.TwinIdempotencyStore,
	lg loggateway.Logger,
) *TwinOpenAPICompatService {
	webhookPolicy := loadWebhookAllowPolicy(lg)
	s := &TwinOpenAPICompatService{
		agents:     agents,
		graphs:     graphs,
		memory:     memory,
		usageUC:    usageUC,
		monitorUC:  monitorUC,
		llmModels:  llmModels,
		mfpDecider: mfpDecider,
		lg:         lg,
		sec: &twinSecurity{
			callers: loadTwinCallers(lg),
			webhook: webhookPolicy,
			idem:    idem,
		},
		startAt: time.Now(),
		// 投递客户端走 webhook 拨号闸（拒连非公网白名单/元数据地址）。
		hc:      webhookPolicy.httpClient(10 * time.Second),
		subs:    make(map[string]*twinRunSub),
		idempot: make(map[string]string),
	}
	s.routes()
	if graphs != nil && graphs.ExecUC() != nil {
		graphs.ExecUC().SetRunEventSink(s)
		// G2：注入图运行用量落账器（经可选接口转调 factory）。twin 运行
		// 的 LLM 用量此后按 "twin-" 会话落账，handleGetRun 据以回填
		// model_used/tokens。usageUC 为 nil 时 recorder 静默跳过。
		graphs.ExecUC().SetGraphUsageRecorder(&twinGraphUsageRecorder{usageUC: usageUC, lg: lg})
	}
	if s.Enabled() {
		names := make([]string, 0, len(s.sec.callers))
		for _, c := range s.sec.callers {
			names = append(names, c.Name)
		}
		lg.Info("twinopenapi 机器面已启用",
			loggateway.StepID("twinopenapi.enabled"),
			loggateway.Str("path", s.Path()),
			loggateway.Str("callers", strings.Join(names, ",")))
	} else {
		lg.Warn("twinopenapi 机器面未启用（缺少 "+TwinOpenAPITokenEnv+"/"+TwinOpenAPICallersEnv+"）",
			loggateway.StepID("twinopenapi.disabled"))
	}
	return s
}

// callerList 注册表访问（nil-safe，测试直构门面时 sec 可能为空）。
func (s *TwinOpenAPICompatService) callerList() []*twinCaller {
	if s == nil || s.sec == nil {
		return nil
	}
	return s.sec.callers
}

// webhookPolicyOf 白名单策略访问（nil-safe；缺省按默认「内网可用」档）。
func (s *TwinOpenAPICompatService) webhookPolicyOf() *webhookAllowPolicy {
	if s == nil || s.sec == nil || s.sec.webhook == nil {
		return &webhookAllowPolicy{}
	}
	return s.sec.webhook
}

// idemStore 幂等存储访问（nil-safe；未接线时 handleCreateRun 退化为进程内缓存）。
func (s *TwinOpenAPICompatService) idemStore() biz.TwinIdempotencyStore {
	if s == nil || s.sec == nil {
		return nil
	}
	return s.sec.idem
}

// Enabled 报告门面是否可用（至少一个 caller token 已配置）。
func (s *TwinOpenAPICompatService) Enabled() bool { return len(s.callerList()) > 0 }

// Path 返回注册前缀。
func (s *TwinOpenAPICompatService) Path() string { return "/api/v1/" }

// Handler 返回门面 http.Handler（compatHandler 接口，供 lazyCompatHandler 包装）。
func (s *TwinOpenAPICompatService) Handler(_ context.Context) (http.Handler, error) {
	return s.mux, nil
}

// routes 注册全部端点（Go 1.22+ ServeMux 模式路由）。
func (s *TwinOpenAPICompatService) routes() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.guard(s.handleHealth))
	mux.HandleFunc("GET /api/v1/agents", s.guard(s.handleListAgents))
	mux.HandleFunc("POST /api/v1/agents", s.guard(s.handleCreateAgent))
	mux.HandleFunc("PUT /api/v1/agents/{id}", s.guard(s.handleUpdateAgent))
	mux.HandleFunc("GET /api/v1/graphs", s.guard(s.handleListGraphs))
	mux.HandleFunc("POST /api/v1/graphs", s.guard(s.handleCreateGraph))
	mux.HandleFunc("GET /api/v1/graphs/{id}", s.guard(s.handleGetGraph))
	mux.HandleFunc("PUT /api/v1/graphs/{id}", s.guard(s.handleUpdateGraph))
	mux.HandleFunc("GET /api/v1/runs", s.guard(s.handleListRuns))
	mux.HandleFunc("POST /api/v1/runs", s.guard(s.handleCreateRun))
	mux.HandleFunc("GET /api/v1/runs/{id}", s.guard(s.handleGetRun))
	mux.HandleFunc("POST /api/v1/runs/{id}/cancel", s.guard(s.handleCancelRun))
	mux.HandleFunc("POST /api/v1/runs/{id}/interrupts/{interrupt_id}/resume", s.guard(s.handleResumeInterrupt))
	mux.HandleFunc("POST /api/v1/memory/facts", s.guard(s.handleWriteMemoryFact))
	mux.HandleFunc("POST /api/v1/memory/fact-pending/{id}/decision", s.guard(s.handleMemoryFactPendingDecision))
	mux.HandleFunc("GET /api/v1/quota/usage", s.guard(s.handleQuotaUsage))
	mux.HandleFunc("GET /api/v1/metrics/agents", s.guard(s.handleAgentMetrics))
	s.mux = mux
}

// guard 机器 token 校验（常量时间比较防时序侧信道）；认证后 caller 入请求上下文。
func (s *TwinOpenAPICompatService) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		token, found := strings.CutPrefix(authz, "Bearer ")
		var caller *twinCaller
		if found {
			caller = s.authenticate(strings.TrimSpace(token))
		}
		if caller == nil {
			writeTwinError(w, http.StatusUnauthorized, "invalid api token")
			return
		}
		next(w, r.WithContext(withTwinCaller(r.Context(), caller)))
	}
}

// ---------------------------------------------------------------------------
// 健康
// ---------------------------------------------------------------------------

func (s *TwinOpenAPICompatService) handleHealth(w http.ResponseWriter, r *http.Request) {
	agentCount := 0
	if res, err := s.agents.List(r.Context(), biz.AgentListQuery{Limit: 1}); err == nil {
		agentCount = res.Total
	}
	graphCount := 0
	if defs, _, err := s.graphs.ListGraphs(r.Context(), 1, ""); err == nil {
		// ListGraphs 无 total 返回，按页拉取计数代价高；健康检查仅需粗估。
		graphCount = len(defs)
	}
	modelCount := 0
	if models, err := s.llmModels.List(r.Context()); err == nil {
		modelCount = len(models)
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{
		"status":         "healthy",
		"version":        twinFacadeVersion(),
		"uptime_seconds": int64(time.Since(s.startAt).Seconds()),
		"agent_count":    agentCount,
		"graph_count":    graphCount,
		"model_count":    modelCount,
	})
}

func twinFacadeVersion() string {
	if v := strings.TrimSpace(os.Getenv("ARANEA_VERSION")); v != "" {
		return v
	}
	return "dev"
}

// ---------------------------------------------------------------------------
// 执行（Run）
// ---------------------------------------------------------------------------

// twinRunInput 机器面 Run 提交载荷。
type twinRunInput struct {
	GraphID        string         `json:"graph_id"`
	AgentID        string         `json:"agent_id"`
	Input          string         `json:"input"`
	Params         map[string]any `json:"params"`
	WebhookURL     string         `json:"webhook_url"`
	WebhookSecret  string         `json:"webhook_secret"`
	IdempotencyKey string         `json:"idempotency_key"`
}

func (s *TwinOpenAPICompatService) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	var in twinRunInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	ctx := r.Context()

	// 幂等键命中直接返回原 run_id。权威存储为 idemStore（重启后仍有效）；
	// 进程内 map 仅为未接线场景的退化缓存。
	if key := strings.TrimSpace(in.IdempotencyKey); key != "" {
		if store := s.idemStore(); store != nil {
			if runID, err := store.LookupRunID(ctx, caller.Name, key); err != nil {
				s.lg.Warn("twinopenapi 幂等键查询失败，按未命中处理",
					loggateway.StepID("twinopenapi.idem_lookup_fail"), loggateway.Err(err))
			} else if runID != "" {
				writeTwinJSON(w, http.StatusOK, map[string]any{"run_id": runID, "idempotent_replay": true})
				return
			}
		} else {
			s.mu.Lock()
			runID, ok := s.idempot[caller.Name+"|"+key]
			s.mu.Unlock()
			if ok {
				writeTwinJSON(w, http.StatusOK, map[string]any{"run_id": runID, "idempotent_replay": true})
				return
			}
		}
	}

	if strings.TrimSpace(in.GraphID) == "" {
		// Agent 直跑（无 Graph 包装）在 v1 不支持：agent 运行时无执行记录
		// 实体，无法支撑状态轮询/取消/审批的闭环语义。
		writeTwinError(w, http.StatusBadRequest, "graph_id is required (agent direct run is not supported; wrap the agent in a graph)")
		return
	}

	def, err := s.graphs.GetGraph(ctx, in.GraphID)
	if err != nil || def == nil || !twinGraphWritable(def, caller) {
		writeTwinError(w, http.StatusNotFound, "graph not found")
		return
	}

	// webhook 提交闸：白名单策略校验（默认档=内网限定由拨号闸兜底）。
	if in.WebhookURL != "" {
		if err := s.webhookPolicyOf().allows(in.WebhookURL); err != nil {
			writeTwinError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	execID := uuid.NewString()
	sessionID := "twin-" + caller.Name + "-" + uuid.NewString()

	// 注册 webhook 订阅（先于执行启动，保证 run.started 先于节点事件到达）。
	if in.WebhookURL != "" {
		secret := in.WebhookSecret
		if secret == "" {
			secret = twinRandomSecret()
		}
		s.registerSub(execID, def, in.WebhookURL, secret)
		// run.started 同步投递，保证事件次序。
		s.postEvent(execID, "run.started", map[string]any{
			"trace_url":   "",
			"total_nodes": len(def.Nodes),
		})
	}

	initialState := map[string]any{}
	for k, v := range in.Params {
		initialState[k] = v
	}
	if in.Input != "" {
		initialState["input"] = in.Input
	}

	if _, err := s.graphs.ExecuteGraph(ctx, in.GraphID, sessionID, execID, initialState); err != nil {
		if in.WebhookURL != "" {
			s.postEvent(execID, "run.failed", map[string]any{"error_message": err.Error()})
			s.unregisterSub(execID)
		}
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if key := strings.TrimSpace(in.IdempotencyKey); key != "" {
		if store := s.idemStore(); store != nil {
			if err := store.SaveRunID(ctx, caller.Name, key, execID, in.GraphID); err != nil {
				s.lg.Warn("twinopenapi 幂等键保存失败（本次提交已生效，重启窗口内重试可能重复执行）",
					loggateway.StepID("twinopenapi.idem_save_fail"), loggateway.Err(err))
			}
		} else {
			s.mu.Lock()
			s.idempot[caller.Name+"|"+key] = execID
			s.mu.Unlock()
		}
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"run_id": execID})
}

// twinRunStatus aranea 执行状态 → 机器面任务状态视图。
func twinRunStatus(status string) string {
	if status == string(biz.GraphExecWaitingHuman) {
		return "waiting_approval"
	}
	return status
}

// twinRunView 组装 run 详情视图（list/get 共用形状）。
func (s *TwinOpenAPICompatService) twinRunView(ctx context.Context, exec *biz.GraphExecution) map[string]any {
	nodeOutputs, _ := exec.CurrentState["node_responses"].(map[string]any)
	runOutput, _ := exec.CurrentState["output"].(string)
	if runOutput == "" {
		runOutput, _ = exec.CurrentState["last_response"].(string)
	}
	nodes := make([]map[string]any, 0, len(exec.Steps))
	sub := s.getSub(exec.ID)
	for _, st := range exec.Steps {
		n := map[string]any{
			"node_id":   st.NodeID,
			"node_name": st.NodeID,
			"node_type": "",
			"status":    st.Status,
		}
		if sub != nil {
			if meta, ok := sub.Nodes[st.NodeID]; ok {
				n["node_name"] = meta.Name
				n["node_type"] = meta.Type
			}
		}
		if st.Error != "" {
			n["error_message"] = st.Error
		}
		if v, ok := nodeOutputs[st.NodeID]; ok {
			if text, ok := v.(string); ok && text != "" {
				n["output_summary"] = text
			}
		}
		nodes = append(nodes, n)
	}
	var durationMs int64
	if exec.FinishedAt != nil {
		durationMs = exec.FinishedAt.Sub(exec.StartedAt).Milliseconds()
	} else {
		durationMs = time.Since(exec.StartedAt).Milliseconds()
	}
	return map[string]any{
		"run_id":        exec.ID,
		"graph_id":      exec.GraphID,
		"status":        twinRunStatus(exec.GetStatus()),
		"started_at":    exec.StartedAt.UTC().Format(time.RFC3339),
		"finished_at":   twinFinishedAtRFC3339(exec),
		"output":        runOutput,
		"error_message": exec.ErrorMessage,
		"nodes":         nodes,
		"duration_ms":   durationMs,
	}
}

// twinFinishedAtRFC3339 终态时间格式化（未终态返回空串）。
func twinFinishedAtRFC3339(exec *biz.GraphExecution) string {
	if exec.FinishedAt == nil {
		return ""
	}
	return exec.FinishedAt.UTC().Format(time.RFC3339)
}

// twinRunGraphAccessible 校验 run 所属图对 caller 可写（租户门禁共用件）：
// 通过 exec→graph 反查归属，系统级 caller 直通。
func (s *TwinOpenAPICompatService) twinRunGraphAccessible(ctx context.Context, exec *biz.GraphExecution, caller *twinCaller) bool {
	if caller.isSystem() || exec == nil {
		return exec != nil
	}
	def, err := s.graphs.GetGraph(ctx, exec.GraphID)
	if err != nil || def == nil {
		return false
	}
	return twinGraphWritable(def, caller)
}

// handleListRuns 机器面 run 列表（P0 补齐的已知缺口：此前消费方被迫绕行
// JWT 交互面 /v1/sessions 做全局枚举）。graph_id 必填（按图归属做租户门禁；
// 消费方从 GET /api/v1/graphs 获得自有图清单后逐图枚举）。
func (s *TwinOpenAPICompatService) handleListRuns(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	q := r.URL.Query()
	graphID := strings.TrimSpace(q.Get("graph_id"))
	if graphID == "" {
		writeTwinError(w, http.StatusBadRequest, "graph_id is required")
		return
	}
	def, err := s.graphs.GetGraph(r.Context(), graphID)
	if err != nil || def == nil || !twinGraphWritable(def, caller) {
		writeTwinError(w, http.StatusNotFound, "graph not found")
		return
	}
	pageSize := 50
	if v := strings.TrimSpace(q.Get("page_size")); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 && n <= 200 {
			pageSize = n
		}
	}
	opt := biz.GraphRunListOption{Status: strings.TrimSpace(q.Get("status"))}
	execs, nextToken, err := s.graphs.ListExecutions(r.Context(), graphID, pageSize, strings.TrimSpace(q.Get("cursor")), opt)
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(execs))
	for _, exec := range execs {
		items = append(items, map[string]any{
			"run_id":      exec.ID,
			"graph_id":    exec.GraphID,
			"status":      twinRunStatus(exec.GetStatus()),
			"started_at":  exec.StartedAt.UTC().Format(time.RFC3339),
			"finished_at": twinFinishedAtRFC3339(exec),
		})
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nextToken})
}

func (s *TwinOpenAPICompatService) handleGetRun(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	exec, err := s.graphs.GetExecution(r.Context(), r.PathValue("id"))
	if err != nil || exec == nil || !s.twinRunGraphAccessible(r.Context(), exec, caller) {
		writeTwinError(w, http.StatusNotFound, "run not found")
		return
	}
	view := s.twinRunView(r.Context(), exec)
	// G2：从用量表聚合本运行的 model/tokens（执行时按 "twin-" 会话落账）。
	modelUsed, tokIn, tokOut := s.runTelemetry(r.Context(), exec)
	view["model_used"] = modelUsed
	view["tokens_input"] = tokIn
	view["tokens_output"] = tokOut
	view["trace_url"] = ""
	writeTwinJSON(w, http.StatusOK, view)
}

// runTelemetry 聚合一次运行的用量遥测：twin 运行落账的 graph_run 行
// 按运行求和，model_used 取输入+输出 token 量最大的模型。无用量数据或
// 查询失败时返回零值（前端 resolveAraneaTraceUrl 对空 model 有兜底，
// tokens=0 与旧版硬编码行为一致）。
//
// 查询键取 exec.ID（execID）：落账行 SessionID=execID（对齐框架 runner
// 真实 session 行），SpiritSessionID/SessionID（"twin-<caller>-<uuid>"）仅作
// 归属判定。日期范围必须显式给出——usage.Usecase.Events 的 normalizeQuery
// 对空 StartDate/EndDate 默认只查最近 30 天，历史运行的遥测会被静默
// 清零，故从运行起始日查到今天。
func (s *TwinOpenAPICompatService) runTelemetry(ctx context.Context, exec *biz.GraphExecution) (modelUsed string, tokIn, tokOut int) {
	if s.usageUC == nil || exec == nil || strings.TrimSpace(exec.ID) == "" {
		return "", 0, 0
	}
	execID := strings.TrimSpace(exec.ID)
	startDate := time.Now().Format("2006-01-02")
	if !exec.StartedAt.IsZero() {
		startDate = exec.StartedAt.Format("2006-01-02")
	}
	events, err := s.usageUC.Events(ctx, usage.Query{
		SessionID: execID,
		UsageKind: KindGraphRun,
		StartDate: startDate,
		EndDate:   time.Now().Format("2006-01-02"),
		Limit:     200,
	})
	if err != nil {
		s.lg.Warn("run 遥测用量查询失败",
			loggateway.StepID("twinopenapi.run_telemetry_fail"),
			loggateway.Err(err),
			loggateway.Str("execution_id", execID),
		)
		return "", 0, 0
	}
	// 按模型聚合，取 token 量最大的模型作为 model_used。
	type sum struct{ in, out int }
	byModel := map[string]*sum{}
	for _, e := range events {
		tokIn += e.InputTokens
		tokOut += e.OutputTokens
		m := strings.TrimSpace(e.ModelDisplayName)
		if m == "" {
			m = strings.TrimSpace(e.ModelAPIID)
		}
		agg := byModel[m]
		if agg == nil {
			agg = &sum{}
			byModel[m] = agg
		}
		agg.in += e.InputTokens
		agg.out += e.OutputTokens
	}
	best := -1
	for m, agg := range byModel {
		if tot := agg.in + agg.out; tot > best {
			best = tot
			modelUsed = m
		}
	}
	return modelUsed, tokIn, tokOut
}

func (s *TwinOpenAPICompatService) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	exec, err := s.graphs.GetExecution(r.Context(), r.PathValue("id"))
	if err != nil || exec == nil || !s.twinRunGraphAccessible(r.Context(), exec, caller) {
		writeTwinError(w, http.StatusNotFound, "run not found")
		return
	}
	if err := s.graphs.CancelExecution(r.Context(), r.PathValue("id")); err != nil {
		writeTwinError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// twinResumeDecision 对齐 twinmonitor ResumeDecision。
type twinResumeDecision struct {
	Approved   bool   `json:"approved"`
	Comment    string `json:"comment"`
	ApproverID uint32 `json:"approver_id"`
}

func (s *TwinOpenAPICompatService) handleResumeInterrupt(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	runID := r.PathValue("id")
	interruptID := r.PathValue("interrupt_id")
	var in twinResumeDecision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	exec, err := s.graphs.GetExecution(r.Context(), runID)
	if err != nil || exec == nil || !s.twinRunGraphAccessible(r.Context(), exec, caller) {
		writeTwinError(w, http.StatusNotFound, "run not found")
		return
	}
	// interrupt_id 约定为中断节点 ID（与 MarkTeamGraphInterrupt 的节点键一致）。
	if node := exec.GetInterruptNode(); node != "" && interruptID != "" && node != interruptID {
		writeTwinError(w, http.StatusBadRequest, "interrupt_id mismatch: run is interrupted at node "+node)
		return
	}
	if _, err := s.graphs.ResumeExecution(r.Context(), runID, map[string]any{
		"approved":     in.Approved,
		"comment":      in.Comment,
		"approver_id":  in.ApproverID,
		"interrupt_id": interruptID,
	}); err != nil {
		writeTwinError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// 记忆与可观测（实现见 twin_openapi_compat_part2.go）
// ---------------------------------------------------------------------------

// twinMemoryFact 对齐 twinmonitor MemoryFact（L3 全局处置经验回写）。
type twinMemoryFact struct {
	Scope    string         `json:"scope"` // global/agent
	Key      string         `json:"key"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

// 以下门面组成部分的实现见 twin_openapi_compat_part2.go（AS-COG-01 拆分）：
//   - 记忆/可观测端点：handleWriteMemoryFact、handleMemoryFactPendingDecision、
//     handleQuotaUsage、handleAgentMetrics；
//   - GraphRunEventSink 事件桥接（OnNodeStarted/OnNodeCompleted/OnRunOutput/
//     OnRunCompleted/OnRunFailed/OnRunCancelled/OnRunWaitingApproval → webhook）；
//   - 图运行用量落账（KindGraphRun、twinGraphUsageRecorder、
//     mergeGraphRunSessionMeta；"twin-<caller>-" 前缀归属判定）；
//   - 订阅注册表（registerSub/unregisterSub/getSub/gcSubsLocked、twinSubTTL）
//     与 HMAC 签名投递（postEvent/postEventAsync、twinEventEnvelope）；
//   - 工具函数（twinRandomSecret、readTwinBody、writeTwinJSON、writeTwinError）。

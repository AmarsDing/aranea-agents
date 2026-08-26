// Package service — twin_openapi_compat.go
//
// TwinOpenAPICompatService 是 twinmonitor（AI 运维治理面）对接 aranea 的
// OpenAPI 兼容门面：将 twinmonitor AraneaClient 期望的 /api/v1/* 契约映射到
// aranea 内部 Usecase 能力（agents/graphs/runs/memory/quota/metrics），
// 并实现 biz.GraphRunEventSink 把图执行生命周期事件以 HMAC 签名 webhook
// 回传 twinmonitor，形成「监控 → 分析 → 执行 → 回写」闭环。
//
// 鉴权：独立于 JWT 用户体系，使用机器对机器 Bearer token
// （环境变量 ARANEA_TWINOPENAPI_TOKEN；未配置时门面禁用，路由不注册）。
// 路由前缀 /api/v1/ 下的门面子树在 server 层注册为 noAuth（由本服务自校验），
// 不影响 /api/v1/admin/* 既有 JWT 保护。
package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	"aranea-agents/pkg/outboundwebhook"
	"aranea-agents/pkg/safego"
)

// TwinOpenAPITokenEnv 是门面机器 token 的环境变量名。
const TwinOpenAPITokenEnv = "ARANEA_TWINOPENAPI_TOKEN"

// twinRunSub 单次执行的 webhook 订阅（内存态；进程重启后由 twinmonitor
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

// TwinOpenAPICompatService twinmonitor OpenAPI 兼容门面。
// 同时实现 biz.GraphRunEventSink（构造时自注册到 GraphExecutionUsecase）。
type TwinOpenAPICompatService struct {
	agents    *biz.AgentUsecase
	graphs    *biz.GraphUsecase
	memory    *biz.MemoryAdminUsecase
	usageUC   *usage.Usecase
	monitorUC *monitor.Usecase
	llmModels *biz.LlmProviderModelUsecase
	lg        loggateway.Logger

	token   string
	mux     *http.ServeMux
	startAt time.Time
	hc      *http.Client

	mu      sync.Mutex
	subs    map[string]*twinRunSub
	idempot map[string]string // idempotency_key → run_id
}

// NewTwinOpenAPICompatService 创建门面并向 GraphExecutionUsecase 注册事件水槽。
// token 取自环境变量 ARANEA_TWINOPENAPI_TOKEN；为空时 Enabled() 返回 false。
func NewTwinOpenAPICompatService(
	agents *biz.AgentUsecase,
	graphs *biz.GraphUsecase,
	memory *biz.MemoryAdminUsecase,
	usageUC *usage.Usecase,
	monitorUC *monitor.Usecase,
	llmModels *biz.LlmProviderModelUsecase,
	lg loggateway.Logger,
) *TwinOpenAPICompatService {
	s := &TwinOpenAPICompatService{
		agents:    agents,
		graphs:    graphs,
		memory:    memory,
		usageUC:   usageUC,
		monitorUC: monitorUC,
		llmModels: llmModels,
		lg:        lg,
		token:     strings.TrimSpace(os.Getenv(TwinOpenAPITokenEnv)),
		startAt:   time.Now(),
		hc:        &http.Client{Timeout: 10 * time.Second},
		subs:      make(map[string]*twinRunSub),
		idempot:   make(map[string]string),
	}
	s.routes()
	if graphs != nil && graphs.ExecUC() != nil {
		graphs.ExecUC().SetRunEventSink(s)
	}
	if s.Enabled() {
		lg.Info("twinmonitor OpenAPI 兼容门面已启用",
			loggateway.StepID("twinopenapi.enabled"), loggateway.Str("path", s.Path()))
	} else {
		lg.Warn("twinmonitor OpenAPI 兼容门面未启用（缺少 "+TwinOpenAPITokenEnv+"）",
			loggateway.StepID("twinopenapi.disabled"))
	}
	return s
}

// Enabled 报告门面是否可用（token 已配置）。
func (s *TwinOpenAPICompatService) Enabled() bool { return s != nil && s.token != "" }

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
	mux.HandleFunc("POST /api/v1/runs", s.guard(s.handleCreateRun))
	mux.HandleFunc("GET /api/v1/runs/{id}", s.guard(s.handleGetRun))
	mux.HandleFunc("POST /api/v1/runs/{id}/cancel", s.guard(s.handleCancelRun))
	mux.HandleFunc("POST /api/v1/runs/{id}/interrupts/{interrupt_id}/resume", s.guard(s.handleResumeInterrupt))
	mux.HandleFunc("POST /api/v1/memory/facts", s.guard(s.handleWriteMemoryFact))
	mux.HandleFunc("GET /api/v1/quota/usage", s.guard(s.handleQuotaUsage))
	mux.HandleFunc("GET /api/v1/metrics/agents", s.guard(s.handleAgentMetrics))
	s.mux = mux
}

// guard 机器 token 校验（常量时间比较防时序侧信道）。
func (s *TwinOpenAPICompatService) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		token, found := strings.CutPrefix(authz, "Bearer ")
		if !found || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(s.token)) != 1 {
			writeTwinError(w, http.StatusUnauthorized, "invalid api token")
			return
		}
		next(w, r)
	}
}

// ---------------------------------------------------------------------------
// 健康与清单
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

func (s *TwinOpenAPICompatService) handleListAgents(w http.ResponseWriter, r *http.Request) {
	res, err := s.agents.List(r.Context(), biz.AgentListQuery{Limit: 1000})
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 注：Agent 清单保持全量返回（不做 ?source= 命名空间过滤）——twin 种子同步
	// 按名匹配的 12 个预设 Agent 实为 aranea 侧 pkg 预设资产，无 twin 标签载体
	// （agents 表无 metadata_json 列，config_json 被 tools/skills 投影占用）；
	// 且 Agent 侧无覆盖写路径，全量可见无正确性风险。命名空间隔离仅落 Graph 侧。
	items := make([]map[string]any, 0, len(res.Items))
	for _, a := range res.Items {
		items = append(items, map[string]any{
			"id":          a.ID,
			"name":        a.DisplayName,
			"description": a.AgentDescription,
			"definition":  twinAgentDefinition(a),
		})
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"items": items})
}

// twinAgentDefinition 组装漂移比对用的 Agent 定义视图。
func twinAgentDefinition(a biz.Agent) map[string]any {
	def := map[string]any{
		"agent_key": a.AgentKey,
		"provider":  a.Provider,
		"model":     a.Model,
		"roles":     a.Roles,
		"status":    a.Status,
	}
	if a.ConfigJSON != "" {
		var cfg map[string]any
		if json.Unmarshal([]byte(a.ConfigJSON), &cfg) == nil {
			def["config"] = cfg
		}
	}
	return def
}

// twinAgentInput twinmonitor Agent 种子同步载荷（对齐 biz.AraneaAgentInput）。
type twinAgentInput struct {
	Name          string         `json:"name"`
	Role          string         `json:"role"`
	Description   string         `json:"description"`
	SystemPrompt  string         `json:"system_prompt"`
	ModelPolicy   string         `json:"model_policy"`
	Temperature   float64        `json:"temperature"`
	ToolWhitelist []string       `json:"tool_whitelist"`
	Tags          []string       `json:"tags"`
	Metadata      map[string]any `json:"metadata"`
}

func (s *TwinOpenAPICompatService) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var in twinAgentInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeTwinError(w, http.StatusBadRequest, "name is required")
		return
	}
	meta, _ := json.Marshal(map[string]any{
		"source":         "twinmonitor",
		"model_policy":   in.ModelPolicy,
		"temperature":    in.Temperature,
		"tool_whitelist": in.ToolWhitelist,
		"tags":           in.Tags,
		"metadata":       in.Metadata,
	})
	agent := biz.Agent{
		DisplayName:      in.Name,
		AgentDescription: in.Description,
		MetadataJSON:     string(meta),
	}
	if in.Role != "" {
		agent.Roles = []string{in.Role}
	}
	var files []biz.AgentPromptFile
	if strings.TrimSpace(in.SystemPrompt) != "" {
		files = append(files, biz.AgentPromptFile{Name: "system.md", Body: in.SystemPrompt})
	}
	created, err := s.agents.CreateWithFilesAndSettings(r.Context(), agent, files, nil)
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"id": created.ID})
}

func (s *TwinOpenAPICompatService) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in twinAgentInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	patch := biz.Agent{
		DisplayName:      in.Name,
		AgentDescription: in.Description,
	}
	if in.Role != "" {
		patch.Roles = []string{in.Role}
	}
	if _, err := s.agents.Update(r.Context(), id, patch); err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if strings.TrimSpace(in.SystemPrompt) != "" {
		// 幂等替换 system.md：存在则更新，否则新建。
		current, err := s.agents.Get(r.Context(), id)
		if err == nil {
			replaced := false
			for _, f := range current.Files {
				if f.Name == "system.md" {
					_, _ = s.agents.UpdatePromptFile(r.Context(), biz.AgentPromptFile{
						ID: f.ID, AgentID: id, Name: "system.md", Body: in.SystemPrompt,
					})
					replaced = true
					break
				}
			}
			if !replaced {
				_, _ = s.agents.CreatePromptFile(r.Context(), biz.AgentPromptFile{
					AgentID: id, Name: "system.md", Body: in.SystemPrompt,
				})
			}
		}
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Graph 校验与导入
// ---------------------------------------------------------------------------

// twinGraphView 图清单/详情视图（definition 为原始定义，供漂移比对与执行参数推断）。
func twinGraphView(def *biz.GraphDefinition) map[string]any {
	nodes := make([]map[string]any, 0, len(def.Nodes))
	for _, n := range def.Nodes {
		nodes = append(nodes, map[string]any{
			"id":   n.ID,
			"type": n.Type,
			"name": twinNodeDisplayName(n),
		})
	}
	return map[string]any{
		"id":      def.ID,
		"name":    def.Name,
		"ref":     def.Name, // graph 引用键：aranea 以名称作为稳定 ref
		"version": fmt.Sprintf("%d", def.Version),
		"definition": map[string]any{
			"description": def.Description,
			"nodes":       nodes,
			"entry_point": def.EntryPoint,
			"metadata":    def.Metadata,
		},
	}
}

// twinNodeDisplayName 节点显示名（描述优先，回退 ID）。
func twinNodeDisplayName(n biz.NodeDef) string {
	if s := strings.TrimSpace(n.Description); s != "" {
		return s
	}
	if s := strings.TrimSpace(n.AgentName); s != "" {
		return s
	}
	return n.ID
}

func (s *TwinOpenAPICompatService) handleListGraphs(w http.ResponseWriter, r *http.Request) {
	defs, _, err := s.graphs.ListGraphs(r.Context(), 1000, "")
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	items := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		if source != "" && !twinGraphInNamespace(d.Metadata, source) {
			continue
		}
		items = append(items, twinGraphView(d))
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"items": items})
}

// twinSourceTag P1 图命名空间隔离：经本门面创建/更新的 Graph 强制打标
// metadata.source=twinmonitor，清单接口支持 ?source= 过滤，
// 使 twinmonitor 同步/解析只见自己的命名空间（Agent 侧无持久化标签载体，不过滤）。
const twinSourceTag = "twinmonitor"

// twinGraphInNamespace 判定 Graph 是否属于指定来源命名空间（P1 命名空间隔离）。
// 命中条件：metadata.source 一致；或查询 twinmonitor 命名空间时带有 twin_seed 种子戳
// （兼容打标机制上线前已注册的存量种子图，其首次漂移更新会被补打 source 标）。
func twinGraphInNamespace(meta map[string]any, source string) bool {
	if meta == nil {
		return false
	}
	if s, _ := meta["source"].(string); s == source {
		return true
	}
	if source == twinSourceTag {
		_, ok := meta["twin_seed"].(map[string]any)
		return ok
	}
	return false
}

// ensureTwinGraphSource 强制打标 Graph 命名空间（合并保留 twin_seed 等既有键）。
func ensureTwinGraphSource(def *biz.GraphDefinition) {
	if def.Metadata == nil {
		def.Metadata = map[string]any{}
	}
	def.Metadata["source"] = twinSourceTag
}

func (s *TwinOpenAPICompatService) handleGetGraph(w http.ResponseWriter, r *http.Request) {
	def, err := s.graphs.GetGraph(r.Context(), r.PathValue("id"))
	if err != nil || def == nil {
		writeTwinError(w, http.StatusNotFound, "graph not found")
		return
	}
	writeTwinJSON(w, http.StatusOK, twinGraphView(def))
}

// handleCreateGraph 导入 Graph JSON（场景导入注册；body 为 GraphDefinition JSON）。
func (s *TwinOpenAPICompatService) handleCreateGraph(w http.ResponseWriter, r *http.Request) {
	body, err := readTwinBody(w, r, 4<<20)
	if err != nil {
		writeTwinError(w, http.StatusBadRequest, err.Error())
		return
	}
	var def biz.GraphDefinition
	if err := json.Unmarshal(body, &def); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid graph json: "+err.Error())
		return
	}
	ensureTwinGraphSource(&def)
	created, err := s.graphs.CreateGraph(r.Context(), &def)
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"id": created.ID})
}

// handleUpdateGraph 原位更新 Graph 定义（twin 种子漂移更新用；ID 不变，版本历史追加）。
// 名称保护：ref=name 是 twin 侧种子同步的匹配键，改名会导致下次同步重复建图，故拒绝改名。
func (s *TwinOpenAPICompatService) handleUpdateGraph(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.graphs.GetGraph(r.Context(), id)
	if err != nil || existing == nil {
		writeTwinError(w, http.StatusNotFound, "graph not found")
		return
	}
	body, err := readTwinBody(w, r, 4<<20)
	if err != nil {
		writeTwinError(w, http.StatusBadRequest, err.Error())
		return
	}
	var def biz.GraphDefinition
	if err := json.Unmarshal(body, &def); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid graph json: "+err.Error())
		return
	}
	if def.Name != "" && def.Name != existing.Name {
		writeTwinError(w, http.StatusBadRequest, "graph name is immutable via this API (ref=name 同步口径)")
		return
	}
	def.ID = id
	def.Name = existing.Name
	ensureTwinGraphSource(&def)
	saved, err := s.graphs.UpdateGraph(r.Context(), &def)
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, twinGraphView(saved))
}

// ---------------------------------------------------------------------------
// 执行（Run）
// ---------------------------------------------------------------------------

// twinRunInput 对齐 twinmonitor RunInput。
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
	var in twinRunInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	ctx := r.Context()

	// 幂等键命中直接返回原 run_id。
	if key := strings.TrimSpace(in.IdempotencyKey); key != "" {
		s.mu.Lock()
		if runID, ok := s.idempot[key]; ok {
			s.mu.Unlock()
			writeTwinJSON(w, http.StatusOK, map[string]any{"run_id": runID})
			return
		}
		s.mu.Unlock()
	}

	if strings.TrimSpace(in.GraphID) == "" {
		// Agent 直跑（无 Graph 包装）在 v1 不支持：agent 运行时无执行记录
		// 实体，无法支撑状态轮询/取消/审批的闭环语义。
		writeTwinError(w, http.StatusBadRequest, "graph_id is required (agent direct run is not supported; wrap the agent in a graph)")
		return
	}

	def, err := s.graphs.GetGraph(ctx, in.GraphID)
	if err != nil || def == nil {
		writeTwinError(w, http.StatusNotFound, "graph not found")
		return
	}

	execID := uuid.NewString()
	sessionID := "twin-" + uuid.NewString()

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
		s.mu.Lock()
		s.idempot[key] = execID
		s.mu.Unlock()
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"run_id": execID})
}

// twinRunStatus aranea 执行状态 → twinmonitor 任务状态视图。
func twinRunStatus(status string) string {
	if status == string(biz.GraphExecWaitingHuman) {
		return "waiting_approval"
	}
	return status
}

func (s *TwinOpenAPICompatService) handleGetRun(w http.ResponseWriter, r *http.Request) {
	exec, err := s.graphs.GetExecution(r.Context(), r.PathValue("id"))
	if err != nil || exec == nil {
		writeTwinError(w, http.StatusNotFound, "run not found")
		return
	}
	status := twinRunStatus(exec.GetStatus())
	// 终态 state（current_state_json）提取节点输出/运行输出，
	// 供 twinmonitor 兜底轮询补齐 output_summary（与 webhook 路径一致）。
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
	writeTwinJSON(w, http.StatusOK, map[string]any{
		"run_id":        exec.ID,
		"graph_id":      exec.GraphID,
		"status":        status,
		"output":        runOutput,
		"error_message": exec.ErrorMessage,
		"nodes":         nodes,
		"model_used":    "",
		"tokens_input":  0,
		"tokens_output": 0,
		"duration_ms":   durationMs,
		"trace_url":     "",
	})
}

func (s *TwinOpenAPICompatService) handleCancelRun(w http.ResponseWriter, r *http.Request) {
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
	runID := r.PathValue("id")
	interruptID := r.PathValue("interrupt_id")
	var in twinResumeDecision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	exec, err := s.graphs.GetExecution(r.Context(), runID)
	if err != nil || exec == nil {
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
// 记忆与可观测
// ---------------------------------------------------------------------------

// twinMemoryFact 对齐 twinmonitor MemoryFact（L3 全局处置经验回写）。
type twinMemoryFact struct {
	Scope    string         `json:"scope"` // global/agent
	Key      string         `json:"key"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

func (s *TwinOpenAPICompatService) handleWriteMemoryFact(w http.ResponseWriter, r *http.Request) {
	var in twinMemoryFact
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(in.Key) == "" || strings.TrimSpace(in.Content) == "" {
		writeTwinError(w, http.StatusBadRequest, "key and content are required")
		return
	}
	scopeType := "global"
	scopeID := "global"
	if strings.TrimSpace(in.Scope) == "agent" {
		scopeType = "agent"
		scopeID = in.Key
	}
	metaJSON, _ := json.Marshal(in.Metadata)
	if _, err := s.memory.UpsertFactRow(r.Context(), biz.FactUpsert{
		ScopeType:    scopeType,
		ScopeID:      scopeID,
		Statement:    in.Content,
		Fingerprint:  in.Key,
		FactKind:     "reference",
		SourceKind:   "twinmonitor",
		MetadataJSON: string(metaJSON),
	}); err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *TwinOpenAPICompatService) handleQuotaUsage(w http.ResponseWriter, r *http.Request) {
	ov, err := s.usageUC.Overview(r.Context(), usage.Query{})
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{
		"tokens_input_total":  ov.Month.InputTokens,
		"tokens_output_total": ov.Month.OutputTokens,
		"cost_total":          float64(ov.Month.TotalCostMicroUSD) / 1e6,
	})
}

func (s *TwinOpenAPICompatService) handleAgentMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := s.monitorUC.GetRunnerMetrics(r.Context(), 24*60)
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{
		"total_runs":      m.TotalRuns,
		"success_count":   m.TotalRuns - m.ErrorRuns,
		"failed_count":    m.ErrorRuns,
		"success_rate":    m.SuccessRate,
		"p95_duration_ms": int64(m.P95DurationMs),
	})
}

// ---------------------------------------------------------------------------
// GraphRunEventSink 实现（事件桥接 → twinmonitor webhook）
// ---------------------------------------------------------------------------

func (s *TwinOpenAPICompatService) OnNodeStarted(_ context.Context, execID, graphID, nodeID string, step int) {
	sub := s.getSub(execID)
	if sub == nil {
		return
	}
	s.mu.Lock()
	sub.NodeStart[nodeID] = time.Now()
	s.mu.Unlock()
	meta := sub.Nodes[nodeID]
	s.postEventAsync(execID, "run.node_started", map[string]any{
		"node_id":   nodeID,
		"node_name": meta.Name,
		"node_type": meta.Type,
		"sequence":  step,
	})
}

func (s *TwinOpenAPICompatService) OnNodeCompleted(_ context.Context, execID, graphID, nodeID string, step int, status, errMsg string) {
	sub := s.getSub(execID)
	if sub == nil {
		return
	}
	s.mu.Lock()
	started, ok := sub.NodeStart[nodeID]
	delete(sub.NodeStart, nodeID)
	s.mu.Unlock()
	var durationMs int64
	if ok {
		durationMs = time.Since(started).Milliseconds()
	}
	meta := sub.Nodes[nodeID]
	payload := map[string]any{
		"node_id":     nodeID,
		"node_name":   meta.Name,
		"node_type":   meta.Type,
		"sequence":    step,
		"status":      status,
		"duration_ms": durationMs,
	}
	if errMsg != "" {
		payload["error_message"] = errMsg
	}
	s.postEventAsync(execID, "run.node_completed", payload)
}

func (s *TwinOpenAPICompatService) OnRunWaitingApproval(_ context.Context, execID, graphID, nodeID string) {
	sub := s.getSub(execID)
	if sub == nil {
		return
	}
	meta := sub.Nodes[nodeID]
	s.postEventAsync(execID, "run.waiting_approval", map[string]any{
		"node_id":      nodeID,
		"interrupt_id": nodeID, // 与 resume 端点的 interrupt_id 约定一致
		"node_name":    meta.Name,
	})
}

// OnRunOutput GraphRunEventSinkOutput 扩展实现：先逐节点同步投递
// run.node_output（aiops 据此回写 ai_task_nodes.output_summary），
// 并把运行级 output 暂存到订阅，供紧随其后的 run.completed 携带。
// 同步投递保证事件顺序：node_output 先于 run.completed 到达。
func (s *TwinOpenAPICompatService) OnRunOutput(_ context.Context, execID, graphID, output string, nodeOutputs map[string]string) {
	sub := s.getSub(execID)
	if sub == nil {
		return
	}
	for nodeID, text := range nodeOutputs {
		meta := sub.Nodes[nodeID]
		s.postEvent(execID, "run.node_output", map[string]any{
			"node_id":        nodeID,
			"node_name":      meta.Name,
			"node_type":      meta.Type,
			"output_summary": text,
		})
	}
	s.mu.Lock()
	sub.FinalOutput = output
	s.mu.Unlock()
}

func (s *TwinOpenAPICompatService) OnRunCompleted(_ context.Context, execID, graphID string, durationMs int64) {
	sub := s.getSub(execID)
	if sub == nil {
		return
	}
	s.mu.Lock()
	output := sub.FinalOutput
	s.mu.Unlock()
	s.postEvent(execID, "run.completed", map[string]any{
		"output":      output,
		"duration_ms": durationMs,
	})
	s.unregisterSub(execID)
}

func (s *TwinOpenAPICompatService) OnRunFailed(_ context.Context, execID, graphID, errMsg string) {
	if s.getSub(execID) == nil {
		return
	}
	s.postEvent(execID, "run.failed", map[string]any{"error_message": errMsg})
	s.unregisterSub(execID)
}

func (s *TwinOpenAPICompatService) OnRunCancelled(_ context.Context, execID, graphID string) {
	if s.getSub(execID) == nil {
		return
	}
	s.postEvent(execID, "run.cancelled", map[string]any{})
	s.unregisterSub(execID)
}

// ---------------------------------------------------------------------------
// 订阅注册表与事件投递
// ---------------------------------------------------------------------------

// twinSubTTL 订阅最大保留时长（超时未终态的订阅由 GC 清理，防泄漏）。
const twinSubTTL = 24 * time.Hour

func (s *TwinOpenAPICompatService) registerSub(execID string, def *biz.GraphDefinition, webhookURL, secret string) {
	sub := &twinRunSub{
		WebhookURL: webhookURL,
		Secret:     secret,
		GraphID:    def.ID,
		CreatedAt:  time.Now(),
		StartedAt:  time.Now(),
		Nodes:      make(map[string]twinNodeMeta, len(def.Nodes)),
		NodeStart:  make(map[string]time.Time),
	}
	for _, n := range def.Nodes {
		sub.Nodes[n.ID] = twinNodeMeta{Name: twinNodeDisplayName(n), Type: n.Type}
	}
	s.mu.Lock()
	s.gcSubsLocked()
	s.subs[execID] = sub
	s.mu.Unlock()
}

func (s *TwinOpenAPICompatService) unregisterSub(execID string) {
	s.mu.Lock()
	delete(s.subs, execID)
	s.mu.Unlock()
}

func (s *TwinOpenAPICompatService) getSub(execID string) *twinRunSub {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subs[execID]
}

// gcSubsLocked 惰性清理过期订阅与幂等键（调用方须持锁）。
func (s *TwinOpenAPICompatService) gcSubsLocked() {
	cutoff := time.Now().Add(-twinSubTTL)
	for id, sub := range s.subs {
		if sub.CreatedAt.Before(cutoff) {
			delete(s.subs, id)
		}
	}
}

// twinEventEnvelope twinmonitor WebhookReceiver 统一事件信封（AraneaWebhookEvent）。
type twinEventEnvelope struct {
	EventID    string         `json:"event_id"`
	EventType  string         `json:"event_type"`
	OccurredAt string         `json:"occurred_at"`
	RunID      string         `json:"run_id"`
	GraphID    string         `json:"graph_id"`
	Payload    map[string]any `json:"payload"`
}

// postEventAsync 异步投递（节点/中断事件，不阻塞图执行事件循环）。
func (s *TwinOpenAPICompatService) postEventAsync(execID, eventType string, payload map[string]any) {
	safego.GoBackground("twinopenapi.event", func() {
		s.postEvent(execID, eventType, payload)
	})
}

// postEvent 同步投递 HMAC 签名事件（X-Webhook-Signature: v1=<hex> +
// X-Webhook-Timestamp，对齐 outboundwebhook 签名规范；twinmonitor 侧按此验签）。
func (s *TwinOpenAPICompatService) postEvent(execID, eventType string, payload map[string]any) {
	sub := s.getSub(execID)
	if sub == nil || sub.WebhookURL == "" {
		return
	}
	env := twinEventEnvelope{
		EventID:    uuid.NewString(),
		EventType:  eventType,
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
		RunID:      execID,
		GraphID:    sub.GraphID,
		Payload:    payload,
	}
	body, err := json.Marshal(env)
	if err != nil {
		s.lg.Warn("twinopenapi 事件序列化失败",
			loggateway.StepID("twinopenapi.marshal_fail"), loggateway.Err(err))
		return
	}
	req, err := http.NewRequest(http.MethodPost, sub.WebhookURL, strings.NewReader(string(body)))
	if err != nil {
		s.lg.Warn("twinopenapi 事件请求构建失败",
			loggateway.StepID("twinopenapi.request_fail"), loggateway.Err(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Aranea-TwinOpenAPI-Bridge/1.0")
	outboundwebhook.AddSignatureHeaders(req, sub.Secret, body)

	resp, err := s.hc.Do(req)
	if err != nil {
		s.lg.Warn("twinopenapi 事件投递失败",
			loggateway.StepID("twinopenapi.delivery_fail"),
			loggateway.Str("run_id", execID), loggateway.Str("event_type", eventType), loggateway.Err(err))
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		s.lg.Warn("twinopenapi 事件投递非 2xx",
			loggateway.StepID("twinopenapi.delivery_fail"),
			loggateway.Str("run_id", execID), loggateway.Str("event_type", eventType),
			loggateway.Int("status_code", resp.StatusCode))
	}
}

// ---------------------------------------------------------------------------
// 工具函数
// ---------------------------------------------------------------------------

// twinRandomSecret 生成随机 webhook 密钥（twinmonitor 未传 webhook_secret 时兜底）。
func twinRandomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return uuid.NewString()
	}
	return hex.EncodeToString(b)
}

func readTwinBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
}

func writeTwinJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeTwinError 输出裸 JSON 错误（twinmonitor doRaw 按 HTTP 状态码分类错误）。
func writeTwinError(w http.ResponseWriter, status int, msg string) {
	writeTwinJSON(w, status, map[string]any{"error": msg})
}

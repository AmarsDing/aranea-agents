package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundwebhook"
	"aranea-agents/pkg/safego"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

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

// twinMemoryFactPendingDecision 对齐 twinmonitor 审批决议回写（R3 3.3 C7 桥）。
// Decision 为 E4 四档（approve/deny/approve_session/approve_always，3.4）；
// 空时按 Approved 布尔兼容映射（旧审批中心两档）。
type twinMemoryFactPendingDecision struct {
	Approved   bool   `json:"approved"`
	Decision   string `json:"decision"`
	Comment    string `json:"comment"`
	ApproverID uint32 `json:"approver_id"`
}

// handleMemoryFactPendingDecision 审批中心决议回写：approve 档回放原 bi-temporal
// 写，deny 仅置态；approve_session/approve_always 另加 E4 免审授权。Decider
// 内部 fail-closed（MarkDecided 仅 pending 行可迁），未找到/已决议/非法决议
// 一律 404，与 twinmonitor 侧 ai_approvals 状态机对齐。
func (s *TwinOpenAPICompatService) handleMemoryFactPendingDecision(w http.ResponseWriter, r *http.Request) {
	if s.mfpDecider == nil {
		writeTwinError(w, http.StatusServiceUnavailable, "memory fact pending decider not wired")
		return
	}
	var in twinMemoryFactPendingDecision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	decision := in.Decision
	if _, ok := biz.NormalizeMemoryFactDecision(decision); !ok {

		decision = biz.MemoryFactDecisionDeny
		if in.Approved {
			decision = biz.MemoryFactDecisionApprove
		}
	}
	approver := "twinmonitor"
	if in.ApproverID != 0 {
		approver = "twinmonitor:" + strconv.FormatUint(uint64(in.ApproverID), 10)
	}
	applied, replayErr := s.mfpDecider.Decide(r.Context(), r.PathValue("id"), decision, approver, in.Comment)
	if !applied {
		writeTwinError(w, http.StatusNotFound, "pending not found or already decided")
		return
	}
	out := map[string]any{"ok": true}
	if replayErr != nil {

		out["replay_error"] = replayErr.Error()
	}
	writeTwinJSON(w, http.StatusOK, out)
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
		"interrupt_id": nodeID,
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

// KindGraphRun 独立图运行（含本门面触发的 twin 运行）的用量行 UsageKind。
// 计入聚合/配额（billable 过滤仅排除 team_turn），供按会话聚合回填
// handleGetRun 的 model_used/tokens，并让 twin 运行成本可见。
const KindGraphRun = "graph_run"

// twinGraphUsageRecorder 把 biz.GraphUsageRecorder 适配到 usage.Usecase，
// 仅对 twin 会话（"twin-" 前缀）落账——团队运行的图执行已有
// team_member/team_turn 落账，按前缀过滤避免与团队路径双重计费。
type twinGraphUsageRecorder struct {
	usageUC *usage.Usecase
	lg      loggateway.Logger
}

// RecordGraphRunUsage implements biz.GraphUsageRecorder.
func (r *twinGraphUsageRecorder) RecordGraphRunUsage(ctx context.Context, in biz.GraphRunUsageInput) error {
	if r == nil || r.usageUC == nil {
		return nil
	}

	if !strings.HasPrefix(in.SessionID, "twin-") {
		return nil
	}
	status := biz.TokenUsageStatusSuccess
	switch in.Status {
	case "failed":
		status = biz.TokenUsageStatusError
	case "cancelled":
		status = "cancelled"
	}

	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	meta = mergeGraphRunSessionMeta(meta, in.SessionID)
	return r.usageUC.RecordAuxLLMUsage(ctx, usage.AuxLLMUsageInput{
		Kind:          KindGraphRun,
		SessionID:     in.RunID,
		RunID:         in.RunID,
		Model:         in.Model,
		Status:        status,
		PromptTok:     in.PromptTok,
		CompletionTok: in.CompletionTok,
		UsageSource:   in.UsageSource,
		MetadataJSON:  meta,
	})
}

// mergeGraphRunSessionMeta 把 aranea 归属会话（"twin-<uuid>"）写入用量行
// metadata（JSON 合并，保留既有键），供归属反查；非法 JSON 时原样返回
// （RecordAuxLLMUsage 侧有默认 "{}" 兜底，不做静默改写）。
func mergeGraphRunSessionMeta(meta, sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return meta
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(meta), &m); err != nil || m == nil {
		return meta
	}
	m["aranea_session_id"] = sessionID
	b, err := json.Marshal(m)
	if err != nil {
		return meta
	}
	return string(b)
}

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

// memory_fact_pending_notifier.go — R3 Phase 3.3 审批中心推送器。
//
// 记忆高风险写扣留（memory_fact_pending 落库）后，经本推送器向 twinmonitor
// 统一审批中心桥接：POST 签名事件 memory.fact_write_pending 到
// /api/v1/monitor/aiops/webhooks/aranea（复用 ApprovalSourceGraph 桥的
// HMAC-SHA256 v1 签名规范，twinmonitor WebhookReceiver 验签/去重/建单
// ai_approvals source=memory_fact_write）。
//
// 配置（env 常驻）：
//
//	TWIN_WEBHOOK_URL     完整 webhook 地址；空时从 TWIN_GATEWAY_URL
//	                     （默认 http://127.0.0.1:8000）派生标准路径
//	TWIN_WEBHOOK_SECRET  HMAC 密钥（twinmonitor ai_aranea_instances.webhook_secret
//	                     同值）；空 → 推送器禁用（NewTwinMemoryFactPendingNotifier 返回 nil）
//
// best-effort 语义：推送失败仅日志告警，不回滚 pending 落库（audit.py
// pending 积压体检覆盖推送缺口）。
package service

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundwebhook"
)

const (
	// twinWebhookURLEnv 完整 webhook 地址 env（优先于网关派生）。
	twinWebhookURLEnv = "TWIN_WEBHOOK_URL"
	// twinWebhookSecretEnv HMAC 密钥 env（与 twinmonitor 实例配置同值）。
	twinWebhookSecretEnv = "TWIN_WEBHOOK_SECRET"
	// twinGatewayURLEnv 网关地址 env（派生 webhook 路径用；与 twinops 工具同源）。
	twinGatewayURLEnv = "TWIN_GATEWAY_URL"
	// twinWebhookPath twinmonitor aranea 事件回写标准路径。
	twinWebhookPath = "/api/v1/monitor/aiops/webhooks/aranea"
	// twinEventFactWritePending 事件类型（twinmonitor 侧按此路由建审批单）。
	twinEventFactWritePending = "memory.fact_write_pending"
)

// twinMemoryFactPendingNotifier 实现 biz.MemoryFactPendingNotifier。
type twinMemoryFactPendingNotifier struct {
	webhookURL string
	secret     string
	hc         *http.Client
	lg         loggateway.Logger
}

// NewTwinMemoryFactPendingNotifier 从 env 构建审批中心推送器；secret 未配置
// 时返回 nil（推送禁用，pending 落库不受影响）。
func NewTwinMemoryFactPendingNotifier(lg loggateway.Logger) biz.MemoryFactPendingNotifier {
	secret := strings.TrimSpace(os.Getenv(twinWebhookSecretEnv))
	if secret == "" {
		return nil
	}
	url := strings.TrimSpace(os.Getenv(twinWebhookURLEnv))
	if url == "" {
		gateway := strings.TrimRight(strings.TrimSpace(os.Getenv(twinGatewayURLEnv)), "/")
		if gateway == "" {
			gateway = "http://127.0.0.1:8000"
		}
		url = gateway + twinWebhookPath
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &twinMemoryFactPendingNotifier{
		webhookURL: url,
		secret:     secret,
		hc:         &http.Client{Timeout: 10 * time.Second},
		lg:         lg.With(loggateway.Domain("memory_fact_pending_notifier")),
	}
}

// NotifyFactWritePending 投递 memory.fact_write_pending 事件（同步、10s
// 超时；调用方为后台 worker 的批处理循环，单次推送预算可接受）。
// run_id 填 pending record ID——接收器信封校验要求非空，且审批单幂等键
// （interrupt 防重）复用该值。
func (n *twinMemoryFactPendingNotifier) NotifyFactWritePending(ctx context.Context, rec biz.MemoryFactPendingRecord) {
	if n == nil || strings.TrimSpace(rec.ID) == "" {
		return
	}
	payload := map[string]any{
		"pending_id":         rec.ID,
		"agent_id":           rec.AgentID,
		"verdict":            rec.Verdict,
		"fact_key":           rec.FactKey,
		"proposed_body":      rec.ProposedBody,
		"prior_body":         rec.PriorBody,
		"adjudicator_reason": rec.AdjudicatorReason,
		"title":              "记忆写审批[" + rec.Verdict + "]：" + truncatePendingTitleRunes(rec.ProposedBody, 60),
	}
	env := twinEventEnvelope{
		EventID:    uuid.NewString(),
		EventType:  twinEventFactWritePending,
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
		RunID:      rec.ID, // 信封非空约束 + 审批单幂等键（见上）
		Payload:    payload,
	}
	body, err := json.Marshal(env)
	if err != nil {
		n.lg.Warn("memory fact pending notify: marshal failed",
			loggateway.Str("pending_id", rec.ID), loggateway.Err(err))
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, strings.NewReader(string(body)))
	if err != nil {
		n.lg.Warn("memory fact pending notify: request build failed",
			loggateway.Str("pending_id", rec.ID), loggateway.Err(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Aranea-MemoryFactPending-Bridge/1.0")
	outboundwebhook.AddSignatureHeaders(req, n.secret, body)

	resp, err := n.hc.Do(req)
	if err != nil {
		n.lg.Warn("memory fact pending notify: delivery failed (pending row kept)",
			loggateway.Str("pending_id", rec.ID), loggateway.Err(err))
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		n.lg.Warn("memory fact pending notify: non-2xx (pending row kept)",
			loggateway.Str("pending_id", rec.ID), loggateway.Int("status_code", resp.StatusCode))
	}
}

// truncatePendingTitleRunes 截断审批标题（rune 安全，避免多字节截半）。
func truncatePendingTitleRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

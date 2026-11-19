package biz

// D6 L-3 embedding 密级闸（E 阶段 W2b V5，需求 §7.3 验收 #7）：
// L3+ 内容拒绝 cloud embedding 通道并审计；bge 本地 embedding 移植为独立
// 拍板项，过渡期 L3+ 内容不 embed 直接拒（摄取管线侧走词法降级，见
// knowledge.BuildIndexedChunks 对 EmbedClassificationBlockedError 的处理）。
//
// 闸口坐标在 knowledge.embedBatchInternal（全量 embedding 流量唯一网络
// 漏斗）；本文件提供判定实现。分层约束：biz 不 import knowledge（无环），
// knowledge 已 import biz（retriever），故接口定义于此、knowledge 消费。

import (
	"context"

	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// SecopsEmbeddingActionBlocked 是 L3+ 内容被拦于 cloud embedding 通道的
// 审计动作（severity warning，与 mask_reject 同级；detail 只带密级/阈值/
// 通道/批量计数/subject 定位符，绝不携带文本内容——明文三不进）。
const SecopsEmbeddingActionBlocked = "secops.embedding.blocked"

// EmbedGateInfo 是密级闸判定的输入快照（由 knowledge 漏斗构造）。
type EmbedGateInfo struct {
	Provider       string // embedder 提供方（openai/ollama/gemini/huggingface）
	BaseURL        string // 端点（审计观测用；本地性已由 Local 给出）
	Local          bool   // 端点为本地通道（内容不出机；knowledge.IsLocalEmbedTarget）
	Classification int    // 内容密级（0=未标；L1-L5 随 classification_labels）
	Subject        string // 审计定位符（文档 id 等；可空）
	BatchSize      int    // 本批文本条数
}

// EmbedClassificationGate 是 embedding 密级门窄口：返回 nil 放行，非 nil
// 拒绝（knowledge 漏斗包装为 EmbedClassificationBlockedError，零网络流量）。
type EmbedClassificationGate interface {
	CheckEmbed(ctx context.Context, info EmbedGateInfo) error
}

// SecopsEmbeddingGate 是 EmbedClassificationGate 的策略实现：密级 ≥ 模型策略
// 本地阈值且通道非本地 → 拒 + 审计 + 指标；其余放行。nil 门/零值门 = 停用
//（全放行，行为同闸引入前）。
type SecopsEmbeddingGate struct {
	policy *SecopsModelPolicyUsecase
	audit  SecopsOffboardAuditRecorder
	lg     loggateway.Logger
}

// NewSecopsEmbeddingGate 装配密级闸（policy nil = 停用）。
func NewSecopsEmbeddingGate(policy *SecopsModelPolicyUsecase, lg loggateway.Logger) *SecopsEmbeddingGate {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SecopsEmbeddingGate{policy: policy, lg: lg}
}

// SetAuditRecorder 后置注入审计写口（构造签名稳定，wire 友好）。
func (g *SecopsEmbeddingGate) SetAuditRecorder(rec SecopsOffboardAuditRecorder) {
	g.audit = rec
}

// CheckEmbed 判定单次 embedding 批量调用可否离机。
func (g *SecopsEmbeddingGate) CheckEmbed(ctx context.Context, info EmbedGateInfo) error {
	if g == nil || g.policy == nil {
		return nil // 门未装配 = 停用
	}
	if info.Classification <= 0 {
		metrics.SecopsEmbedGateTotal.WithLabelValues("allow").Inc()
		return nil
	}
	// 阈值读取失败保守回落默认值（fail-closed 向：宁可误拦不可放行）。
	threshold := SecopsDefaultLocalThreshold
	p, err := g.policy.GetPolicy(ctx)
	if err != nil {
		g.lg.Warn("secops embedding gate: policy read failed, falling back to default threshold",
			loggateway.StepID("secops.embedding_gate"), loggateway.Err(err))
	} else if p != nil && p.LocalThreshold > 0 {
		threshold = p.LocalThreshold
	}
	if info.Classification < threshold {
		metrics.SecopsEmbedGateTotal.WithLabelValues("allow").Inc()
		return nil
	}
	if info.Local {
		metrics.SecopsEmbedGateTotal.WithLabelValues("allow_local").Inc()
		return nil
	}
	metrics.SecopsEmbedGateTotal.WithLabelValues("blocked").Inc()
	g.recordBlockedAudit(ctx, info, threshold)
	return apierror.Forbidden(apierror.DomainKnowledge,
		"content classification L%d requires local embedding channel (threshold L%d)", info.Classification, threshold)
}

// recordBlockedAudit 拒绝事件审计（best-effort：拒绝决策已成立，审计失败
// 不翻转决策——与 mask RecordMaskAudit 严格模式不同，这里不存在「放行依赖
// 审计成功」的面；失败只进进程日志）。
func (g *SecopsEmbeddingGate) recordBlockedAudit(ctx context.Context, info EmbedGateInfo, threshold int) {
	if g.audit == nil {
		return
	}
	entry, err := buildSecopsAuditEntry(ctx, SecopsEmbeddingActionBlocked, info.Subject, map[string]any{
		"classification": info.Classification,
		"threshold":      threshold,
		"provider":       info.Provider,
		"local":          info.Local,
		"batch_size":     info.BatchSize,
	}, "warning")
	if err != nil {
		g.lg.Warn("secops embedding gate: audit entry build failed",
			loggateway.StepID("secops.embedding_gate"), loggateway.Err(err))
		return
	}
	if err := g.audit.InsertAuditLog(ctx, entry); err != nil {
		g.lg.Warn("secops embedding gate: audit persist failed (block stands)",
			loggateway.StepID("secops.embedding_gate"), loggateway.Err(err))
	}
}

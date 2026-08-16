package knowledge

import (
	"context"

	"aranea-agents/pkg/loggateway"
)

// 自治理知识图谱 M3 演化时序层端口。
// supersedes 版本链：同 fact_id 再写入/仲裁 supersede 时旧段快照留痕（可审计可回滚），
// 不污染 links 表；写入时冲突检测：LLM 仲裁新事实 vs 同词条既有段，
// contradicts 落治理提案（高风险人工二审），supersedes 走版本链。

// 治理提案 kind（M3.2 起用 conflict；其余 M4 dream_cycle 产出）。
const (
	ProposalKindConflict = "conflict"
	// ProposalKindStale 陈旧词条标记（M4，低风险自动应用）。
	ProposalKindStale = "stale"
	// ProposalKindOrphan 孤儿词条（M4，高风险人工二审）。
	ProposalKindOrphan = "orphan"
	// ProposalRiskHigh 高风险：仅留提案待人工二审，不自动应用。
	ProposalRiskHigh = "high"
	// ProposalRiskLow 低风险：M4 治理周期可自动应用。
	ProposalRiskLow = "low"
)

// 治理提案 status。
const (
	ProposalStatusPending  = "pending"
	ProposalStatusApplied  = "applied"
	ProposalStatusRejected = "rejected"
)

// FactVersion 一条 supersedes 版本链记录（旧段快照）。
type FactVersion struct {
	CollectionID string
	DocID        string
	FactID       string // 可空（旧段无 fact_id 时留空）
	OldBody      string // 被顶替的旧段整段（含 H2 标题行）
	NewBody      string // 新段整段
}

// FactVersionRepo 版本链持久化窄接口（M3.1）。派生留痕，失败不阻断写回主流程。
type FactVersionRepo interface {
	InsertFactVersion(ctx context.Context, v FactVersion) error
}

// GovernanceProposal 一条治理提案（M3.2 矛盾仲裁；M4 治理周期复用）。
type GovernanceProposal struct {
	CollectionID string
	Kind         string         // conflict/stale/orphan/decay/merge/moc_emerge/relation_promote/distill
	Risk         string         // low / high
	Payload      map[string]any // 证据载荷（doc_id/旧段/新段/理由等），JSONB 落库
	// Status 空 = pending（人工二审）；M4 低风险自动应用置 applied（留痕即生效）。
	Status       string
}

// GovernanceProposalRepo 提案持久化窄接口（M3.2 起）。低风险提案 M4 自动应用；
// 高风险仅 pending，人工二审后置 applied/rejected。
type GovernanceProposalRepo interface {
	InsertProposal(ctx context.Context, p GovernanceProposal) error
}

// WriteBackFactBlock 词条页内一个既有事实段（H2 小节）的轻量投影（仲裁候选）。
type WriteBackFactBlock struct {
	Heading string // H2 标题行文本（kind）
	Body    string // 整段正文（含标题行）
	FactID  string // 段内 fact_id 标记；空 = 无 ID 段（不可作 supersede 目标）
}

// WriteBackArbitration 一条新事实的仲裁结论。
type WriteBackArbitration struct {
	FactIndex    int     // 对应输入新事实切片下标
	Verdict      string  // unrelated / supersedes / contradicts
	TargetFactID string  // supersedes/contradicts 目标段的 fact_id
	Confidence   float64 // 0-1；低于门槛的结论调用方忽略
	Reason       string  // 仲裁理由（提案载荷用）
}

// WriteBackArbiter 写回冲突检测窄接口（M3.2；生产实现 internal/knowledge.WriteBackArbiter）。
// 同词条既有段为候选（同页即同主题，零检索成本）；实现方批量一次 LLM 调用仲裁。
type WriteBackArbiter interface {
	ArbitrateWriteBack(ctx context.Context, title string, existing []WriteBackFactBlock, news []WriteBackFact) ([]WriteBackArbitration, error)
}

// SetEvolutionRepos 接线 M3 演化时序持久化（可选能力；nil 时版本链/提案降级跳过，
// 写回主流程语义不变——与 M3 前行为一致）。
func (u *Usecase) SetEvolutionRepos(versions FactVersionRepo, proposals GovernanceProposalRepo) {
	u.factVersions = versions
	u.proposals = proposals
}

// SetWriteBackArbiter 接线写回冲突仲裁器（可选；nil 时新事实一律追加，不仲裁）。
func (u *Usecase) SetWriteBackArbiter(a WriteBackArbiter) {
	u.arbiter = a
}

// recordFactVersion 留痕一条 supersedes 版本（best-effort：失败 Warn 不阻断写回）。
func (u *Usecase) recordFactVersion(ctx context.Context, collectionID, docID, factID, oldBody, newBody string) {
	if u == nil || u.factVersions == nil || oldBody == "" || oldBody == newBody {
		return
	}
	if err := u.factVersions.InsertFactVersion(ctx, FactVersion{
		CollectionID: collectionID,
		DocID:        docID,
		FactID:       factID,
		OldBody:      oldBody,
		NewBody:      newBody,
	}); err != nil {
		u.lg.Warn("supersedes 版本留痕失败（正文已更新，留痕缺失可容忍）",
			loggateway.StepID("knowledge.evolution.version"),
			loggateway.Str("doc_id", docID),
			loggateway.Str("fact_id", factID),
			loggateway.Err(err),
		)
	}
}

// recordConflictProposal 留痕一条矛盾仲裁提案（best-effort 同上）。
func (u *Usecase) recordConflictProposal(ctx context.Context, collectionID, docID string, payload map[string]any) {
	if u == nil || u.proposals == nil {
		return
	}
	if err := u.proposals.InsertProposal(ctx, GovernanceProposal{
		CollectionID: collectionID,
		Kind:         ProposalKindConflict,
		Risk:         ProposalRiskHigh,
		Payload:      payload,
	}); err != nil {
		u.lg.Warn("矛盾治理提案留痕失败",
			loggateway.StepID("knowledge.evolution.proposal"),
			loggateway.Str("doc_id", docID),
			loggateway.Err(err),
		)
	}
}

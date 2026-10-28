package knowledge

import (
	"context"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// 自治理知识图谱 M3 演化时序层端口。
// 版本链：同 fact_id 再写入/仲裁 update 时旧段快照留痕（可审计可回滚），
// 不污染 links 表；写入时冲突检测：LLM 仲裁新事实 vs 同词条既有段（G-40
// 四操作 add/update/delete/noop，Mem0 词表对齐）——delete 高置信落治理提案
// 人工二审、低置信带自动失效走版本链墓碑，update 走版本链顶替。

// 治理提案 kind（M3.2 起用 conflict；其余 M4 dream_cycle 产出）。
const (
	ProposalKindConflict = "conflict"
	// ProposalKindStale 陈旧词条标记（M4，低风险自动应用）。
	ProposalKindStale = "stale"
	// ProposalKindOrphan 孤儿词条（M4，高风险人工二审）。
	ProposalKindOrphan = "orphan"
	// ProposalKindDistill 高频词条反向蒸馏 memory_fact（M4，低风险自动应用）。
	ProposalKindDistill = "distill"
	// ProposalKindMOCEmerge hub 簇蒸馏 MOC 词条（M4，高风险人工二审）。
	ProposalKindMOCEmerge = "moc_emerge"
	// ProposalKindEntityMerge 实体消解合并裁决（G-37 ADR-D，高风险人工二审）。
	ProposalKindEntityMerge = "entity_merge"
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
	// ProposalDecisionKeepOld 事实段冲突：保留旧段、删除新段，终态仍落 applied。
	ProposalDecisionKeepOld = "keep_old"
	// ProposalDecisionKeepNew 事实段冲突：保留新段、删除旧段，并继承旧 fact_id 作为 lineage。
	ProposalDecisionKeepNew = "keep_new"
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

// FactVersionReader 版本链读侧（G-40 c1）：墓碑回放的唯一消费口。
// 与写口拆开，避免只实现 Insert 的测试桩被迫补读。
type FactVersionReader interface {
	LatestFactVersion(ctx context.Context, docID, factID string) (FactVersion, bool, error)
}

// FactEdgeCloser D8：事实段自动失效后，关闭以该段正文为证据的 active semantic 边。
// nil 时写回零旁效应。
type FactEdgeCloser interface {
	CloseSemanticEdgesEvidencedBy(ctx context.Context, collectionID, docID, evidence string) (int, error)
}

// GovernanceProposal 一条治理提案（M3.2 矛盾仲裁；M4 治理周期复用）。
type GovernanceProposal struct {
	CollectionID string
	Kind         string         // conflict/stale/orphan/decay/merge/moc_emerge/relation_promote/distill
	Risk         string         // low / high
	Payload      map[string]any // 证据载荷（doc_id/旧段/新段/理由等），JSONB 落库
	// Status 空 = pending（人工二审）；M4 低风险自动应用置 applied（留痕即生效）。
	Status string
}

// GovernanceProposalRepo 提案持久化窄接口（M3.2 起）。低风险提案 M4 自动应用；
// 高风险仅 pending，人工二审后置 applied/rejected。
type GovernanceProposalRepo interface {
	InsertProposal(ctx context.Context, p GovernanceProposal) error
}

type governanceProposalDeduper interface {
	HasProposal(ctx context.Context, collectionID, kind, dedupKey string, statuses []string) (bool, error)
}

// WriteBackFactBlock 词条页内一个既有事实段（H2 小节）的轻量投影（仲裁候选）。
type WriteBackFactBlock struct {
	Heading string // H2 标题行文本（kind）
	Body    string // 整段正文（含标题行）
	FactID  string // 段内 fact_id 标记；空 = 无 ID 段（不可作 supersede 目标）
}

// WriteBackOp 写回仲裁四操作（G-40 ARB4；与 Mem0 业界词表对齐，字节稳定枚举）。
// 解析层兼容旧三值映射：supersedes→update、contradicts→delete、unrelated→noop。
type WriteBackOp string

const (
	// WriteBackOpAdd 无冲突直接追加；仲裁未判（不在输出列表）默认即 add。
	WriteBackOpAdd WriteBackOp = "add"
	// WriteBackOpUpdate 新事实是既有段同一事实的更新/修正——高置信（≥0.8）走版本链顶替。
	WriteBackOpUpdate WriteBackOp = "update"
	// WriteBackOpDelete 新事实与既有段矛盾——置信分带：≥0.7 人工二审（高风险提案），
	// [0.5,0.7) 自动失效（旧段移除+版本链墓碑+低风险 applied 提案审计痕）。
	WriteBackOpDelete WriteBackOp = "delete"
	// WriteBackOpNoop 语义重复/已被覆盖——零旁效应跳过（必须显式判定，未判不 noop 化）。
	WriteBackOpNoop WriteBackOp = "noop"
)

// WriteBackArbitration 一条新事实的仲裁结论（G-40：四操作词表，旧 Verdict 字段同批删除）。
type WriteBackArbitration struct {
	FactIndex    int         // 对应输入新事实切片下标
	Operation    WriteBackOp // add / update / delete / noop
	TargetFactID string      // update/delete 目标段的 fact_id
	Confidence   float64     // 0-1；低于门槛的结论调用方忽略
	Reason       string      // 仲裁理由（提案载荷用）
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

// SetFactEdgeCloser 接线 D8 边关闭（可选；nil 时自动失效不碰 knowledge_links）。
func (u *Usecase) SetFactEdgeCloser(c FactEdgeCloser) {
	if u == nil {
		return
	}
	u.factEdgeCloser = c
}

// closeEvidencedEdges 自动失效成功后 best-effort 关边。失败只 Warn。
func (u *Usecase) closeEvidencedEdges(ctx context.Context, collectionID, docID, evidence string) {
	if u == nil || u.factEdgeCloser == nil || strings.TrimSpace(evidence) == "" {
		return
	}
	if _, err := u.factEdgeCloser.CloseSemanticEdgesEvidencedBy(ctx, collectionID, docID, evidence); err != nil {
		u.lg.Warn("事实失效关边失败（正文已落，边可下轮重抽自愈）",
			loggateway.StepID("knowledge.writeback.edge_close"),
			loggateway.Str("doc_id", docID),
			loggateway.Err(err),
		)
	}
}

// recordFactVersion 留痕一条事实段版本（best-effort：失败 Warn 不阻断写回）。
// G-40 墓碑豁免（D4）：NewBody 空串=事实已被 DELETE 自动失效移除（回放方读空即
// 墓碑），旧段非空即可入档；空旧段与新旧一致的幂等重写仍跳过。
func (u *Usecase) recordFactVersion(ctx context.Context, collectionID, docID, factID, oldBody, newBody string) {
	if u == nil || u.factVersions == nil || oldBody == "" || (oldBody == newBody && newBody != "") {
		return
	}
	if err := u.factVersions.InsertFactVersion(ctx, FactVersion{
		CollectionID: collectionID,
		DocID:        docID,
		FactID:       factID,
		OldBody:      oldBody,
		NewBody:      newBody,
	}); err != nil {
		u.lg.Warn("事实段版本留痕失败（正文已更新，留痕缺失可容忍）",
			loggateway.StepID("knowledge.evolution.version"),
			loggateway.Str("doc_id", docID),
			loggateway.Str("fact_id", factID),
			loggateway.Err(err),
		)
	}
}

// recordGovernanceProposal 留痕一条治理提案（best-effort 同上；dedup_key 幂等闸
// 全状态探测 pending/applied/rejected；G-40 自动失效审计单的键带 :auto 命名空间
// 后缀，与高风险人工二审单跨带不互吞——见 writeback_entry.go C 批修）。
func (u *Usecase) recordGovernanceProposal(ctx context.Context, collectionID, docID, kind, risk, status string, payload map[string]any) {
	if u == nil || u.proposals == nil {
		return
	}
	if dedupKey, _ := payload["dedup_key"].(string); dedupKey != "" {
		if deduper, ok := u.proposals.(governanceProposalDeduper); ok {
			exists, err := deduper.HasProposal(ctx, collectionID, kind, dedupKey, []string{
				ProposalStatusPending, ProposalStatusApplied, ProposalStatusRejected,
			})
			if err != nil {
				u.lg.Warn("治理提案去重探测失败（放行）",
					loggateway.StepID("knowledge.evolution.proposal"),
					loggateway.Str("doc_id", docID),
					loggateway.Str("kind", kind),
					loggateway.Str("dedup_key", dedupKey),
					loggateway.Err(err),
				)
			} else if exists {
				return
			}
		}
	}
	if err := u.proposals.InsertProposal(ctx, GovernanceProposal{
		CollectionID: collectionID,
		Kind:         kind,
		Risk:         risk,
		Status:       status,
		Payload:      payload,
	}); err != nil {
		u.lg.Warn("治理提案留痕失败",
			loggateway.StepID("knowledge.evolution.proposal"),
			loggateway.Str("doc_id", docID),
			loggateway.Str("kind", kind),
			loggateway.Err(err),
		)
	}
}

// recordConflictProposal 留痕一条矛盾仲裁提案（高风险 pending，人工二审）。
func (u *Usecase) recordConflictProposal(ctx context.Context, collectionID, docID string, payload map[string]any) {
	u.recordGovernanceProposal(ctx, collectionID, docID, ProposalKindConflict, ProposalRiskHigh, "", payload)
}

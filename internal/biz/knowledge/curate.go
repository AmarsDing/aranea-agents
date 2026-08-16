package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 自治理知识图谱 M4 自治理层：dream_cycle 接管词条治理。
// 编排五类治理任务——低风险自动应用（留痕即可审计），高风险仅产 pending 提案
// 待人工二审（ResolveGovernanceProposal 闭环）：
//
//	decay            Hebbian 弱边周期衰减（weight_f *= 0.9，< 0.05 关闭）——自动
//	stale            陈旧词条标记（语义边关闭比例高 + 长期未检索）——自动（applied 留痕）
//	relation_promote 候选谓词 use_count 超阈值提升 promoted——自动
//	conflict         active contradicts 边——高风险 pending 提案
//	orphan           度=0 且长期未检索的词条——高风险 pending 提案
//
// 全部依赖 M1 双时态边 + access_log、M2 谓词词表、M3 提案表。
// dry_run 只探测不写入（decay 走 COUNT 预估，提案/promote 不落库）。

// 治理阈值默认值（单一事实源）。
const (
	defaultDecayFactor        = 0.9
	defaultDecayMinWeight     = 0.05
	defaultPromoteMinUseCount = 3
	defaultOrphanInactiveDays = 30
	defaultStaleInactiveDays  = 30
	// curateProposalLimit 每类提案单轮上限（防单轮提案风暴）。
	curateProposalLimit = 50
)

// CurateOptions 一轮治理的输入。
type CurateOptions struct {
	// CollectionID 目标集合；空 = 写回落点（LookupWriteBackHome：团队知识收件箱优先）。
	CollectionID string
	// Workspace 仅 CollectionID 空时用于解析写回落点。
	Workspace string
	// DryRun 只探测不写入。
	DryRun             bool
	DecayFactor        float64 // 0 → defaultDecayFactor
	DecayMinWeight     float64 // 0 → defaultDecayMinWeight
	PromoteMinUseCount int     // 0 → defaultPromoteMinUseCount
	OrphanInactiveDays int     // 0 → defaultOrphanInactiveDays
	StaleInactiveDays  int     // 0 → defaultStaleInactiveDays
}

// CurateProposal 一条本轮新产提案的报告投影。
type CurateProposal struct {
	Kind     string
	Risk     string
	Status   string
	DedupKey string
}

// CurateReport 一轮治理的结果。
type CurateReport struct {
	CollectionID      string
	DryRun            bool
	DecayedEdges      int      // co_activated 边参与衰减数
	ClosedEdges       int      // 衰减后跌破阈值被关闭数
	PromotedRelations []string // 本轮提升 promoted 的谓词（dry_run 为预估清单）
	StaleMarked       int      // 本轮标记的陈旧词条数
	ProposalsPending  int      // 本轮新产高风险 pending 提案数
	Proposals         []CurateProposal
	Actions           []string // 实际执行的治理任务名
}

// StaleEntryStat 陈旧词条候选（语义边关闭比例高 + 长期未检索）。
type StaleEntryStat struct {
	DocID          string
	RelPath        string
	LastAccessDays int     // 距今天数；从未检索为大值
	ClosedRatio    float64 // 出向 semantic 边已关闭比例
}

// OrphanEntryStat 孤儿词条候选（无任何 active 边 + 长期未检索）。
type OrphanEntryStat struct {
	DocID          string
	RelPath        string
	LastAccessDays int
}

// ContradictsEdgeStat 一条 active contradicts 语义边（M2 抽取/M3 仲裁产物）。
type ContradictsEdgeStat struct {
	DocID       string
	TargetDocID string
	Context     string
	Confidence  float64
}

// GovernanceProposalView 治理提案完整视图（人工二审列表/工具展示用）。
type GovernanceProposalView struct {
	ID           int64
	CollectionID string
	Kind         string
	Risk         string
	Status       string
	Payload      map[string]any
	CreatedAt    time.Time
	ResolvedAt   time.Time // 零值 = 未审
}

// KnowledgeCurateRepo M4 治理数据端口（窄接口，data 层 knowledgeRepo 实现）。
type KnowledgeCurateRepo interface {
	// DecayCoActivatedEdges 周期衰减：活跃 co_activated 边 weight_f *= factor，
	// 跌破 minWeight 的置 valid_to 关闭。dryRun 时只 COUNT 预估不 UPDATE。
	DecayCoActivatedEdges(ctx context.Context, collectionID string, factor, minWeight float64, dryRun bool) (decayed, closed int, err error)
	// ListPromotableRelations 列出 use_count 达阈值的 candidate 谓词。
	ListPromotableRelations(ctx context.Context, minUseCount int) ([]string, error)
	// PromoteRelation candidate → promoted（幂等：非 candidate 不动）。
	PromoteRelation(ctx context.Context, relation string) error
	// ListStaleEntries 陈旧词条候选（出向 semantic 边关闭比例 ≥0.5 且超 inactiveDays 未检索）。
	ListStaleEntries(ctx context.Context, collectionID string, inactiveDays, limit int) ([]StaleEntryStat, error)
	// ListOrphanEntries 孤儿词条候选（无任何 active 边且超 inactiveDays 未检索）。
	ListOrphanEntries(ctx context.Context, collectionID string, inactiveDays, limit int) ([]OrphanEntryStat, error)
	// ListContradictsEdges active contradicts 语义边。
	ListContradictsEdges(ctx context.Context, collectionID string, limit int) ([]ContradictsEdgeStat, error)
	// HasProposal 是否已存在同 dedup_key 且 status 在 statuses 内的提案（去重防风暴）。
	HasProposal(ctx context.Context, collectionID, kind, dedupKey string, statuses []string) (bool, error)
	// ListGovernanceProposals 治理提案列表（人工二审出口）；collectionID/status 空 = 不过滤。
	ListGovernanceProposals(ctx context.Context, collectionID, status string, limit int) ([]GovernanceProposalView, error)
	// ResolveGovernanceProposal 人工二审闭环：pending → applied/rejected。
	ResolveGovernanceProposal(ctx context.Context, id int64, status string) error
}

// SetCurateRepo 接线 M4 治理数据端口（可选；nil 时 CurateKnowledge 显式报不可用）。
func (u *Usecase) SetCurateRepo(repo KnowledgeCurateRepo) {
	u.curate = repo
}

// CurateKnowledge 执行一轮词条治理。编排顺序：低风险自动项先行（decay/promote/stale），
// 高风险探测随后（conflict/orphan 提案）；单任务失败 Warn 降级继续，不中断整轮。
func (u *Usecase) CurateKnowledge(ctx context.Context, opts CurateOptions) (CurateReport, error) {
	if u == nil || u.curate == nil {
		return CurateReport{}, apierror.Unavailable(apierror.DomainKnowledge, "knowledge curate repo not wired")
	}
	colID := strings.TrimSpace(opts.CollectionID)
	if colID == "" {
		col, found, err := u.LookupWriteBackHome(ctx, opts.Workspace)
		if err != nil {
			return CurateReport{}, err
		}
		if !found {
			return CurateReport{}, apierror.NotFound(apierror.DomainKnowledge, "no team knowledge collection to curate")
		}
		colID = col.ID
	}
	if opts.DecayFactor <= 0 || opts.DecayFactor >= 1 {
		opts.DecayFactor = defaultDecayFactor
	}
	if opts.DecayMinWeight <= 0 {
		opts.DecayMinWeight = defaultDecayMinWeight
	}
	if opts.PromoteMinUseCount <= 0 {
		opts.PromoteMinUseCount = defaultPromoteMinUseCount
	}
	if opts.OrphanInactiveDays <= 0 {
		opts.OrphanInactiveDays = defaultOrphanInactiveDays
	}
	if opts.StaleInactiveDays <= 0 {
		opts.StaleInactiveDays = defaultStaleInactiveDays
	}

	rep := CurateReport{CollectionID: colID, DryRun: opts.DryRun}

	// ── decay：Hebbian 弱边周期衰减（自动） ─────────────────────────────
	decayed, closed, err := u.curate.DecayCoActivatedEdges(ctx, colID, opts.DecayFactor, opts.DecayMinWeight, opts.DryRun)
	if err != nil {
		u.lg.Warn("治理：co_activated 边衰减失败", loggateway.StepID("knowledge.curate.decay"), loggateway.Err(err))
	} else {
		rep.DecayedEdges, rep.ClosedEdges = decayed, closed
		rep.Actions = append(rep.Actions, "decay")
	}

	// ── relation_promote：候选谓词提升（自动） ─────────────────────────
	promotable, err := u.curate.ListPromotableRelations(ctx, opts.PromoteMinUseCount)
	if err != nil {
		u.lg.Warn("治理：候选谓词扫描失败", loggateway.StepID("knowledge.curate.promote"), loggateway.Err(err))
	} else {
		for _, rel := range promotable {
			if opts.DryRun {
				rep.PromotedRelations = append(rep.PromotedRelations, rel)
				continue
			}
			if perr := u.curate.PromoteRelation(ctx, rel); perr != nil {
				u.lg.Warn("治理：谓词提升失败", loggateway.StepID("knowledge.curate.promote"),
					loggateway.Str("relation", rel), loggateway.Err(perr))
				continue
			}
			rep.PromotedRelations = append(rep.PromotedRelations, rel)
		}
		if len(promotable) > 0 {
			rep.Actions = append(rep.Actions, "relation_promote")
		}
	}

	// ── stale：陈旧词条标记（低风险自动应用，applied 留痕即标记） ────────
	stales, err := u.curate.ListStaleEntries(ctx, colID, opts.StaleInactiveDays, curateProposalLimit)
	if err != nil {
		u.lg.Warn("治理：陈旧词条扫描失败", loggateway.StepID("knowledge.curate.stale"), loggateway.Err(err))
	} else {
		for _, s := range stales {
			dedup := "stale:" + s.DocID
			if u.skipProposal(ctx, colID, ProposalKindStale, dedup, []string{ProposalStatusPending, ProposalStatusApplied}) {
				continue
			}
			p := GovernanceProposal{
				CollectionID: colID, Kind: ProposalKindStale, Risk: ProposalRiskLow, Status: ProposalStatusApplied,
				Payload: map[string]any{
					"dedup_key": dedup, "doc_id": s.DocID, "rel_path": s.RelPath,
					"last_access_days": s.LastAccessDays, "closed_ratio": s.ClosedRatio,
				},
			}
			if !opts.DryRun {
				u.recordCurateProposal(ctx, p)
			}
			rep.StaleMarked++
			rep.Proposals = append(rep.Proposals, CurateProposal{Kind: p.Kind, Risk: p.Risk, Status: ProposalStatusApplied, DedupKey: dedup})
		}
		if rep.StaleMarked > 0 {
			rep.Actions = append(rep.Actions, "stale")
		}
	}

	// ── conflict：active contradicts 边 → 高风险 pending 提案（人工二审） ──
	conflicts, err := u.curate.ListContradictsEdges(ctx, colID, curateProposalLimit)
	if err != nil {
		u.lg.Warn("治理：contradicts 边扫描失败", loggateway.StepID("knowledge.curate.conflict"), loggateway.Err(err))
	} else {
		for _, c := range conflicts {
			dedup := "conflict:" + c.DocID + "→" + c.TargetDocID
			// 去重含 rejected：人工已否决的同矛盾不再重复提案（拒绝即沉默，
			// 否则每轮 dream 重复骚扰二审人）。
			if u.skipProposal(ctx, colID, ProposalKindConflict, dedup, []string{ProposalStatusPending, ProposalStatusRejected}) {
				continue
			}
			p := GovernanceProposal{
				CollectionID: colID, Kind: ProposalKindConflict, Risk: ProposalRiskHigh, Status: ProposalStatusPending,
				Payload: map[string]any{
					"dedup_key": dedup, "doc_id": c.DocID, "target_doc_id": c.TargetDocID,
					"context": c.Context, "confidence": c.Confidence, "arbiter": "curate_scan",
				},
			}
			if !opts.DryRun {
				u.recordCurateProposal(ctx, p)
			}
			rep.ProposalsPending++
			rep.Proposals = append(rep.Proposals, CurateProposal{Kind: p.Kind, Risk: p.Risk, Status: ProposalStatusPending, DedupKey: dedup})
		}
		if len(conflicts) > 0 {
			rep.Actions = append(rep.Actions, "conflict")
		}
	}

	// ── orphan：孤儿词条 → 高风险 pending 提案（人工二审） ───────────────
	orphans, err := u.curate.ListOrphanEntries(ctx, colID, opts.OrphanInactiveDays, curateProposalLimit)
	if err != nil {
		u.lg.Warn("治理：孤儿词条扫描失败", loggateway.StepID("knowledge.curate.orphan"), loggateway.Err(err))
	} else {
		for _, o := range orphans {
			dedup := "orphan:" + o.DocID
			// 去重含 rejected：同 conflict 的拒绝即沉默语义。
			if u.skipProposal(ctx, colID, ProposalKindOrphan, dedup, []string{ProposalStatusPending, ProposalStatusRejected}) {
				continue
			}
			p := GovernanceProposal{
				CollectionID: colID, Kind: ProposalKindOrphan, Risk: ProposalRiskHigh, Status: ProposalStatusPending,
				Payload: map[string]any{
					"dedup_key": dedup, "doc_id": o.DocID, "rel_path": o.RelPath,
					"last_access_days": o.LastAccessDays,
				},
			}
			if !opts.DryRun {
				u.recordCurateProposal(ctx, p)
			}
			rep.ProposalsPending++
			rep.Proposals = append(rep.Proposals, CurateProposal{Kind: p.Kind, Risk: p.Risk, Status: ProposalStatusPending, DedupKey: dedup})
		}
		if len(orphans) > 0 {
			rep.Actions = append(rep.Actions, "orphan")
		}
	}

	u.lg.Info("知识库词条治理完成",
		loggateway.StepID("knowledge.curate"),
		loggateway.Str("collection_id", colID),
		loggateway.Int("decayed", rep.DecayedEdges),
		loggateway.Int("closed", rep.ClosedEdges),
		loggateway.Int("promoted", len(rep.PromotedRelations)),
		loggateway.Int("stale", rep.StaleMarked),
		loggateway.Int("proposals_pending", rep.ProposalsPending),
	)
	return rep, nil
}

// listProposalsDefaultLimit 二审列表默认上限。
const listProposalsDefaultLimit = 50

// ListGovernanceProposals 人工二审列表出口（M4 补丁：此前提案只写不读，死信堆积）。
// status 空 = 全部；合法值 pending/applied/rejected。limit 0 = 默认 50，上限 200。
func (u *Usecase) ListGovernanceProposals(ctx context.Context, collectionID, status string, limit int) ([]GovernanceProposalView, error) {
	if u == nil || u.curate == nil {
		return nil, apierror.Unavailable(apierror.DomainKnowledge, "knowledge curate repo not wired")
	}
	switch status {
	case "", ProposalStatusPending, ProposalStatusApplied, ProposalStatusRejected:
	default:
		return nil, apierror.BadRequest(apierror.DomainKnowledge, fmt.Sprintf("invalid proposal status %q", status))
	}
	if limit <= 0 {
		limit = listProposalsDefaultLimit
	}
	if limit > 200 {
		limit = 200
	}
	return u.curate.ListGovernanceProposals(ctx, collectionID, status, limit)
}

// ResolveGovernanceProposal 人工二审闭环：pending → applied/rejected（其他状态非法）。
func (u *Usecase) ResolveGovernanceProposal(ctx context.Context, id int64, status string) error {
	if u == nil || u.curate == nil {
		return apierror.Unavailable(apierror.DomainKnowledge, "knowledge curate repo not wired")
	}
	if status != ProposalStatusApplied && status != ProposalStatusRejected {
		return apierror.BadRequest(apierror.DomainKnowledge, fmt.Sprintf("invalid proposal status %q", status))
	}
	return u.curate.ResolveGovernanceProposal(ctx, id, status)
}

// skipProposal 提案去重：同 kind+dedup_key 且处于指定状态的提案已存在时跳过（防周期风暴）。
// 探测失败保守放行（宁可重复提案，不丢治理信号）。
func (u *Usecase) skipProposal(ctx context.Context, collectionID, kind, dedup string, statuses []string) bool {
	exists, err := u.curate.HasProposal(ctx, collectionID, kind, dedup, statuses)
	if err != nil {
		u.lg.Warn("治理：提案去重探测失败（放行）", loggateway.StepID("knowledge.curate.dedup"),
			loggateway.Str("dedup_key", dedup), loggateway.Err(err))
		return false
	}
	return exists
}

// recordCurateProposal 落库一条治理提案（best-effort：失败 Warn 不中断整轮）。
func (u *Usecase) recordCurateProposal(ctx context.Context, p GovernanceProposal) {
	if u.proposals == nil {
		return
	}
	if err := u.proposals.InsertProposal(ctx, p); err != nil {
		u.lg.Warn("治理提案落库失败", loggateway.StepID("knowledge.curate.proposal"),
			loggateway.Str("kind", p.Kind), loggateway.Err(err))
	}
}

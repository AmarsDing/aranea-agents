package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 自治理知识图谱 M4 自治理层：dream_cycle 接管词条治理。
// 编排七类治理任务——低风险自动应用（留痕即可审计），高风险仅产 pending 提案
// 待人工二审（ResolveGovernanceProposal 闭环）：
//
//	decay            Hebbian 弱边周期衰减（weight_f *= 0.9，< 0.05 关闭）——自动
//	stale            陈旧词条标记（语义边关闭比例高 + 长期未检索）——自动（applied 留痕）
//	relation_promote 候选谓词 use_count 超阈值提升 promoted——自动
//	distill          高频词条摘要卡反向蒸馏 memory_fact（scope=workspace 注入 L0）——自动
//	conflict         active contradicts 边——高风险 pending 提案
//	orphan           度=0 且长期未检索的词条——高风险 pending 提案
//	moc_emerge       hub 簇（规模+密度双达阈值）蒸馏 MOC 词条——高风险 pending 提案
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
	// distill 任务：高频词条反向蒸馏 memory_fact。
	defaultDistillSinceDays = 30 // 热度统计窗口
	defaultDistillMinHits   = 5  // 窗口内最低检索命中数
	defaultDistillLimit     = 20 // 单轮蒸馏上限
	// moc_emerge 任务：hub 簇涌现 MOC 提案。
	defaultMOCMinDegree = 5   // hub 入选度数（active 边，出入向合计）
	defaultMOCMinMember = 4   // 簇最小规模（hub + 邻居）
	defaultMOCDensity   = 0.3 // 簇内边密度阈值（实际边 / 完全图）
	mocHubScanLimit     = 10  // 单轮扫描的 hub 上限（每 hub 一次密度 COUNT）
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
	DistillSinceDays   int     // 0 → defaultDistillSinceDays
	DistillMinHits     int     // 0 → defaultDistillMinHits
	MOCMinDegree       int     // 0 → defaultMOCMinDegree
	MOCMinMember       int     // 0 → defaultMOCMinMember
	MinMOCDensity      float64 // 0 → defaultMOCDensity
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
	DistilledFacts    int      // 本轮蒸馏进 memory_fact 的词条数
	HubsScanned       int      // 本轮参与密度检测的 hub 数
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

// HubMember MOC 簇成员（hub 的 1 跳邻居，entries/* 词条）。
type HubMember struct {
	DocID   string
	RelPath string
}

// HubClusterStat hub 簇候选：entries/* 中 active 边度数达阈值的词条及其 1 跳邻居。
// 密度由业务层另查 CountActiveEdgesWithin 计算（数据层每 hub 一次 COUNT）。
type HubClusterStat struct {
	HubDocID   string
	HubRelPath string
	Degree     int // active 边度数（出入向合计，仅 entries 对 entries 边）
	Neighbors  []HubMember
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
	// ListHubClusters hub 簇候选（entries/* 中 active 边度数 >= minDegree，按度数降序，limit 截断）。
	ListHubClusters(ctx context.Context, collectionID string, minDegree, limit int) ([]HubClusterStat, error)
	// CountActiveEdgesWithin docIDs 集合内部两端均在的 active 无向边对数
	//（LEAST/GREATEST 去重：反向并存/同对多类型计 1 对，密度口径上限 1.0）。
	CountActiveEdgesWithin(ctx context.Context, collectionID string, docIDs []string) (int, error)
	// HasProposal 是否已存在同 dedup_key 且 status 在 statuses 内的提案（去重防风暴）。
	HasProposal(ctx context.Context, collectionID, kind, dedupKey string, statuses []string) (bool, error)
	// ListGovernanceProposals 治理提案列表（人工二审出口）；collectionID/status 空 = 不过滤。
	ListGovernanceProposals(ctx context.Context, collectionID, status string, limit int) ([]GovernanceProposalView, error)
	// GetGovernanceProposal 单提案读取（P1-b：applied 处置前取 kind/payload + pending 守卫）。
	GetGovernanceProposal(ctx context.Context, id int64) (GovernanceProposalView, error)
	// ResolveGovernanceProposal 人工二审闭环：pending → applied/rejected。
	ResolveGovernanceProposal(ctx context.Context, id int64, status string) error
	// CloseContradictsEdges 关闭 docID ↔ targetDocIDs 间的 active contradicts 语义边
	// （P1-b：conflict 提案 applied 处置；双方向均关，失效不删除留痕可审计）。
	CloseContradictsEdges(ctx context.Context, collectionID, docID string, targetDocIDs []string) (int, error)
	// MarkStaleEntries 置位 documents.stale_at（P1-c：stale 标记落地文档字段，
	// 检索侧降权消费；幂等——已置位行不动，保留首判时间）。
	MarkStaleEntries(ctx context.Context, docIDs []string) error
}

// SetCurateRepo 接线 M4 治理数据端口（可选；nil 时 CurateKnowledge 显式报不可用）。
func (u *Usecase) SetCurateRepo(repo KnowledgeCurateRepo) {
	u.curate = repo
}

// SetDistillRepos 接线 M4 distill 任务端口（可选；任一为 nil 时 distill 任务跳过，
// 其余治理任务不受影响）。hot 为高频文档检索（knowledgeRepo 断言），writer 为
// memory_fact 写入适配（生产由 wire 层包 MemoryAdminUsecase）。
func (u *Usecase) SetDistillRepos(hot HotDocumentLister, writer DistillFactWriter) {
	u.hotDocs = hot
	u.distill = writer
}

// CurateKnowledge 执行一轮词条治理。编排顺序：低风险自动项先行（decay/promote/stale/distill），
// 高风险探测随后（conflict/orphan/moc_emerge 提案）；单任务失败 Warn 降级继续，不中断整轮。
func (u *Usecase) CurateKnowledge(ctx context.Context, opts CurateOptions) (CurateReport, error) {
	if u == nil || u.curate == nil {
		return CurateReport{}, apierror.Unavailable(apierror.DomainKnowledge, "knowledge curate repo not wired")
	}
	colID := strings.TrimSpace(opts.CollectionID)
	var homeCol Collection // distill 需要 Workspace 定 scope、VaultBackend 判团队库
	if colID == "" {
		col, found, err := u.LookupWriteBackHome(ctx, opts.Workspace)
		if err != nil {
			return CurateReport{}, err
		}
		if !found {
			return CurateReport{}, apierror.NotFound(apierror.DomainKnowledge, "no team knowledge collection to curate")
		}
		colID = col.ID
		homeCol = col
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
	if opts.DistillSinceDays <= 0 {
		opts.DistillSinceDays = defaultDistillSinceDays
	}
	if opts.DistillMinHits <= 0 {
		opts.DistillMinHits = defaultDistillMinHits
	}
	if opts.MOCMinDegree <= 0 {
		opts.MOCMinDegree = defaultMOCMinDegree
	}
	if opts.MOCMinMember <= 0 {
		opts.MOCMinMember = defaultMOCMinMember
	}
	if opts.MinMOCDensity <= 0 {
		opts.MinMOCDensity = defaultMOCDensity
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
	// P1-c：标记落地 documents.stale_at（检索侧降权消费），提案行仍为审计留痕。
	// 先置位后落提案——置位失败则不落提案/不计数，下轮 dream 自然重试（无 applied
	// 提案去重不拦截）；dry_run 只探测不置位。
	stales, err := u.curate.ListStaleEntries(ctx, colID, opts.StaleInactiveDays, curateProposalLimit)
	if err != nil {
		u.lg.Warn("治理：陈旧词条扫描失败", loggateway.StepID("knowledge.curate.stale"), loggateway.Err(err))
	} else {
		type staleHit struct {
			stat  StaleEntryStat
			dedup string
		}
		var hits []staleHit
		for _, s := range stales {
			dedup := "stale:" + s.DocID
			if u.skipProposal(ctx, colID, ProposalKindStale, dedup, []string{ProposalStatusPending, ProposalStatusApplied}) {
				continue
			}
			hits = append(hits, staleHit{stat: s, dedup: dedup})
		}
		marked := opts.DryRun || len(hits) == 0 // dry_run/无候选：无需置位
		if !marked {
			ids := make([]string, 0, len(hits))
			for _, h := range hits {
				ids = append(ids, h.stat.DocID)
			}
			if merr := u.curate.MarkStaleEntries(ctx, ids); merr != nil {
				u.lg.Warn("治理：stale_at 置位失败（本轮不落提案，下轮重试）",
					loggateway.StepID("knowledge.curate.stale"), loggateway.Err(merr))
			} else {
				marked = true
			}
		}
		if marked {
			for _, h := range hits {
				p := GovernanceProposal{
					CollectionID: colID, Kind: ProposalKindStale, Risk: ProposalRiskLow, Status: ProposalStatusApplied,
					Payload: map[string]any{
						"dedup_key": h.dedup, "doc_id": h.stat.DocID, "rel_path": h.stat.RelPath,
						"last_access_days": h.stat.LastAccessDays, "closed_ratio": h.stat.ClosedRatio,
					},
				}
				if !opts.DryRun {
					u.recordCurateProposal(ctx, p)
				}
				rep.StaleMarked++
				rep.Proposals = append(rep.Proposals, CurateProposal{Kind: p.Kind, Risk: p.Risk, Status: ProposalStatusApplied, DedupKey: h.dedup})
			}
		}
		if rep.StaleMarked > 0 {
			rep.Actions = append(rep.Actions, "stale")
		}
	}

	// ── distill：高频词条反向蒸馏 memory_fact（低风险自动，applied 留痕） ────
	// 只对团队库蒸馏（local vault 的归属是个人工作区，事实注入链不覆盖）；
	// distill 端口未接线时静默跳过（治理其余任务不受影响）。
	rep.DistilledFacts = u.runDistillTask(ctx, homeCol, colID, opts, &rep)

	// ── conflict：active contradicts 边 → 高风险 pending 提案（人工二审） ──
	conflicts, err := u.curate.ListContradictsEdges(ctx, colID, curateProposalLimit)
	if err != nil {
		u.lg.Warn("治理：contradicts 边扫描失败", loggateway.StepID("knowledge.curate.conflict"), loggateway.Err(err))
	} else {
		for _, c := range conflicts {
			dedup := "conflict:" + c.DocID + "→" + c.TargetDocID
			// 去重含 rejected+applied：人工已否决的同矛盾不再重复提案（拒绝即沉默，
			// 否则每轮 dream 重复骚扰二审人）；applied 已实际处置（关边）即终态，
			// 不再周期重提（P1-b，与 moc_emerge 三态全含同口径）。
			if u.skipProposal(ctx, colID, ProposalKindConflict, dedup, []string{ProposalStatusPending, ProposalStatusRejected, ProposalStatusApplied}) {
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
			// 去重含 rejected+applied：同 conflict 的拒绝即沉默语义；applied 已删词条
			// 即终态（P1-b）。
			if u.skipProposal(ctx, colID, ProposalKindOrphan, dedup, []string{ProposalStatusPending, ProposalStatusRejected, ProposalStatusApplied}) {
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

	// ── moc_emerge：hub 簇蒸馏 MOC 词条 → 高风险 pending 提案（人工二审） ──
	rep.HubsScanned = u.runMOCEmergeTask(ctx, colID, opts, &rep)

	u.lg.Info("知识库词条治理完成",
		loggateway.StepID("knowledge.curate"),
		loggateway.Str("collection_id", colID),
		loggateway.Int("decayed", rep.DecayedEdges),
		loggateway.Int("closed", rep.ClosedEdges),
		loggateway.Int("promoted", len(rep.PromotedRelations)),
		loggateway.Int("stale", rep.StaleMarked),
		loggateway.Int("distilled", rep.DistilledFacts),
		loggateway.Int("hubs_scanned", rep.HubsScanned),
		loggateway.Int("proposals_pending", rep.ProposalsPending),
	)
	return rep, nil
}

// CurateAllTeamKnowledge 枚举 opts.Workspace 下全部团队库逐库治理（dream 周期入口；
// workspace 空 = 跨租户全平台团队库）。opts.CollectionID 非空时退化单库
// CurateKnowledge（knowledge_curate 工具指定集合语义不变）。
// 单库失败 Warn 降级继续，其余库不受影响；无团队库返回与单库版一致的
// NotFound；全部失败返回最后错误（调用方按既有 Warn 降级路径留痕）。
// relation_promote 为全库口径，第二库起 ListPromotableRelations 自然为空（幂等）。
func (u *Usecase) CurateAllTeamKnowledge(ctx context.Context, opts CurateOptions) ([]CurateReport, error) {
	if u == nil || u.curate == nil {
		return nil, apierror.Unavailable(apierror.DomainKnowledge, "knowledge curate repo not wired")
	}
	if strings.TrimSpace(opts.CollectionID) != "" {
		rep, err := u.CurateKnowledge(ctx, opts)
		if err != nil {
			return nil, err
		}
		return []CurateReport{rep}, nil
	}
	cols, _, err := u.collections.ListCollections(ctx, opts.Workspace, 1000, 0)
	if err != nil {
		return nil, err
	}
	var reports []CurateReport
	var lastErr error
	for _, col := range cols {
		if col.VaultBackend != VaultBackendTeam {
			continue
		}
		per := opts
		per.CollectionID = col.ID
		rep, cerr := u.CurateKnowledge(ctx, per)
		if cerr != nil {
			u.lg.Warn("治理：单库治理失败（跳过，其余库继续）",
				loggateway.StepID("knowledge.curate"),
				loggateway.Str("collection_id", col.ID), loggateway.Err(cerr))
			lastErr = cerr
			continue
		}
		reports = append(reports, rep)
	}
	if len(reports) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, apierror.NotFound(apierror.DomainKnowledge, "no team knowledge collection to curate")
	}
	return reports, nil
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
// P1-b（2026-08-16）：applied 先执行实际处置（orphan 删词条 / conflict 关 contradicts
// 边）再落终态——此前 applied 仅改状态无实效，已处置事项因数据未变被周期性重提。
// 处置失败返回错误且提案停留 pending（可重审）；非 pending 提案拒绝重复处置。
func (u *Usecase) ResolveGovernanceProposal(ctx context.Context, id int64, status string) error {
	if u == nil || u.curate == nil {
		return apierror.Unavailable(apierror.DomainKnowledge, "knowledge curate repo not wired")
	}
	if status != ProposalStatusApplied && status != ProposalStatusRejected {
		return apierror.BadRequest(apierror.DomainKnowledge, fmt.Sprintf("invalid proposal status %q", status))
	}
	if status == ProposalStatusApplied {
		view, err := u.curate.GetGovernanceProposal(ctx, id)
		if err != nil {
			return err
		}
		if view.Status != ProposalStatusPending {
			return apierror.BadRequest(apierror.DomainKnowledge, fmt.Sprintf("proposal %d not pending (status %s)", id, view.Status))
		}
		if err := u.applyGovernanceDisposal(ctx, view); err != nil {
			return err
		}
	}
	return u.curate.ResolveGovernanceProposal(ctx, id, status)
}

// applyGovernanceDisposal 按提案 kind 执行 applied 实际处置。payload 缺关键字段
// 降级 no-op（历史提案/手工数据不阻断二审）；moc_emerge 无自动处置（人工建 MOC，
// 设计如此——「MOC 是否参与默认检索」仍是设计开放问题）。
func (u *Usecase) applyGovernanceDisposal(ctx context.Context, view GovernanceProposalView) error {
	switch view.Kind {
	case ProposalKindOrphan:
		docID, _ := view.Payload["doc_id"].(string)
		if strings.TrimSpace(docID) == "" {
			return nil
		}
		// 文档已不存在（并发已删）视为幂等成功；其余错误上抛（提案停留 pending）。
		if err := u.DeleteDocument(ctx, docID); err != nil && !apierror.IsCode(err, apierror.CodeNotFound) {
			return fmt.Errorf("orphan disposal: delete document %s: %w", docID, err)
		}
		u.lg.Info("治理：orphan 提案 applied，孤儿词条已删除",
			loggateway.StepID("knowledge.curate.orphan"),
			loggateway.Str("doc_id", docID),
			loggateway.Int64("proposal_id", view.ID),
		)
	case ProposalKindConflict:
		docID, _ := view.Payload["doc_id"].(string)
		targetID, _ := view.Payload["target_doc_id"].(string)
		if strings.TrimSpace(docID) == "" || strings.TrimSpace(targetID) == "" {
			return nil
		}
		if _, err := u.curate.CloseContradictsEdges(ctx, view.CollectionID, docID, []string{targetID}); err != nil {
			return fmt.Errorf("conflict disposal: close contradicts %s↔%s: %w", docID, targetID, err)
		}
		u.lg.Info("治理：conflict 提案 applied，contradicts 边已关闭",
			loggateway.StepID("knowledge.curate.conflict"),
			loggateway.Str("doc_id", docID),
			loggateway.Str("target_doc_id", targetID),
			loggateway.Int64("proposal_id", view.ID),
		)
	}
	return nil
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

// runDistillTask 高频词条反向蒸馏：热度窗口内高召回词条的摘要卡蒸馏成 L3 轻量事实
// （scope=workspace 全工作区注入）。幂等双保险——fingerprint 冲突键 upsert +
// dedup_key 带摘要 hash（摘要不变跳过，变了重蒸馏）。
// 返回本轮蒸馏条数；未接线/非团队库/无 workspace 时返回 0（静默跳过）。
func (u *Usecase) runDistillTask(ctx context.Context, homeCol Collection, colID string, opts CurateOptions, rep *CurateReport) int {
	if u.hotDocs == nil || u.distill == nil {
		return 0
	}
	if homeCol.ID == "" && u.collections != nil {
		col, err := u.collections.GetCollection(ctx, colID)
		if err != nil {
			u.lg.Warn("治理：distill 集合解析失败（跳过）", loggateway.StepID("knowledge.curate.distill"),
				loggateway.Str("collection_id", colID), loggateway.Err(err))
			return 0
		}
		homeCol = col
	}
	if homeCol.VaultBackend != VaultBackendTeam || strings.TrimSpace(homeCol.Workspace) == "" {
		return 0
	}
	hotIDs, err := u.hotDocs.ListHotDocuments(ctx, colID, opts.DistillSinceDays, opts.DistillMinHits, defaultDistillLimit)
	if err != nil {
		u.lg.Warn("治理：高频词条扫描失败", loggateway.StepID("knowledge.curate.distill"), loggateway.Err(err))
		return 0
	}
	distilled := 0
	for _, docID := range hotIDs {
		doc, derr := u.documents.GetDocument(ctx, docID)
		if derr != nil {
			u.lg.Warn("治理：distill 词条读取失败（跳过）", loggateway.StepID("knowledge.curate.distill"),
				loggateway.Str("doc_id", docID), loggateway.Err(derr))
			continue
		}
		summary := strings.TrimSpace(doc.Summary)
		if summary == "" || !strings.HasPrefix(doc.RelPath, "entries/") {
			continue
		}
		hash := doc.SummaryHash
		if hash == "" {
			hash = doc.ContentHash
		}
		if len(hash) > 12 {
			hash = hash[:12]
		}
		dedup := "distill:" + doc.ID + ":" + hash
		// 去重只看 applied：摘要 hash 变更是新 dedup_key 自然重蒸馏；
		// 同摘要重复周期命中既有 applied 提案即跳过（fingerprint upsert 兜底幂等）。
		if u.skipProposal(ctx, colID, ProposalKindDistill, dedup, []string{ProposalStatusApplied}) {
			continue
		}
		if !opts.DryRun {
			if werr := u.distill.UpsertDistilledFact(ctx, DistilledFact{
				ScopeType:   "workspace",
				ScopeID:     homeCol.Workspace,
				Statement:   summary,
				Fingerprint: "kdistill:" + doc.ID,
				TagsJSON:    marshalStringArray(doc.Tags),
				SourceDocID: doc.ID,
				SourcePath:  doc.RelPath,
			}); werr != nil {
				u.lg.Warn("治理：蒸馏事实写入失败（跳过）", loggateway.StepID("knowledge.curate.distill"),
					loggateway.Str("doc_id", doc.ID), loggateway.Err(werr))
				continue
			}
			u.recordCurateProposal(ctx, GovernanceProposal{
				CollectionID: colID, Kind: ProposalKindDistill, Risk: ProposalRiskLow, Status: ProposalStatusApplied,
				Payload: map[string]any{
					"dedup_key": dedup, "doc_id": doc.ID, "rel_path": doc.RelPath,
					"scope_type": "workspace", "scope_id": homeCol.Workspace,
				},
			})
		}
		distilled++
		rep.Proposals = append(rep.Proposals, CurateProposal{Kind: ProposalKindDistill, Risk: ProposalRiskLow, Status: ProposalStatusApplied, DedupKey: dedup})
	}
	if distilled > 0 {
		rep.Actions = append(rep.Actions, "distill")
	}
	return distilled
}

// runMOCEmergeTask hub 簇涌现 MOC：entries/* 中度数达阈值的 hub 词条，其 1 跳
// 邻居构成概念簇；规模与边密度双达阈值 → 高风险 pending 提案（人工二审后手动
// 建 MOC 词条——「MOC 是否参与默认检索」仍是设计开放问题，不自动落地）。
// 返回本轮参与密度检测的 hub 数。
func (u *Usecase) runMOCEmergeTask(ctx context.Context, colID string, opts CurateOptions, rep *CurateReport) int {
	hubs, err := u.curate.ListHubClusters(ctx, colID, opts.MOCMinDegree, mocHubScanLimit)
	if err != nil {
		u.lg.Warn("治理：hub 簇扫描失败", loggateway.StepID("knowledge.curate.moc"), loggateway.Err(err))
		return 0
	}
	produced := 0
	for _, h := range hubs {
		n := len(h.Neighbors) + 1
		if n < opts.MOCMinMember {
			continue
		}
		docIDs := make([]string, 0, n)
		docIDs = append(docIDs, h.HubDocID)
		members := make([]string, 0, len(h.Neighbors))
		for _, m := range h.Neighbors {
			docIDs = append(docIDs, m.DocID)
			members = append(members, m.RelPath)
		}
		edges, cerr := u.curate.CountActiveEdgesWithin(ctx, colID, docIDs)
		if cerr != nil {
			u.lg.Warn("治理：hub 簇密度检测失败（跳过）", loggateway.StepID("knowledge.curate.moc"),
				loggateway.Str("hub_doc_id", h.HubDocID), loggateway.Err(cerr))
			continue
		}
		density := float64(edges) / (float64(n*(n-1)) / 2)
		if density < opts.MinMOCDensity {
			continue
		}
		dedup := "moc:" + h.HubDocID
		// 去重三态全含：人工已建 MOC（applied）或否决（rejected）的同 hub 不再提案。
		if u.skipProposal(ctx, colID, ProposalKindMOCEmerge, dedup,
			[]string{ProposalStatusPending, ProposalStatusApplied, ProposalStatusRejected}) {
			continue
		}
		title := mocSuggestedTitle(h.HubRelPath)
		p := GovernanceProposal{
			CollectionID: colID, Kind: ProposalKindMOCEmerge, Risk: ProposalRiskHigh, Status: ProposalStatusPending,
			Payload: map[string]any{
				"dedup_key": dedup, "hub_doc_id": h.HubDocID, "hub_rel_path": h.HubRelPath,
				"degree": h.Degree, "members": members, "density": density,
				"suggested_title": title, "suggested_path": "moc/" + title + ".md",
			},
		}
		if !opts.DryRun {
			u.recordCurateProposal(ctx, p)
		}
		produced++
		rep.ProposalsPending++
		rep.Proposals = append(rep.Proposals, CurateProposal{Kind: p.Kind, Risk: p.Risk, Status: ProposalStatusPending, DedupKey: dedup})
	}
	if produced > 0 {
		rep.Actions = append(rep.Actions, "moc_emerge")
	}
	return len(hubs)
}

// mocSuggestedTitle 取词条 rel_path 的 basename（去 .md）作 MOC 建议标题。
func mocSuggestedTitle(relPath string) string {
	base := relPath
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(base, ".md")
}

// marshalStringArray 序列化字符串切片为 JSON 数组（nil/空 → ""，调用方留空语义）。
func marshalStringArray(items []string) string {
	if len(items) == 0 {
		return ""
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	return string(raw)
}

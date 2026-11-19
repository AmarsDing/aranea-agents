// Package approval 实现统一审批中心聚合器（E 阶段 W1b，需求文档 §5.3 拍板 A-1）。
//
// 设计红线（照抄 workbench server/approvals.go「不返回无权对象」模式）：
//   - 只读聚合：决议出口仍走各领域 RPC（agentbridge ReviewTask、进化建议
//     Apply/Reject…），本包不开写旁路；
//   - 逐类按角色/密级/reviewer 规则在源头过滤——无权对象连数量与元数据都
//     不返回（Summary 计数同样逐源过闸）；
//   - 条目形状统一为 Item，proto 映射在 service 层（approval_center.go）。
//
// 一期接入三类现成源（需求文档 §5.4）：
//   - task_review：agentbridge 编码任务验收（review_status='' 的终态任务）；
//   - evolution_suggestion：统一进化存储中 target=agent 的 pending 建议；
//   - skill_approval：统一进化存储 target=skill 的 pending 建议。
// W1c 接入 schedule_draft（office 排期草案，source_schedule_draft.go）。
// mask_proposal/offboard_export/memory_fact 预留（随 D6/D3 落位接入），
// kind 常量已定义、源未注册时对调用方表现为空 lane。
package approval

import (
	"context"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/auth"
)

// 审批类别（与 api/kratos/approval/v1 ApprovalKind 枚举同名小写）。
const (
	KindTaskReview          = "task_review"
	KindMaskProposal        = "mask_proposal"
	KindOffboardExport      = "offboard_export"
	KindEvolutionSuggestion = "evolution_suggestion"
	KindSkillApproval       = "skill_approval"
	KindScheduleDraft       = "schedule_draft"
	KindMemoryFact          = "memory_fact"
)

// 风险级（与 proto RiskLevel 同名小写）。一期为展示级提示；A-4 矩阵消费
// 接线（W3）后才参与决议流控。
const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

// 逐类可见性闸门的权限码（「不返回无权对象」的判定依据）：
//   - PermCodingDispatch 与 agentbridge ReviewTask RPC 的权限码一致——任务
//     验收条目只对实际可决议者可见（决议 RPC 见
//     api/kratos/agentbridge/v1/agentbridge.proto 的 coding.dispatch 注解）；
//   - PermSelfImprovement 与 /self-improvement 控制台同档——进化建议与技能
//     审批是运维向管线条目。
const (
	PermCodingDispatch  = "coding.dispatch"
	PermSelfImprovement = "system.self_improve"
)

// Item 是统一审批条目（proto ApprovalItem 的领域形态）。
type Item struct {
	Kind        string
	ID          string // 领域内条目 id（决议时回传各领域 RPC）
	Title       string
	Summary     string
	ContextJSON string // 类别特定上下文（JSON object 字符串）
	RiskLevel   string
	Requester   string // 申请人/来源 agent（不可自批判定展示用）
	// AgentKey 归因用 agent key（任务验收/日程草案=AgentID，进化建议=TargetID）；
	// 空 = 无法归因（业务线看板显示 —，不做摊派）。
	AgentKey  string
	CreatedAt time.Time
}

// Viewer 是聚合视角的调用方主体（由 JWT 主体派生，禁止请求体自报）。
type Viewer struct {
	UserID      int64
	WorkspaceID string
	has         func(code string) bool
}

// Has 报告主体是否持有权限码（含 * 与 <域>.* 通配，语义见 pkg/auth）。
func (v Viewer) Has(code string) bool {
	return v.has != nil && v.has(code)
}

// ViewerFromContext 从 ctx 的 JWT 主体派生 Viewer；无主体时返回零值
// Viewer（所有 Has 判定为 false——诚实失败，权限过滤收紧为空集而不是放行）。
func ViewerFromContext(ctx context.Context) Viewer {
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return Viewer{}
	}
	return Viewer{
		UserID:      a.UserID,
		WorkspaceID: a.EffectiveWorkspaceID(),
		has:         a.HasPermission,
	}
}

// Source 是一类审批源的统一读口。实现必须在源头应用「不返回无权对象」
// 过滤（viewer 无权时返回空集 + 0，不返回错误——无权不是故障）。
//
// Stability:evolving
type Source interface {
	// Kind 返回该源对应的审批类别常量。
	Kind() string
	// List 返回对 viewer 可见的待批条目（按 CreatedAt 倒序），最多 limit 条。
	List(ctx context.Context, v Viewer, limit int) ([]Item, error)
	// Count 返回对 viewer 可见的待批总数（无权时 0）。
	Count(ctx context.Context, v Viewer) (int64, error)
}

// maxSourceFetch 是单源单次拉取上限：跨源合并分页在内存进行（各类源
// 异构、无统一存储），page*pageSize 超出该上限时截断（一期上限，超越
// 此深度的分页由各域原生列表页承担）。
const maxSourceFetch = 1000

// Aggregator 跨源合并审批条目（表驱动：注册即接入）。
type Aggregator struct {
	sources map[string]Source
	order   []string // 稳定输出序（Summary/全量 List 的 kind 分组顺序）
}

// NewAggregator 装配聚合器；同 kind 重复注册后者覆盖前者（顺序不变）。
func NewAggregator(sources ...Source) *Aggregator {
	a := &Aggregator{sources: make(map[string]Source, len(sources))}
	for _, s := range sources {
		if s == nil {
			continue
		}
		k := s.Kind()
		if _, dup := a.sources[k]; !dup {
			a.order = append(a.order, k)
		}
		a.sources[k] = s
	}
	return a
}

// Kinds 返回已注册类别（注册顺序）。
func (a *Aggregator) Kinds() []string {
	out := make([]string, len(a.order))
	copy(out, a.order)
	return out
}

// List 聚合待批条目：kind 为空串=全部类别；requester 非空=按请求人精确筛选；
// 分页跨源合并（按 CreatedAt 倒序、同刻按 Kind/ID 字典序稳定排）。total 为
// 可见条目总数（各源过闸后的 Count 之和；requester 筛选时取合并后条数，
// 单源上限 maxSourceFetch）。
func (a *Aggregator) List(ctx context.Context, v Viewer, kind, requester string, page, pageSize int) ([]Item, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	requester = strings.TrimSpace(requester)
	kinds := a.selectKinds(kind)
	if kinds == nil {
		return nil, 0, errUnknownKind(kind)
	}

	fetch := page * pageSize
	// requester 筛选需要在合并后过滤，取满单源上限才能保证 total 准确。
	if requester != "" || fetch > maxSourceFetch {
		fetch = maxSourceFetch
	}
	var merged []Item
	var total int64
	for _, k := range kinds {
		src := a.sources[k]
		items, err := src.List(ctx, v, fetch)
		if err != nil {
			return nil, 0, err
		}
		if requester != "" {
			for _, it := range items {
				if it.Requester == requester {
					merged = append(merged, it)
				}
			}
			continue
		}
		n, err := src.Count(ctx, v)
		if err != nil {
			return nil, 0, err
		}
		merged = append(merged, items...)
		total += n
	}
	if requester != "" {
		total = int64(len(merged))
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if !merged[i].CreatedAt.Equal(merged[j].CreatedAt) {
			return merged[i].CreatedAt.After(merged[j].CreatedAt)
		}
		if merged[i].Kind != merged[j].Kind {
			return merged[i].Kind < merged[j].Kind
		}
		return merged[i].ID < merged[j].ID
	})
	off := (page - 1) * pageSize
	if off >= len(merged) {
		return []Item{}, total, nil
	}
	end := off + pageSize
	if end > len(merged) {
		end = len(merged)
	}
	return merged[off:end], total, nil
}

// Summary 返回逐类可见计数（铃铛/金门徽标用）。无权类别不出现（不泄漏
// 存在性）；已注册但当前为零的类别以 0 出现（便于前端渲染空 lane）。
func (a *Aggregator) Summary(ctx context.Context, v Viewer) (map[string]int64, int64, error) {
	counts := make(map[string]int64, len(a.order))
	var total int64
	for _, k := range a.order {
		n, err := a.sources[k].Count(ctx, v)
		if err != nil {
			return nil, 0, err
		}
		counts[k] = n
		total += n
	}
	return counts, total, nil
}

// selectKinds 解析类别过滤：空串=全部已注册类别；未知类别返回 nil。
func (a *Aggregator) selectKinds(kind string) []string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return a.Kinds()
	}
	if _, ok := a.sources[kind]; !ok {
		return nil
	}
	return []string{kind}
}

type unknownKindError string

func (e unknownKindError) Error() string {
	return "approval: unknown kind " + string(e)
}

// IsUnknownKind 报告错误是否为「未注册/未知审批类别」（service 层映 400）。
func IsUnknownKind(err error) bool {
	_, ok := err.(unknownKindError)
	return ok
}

func errUnknownKind(kind string) error { return unknownKindError(kind) }

// firstLine 取首行并截断（任务标题派生自用）。
func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}

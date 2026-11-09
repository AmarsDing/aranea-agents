package approval

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	bridge "aranea-agents/internal/biz/agentbridge"
)

// ============================================================================
// 三类一期现成源（需求文档 §5.4）。窄端口接口便于测试桩注入；真实实现分别
// 由 data 层 codingTaskRepo / unifiedEvolutionRepo 满足（wire 装配）。
// ============================================================================

// CodingTaskReviewReader 是 agentbridge 任务验收待办的窄读口。
// Stability:evolving
type CodingTaskReviewReader interface {
	// ListPendingReview 返回待验收（review_status=''）的终态编码任务，
	// 按创建时间倒序；workspaceID 非空时按租户过滤。
	ListPendingReview(ctx context.Context, workspaceID string, limit int) ([]*bridge.CodingTask, int, error)
}

// EvolutionQueryReader 是统一进化存储的窄读口（= biz.UnifiedEvolutionQueryReader
// 的子集：聚合只需 List/Count 两方法）。
// Stability:evolving
type EvolutionQueryReader interface {
	ListByTarget(ctx context.Context, targetType string, targetID string, workspaceID string, status string, limit, offset int) ([]biz.UnifiedEvolutionSuggestion, error)
	CountByTarget(ctx context.Context, targetType string, targetID string, workspaceID string, status string) (int, error)
}

// taskReviewSource 任务验收源。可见性闸门=PermCodingDispatch（与决议 RPC
// ReviewTask 的权限码一致：无权决议者连条目都不可见）。租户过滤在源内
// SQL 完成（workspace = viewer 所属空间）。
type taskReviewSource struct {
	tasks CodingTaskReviewReader
}

// NewTaskReviewSource 装配任务验收源；tasks 为 nil 时不注册（装配方负责跳过）。
func NewTaskReviewSource(tasks CodingTaskReviewReader) Source {
	if tasks == nil {
		return nil
	}
	return &taskReviewSource{tasks: tasks}
}

func (s *taskReviewSource) Kind() string { return KindTaskReview }

func (s *taskReviewSource) List(ctx context.Context, v Viewer, limit int) ([]Item, error) {
	if !v.Has(PermCodingDispatch) {
		return nil, nil
	}
	tasks, _, err := s.tasks.ListPendingReview(ctx, v.WorkspaceID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, codingTaskToItem(t))
	}
	return out, nil
}

func (s *taskReviewSource) Count(ctx context.Context, v Viewer) (int64, error) {
	if !v.Has(PermCodingDispatch) {
		return 0, nil
	}
	_, total, err := s.tasks.ListPendingReview(ctx, v.WorkspaceID, 1)
	if err != nil {
		return 0, err
	}
	return int64(total), nil
}

func codingTaskToItem(t *bridge.CodingTask) Item {
	ctxJSON, _ := json.Marshal(map[string]any{
		"workspace":  t.Workspace,
		"agent_id":   t.AgentID,
		"session_id": t.SessionID,
		"status":     string(t.Status),
		"origin":     t.Origin,
		"error":      t.Error,
	})
	return Item{
		Kind:        KindTaskReview,
		ID:          t.ID,
		Title:       firstLine(t.Prompt, 80),
		Summary:     firstLine(t.Summary, 200),
		ContextJSON: string(ctxJSON),
		// 编码任务涉及主机命令执行面，默认中风险（矩阵生效后由 A-4 覆盖）。
		RiskLevel: RiskMedium,
		Requester: t.AgentID,
		CreatedAt: parseRFC3339(t.CreatedAt),
	}
}

// evolutionSource 统一进化存储源（target=agent → 进化建议类；target=skill
// → 技能审批类）。可见性闸门=PermSelfImprovement（自改进控制台同档）；
// 租户过滤由存储层 ListByTarget 的 workspaceID 语义承担（自有 + '' 共享行）。
type evolutionSource struct {
	kind       string
	targetType string
	reader     EvolutionQueryReader
}

// NewEvolutionSuggestionSource 装配进化建议源（target=agent）。
func NewEvolutionSuggestionSource(reader EvolutionQueryReader) Source {
	if reader == nil {
		return nil
	}
	return &evolutionSource{kind: KindEvolutionSuggestion, targetType: string(biz.EvolutionTargetAgent), reader: reader}
}

// NewSkillApprovalSource 装配技能审批源（target=skill）。
func NewSkillApprovalSource(reader EvolutionQueryReader) Source {
	if reader == nil {
		return nil
	}
	return &evolutionSource{kind: KindSkillApproval, targetType: string(biz.EvolutionTargetSkill), reader: reader}
}

func (s *evolutionSource) Kind() string { return s.kind }

func (s *evolutionSource) List(ctx context.Context, v Viewer, limit int) ([]Item, error) {
	if !v.Has(PermSelfImprovement) {
		return nil, nil
	}
	rows, err := s.reader.ListByTarget(ctx, s.targetType, "", v.WorkspaceID, string(biz.UnifiedEvolutionStatePending), limit, 0)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for i := range rows {
		out = append(out, evolutionToItem(s.kind, &rows[i]))
	}
	return out, nil
}

func (s *evolutionSource) Count(ctx context.Context, v Viewer) (int64, error) {
	if !v.Has(PermSelfImprovement) {
		return 0, nil
	}
	n, err := s.reader.CountByTarget(ctx, s.targetType, "", v.WorkspaceID, string(biz.UnifiedEvolutionStatePending))
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}

func evolutionToItem(kind string, s *biz.UnifiedEvolutionSuggestion) Item {
	title := s.DraftName
	if title == "" {
		title = s.MetaString(biz.EvoMetaTitle)
	}
	if title == "" {
		title = s.TargetID
	}
	ctxJSON, _ := json.Marshal(map[string]any{
		"target_type":      string(s.TargetType),
		"target_id":        s.TargetID,
		"action_type":      string(s.ActionType),
		"trigger_source":   s.TriggerSource,
		"lifecycle_status": s.LifecycleStatus,
		"sandbox_passed":   s.SandboxPassed,
		"merge_target_id":  s.MergeTargetID,
		"workspace_id":     s.WorkspaceID,
	})
	return Item{
		Kind:        kind,
		ID:          s.ID,
		Title:       firstLine(title, 80),
		Summary:     firstLine(s.TriggerReason, 200),
		ContextJSON: string(ctxJSON),
		RiskLevel:   evolutionRisk(s.Priority),
		Requester:   s.TriggerSource,
		CreatedAt:   s.CreatedAt,
	}
}

// evolutionRisk 由 priority（0=低 1=中 2=高 3=紧急）映射三级风险。
func evolutionRisk(priority int) string {
	switch {
	case priority >= 2:
		return RiskHigh
	case priority == 1:
		return RiskMedium
	default:
		return RiskLow
	}
}

func parseRFC3339(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

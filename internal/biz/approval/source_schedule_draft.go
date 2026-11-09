package approval

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz/office"
)

// ============================================================================
// schedule_draft 源（E 阶段 W1c，需求文档 §2.3「agent 规划草案逐项审批」）。
// 可见性闸门=office.schedule（与决议 RPC Approve/RejectScheduleDraft 的权限
// 码一致：无权决议者连条目都不可见）。租户过滤在源内 SQL 完成（workspace
// = viewer 所属空间）。
// ============================================================================

// PermOfficeSchedule 日程域权限码（schedule_draft 可见性闸门）。
const PermOfficeSchedule = "office.schedule"

// ScheduleDraftReader 日程草案窄读口（签名与 office.DraftRepo 对应方法一致，
// data 层 office 仓储直接满足）。
// Stability:evolving
type ScheduleDraftReader interface {
	ListPendingDrafts(ctx context.Context, workspaceID string, limit int) ([]*office.ScheduleDraft, error)
	CountPendingDrafts(ctx context.Context, workspaceID string) (int64, error)
}

type scheduleDraftSource struct {
	drafts ScheduleDraftReader
}

// NewScheduleDraftSource 装配日程草案源；drafts 为 nil 时不注册。
func NewScheduleDraftSource(drafts ScheduleDraftReader) Source {
	if drafts == nil {
		return nil
	}
	return &scheduleDraftSource{drafts: drafts}
}

func (s *scheduleDraftSource) Kind() string { return KindScheduleDraft }

func (s *scheduleDraftSource) List(ctx context.Context, v Viewer, limit int) ([]Item, error) {
	if !v.Has(PermOfficeSchedule) {
		return nil, nil
	}
	rows, err := s.drafts.ListPendingDrafts(ctx, v.WorkspaceID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for _, d := range rows {
		out = append(out, scheduleDraftToItem(d))
	}
	return out, nil
}

func (s *scheduleDraftSource) Count(ctx context.Context, v Viewer) (int64, error) {
	if !v.Has(PermOfficeSchedule) {
		return 0, nil
	}
	return s.drafts.CountPendingDrafts(ctx, v.WorkspaceID)
}

// scheduleDraftToItem 草案 → 统一条目。context_json 携带完整 items（审批人
// 须在详情里看到要批准什么）。
func scheduleDraftToItem(d *office.ScheduleDraft) Item {
	ctxJSON, _ := json.Marshal(map[string]any{
		"agent_id":     d.AgentID,
		"session_id":   d.SessionID,
		"user_id":      d.UserID,
		"workspace_id": d.WorkspaceID,
		"item_count":   len(d.Items),
		"items":        d.Items,
	})
	title := fmt.Sprintf("agent 排期草案（%d 项）", len(d.Items))
	if len(d.Items) > 0 && d.Items[0].Title != "" {
		title = firstLine(d.Items[0].Title, 80)
		if len(d.Items) > 1 {
			title = fmt.Sprintf("%s 等 %d 项", title, len(d.Items))
		}
	}
	return Item{
		Kind:        KindScheduleDraft,
		ID:          d.ID,
		Title:       title,
		Summary:     fmt.Sprintf("来自 %s 的排期建议，共 %d 项", firstNonEmpty(d.AgentID, "agent"), len(d.Items)),
		ContextJSON: string(ctxJSON),
		// agent 草案=中风险（需求文档 §2.3 风险分级；A-4 矩阵生效后可覆盖）。
		RiskLevel: RiskMedium,
		Requester: d.AgentID,
		CreatedAt: d.CreatedAt,
	}
}

func firstNonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

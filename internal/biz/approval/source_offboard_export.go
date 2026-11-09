package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"aranea-agents/internal/biz"
)

// ============================================================================
// offboard_export 源（E 阶段 W2a D3，需求文档 §5.5 第三类「离职导出」）。
// 可见性闸门=user.offboard（sysadmin 专属码位，builtin 角色表唯一持有者；
// superadmin 经 * 通配可见）+ 非自提过滤（RequesterID == viewer.UserID 的
// 条目不返回——不可自批在审批中心侧的展示兜底；服务端强制在
// SecopsOffboardUsecase.ApproveOffboardExport）。
// wb 对板（approvals.go L19-108）：「离职导出（仅 sysadmin 且非自提）」。
// ============================================================================

// PermUserOffboard 是离职处置域权限码（service/middleware 与本闸门同源；
// pkg/auth/permissions.go 中 sysadmin 专属）。
const PermUserOffboard = "user.offboard"

// OffboardRequestReader 是离职申请单窄读口（data 层 SecopsOffboardRequestRepo
// 方法集超集，wire 侧直接转接口）。biz.SecopsOffboardRequest 定义在 biz 根包
//（sources.go 已 import biz 根包，无环）。
type OffboardRequestReader interface {
	// ListPending 返回 status='requested' 的申请（created_at 倒序）。
	ListPending(ctx context.Context, limit int) ([]*biz.SecopsOffboardRequest, error)
}

type offboardExportSource struct {
	requests OffboardRequestReader
}

// NewOffboardExportSource 装配离职导出源；requests 为 nil 时不注册。
func NewOffboardExportSource(requests OffboardRequestReader) Source {
	if requests == nil {
		return nil
	}
	return &offboardExportSource{requests: requests}
}

func (s *offboardExportSource) Kind() string { return KindOffboardExport }

func (s *offboardExportSource) List(ctx context.Context, v Viewer, limit int) ([]Item, error) {
	rows, ok, err := s.visible(ctx, v, limit)
	if !ok || err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for _, r := range rows {
		// 非自提：申请人看不到自己的单（不能自批，展示无意义）。
		if r.RequesterID == v.UserID {
			continue
		}
		out = append(out, offboardRequestToItem(r))
	}
	return out, nil
}

func (s *offboardExportSource) Count(ctx context.Context, v Viewer) (int64, error) {
	rows, ok, err := s.visible(ctx, v, maxSourceFetch)
	if !ok || err != nil {
		return 0, err
	}
	var n int64
	for _, r := range rows {
		if r.RequesterID != v.UserID {
			n++
		}
	}
	return n, nil
}

// visible 是闸门 + 拉取公共段：无权返回 ok=false（空集而非错误——
// 「不返回无权对象」模式）。
func (s *offboardExportSource) visible(ctx context.Context, v Viewer, limit int) ([]*biz.SecopsOffboardRequest, bool, error) {
	if !v.Has(PermUserOffboard) {
		return nil, false, nil
	}
	rows, err := s.requests.ListPending(ctx, limit)
	if err != nil {
		return nil, false, err
	}
	return rows, true, nil
}

// offboardRequestToItem 申请单 → 统一条目。context_json 携带审批人决议所需
// 最小上下文（目标用户/申请人/单号）；个人数据本体不进审批中心（导出在
// 决议后走 GetOffboardExport 专口）。
func offboardRequestToItem(r *biz.SecopsOffboardRequest) Item {
	ctxJSON, _ := json.Marshal(map[string]any{
		"request_id":   r.ID,
		"user_id":      r.TargetUserID,
		"requester_id": r.RequesterID,
		"status":       r.Status,
	})
	return Item{
		Kind:        KindOffboardExport,
		ID:          r.ID,
		Title:       fmt.Sprintf("离职数据导出审批：用户 #%d", r.TargetUserID),
		Summary:     fmt.Sprintf("申请人 #%d 请求导出该用户 user 域全量记忆（决议后 sysadmin 执行导出）", r.RequesterID),
		ContextJSON: string(ctxJSON),
		// 个人数据导出=高风险（需求 §5.5 风险分级口径）。
		RiskLevel: RiskHigh,
		Requester: strconv.FormatInt(r.RequesterID, 10),
		CreatedAt: r.CreatedAt,
	}
}

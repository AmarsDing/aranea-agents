package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/secops/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ============================================================================
// SecopsService 实现 secops.v1 的离职处置五步（E 阶段 W2a D3，需求 §6.3）。
// 标签/模型策略/脱敏五闸等 RPC 由 D6（W2b）收编——本结构嵌入
// UnimplementedSecopsServiceServer，未实现的 RPC 返回 Unimplemented。
//
// 权限模型（双保险）：
//   - middleware 硬判：六个离职 RPC 登记为 user.offboard（sysadmin 专属码位，
//     superadmin 经 * 通配）——登记表由 make api 生成，非手工。
//   - service 兜底：每 RPC 复核调用者身份（防登记表被旁路或豁免误配）。
//     freeze/request/delete/list 要求 HasPermission("user.offboard")；
//     approve/export 要求 HasRole(sysadmin|superadmin)——需求钉死「审批/导出
//     仅 sysadmin」，码位持有不足以授权（DB 自定义角色可被授予 user.offboard，
//     但审批个人数据导出必须是 sysadmin 角色本身）。
//
// wb 偏离点（W4 复核）：wb freeze/request 允许 secadmin——aranea 收敛为
// sysadmin 专属（用户生命周期=user.* 码位）；ListOffboardRequests 不含
// auditor（auditor 走 audit.read 审计面）。
// ============================================================================

// SecopsService 是 secops.v1 服务实现。
type SecopsService struct {
	v1.UnimplementedSecopsServiceServer
	offboard *biz.SecopsOffboardUsecase
	lg       loggateway.Logger
}

// NewSecopsService 装配 secops 服务。
func NewSecopsService(offboard *biz.SecopsOffboardUsecase, lg loggateway.Logger) *SecopsService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SecopsService{offboard: offboard, lg: lg}
}

func (s *SecopsService) requireWired() error {
	if s.offboard == nil {
		return apierror.Internal(apierror.DomainAdmin, "secops offboard usecase not wired")
	}
	return nil
}

// secopsCaller 取 JWT 主体；无主体 → Unauthorized（middleware 之前不得放行）。
func secopsCaller(ctx context.Context) (*auth.Auth, error) {
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return nil, apierror.Unauthorized(apierror.DomainAdmin, "authentication required")
	}
	return a, nil
}

// requireOffboardPermission 兜底闸门：user.offboard（sysadmin 专属码位）。
func requireOffboardPermission(a *auth.Auth) error {
	if !a.HasPermission("user.offboard") {
		return apierror.Forbidden(apierror.DomainAdmin, "user.offboard permission required")
	}
	return nil
}

// requireSysadminRole 是 approve/export 的角色硬门（需求 §6.3：sysadmin 审批
// 且不可自批；superadmin 为内建全权角色，与 HasAnyRole 既有用法一致）。
func requireSysadminRole(a *auth.Auth) error {
	if !a.HasAnyRole("sysadmin", "superadmin") {
		return apierror.Forbidden(apierror.DomainAdmin, "sysadmin role required")
	}
	return nil
}

// parseOffboardUserID 解析路径/体里的用户 id（string→int64，去空白）。
func parseOffboardUserID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, apierror.BadRequest(apierror.DomainAdmin, "user_id must be a positive integer")
	}
	return id, nil
}

// ── 离职五步 ─────────────────────────────────────────────────────────

// FreezeUser 第一步：冻结（禁登录 + 吊销全部会话）。
func (s *SecopsService) FreezeUser(ctx context.Context, req *v1.FreezeUserRequest) (*emptypb.Empty, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireOffboardPermission(caller); err != nil {
		return nil, err
	}
	uid, err := parseOffboardUserID(req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err := s.offboard.FreezeUser(ctx, uid, caller.UserID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// RequestOffboardExport 第二步：发起导出申请（审批中心「离职导出」类数据源）。
func (s *SecopsService) RequestOffboardExport(ctx context.Context, req *v1.RequestOffboardExportRequest) (*v1.OffboardRequest, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireOffboardPermission(caller); err != nil {
		return nil, err
	}
	uid, err := parseOffboardUserID(req.GetUserId())
	if err != nil {
		return nil, err
	}
	out, err := s.offboard.RequestOffboardExport(ctx, uid, caller.UserID)
	if err != nil {
		return nil, err
	}
	return pbOffboardRequest(out), nil
}

// ApproveOffboardExport 第三步：审批（仅 sysadmin 且不可自批——角色门在此，
// 自批判定在 biz 层按数字 id 强制）。
func (s *SecopsService) ApproveOffboardExport(ctx context.Context, req *v1.ApproveOffboardExportRequest) (*v1.OffboardRequest, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSysadminRole(caller); err != nil {
		return nil, err
	}
	uid, err := parseOffboardUserID(req.GetUserId())
	if err != nil {
		return nil, err
	}
	out, err := s.offboard.ApproveOffboardExport(ctx, uid, caller.UserID, req.GetApprove(), req.GetNote())
	if err != nil {
		return nil, err
	}
	return pbOffboardRequest(out), nil
}

// GetOffboardExport 第四步：凭 approved 单导出 user 域全量（解密；仅 sysadmin）。
func (s *SecopsService) GetOffboardExport(ctx context.Context, req *v1.GetOffboardExportRequest) (*v1.OffboardExport, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSysadminRole(caller); err != nil {
		return nil, err
	}
	uid, err := parseOffboardUserID(req.GetUserId())
	if err != nil {
		return nil, err
	}
	out, err := s.offboard.GetOffboardExport(ctx, uid, caller.UserID)
	if err != nil {
		return nil, err
	}
	return &v1.OffboardExport{
		UserId:     strconv.FormatInt(out.UserID, 10),
		ItemsJson:  out.ItemsJSON,
		ExportedAt: timestamppb.New(out.ExportedAt),
	}, nil
}

// DeleteOffboardedUser 第五步：删除（前置 approved + 已导出，否则拒）。
func (s *SecopsService) DeleteOffboardedUser(ctx context.Context, req *v1.DeleteOffboardedUserRequest) (*emptypb.Empty, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireOffboardPermission(caller); err != nil {
		return nil, err
	}
	uid, err := parseOffboardUserID(req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err := s.offboard.DeleteOffboardedUser(ctx, uid, caller.UserID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ListOffboardRequests 查询最近申请单（sysadmin 控制台）。
func (s *SecopsService) ListOffboardRequests(ctx context.Context, _ *emptypb.Empty) (*v1.ListOffboardRequestsResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireOffboardPermission(caller); err != nil {
		return nil, err
	}
	rows, err := s.offboard.ListOffboardRequests(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.OffboardRequest, 0, len(rows))
	for _, r := range rows {
		items = append(items, pbOffboardRequest(r))
	}
	return &v1.ListOffboardRequestsResponse{Items: items}, nil
}

// ── 转换 ────────────────────────────────────────────────────────────

func pbOffboardRequest(r *biz.SecopsOffboardRequest) *v1.OffboardRequest {
	if r == nil {
		return nil
	}
	out := &v1.OffboardRequest{
		Id:          r.ID,
		UserId:      strconv.FormatInt(r.TargetUserID, 10),
		RequestedBy: strconv.FormatInt(r.RequesterID, 10),
		Status:      r.Status,
		ApprovedBy:  strconv.FormatInt(r.ApproverID, 10),
		CreatedAt:   pbTimestampOrNil(r.CreatedAt),
		ApprovedAt:  pbTimestampOrNil(r.ApprovedAt),
	}
	return out
}

// pbTimestampOrNil 零值 → nil（proto optional 语义；审批前无 approved_at）。
func pbTimestampOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

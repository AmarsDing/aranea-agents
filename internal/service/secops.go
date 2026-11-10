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
// SecopsService 实现 secops.v1：离职处置五步（E 阶段 W2a D3，需求 §6.3）
// + 密级标签/模型策略四 RPC（E 阶段 W2b D6 L-1/L-4，需求 §7.3）。
// 脱敏五闸等 RPC 由 D6 后续纵列收编——本结构嵌入
// UnimplementedSecopsServiceServer，未实现的 RPC 返回 Unimplemented。
//
// 权限模型（双保险）：
//   - middleware 硬判：六个离职 RPC 登记为 user.offboard（sysadmin 专属码
//     位，superadmin 经 * 通配）；标签读/模型策略读写登记为 secops.policy；
//     标签列表登记为 authenticated——登记表由 make api 生成，非手工。
//   - service 兜底：每 RPC 复核调用者身份（防登记表被旁路或豁免误配）。
//     离职 RPC：freeze/request/delete/list 要求 HasPermission("user.offboard")，
//     approve/export 要求 HasRole(sysadmin|superadmin)——需求钉死「审批/导出
//     仅 sysadmin」，码位持有不足以授权（DB 自定义角色可被授予 user.offboard，
//     但审批个人数据导出必须是 sysadmin 角色本身）。
//     密级标签写/模型策略读写：HasPermission("secops.policy")（secadmin 域）；
//     标签列表：仅要求认证态（任何登录用户可读密级词表）。
//
// wb 偏离点（W4 复核）：wb freeze/request 允许 secadmin——aranea 收敛为
// sysadmin 专属（用户生命周期=user.* 码位）；ListOffboardRequests 不含
// auditor（auditor 走 audit.read 审计面）。
// ============================================================================

// SecopsService 是 secops.v1 服务实现。
type SecopsService struct {
	v1.UnimplementedSecopsServiceServer
	offboard       *biz.SecopsOffboardUsecase
	classification *biz.SecopsClassificationUsecase
	policy         *biz.SecopsModelPolicyUsecase
	mask           *biz.SecopsMaskProposalUsecase
	lg             loggateway.Logger
}

// NewSecopsService 装配 secops 服务。
func NewSecopsService(offboard *biz.SecopsOffboardUsecase, classification *biz.SecopsClassificationUsecase, policy *biz.SecopsModelPolicyUsecase, mask *biz.SecopsMaskProposalUsecase, lg loggateway.Logger) *SecopsService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SecopsService{offboard: offboard, classification: classification, policy: policy, mask: mask, lg: lg}
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

// requireSecopsPolicyPermission 是密级标签写/模型策略读写的兜底闸门：
// secops.policy 码位（secadmin 域；sysadmin/superadmin 经角色权限映射持有）。
func requireSecopsPolicyPermission(a *auth.Auth) error {
	if !a.HasPermission("secops.policy") {
		return apierror.Forbidden(apierror.DomainAdmin, "secops.policy permission required")
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

// ── 密级标签 + 模型策略（D6 W2b，需求 §7.3 验收 #2/#5 数据面）──────────

// requireClassificationWired/requirePolicyWired 是各自 usecase 的装配闸
// （与 requireWired 同契约；gate probe 测试用 nil usecase 钉死闸门序）。
func (s *SecopsService) requireClassificationWired() error {
	if s.classification == nil {
		return apierror.Internal(apierror.DomainAdmin, "secops classification usecase not wired")
	}
	return nil
}

func (s *SecopsService) requirePolicyWired() error {
	if s.policy == nil {
		return apierror.Internal(apierror.DomainAdmin, "secops model policy usecase not wired")
	}
	return nil
}

// ListClassificationLabels 密级词表（authenticated：任何登录用户可读）。
func (s *SecopsService) ListClassificationLabels(ctx context.Context, _ *emptypb.Empty) (*v1.ListClassificationLabelsResponse, error) {
	if _, err := secopsCaller(ctx); err != nil {
		return nil, err
	}
	if err := s.requireClassificationWired(); err != nil {
		return nil, err
	}
	rows, err := s.classification.ListLabels(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.ClassificationLabel, 0, len(rows))
	for _, r := range rows {
		items = append(items, &v1.ClassificationLabel{
			Level:       int32(r.Level),
			Name:        r.Name,
			Description: r.Description,
		})
	}
	return &v1.ListClassificationLabelsResponse{Items: items}, nil
}

// PutClassificationLabel 新增/改名密级标签（secops.policy；写后审计在 biz）。
func (s *SecopsService) PutClassificationLabel(ctx context.Context, req *v1.PutClassificationLabelRequest) (*v1.ClassificationLabel, error) {
	if err := s.requireClassificationWired(); err != nil {
		return nil, err
	}
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	out, err := s.classification.PutLabel(ctx, int(req.GetLevel()), req.GetName(), req.GetDescription(), caller.UserID)
	if err != nil {
		return nil, err
	}
	return &v1.ClassificationLabel{
		Level:       int32(out.Level),
		Name:        out.Name,
		Description: out.Description,
	}, nil
}

// GetModelPolicy 读模型路由策略（secops.policy；阈值+更新人+更新时间）。
func (s *SecopsService) GetModelPolicy(ctx context.Context, _ *emptypb.Empty) (*v1.ModelPolicy, error) {
	if err := s.requirePolicyWired(); err != nil {
		return nil, err
	}
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	out, err := s.policy.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.ModelPolicy{
		LocalThreshold: int32(out.LocalThreshold),
		UpdatedBy:      out.UpdatedBy,
		UpdatedAt:      pbTimestampOrNil(out.UpdatedAt),
	}, nil
}

// SetModelPolicy 更新本地阈值（secops.policy；1..5 校验在 biz，变更审计
// secops.classification.policy_update 记新旧阈值+actor）。
func (s *SecopsService) SetModelPolicy(ctx context.Context, req *v1.SetModelPolicyRequest) (*v1.ModelPolicy, error) {
	if err := s.requirePolicyWired(); err != nil {
		return nil, err
	}
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	out, err := s.policy.SetModelPolicy(ctx, int(req.GetLocalThreshold()), caller.UserID)
	if err != nil {
		return nil, err
	}
	return &v1.ModelPolicy{
		LocalThreshold: int32(out.LocalThreshold),
		UpdatedBy:      out.UpdatedBy,
		UpdatedAt:      pbTimestampOrNil(out.UpdatedAt),
	}, nil
}

// ── 脱敏五闸（D6 V4：提案/样本试跑/静态检查/审批/激活）────────────────
// 权限：全组 secops.policy（secadmin 域）；提案永不自动生效——审批不可自批
// 由 biz 用例硬校验（approve/reject 同一校验），批准只生成 shadow 规则。

func (s *SecopsService) requireMaskWired() error {
	if s.mask == nil {
		return apierror.Internal(apierror.DomainAdmin, "secops mask proposal usecase not wired")
	}
	return nil
}

// ListMaskRules 列脱敏规则（含 shadow/灰度/禁用；安全管理员维护视图）。
func (s *SecopsService) ListMaskRules(ctx context.Context, _ *emptypb.Empty) (*v1.ListMaskRulesResponse, error) {
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	if err := s.requireMaskWired(); err != nil {
		return nil, err
	}
	rules, err := s.mask.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.MaskRule, 0, len(rules))
	for _, r := range rules {
		items = append(items, &v1.MaskRule{
			Id:         r.ID,
			Entity:     r.Entity,
			Pattern:    r.Pattern,
			Operator:   r.Operator,
			Enabled:    r.Enabled,
			Shadow:     r.Shadow,
			RolloutPct: int32(r.RolloutPct),
			Version:    int64(r.Version),
		})
	}
	return &v1.ListMaskRulesResponse{Items: items}, nil
}

// ListMaskProposals 分页列提案（status 空 = 全部）。
func (s *SecopsService) ListMaskProposals(ctx context.Context, req *v1.ListMaskProposalsRequest) (*v1.ListMaskProposalsResponse, error) {
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	if err := s.requireMaskWired(); err != nil {
		return nil, err
	}
	items, total, err := s.mask.ListProposals(ctx, req.GetStatus(), int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MaskProposal, 0, len(items))
	for i := range items {
		out = append(out, pbMaskProposal(&items[i]))
	}
	return &v1.ListMaskProposalsResponse{Items: out, Total: total}, nil
}

// CreateMaskProposal 闸门1：提案入库（恒 proposed，AI/手工同等待遇）。
func (s *SecopsService) CreateMaskProposal(ctx context.Context, req *v1.CreateMaskProposalRequest) (*v1.MaskProposal, error) {
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	if err := s.requireMaskWired(); err != nil {
		return nil, err
	}
	out, err := s.mask.CreateProposal(ctx, biz.MaskProposalInput{
		Entity:       req.GetEntity(),
		Pattern:      req.GetPattern(),
		WordlistJSON: req.GetWordlistJson(),
		Operator:     req.GetOperator(),
	}, strconv.FormatInt(caller.UserID, 10))
	if err != nil {
		return nil, err
	}
	return pbMaskProposal(out), nil
}

// RunMaskGates 闸门2+3：样本试跑 → 静态检查（任一不过落终态，不进审批）。
func (s *SecopsService) RunMaskGates(ctx context.Context, req *v1.MaskProposalActionRequest) (*v1.MaskProposal, error) {
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	if err := s.requireMaskWired(); err != nil {
		return nil, err
	}
	out, err := s.mask.RunMaskGates(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return pbMaskProposal(out), nil
}

// ApproveMaskProposal 闸门4：人工审批（不可自批；批准只生成 shadow 规则）。
func (s *SecopsService) ApproveMaskProposal(ctx context.Context, req *v1.MaskProposalActionRequest) (*v1.MaskProposal, error) {
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	if err := s.requireMaskWired(); err != nil {
		return nil, err
	}
	out, err := s.mask.Approve(ctx, req.GetId(), strconv.FormatInt(caller.UserID, 10))
	if err != nil {
		return nil, err
	}
	return pbMaskProposal(out), nil
}

// RejectMaskProposal 闸门4（拒绝分支）：仅 pending_approval 可拒。
func (s *SecopsService) RejectMaskProposal(ctx context.Context, req *v1.MaskProposalActionRequest) (*v1.MaskProposal, error) {
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	if err := s.requireMaskWired(); err != nil {
		return nil, err
	}
	out, err := s.mask.Reject(ctx, req.GetId(), strconv.FormatInt(caller.UserID, 10))
	if err != nil {
		return nil, err
	}
	return pbMaskProposal(out), nil
}

// ActivateMaskProposal 闸门5→生效：影子规则转正并按 rollout_pct 灰度。
func (s *SecopsService) ActivateMaskProposal(ctx context.Context, req *v1.ActivateMaskProposalRequest) (*v1.MaskProposal, error) {
	caller, err := secopsCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireSecopsPolicyPermission(caller); err != nil {
		return nil, err
	}
	if err := s.requireMaskWired(); err != nil {
		return nil, err
	}
	out, err := s.mask.Activate(ctx, req.GetId(), int(req.GetRolloutPct()))
	if err != nil {
		return nil, err
	}
	return pbMaskProposal(out), nil
}

// ── 转换 ────────────────────────────────────────────────────────────

// pbMaskProposal 提案 → proto（sample_recall 为 proto double：nil → 0，
// 未跑闸与召回 0 在协议层不可区分，终态靠 status 判定）。
func pbMaskProposal(p *biz.MaskProposal) *v1.MaskProposal {
	if p == nil {
		return nil
	}
	out := &v1.MaskProposal{
		Id:                 p.ID,
		Entity:             p.Entity,
		Pattern:            p.Pattern,
		WordlistJson:       p.WordlistJSON,
		Operator:           p.Operator,
		Status:             p.Status,
		SampleDetailJson:   p.SampleDetailJSON,
		StaticFindingsJson: p.StaticFindingsJSON,
		CreatedBy:          p.CreatedBy,
		ReviewedBy:         p.ReviewedBy,
		ReviewedAt:         pbTimestampOrNil(p.ReviewedAt),
		CreatedAt:          pbTimestampOrNil(p.CreatedAt),
	}
	if p.SampleRecall != nil {
		out.SampleRecall = *p.SampleRecall
	}
	return out
}

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

package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	v1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// approval_policy.go —— P0-5 phase 2（D5）：审批姿态表管理面 6 RPC。
// 门禁=HasAdminAccess 平台管理员（规则即平台安全姿态——普通租户可写等于绕过
// 运行时审批闸，与 requireParamRuleAdmin 同标准）；RBAC scope=session owner
// 模型的粒度化随 M67/M78 组织域入口另行记档，本批不越界。写路径全量
// recordAudit（AdminAuditEntry action=approval_policy；IP/UA/RequestID 经
// audit_meta.go 提取）；单源校验=biz.ValidateApprovalPolicy（usecase 与仓储
// 二次防御同款）；zero-migration——session_approval_policies.mode 为 TEXT
// 无 CHECK，第四档 auto 直接可写。

// requireApprovalPolicyAdmin 守护审批策略管理端点：与 requireParamRuleAdmin
// 同标准（JWT + HasAdminAccess）。
func (s *ToolService) requireApprovalPolicyAdmin(ctx context.Context) error {
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return auth.ErrUnauthorized
	}
	if !a.HasAdminAccess() {
		return auth.ErrForbidden
	}
	return nil
}

// approvalPolicyActor 提取审计操作者标识（CreatedBy/审计双写同源）。
func approvalPolicyActor(ctx context.Context) string {
	if a, ok := auth.FromContext(ctx); ok && a != nil {
		return strconv.FormatInt(a.UserID, 10)
	}
	return "system"
}

func approvalPolicyToProto(p biztool.ApprovalPolicy) *v1.ApprovalPolicy {
	return &v1.ApprovalPolicy{
		Id:            p.ID,
		Scope:         p.Scope,
		ScopeKey:      p.ScopeKey,
		ToolPattern:   p.ToolPattern,
		CommandPrefix: p.CommandPrefix,
		ArgsPattern:   p.ArgsPattern,
		Mode:          p.Mode,
		Priority:      int32(p.Priority),
		Enabled:       p.Enabled,
		ShadowOnly:    p.ShadowOnly,
		ExpiresAt:     p.ExpiresAt,
		CreatedAt:     p.CreatedAt,
		CreatedBy:     p.CreatedBy,
	}
}

// ListApprovalPolicies 返回 (scope, scope_key) 下全部规则（管理面可见
// disabled 与已过期供清理）。Admin only。
func (s *ToolService) ListApprovalPolicies(ctx context.Context, req *v1.ListApprovalPoliciesRequest) (*v1.ListApprovalPoliciesResponse, error) {
	if err := s.requireApprovalPolicyAdmin(ctx); err != nil {
		return nil, err
	}
	rows, err := s.uc.ListApprovalPolicies(ctx, req.GetScope(), req.GetScopeKey())
	if err != nil {
		return nil, err
	}
	items := make([]*v1.ApprovalPolicy, 0, len(rows))
	for i := range rows {
		items = append(items, approvalPolicyToProto(rows[i]))
	}
	return &v1.ListApprovalPoliciesResponse{Items: items}, nil
}

// GetApprovalPolicy 单条读取（审计/详情）。Admin only。
func (s *ToolService) GetApprovalPolicy(ctx context.Context, req *v1.GetApprovalPolicyRequest) (*v1.ApprovalPolicy, error) {
	if err := s.requireApprovalPolicyAdmin(ctx); err != nil {
		return nil, err
	}
	p, err := s.uc.GetApprovalPolicyByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, apierror.NotFound("TOOL", "approval policy not found")
		}
		return nil, err
	}
	return approvalPolicyToProto(p), nil
}

// UpsertApprovalPolicy 新建/更新规则（id 键幂等）。CreatedBy/CreatedAt 为
// server 权威：新建=操作者+now（usecase 补戳）；更新保留原值。gate 逐调用读
// 规则，下个工具调用即生效。Admin only。
func (s *ToolService) UpsertApprovalPolicy(ctx context.Context, req *v1.UpsertApprovalPolicyRequest) (*v1.ApprovalPolicy, error) {
	if err := s.requireApprovalPolicyAdmin(ctx); err != nil {
		return nil, err
	}
	p := biztool.ApprovalPolicy{
		ID:            req.GetId(),
		Scope:         req.GetScope(),
		ScopeKey:      req.GetScopeKey(),
		ToolPattern:   req.GetToolPattern(),
		CommandPrefix: req.GetCommandPrefix(),
		ArgsPattern:   req.GetArgsPattern(),
		Mode:          req.GetMode(),
		Priority:      int(req.GetPriority()),
		Enabled:       req.GetEnabled(),
		ShadowOnly:    req.GetShadowOnly(),
		ExpiresAt:     req.GetExpiresAt(),
	}
	// 更新路径保留既有 CreatedBy/CreatedAt（server 权威审计元数据）。
	if existing, err := s.uc.GetApprovalPolicyByID(ctx, p.ID); err == nil {
		p.CreatedBy = existing.CreatedBy
		p.CreatedAt = existing.CreatedAt
	} else if !errors.Is(err, shared.ErrNotFound) {
		return nil, err
	} else {
		p.CreatedBy = approvalPolicyActor(ctx)
	}
	if err := s.uc.UpsertApprovalPolicy(ctx, p); err != nil {
		return nil, err
	}
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbUpdate, "approval_policy"),
		Resource:   "approval_policy",
		ResourceID: p.ID,
		Summary:    fmt.Sprintf("scope=%s:%s tool_pattern=%s mode=%s enabled=%v shadow_only=%v", p.Scope, p.ScopeKey, p.ToolPattern, p.Mode, p.Enabled, p.ShadowOnly),
	})
	stored, err := s.uc.GetApprovalPolicyByID(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return approvalPolicyToProto(stored), nil
}

// DeleteApprovalPolicy 幂等删除。Admin only。
func (s *ToolService) DeleteApprovalPolicy(ctx context.Context, req *v1.DeleteApprovalPolicyRequest) (*emptypb.Empty, error) {
	if err := s.requireApprovalPolicyAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.DeleteApprovalPolicy(ctx, req.GetId()); err != nil {
		return nil, err
	}
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbDelete, "approval_policy"),
		Resource:   "approval_policy",
		ResourceID: req.GetId(),
	})
	return &emptypb.Empty{}, nil
}

// ImportApprovalPoliciesTOML 批量导入：TOML 文档经 ValidateApprovalPolicy
// 单源校验逐行解析，合法行 id 键幂等 upsert（不中断整批），非法行逐条回报
// （"policies[i]: <reason>"，预脱敏不回显原文）。文档级语法错误整单拒收。
// CreatedBy=操作者、CreatedAt=now（导入不接受审计元数据伪造）。Admin only。
func (s *ToolService) ImportApprovalPoliciesTOML(ctx context.Context, req *v1.ImportApprovalPoliciesTOMLRequest) (*v1.ImportApprovalPoliciesTOMLResponse, error) {
	if err := s.requireApprovalPolicyAdmin(ctx); err != nil {
		return nil, err
	}
	policies, rowErrs, err := biztool.ParseApprovalPoliciesTOML([]byte(req.GetContent()))
	if err != nil {
		return nil, apierror.BadRequest("TOOL", err.Error())
	}
	actor := approvalPolicyActor(ctx)
	imported := 0
	errs := append([]string{}, rowErrs...)
	for i := range policies {
		p := policies[i]
		// 导入路径同 upsert 纪律：审计元数据 server 权威——既有行保留原
		// CreatedBy/CreatedAt，新行=操作者+now。
		if existing, gerr := s.uc.GetApprovalPolicyByID(ctx, p.ID); gerr == nil {
			p.CreatedBy = existing.CreatedBy
			p.CreatedAt = existing.CreatedAt
		} else if errors.Is(gerr, shared.ErrNotFound) {
			p.CreatedBy = actor
		} else {
			errs = append(errs, fmt.Sprintf("%s: lookup failed: %v", p.ID, gerr))
			continue
		}
		if uerr := s.uc.UpsertApprovalPolicy(ctx, p); uerr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p.ID, uerr))
			continue
		}
		imported++
	}
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbUpdate, "approval_policy"),
		Resource:   "approval_policy",
		ResourceID: "batch-import",
		Summary:    fmt.Sprintf("imported=%d errors=%d", imported, len(errs)),
	})
	return &v1.ImportApprovalPoliciesTOMLResponse{
		ImportedCount: int32(imported),
		Errors:        errs,
	}, nil
}

// ExportApprovalPoliciesTOML 导出 (scope, scope_key) 下全部规则为规范 TOML
// 文档（确定性输出，diff/归档友好；审计元数据不携带）。Admin only。
func (s *ToolService) ExportApprovalPoliciesTOML(ctx context.Context, req *v1.ExportApprovalPoliciesTOMLRequest) (*v1.ExportApprovalPoliciesTOMLResponse, error) {
	if err := s.requireApprovalPolicyAdmin(ctx); err != nil {
		return nil, err
	}
	rows, err := s.uc.ListApprovalPolicies(ctx, req.GetScope(), req.GetScopeKey())
	if err != nil {
		return nil, err
	}
	data, err := biztool.MarshalApprovalPoliciesTOML(rows)
	if err != nil {
		return nil, apierror.Internal("TOOL", "export marshal failed: "+err.Error())
	}
	return &v1.ExportApprovalPoliciesTOMLResponse{Content: string(data)}, nil
}

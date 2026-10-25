package tool

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// tool_approval_policy_uc.go —— P0-5 审批工程化批 C（2026-10-22）：ToolUsecase
// 的审批策略读口（gate）与管理面读写面。仓储装配经 WithApprovalPolicyStore
// option（tool.go），未装配时所有管理面方法报 apierror.Internal、gate 读口
// 返回 nil——零行为变更兜底，与 ListEnabledParamRulesForGate 同一先例。

// ListEnabledApprovalPolicies 实现 biztool.ApprovalPolicyReader：规范化后
// 透传仓储。nil store 返回 nil, nil（gate 不注册）。
func (u *ToolUsecase) ListEnabledApprovalPolicies(ctx context.Context, scope, scopeKey string, nowUnix int64) ([]ApprovalPolicy, error) {
	if u == nil || u.approvalPolicies == nil {
		return nil, nil
	}
	return u.approvalPolicies.ListEnabledApprovalPolicies(ctx, strings.TrimSpace(scope), strings.TrimSpace(scopeKey), nowUnix)
}

// HasApprovalPolicyStore 报告 approvalPolicies 是否已装配（装配观测位）。
// 对抗评审 #9 校正：当前无消费方（曾经的注释误称 agent 包经可选接口断言
// 消费，grep 证据为零）；保留此出口与 HasParamRuleStore 同一先例，新增
// 消费者时同 PR 更新此注释。
func (u *ToolUsecase) HasApprovalPolicyStore() bool {
	return u != nil && u.approvalPolicies != nil
}

// ListApprovalPolicies 返回 (scope, scopeKey) 下全部规则（管理面可见 disabled
// 与已过期，供清理）。
func (u *ToolUsecase) ListApprovalPolicies(ctx context.Context, scope, scopeKey string) ([]ApprovalPolicy, error) {
	if u == nil || u.approvalPolicies == nil {
		return nil, apierror.Internal("TOOL", "approval policy store unavailable")
	}
	scope = strings.TrimSpace(scope)
	scopeKey = strings.TrimSpace(scopeKey)
	if scope == "" || scopeKey == "" {
		return nil, apierror.BadRequest("TOOL", "scope and scope_key are required")
	}
	return u.approvalPolicies.ListApprovalPolicies(ctx, scope, scopeKey)
}

// GetApprovalPolicyByID 管理面单条读取（审计/详情）。
func (u *ToolUsecase) GetApprovalPolicyByID(ctx context.Context, id string) (ApprovalPolicy, error) {
	if u == nil || u.approvalPolicies == nil {
		return ApprovalPolicy{}, apierror.Internal("TOOL", "approval policy store unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ApprovalPolicy{}, apierror.BadRequest("TOOL", "id is required")
	}
	return u.approvalPolicies.GetApprovalPolicyByID(ctx, id)
}

// UpsertApprovalPolicy 新建/更新规则。ValidateApprovalPolicy 单一校验函数
// （与仓储同函数二次防御——写路径不允许绕过 usecase 校验直插）。
// CreatedAt 缺省置当前时间；Enabled 语义由调用方显式给出（默认零值 false=
// 显式先 shadow/disabled 再启用的纪律，防误开 never）。
func (u *ToolUsecase) UpsertApprovalPolicy(ctx context.Context, p ApprovalPolicy) error {
	if u == nil || u.approvalPolicies == nil {
		return apierror.Internal("TOOL", "approval policy store unavailable")
	}
	p.Scope = strings.TrimSpace(p.Scope)
	p.ScopeKey = strings.TrimSpace(p.ScopeKey)
	p.ID = strings.TrimSpace(p.ID)
	p.ToolPattern = strings.TrimSpace(p.ToolPattern)
	if err := ValidateApprovalPolicy(p); err != nil {
		return err
	}
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().Unix()
	}
	return u.approvalPolicies.UpsertApprovalPolicy(ctx, p)
}

// DeleteApprovalPolicy 幂等删除。
func (u *ToolUsecase) DeleteApprovalPolicy(ctx context.Context, id string) error {
	if u == nil || u.approvalPolicies == nil {
		return apierror.Internal("TOOL", "approval policy store unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return apierror.BadRequest("TOOL", "id is required")
	}
	return u.approvalPolicies.DeleteApprovalPolicy(ctx, id)
}

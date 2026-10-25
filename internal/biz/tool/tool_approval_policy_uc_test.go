package tool

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
)

// tool_approval_policy_uc_test.go —— 对抗评审 #9：P0-5 批 C 挂出的
// ToolUsecase 审批策略读口 + 管理面五方法零覆盖缺口回补。钉死 nil store
// 兜底矩阵、bad-request 前置、Validate 二次防御、CreatedAt 缺省回填。

func TestApprovalPolicyUC_NilStoreMatrix(t *testing.T) {
	t.Parallel()
	u := &ToolUsecase{} // 未装配 approvalPolicies
	ctx := context.Background()

	if u.HasApprovalPolicyStore() {
		t.Fatal("unwired usecase must report no store")
	}
	// gate 读口：nil store → (nil, nil)（gate 不注册，OFF 兜底钉死）。
	if rows, err := u.ListEnabledApprovalPolicies(ctx, "agent", "a1", 0); err != nil || rows != nil {
		t.Fatalf("ListEnabled nil store = (%v,%v), want (nil,nil)", rows, err)
	}
	// 管理面五方法：nil store → Internal（不静默吞、不倒卖 NotFound）。
	for name, call := range map[string]func() error{
		"List":   func() error { _, err := u.ListApprovalPolicies(ctx, "agent", "a1"); return err },
		"Get":    func() error { _, err := u.GetApprovalPolicyByID(ctx, "p1"); return err },
		"Upsert": func() error { return u.UpsertApprovalPolicy(ctx, ApprovalPolicy{ID: "p1", Scope: "agent", ScopeKey: "a1", Mode: "never", ToolPattern: "*"}) },
		"Delete": func() error { return u.DeleteApprovalPolicy(ctx, "p1") },
	} {
		err := call()
		var apiErr *apierror.Error
		if !errors.As(err, &apiErr) || apiErr.Code != apierror.CodeInternal {
			t.Fatalf("%s nil store err = %v, want apierror Internal", name, err)
		}
	}
}

func TestApprovalPolicyUC_BadRequestMatrix(t *testing.T) {
	t.Parallel()
	u := &ToolUsecase{approvalPolicies: NewInMemApprovalPolicyStore()}
	ctx := context.Background()

	badReq := func(name string, err error) {
		t.Helper()
		var apiErr *apierror.Error
		if !errors.As(err, &apiErr) || apiErr.Code != apierror.CodeBadRequest {
			t.Fatalf("%s err = %v, want BadRequest", name, err)
		}
	}
	_, err := u.ListApprovalPolicies(ctx, "", "a1")
	badReq("empty scope", err)
	_, err = u.ListApprovalPolicies(ctx, "agent", "")
	badReq("empty scope_key", err)
	_, err = u.GetApprovalPolicyByID(ctx, "")
	badReq("empty id", err)
	badReq("empty id delete", u.DeleteApprovalPolicy(ctx, ""))
	// Validate 二次防御：即便仓储宽松注入，usecase 必须先拒
	// （写路径不允许绕过 usecase 校验直插）。
	badReq("bogus scope", u.UpsertApprovalPolicy(ctx, ApprovalPolicy{ID: "p1", Scope: "bogus", ScopeKey: "a1", Mode: "never", ToolPattern: "*"}))
	badReq("bogus mode", u.UpsertApprovalPolicy(ctx, ApprovalPolicy{ID: "p1", Scope: "agent", ScopeKey: "a1", Mode: "bogus", ToolPattern: "*"}))
	badReq("empty tool_pattern", u.UpsertApprovalPolicy(ctx, ApprovalPolicy{ID: "p1", Scope: "agent", ScopeKey: "a1", Mode: "never", ToolPattern: ""}))
}

func TestApprovalPolicyUC_CreatedAtDefaultingAndDelete(t *testing.T) {
	t.Parallel()
	store := NewInMemApprovalPolicyStore()
	u := &ToolUsecase{approvalPolicies: store}
	ctx := context.Background()

	p := ApprovalPolicy{ID: "p-ct", Scope: "agent", ScopeKey: "a1", Mode: "never", ToolPattern: "*", Enabled: true}
	if err := u.UpsertApprovalPolicy(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := u.GetApprovalPolicyByID(ctx, "p-ct")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CreatedAt <= 0 {
		t.Fatalf("CreatedAt must default to now, got %d", got.CreatedAt)
	}
	// 显式 CreatedAt 不被覆盖。
	p2 := ApprovalPolicy{ID: "p-ct2", Scope: "agent", ScopeKey: "a1", Mode: "ask", ToolPattern: "*", Enabled: true, CreatedAt: 42}
	if err := u.UpsertApprovalPolicy(ctx, p2); err != nil {
		t.Fatalf("upsert explicit: %v", err)
	}
	got2, err := u.GetApprovalPolicyByID(ctx, "p-ct2")
	if err != nil || got2.CreatedAt != 42 {
		t.Fatalf("explicit CreatedAt = (%d,%v), want (42,nil)", got2.CreatedAt, err)
	}
	// 管理面 List 透传全量（含 disabled）；Delete 幂等生效。
	if rows, err := u.ListApprovalPolicies(ctx, "agent", "a1"); err != nil || len(rows) != 2 {
		t.Fatalf("admin list = (%d,%v), want (2,nil)", len(rows), err)
	}
	if err := u.DeleteApprovalPolicy(ctx, "p-ct"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := u.DeleteApprovalPolicy(ctx, "p-ct"); err != nil {
		t.Fatalf("delete must be idempotent: %v", err)
	}
	if _, err := u.GetApprovalPolicyByID(ctx, "p-ct"); err == nil {
		t.Fatal("deleted policy must not be readable")
	}
}

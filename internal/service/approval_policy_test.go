package service

import (
	"context"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// D5（P0-5 phase 2）：审批姿态表管理面 6 RPC 钉板。仓=biztool 内存实现
// （管理面校验经 ValidateApprovalPolicy 单源——与 PG 仓同款二次防御）。

func newApprovalPolicyTestService(store biztool.ApprovalPolicyStore) *ToolService {
	uc := biztool.NewToolUsecase(nil, nil, loggateway.NewNoop(), biztool.WithApprovalPolicyStore(store))
	agents := biz.NewAgentUsecase(biz.AgentUsecaseDeps{
		Reader:   &batchAgentReader{agents: map[string]biz.Agent{"agent-1": {ID: "agent-1"}}},
		Settings: batchSettingsRepo{},
		Files:    batchFilesRepo{},
		Lg:       loggateway.NewNoop(),
	})
	return NewToolService(uc, agents, nil)
}

func approvalPolicyAdminCtx() context.Context {
	return auth.NewContext(toolGrantCtx(), &auth.Auth{UserID: 7, Access: "admin"})
}

func seedApprovalPolicy(t *testing.T, store biztool.ApprovalPolicyStore, p biztool.ApprovalPolicy) {
	t.Helper()
	if p.ID == "" {
		p.ID = "p-1"
	}
	if p.Scope == "" {
		p.Scope = "session"
	}
	if p.ScopeKey == "" {
		p.ScopeKey = "sess-1"
	}
	if p.ToolPattern == "" {
		p.ToolPattern = "*"
	}
	if p.Mode == "" {
		p.Mode = "ask"
	}
	p.CreatedAt = 1700000000
	p.CreatedBy = "seed"
	if err := store.UpsertApprovalPolicy(context.Background(), p); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// TestToolService_ApprovalPolicyRPCs_RequireAdmin 钉死门禁：无 claims →
// Unauthorized、非 admin → Forbidden；六个端点同标准（HasAdminAccess）。
func TestToolService_ApprovalPolicyRPCs_RequireAdmin(t *testing.T) {
	svc := newApprovalPolicyTestService(biztool.NewInMemApprovalPolicyStore())
	noAuth := toolGrantCtx()
	userAuth := auth.NewContext(toolGrantCtx(), &auth.Auth{UserID: 2, Access: "user"})

	list := &v1.ListApprovalPoliciesRequest{Scope: "session", ScopeKey: "sess-1"}
	if _, err := svc.ListApprovalPolicies(noAuth, list); err != auth.ErrUnauthorized {
		t.Fatalf("list no-auth err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.ListApprovalPolicies(userAuth, list); err != auth.ErrForbidden {
		t.Fatalf("list user err = %v, want ErrForbidden", err)
	}
	get := &v1.GetApprovalPolicyRequest{Id: "p-1"}
	if _, err := svc.GetApprovalPolicy(noAuth, get); err != auth.ErrUnauthorized {
		t.Fatalf("get no-auth err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.GetApprovalPolicy(userAuth, get); err != auth.ErrForbidden {
		t.Fatalf("get user err = %v, want ErrForbidden", err)
	}
	up := &v1.UpsertApprovalPolicyRequest{Id: "p-1", Scope: "session", ScopeKey: "sess-1", ToolPattern: "*", Mode: "ask"}
	if _, err := svc.UpsertApprovalPolicy(noAuth, up); err != auth.ErrUnauthorized {
		t.Fatalf("upsert no-auth err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.UpsertApprovalPolicy(userAuth, up); err != auth.ErrForbidden {
		t.Fatalf("upsert user err = %v, want ErrForbidden", err)
	}
	del := &v1.DeleteApprovalPolicyRequest{Id: "p-1"}
	if _, err := svc.DeleteApprovalPolicy(noAuth, del); err != auth.ErrUnauthorized {
		t.Fatalf("delete no-auth err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.DeleteApprovalPolicy(userAuth, del); err != auth.ErrForbidden {
		t.Fatalf("delete user err = %v, want ErrForbidden", err)
	}
	imp := &v1.ImportApprovalPoliciesTOMLRequest{Content: ""}
	if _, err := svc.ImportApprovalPoliciesTOML(noAuth, imp); err != auth.ErrUnauthorized {
		t.Fatalf("import no-auth err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.ImportApprovalPoliciesTOML(userAuth, imp); err != auth.ErrForbidden {
		t.Fatalf("import user err = %v, want ErrForbidden", err)
	}
	exp := &v1.ExportApprovalPoliciesTOMLRequest{Scope: "session", ScopeKey: "sess-1"}
	if _, err := svc.ExportApprovalPoliciesTOML(noAuth, exp); err != auth.ErrUnauthorized {
		t.Fatalf("export no-auth err = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.ExportApprovalPoliciesTOML(userAuth, exp); err != auth.ErrForbidden {
		t.Fatalf("export user err = %v, want ErrForbidden", err)
	}
}

// TestToolService_ListApprovalPolicies_Scoped 钉死作用域过滤与全量可见
// （管理面可见 disabled/已过期行供清理——与 ListEnabled 读口区分）。
func TestToolService_ListApprovalPolicies_Scoped(t *testing.T) {
	store := biztool.NewInMemApprovalPolicyStore()
	seedApprovalPolicy(t, store, biztool.ApprovalPolicy{ID: "p-a", Priority: 50})
	b := biztool.ApprovalPolicy{ID: "p-b", Priority: 10, Enabled: false}
	seedApprovalPolicy(t, store, b)
	c := biztool.ApprovalPolicy{ID: "p-c", Scope: "agent", ScopeKey: "agent-1"}
	seedApprovalPolicy(t, store, c)
	svc := newApprovalPolicyTestService(store)

	resp, err := svc.ListApprovalPolicies(approvalPolicyAdminCtx(), &v1.ListApprovalPoliciesRequest{Scope: "session", ScopeKey: "sess-1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.GetItems()) != 2 || resp.GetItems()[0].GetId() != "p-b" || resp.GetItems()[1].GetId() != "p-a" {
		t.Fatalf("scoped list wrong (priority asc): %+v", resp.GetItems())
	}
	// scope/scope_key 必填（管理面读口径与 usecase 同闸）。
	if _, err := svc.ListApprovalPolicies(approvalPolicyAdminCtx(), &v1.ListApprovalPoliciesRequest{}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("empty scope err = %v, want BadRequest", err)
	}
}

func TestToolService_GetApprovalPolicy(t *testing.T) {
	store := biztool.NewInMemApprovalPolicyStore()
	seedApprovalPolicy(t, store, biztool.ApprovalPolicy{ID: "p-a", Mode: "trusted", ExpiresAt: 1893456000})
	svc := newApprovalPolicyTestService(store)

	got, err := svc.GetApprovalPolicy(approvalPolicyAdminCtx(), &v1.GetApprovalPolicyRequest{Id: "p-a"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetMode() != "trusted" || got.GetExpiresAt() != 1893456000 || got.GetCreatedBy() != "seed" {
		t.Fatalf("got = %+v", got)
	}
	if _, err := svc.GetApprovalPolicy(approvalPolicyAdminCtx(), &v1.GetApprovalPolicyRequest{Id: "missing"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("missing err = %v, want NotFound", err)
	}
}

// TestToolService_UpsertApprovalPolicy_ServerAuthoritativeAudit 钉死审计元数据
// server 权威：新建 CreatedBy=操作者（actor）+CreatedAt 补戳；更新保留原值；
// auto 第四档通行；坏 mode/pattern BadRequest（单源校验）。
func TestToolService_UpsertApprovalPolicy_ServerAuthoritativeAudit(t *testing.T) {
	store := biztool.NewInMemApprovalPolicyStore()
	seedApprovalPolicy(t, store, biztool.ApprovalPolicy{ID: "p-old"})
	svc := newApprovalPolicyTestService(store)

	// 新建：actor 落 CreatedBy，CreatedAt 补戳。
	got, err := svc.UpsertApprovalPolicy(approvalPolicyAdminCtx(), &v1.UpsertApprovalPolicyRequest{
		Id: "p-new", Scope: "session", ScopeKey: "sess-1", ToolPattern: "shell*", Mode: "auto", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.GetMode() != "auto" || got.GetCreatedBy() != "7" || got.GetCreatedAt() == 0 {
		t.Fatalf("create stamps wrong: %+v", got)
	}
	// 更新既有行：审计元数据不被覆写。
	got, err = svc.UpsertApprovalPolicy(approvalPolicyAdminCtx(), &v1.UpsertApprovalPolicyRequest{
		Id: "p-old", Scope: "session", ScopeKey: "sess-1", ToolPattern: "*", Mode: "never", Priority: 5, Enabled: true,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.GetCreatedBy() != "seed" || got.GetCreatedAt() != 1700000000 {
		t.Fatalf("update must preserve CreatedBy/CreatedAt: %+v", got)
	}
	if got.GetMode() != "never" || got.GetPriority() != 5 {
		t.Fatalf("update payload wrong: %+v", got)
	}
	// 坏 mode / 空通配 BadRequest（usecase 经 ValidateApprovalPolicy）。
	if _, err := svc.UpsertApprovalPolicy(approvalPolicyAdminCtx(), &v1.UpsertApprovalPolicyRequest{
		Id: "p-bad", Scope: "session", ScopeKey: "sess-1", ToolPattern: "*", Mode: "assisted",
	}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("bad mode err = %v, want BadRequest", err)
	}
	if _, err := svc.UpsertApprovalPolicy(approvalPolicyAdminCtx(), &v1.UpsertApprovalPolicyRequest{
		Id: "p-bad", Scope: "session", ScopeKey: "sess-1", ToolPattern: "re:[", Mode: "ask",
	}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("bad pattern err = %v, want BadRequest", err)
	}
}

// TestToolService_DeleteApprovalPolicy_Idempotent 钉死删除幂等。
func TestToolService_DeleteApprovalPolicy_Idempotent(t *testing.T) {
	store := biztool.NewInMemApprovalPolicyStore()
	seedApprovalPolicy(t, store, biztool.ApprovalPolicy{ID: "p-a"})
	svc := newApprovalPolicyTestService(store)
	for i := 0; i < 2; i++ {
		if _, err := svc.DeleteApprovalPolicy(approvalPolicyAdminCtx(), &v1.DeleteApprovalPolicyRequest{Id: "p-a"}); err != nil {
			t.Fatalf("delete #%d: %v", i, err)
		}
	}
	resp, err := svc.ListApprovalPolicies(approvalPolicyAdminCtx(), &v1.ListApprovalPoliciesRequest{Scope: "session", ScopeKey: "sess-1"})
	if err != nil || len(resp.GetItems()) != 0 {
		t.Fatalf("after delete: %v %+v", err, resp)
	}
}

// TestToolService_ImportApprovalPoliciesTOML 钉死 TOML 批量导入：合法行幂等
// upsert（既有行保留 CreatedBy/CreatedAt、新行=操作者+戳）、非法行逐条回报
// 不断批、文档级语法错误整单 BadRequest。
func TestToolService_ImportApprovalPoliciesTOML(t *testing.T) {
	store := biztool.NewInMemApprovalPolicyStore()
	seedApprovalPolicy(t, store, biztool.ApprovalPolicy{ID: "p-keep"})
	svc := newApprovalPolicyTestService(store)

	doc := `
[[policies]]
id = "p-keep"
scope = "session"
scope_key = "sess-1"
tool_pattern = "*"
mode = "never"
priority = 1

[[policies]]
id = "p-fresh"
scope = "agent"
scope_key = "agent-1"
tool_pattern = "shell*"
mode = "auto"
enabled = true

[[policies]]
id = "p-bad"
scope = "session"
scope_key = "sess-1"
tool_pattern = "*"
mode = "assisted"
`
	resp, err := svc.ImportApprovalPoliciesTOML(approvalPolicyAdminCtx(), &v1.ImportApprovalPoliciesTOMLRequest{Content: doc})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if resp.GetImportedCount() != 2 {
		t.Fatalf("imported = %d, want 2 (errors=%v)", resp.GetImportedCount(), resp.GetErrors())
	}
	if len(resp.GetErrors()) != 1 || !strings.Contains(resp.GetErrors()[0], "mode") {
		t.Fatalf("errors = %v, want 1 row error naming mode", resp.GetErrors())
	}
	// p-keep 更新后保留原审计元数据。
	keep, err := svc.GetApprovalPolicy(approvalPolicyAdminCtx(), &v1.GetApprovalPolicyRequest{Id: "p-keep"})
	if err != nil {
		t.Fatalf("get p-keep: %v", err)
	}
	if keep.GetMode() != "never" || keep.GetPriority() != 1 {
		t.Fatalf("p-keep not updated: %+v", keep)
	}
	if keep.GetCreatedBy() != "seed" || keep.GetCreatedAt() != 1700000000 {
		t.Fatalf("import must preserve audit metadata: %+v", keep)
	}
	// p-fresh 新行=操作者+戳。
	fresh, err := svc.GetApprovalPolicy(approvalPolicyAdminCtx(), &v1.GetApprovalPolicyRequest{Id: "p-fresh"})
	if err != nil {
		t.Fatalf("get p-fresh: %v", err)
	}
	if fresh.GetCreatedBy() != "7" || fresh.GetCreatedAt() == 0 || !fresh.GetEnabled() {
		t.Fatalf("p-fresh stamps wrong: %+v", fresh)
	}
	// 文档级语法错误整单拒收。
	if _, err := svc.ImportApprovalPoliciesTOML(approvalPolicyAdminCtx(), &v1.ImportApprovalPoliciesTOMLRequest{Content: "[[policies]\nbroken"}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("syntax error err = %v, want BadRequest", err)
	}
}

// TestToolService_ExportApprovalPoliciesTOML_Roundtrip 钉死导出形状：确定性
// TOML 文档、作用域过滤、不经审计元数据；导出→导入回环幂等。
func TestToolService_ExportApprovalPoliciesTOML_Roundtrip(t *testing.T) {
	store := biztool.NewInMemApprovalPolicyStore()
	seedApprovalPolicy(t, store, biztool.ApprovalPolicy{ID: "p-a", Mode: "ask", Priority: 10})
	b := biztool.ApprovalPolicy{ID: "p-b", Mode: "auto", Enabled: true, ShadowOnly: true, ExpiresAt: 1893456000}
	seedApprovalPolicy(t, store, b)
	c := biztool.ApprovalPolicy{ID: "p-c", Scope: "agent", ScopeKey: "agent-1", Mode: "trusted"}
	seedApprovalPolicy(t, store, c)
	svc := newApprovalPolicyTestService(store)

	resp, err := svc.ExportApprovalPoliciesTOML(approvalPolicyAdminCtx(), &v1.ExportApprovalPoliciesTOMLRequest{Scope: "session", ScopeKey: "sess-1"})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	content := resp.GetContent()
	if !strings.Contains(content, `id = 'p-a'`) || !strings.Contains(content, `id = 'p-b'`) || strings.Contains(content, `id = 'p-c'`) {
		t.Fatalf("export scope filter wrong:\n%s", content)
	}
	if strings.Contains(content, "created_") {
		t.Fatalf("export must not carry audit metadata:\n%s", content)
	}
	// 回环：清空 scope 后导入导出物，行级零错。
	fresh := biztool.NewInMemApprovalPolicyStore()
	svc2 := newApprovalPolicyTestService(fresh)
	imp, err := svc2.ImportApprovalPoliciesTOML(approvalPolicyAdminCtx(), &v1.ImportApprovalPoliciesTOMLRequest{Content: content})
	if err != nil || imp.GetImportedCount() != 2 || len(imp.GetErrors()) != 0 {
		t.Fatalf("roundtrip import: %v %+v", err, imp)
	}
	got, err := svc2.GetApprovalPolicy(approvalPolicyAdminCtx(), &v1.GetApprovalPolicyRequest{Id: "p-b"})
	if err != nil {
		t.Fatalf("roundtrip get: %v", err)
	}
	if got.GetMode() != "auto" || !got.GetShadowOnly() || got.GetExpiresAt() != 1893456000 {
		t.Fatalf("roundtrip p-b wrong: %+v", got)
	}
}

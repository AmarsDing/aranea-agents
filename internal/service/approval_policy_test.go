package service

import (
	"context"
	"fmt"
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

// ─── D5-f（P0-5 phase 2 评审修复批）钉板 ──────────────────────────────────

// approvalPolicyAuditMockRepo 捕获 admin audit 写入（F18）：嵌入
// runnerCompletionMockRepo 继承全部接口，仅 InsertAuditLog 落桶。
type approvalPolicyAuditMockRepo struct {
	runnerCompletionMockRepo
	audits []biz.AuditLog
}

func (r *approvalPolicyAuditMockRepo) InsertAuditLog(_ context.Context, a biz.AuditLog) error {
	r.audits = append(r.audits, a)
	return nil
}

func newApprovalPolicyAuditTestService(store biztool.ApprovalPolicyStore, monRepo *approvalPolicyAuditMockRepo) *ToolService {
	uc := biztool.NewToolUsecase(nil, nil, loggateway.NewNoop(), biztool.WithApprovalPolicyStore(store))
	agents := biz.NewAgentUsecase(biz.AgentUsecaseDeps{
		Reader:   &batchAgentReader{agents: map[string]biz.Agent{"agent-1": {ID: "agent-1"}}},
		Settings: batchSettingsRepo{},
		Files:    batchFilesRepo{},
		Lg:       loggateway.NewNoop(),
	})
	return NewToolService(uc, agents, biz.NewMonitorUsecase(monRepo, monRepo, monRepo, monRepo, monRepo, nil))
}

// TestToolService_ApprovalPolicy_AuditEntries 钉死 D5-f F7/F18：写路径审计
// 条目 Action/ResourceID 齐备，且 Summary 信息论裁剪——运行时用于拒收
// never 的规则刻意只暴露规则 ID，审计 Summary 若携带 tool_pattern/scope_key，
// 经无本表 admin 门禁的 GET /v1/monitor/audit 即可侧信道拼出完整规则面。
// 对齐 param_rule 先例（Summary 只带规则键与姿态值）。
func TestToolService_ApprovalPolicy_AuditEntries(t *testing.T) {
	store := biztool.NewInMemApprovalPolicyStore()
	repo := &approvalPolicyAuditMockRepo{}
	svc := newApprovalPolicyAuditTestService(store, repo)

	if _, err := svc.UpsertApprovalPolicy(approvalPolicyAdminCtx(), &v1.UpsertApprovalPolicyRequest{
		Id: "p-audit", Scope: "session", ScopeKey: "s3cr3t-scope-xyz", ToolPattern: "internal_tool_*",
		Mode: "auto", Enabled: true, ShadowOnly: true,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(repo.audits) != 1 {
		t.Fatalf("upsert audit rows = %d, want 1", len(repo.audits))
	}
	up := repo.audits[0]
	if up.Action != "update.approval_policy" || up.Resource != "approval_policy" || up.ResourceID != "p-audit" {
		t.Fatalf("upsert audit wrong: %+v", up)
	}
	if !strings.Contains(up.Detail, "mode=auto") || !strings.Contains(up.Detail, "id=p-audit") {
		t.Fatalf("upsert audit summary must carry stance fields, got %q", up.Detail)
	}
	if strings.Contains(up.Detail, "s3cr3t-scope-xyz") || strings.Contains(up.Detail, "internal_tool_*") {
		t.Fatalf("F7: audit summary must not leak scope_key/tool_pattern (信息论保护), got %q", up.Detail)
	}
	if up.Actor != "7" {
		t.Fatalf("audit actor = %q, want 7 (ctx auth 主体)", up.Actor)
	}
	if _, err := svc.DeleteApprovalPolicy(approvalPolicyAdminCtx(), &v1.DeleteApprovalPolicyRequest{Id: "p-audit"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(repo.audits) != 2 {
		t.Fatalf("delete audit rows = %d, want 2", len(repo.audits))
	}
	del := repo.audits[1]
	if del.Action != "delete.approval_policy" || del.ResourceID != "p-audit" {
		t.Fatalf("delete audit wrong: %+v", del)
	}
	doc := `
[[policies]]
id = "p-imp"
scope = "session"
scope_key = "imp-secret-scope"
tool_pattern = "imp_secret_*"
mode = "ask"
`
	if _, err := svc.ImportApprovalPoliciesTOML(approvalPolicyAdminCtx(), &v1.ImportApprovalPoliciesTOMLRequest{Content: doc}); err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(repo.audits) != 3 {
		t.Fatalf("import audit rows = %d, want 3", len(repo.audits))
	}
	imp := repo.audits[2]
	if imp.Action != "update.approval_policy" || imp.ResourceID != "batch-import" {
		t.Fatalf("import audit wrong: %+v", imp)
	}
	if !strings.Contains(imp.Detail, "imported=1") {
		t.Fatalf("import audit summary must carry counts, got %q", imp.Detail)
	}
	if strings.Contains(imp.Detail, "imp-secret-scope") || strings.Contains(imp.Detail, "imp_secret_*") {
		t.Fatalf("import audit summary must not leak scope_key/tool_pattern, got %q", imp.Detail)
	}
}

// TestToolService_ImportApprovalPoliciesTOML_DuplicateID D5-f F4：同文档同
// id 重复行=逐条计入错误平面不静默吞（后写覆盖同键即审计噪声+语义歧义）；
// 首行照常导入。
func TestToolService_ImportApprovalPoliciesTOML_DuplicateID(t *testing.T) {
	store := biztool.NewInMemApprovalPolicyStore()
	svc := newApprovalPolicyTestService(store)
	doc := `
[[policies]]
id = "p-dup"
scope = "session"
scope_key = "sess-1"
tool_pattern = "*"
mode = "ask"

[[policies]]
id = "p-dup"
scope = "session"
scope_key = "sess-1"
tool_pattern = "*"
mode = "never"
`
	resp, err := svc.ImportApprovalPoliciesTOML(approvalPolicyAdminCtx(), &v1.ImportApprovalPoliciesTOMLRequest{Content: doc})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if resp.GetImportedCount() != 1 {
		t.Fatalf("imported = %d, want 1 (首位行)", resp.GetImportedCount())
	}
	if len(resp.GetErrors()) != 1 || !strings.Contains(resp.GetErrors()[0], "duplicate id in document") {
		t.Fatalf("dup id must be reported, got %v", resp.GetErrors())
	}
}

// failingApprovalPolicyStore 在 Upsert 上例出差（F8 钉板用）：错误面统一
// 预脱敏——底层错误可能拼入 DSN/内部细节，不复邮寄给导入调用方。
type failingApprovalPolicyStore struct {
	biztool.ApprovalPolicyStore
	err error
}

func (s *failingApprovalPolicyStore) UpsertApprovalPolicy(context.Context, biztool.ApprovalPolicy) error {
	return s.err
}

// TestToolService_ImportApprovalPoliciesTOML_StoreErrorRedacted 钉死 D5-f
// F8：导入仓储错误经 preview.RedactAndTruncate 预脱敏（≤200 字节）后才进
// response errors——直接透传会把 DSN/堆栈片段复邮给管理面调用方。
func TestToolService_ImportApprovalPoliciesTOML_StoreErrorRedacted(t *testing.T) {
	inner := biztool.NewInMemApprovalPolicyStore()
	store := &failingApprovalPolicyStore{
		ApprovalPolicyStore: inner,
		err:                 fmt.Errorf("pq: connection to postgres://admin:secret-password@10.0.0.9:5432/aranea failed: %s", strings.Repeat("x", 1000)),
	}
	svc := newApprovalPolicyTestService(store)
	doc := `
[[policies]]
id = "p-fail"
scope = "session"
scope_key = "sess-1"
tool_pattern = "*"
mode = "ask"
`
	resp, err := svc.ImportApprovalPoliciesTOML(approvalPolicyAdminCtx(), &v1.ImportApprovalPoliciesTOMLRequest{Content: doc})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if resp.GetImportedCount() != 0 {
		t.Fatalf("imported = %d, want 0", resp.GetImportedCount())
	}
	if len(resp.GetErrors()) != 1 {
		t.Fatalf("errors = %v, want 1 store error", resp.GetErrors())
	}
	entry := resp.GetErrors()[0]
	if !strings.HasPrefix(entry, "p-fail: ") {
		t.Fatalf("error entry must name row id, got %q", entry)
	}
	if len(entry) > len("p-fail: ")+200 {
		t.Fatalf("store error must be truncated to 200, got %d bytes", len(entry))
	}
	// D5-f3（自复审 wf_d5613cd2-059 收口）：脱敏钉必须断言 secret 本体
	// 缺席——纯截断实现（前 200 字节完整携带 DSN 口令）在仅钉前缀+长度
	// 时假绿。dsnPasswordRE 吞掉 scheme://user:password@ 整段，故口令与
	// 用户名双双不得出现，脱敏占位符必须在场。
	if strings.Contains(entry, "secret-password") {
		t.Fatalf("store error must redact the DSN password, got %q", entry)
	}
	if strings.Contains(entry, "postgres://admin") {
		t.Fatalf("store error must redact the DSN userinfo, got %q", entry)
	}
	if !strings.Contains(entry, "[secret redacted]") {
		t.Fatalf("store error must carry the redaction marker, got %q", entry)
	}
}

// TestToolService_ExportApprovalPoliciesTOML_EmptyRoundtrip D5-f F17：空
// 集合导出→导入回环=0 行零错误（交换格式的空档不得触发语法错误/幻影行）。
func TestToolService_ExportApprovalPoliciesTOML_EmptyRoundtrip(t *testing.T) {
	svc := newApprovalPolicyTestService(biztool.NewInMemApprovalPolicyStore())
	exp, err := svc.ExportApprovalPoliciesTOML(approvalPolicyAdminCtx(), &v1.ExportApprovalPoliciesTOMLRequest{Scope: "session", ScopeKey: "empty-scope"})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	fresh := newApprovalPolicyTestService(biztool.NewInMemApprovalPolicyStore())
	imp, err := fresh.ImportApprovalPoliciesTOML(approvalPolicyAdminCtx(), &v1.ImportApprovalPoliciesTOMLRequest{Content: exp.GetContent()})
	if err != nil {
		t.Fatalf("empty roundtrip import must not error: %v", err)
	}
	if imp.GetImportedCount() != 0 || len(imp.GetErrors()) != 0 {
		t.Fatalf("empty roundtrip: imported=%d errors=%v, want 0/0", imp.GetImportedCount(), imp.GetErrors())
	}
}

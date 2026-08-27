package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/decision/v1"
	"aranea-agents/internal/biz/decision"
	bizsession "aranea-agents/internal/biz/session"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

// fakeDecisionQueryRepo 是 service 层的 QueryRepo 测试替身。
type fakeDecisionQueryRepo struct {
	items      []decision.Record
	total      int64
	upstream   []decision.Record
	downstream []decision.Record
	planner    *decision.Record
	lastFilter decision.ListFilter
}

func (f *fakeDecisionQueryRepo) ListRecords(_ context.Context, filter decision.ListFilter) ([]decision.Record, int64, error) {
	f.lastFilter = filter
	return f.items, f.total, nil
}

func (f *fakeDecisionQueryRepo) GetByKey(_ context.Context, key string) (*decision.Record, error) {
	for i := range f.items {
		if f.items[i].DecisionKey == key {
			return &f.items[i], nil
		}
	}
	return nil, nil
}

func (f *fakeDecisionQueryRepo) ListUpstream(context.Context, int64, int) ([]decision.Record, error) {
	return f.upstream, nil
}

func (f *fakeDecisionQueryRepo) ListDownstream(context.Context, int64, int) ([]decision.Record, error) {
	return f.downstream, nil
}

func (f *fakeDecisionQueryRepo) FindVirtualParentPlanner(context.Context, decision.SourceRef, string, int64) (*decision.Record, error) {
	return f.planner, nil
}

// SessionGateStats 实现 SessionGateStatsRepo 窄接口（钉死 handler 聚合映射）。
func (f *fakeDecisionQueryRepo) SessionGateStats(_ context.Context, sessionID string) (decision.RunGateStats, error) {
	if sessionID == "sess-a" {
		return decision.RunGateStats{LoopGuardBlocks: 3, ParamRuleDenies: 1, BudgetTripped: true}, nil
	}
	return decision.RunGateStats{}, nil
}

// fakeSessionWorkspaceReader 是 SessionWorkspaceReader 的测试替身（T5 四轮
// 审查 IDOR 校验）：按 id 返回预置 session，未预置返回 NotFound 语义错误。
type fakeSessionWorkspaceReader struct {
	byID map[string]bizsession.Session
}

func (f *fakeSessionWorkspaceReader) Get(_ context.Context, id string) (bizsession.Session, error) {
	if s, ok := f.byID[id]; ok {
		return s, nil
	}
	return bizsession.Session{}, apierror.NotFound("SESSION", "session not found")
}

func newDecisionTestService(items []decision.Record, total int64) *DecisionRecordService {
	return NewDecisionRecordService(decision.NewQueryUsecase(&fakeDecisionQueryRepo{items: items, total: total}), nil, nil)
}

// TestDecisionRecordService_Get_NotFound：未命中 decision_key 返回 NotFound
// 而非空消息（前端据此区分「不存在」与「空记录」）。
func TestDecisionRecordService_Get_NotFound(t *testing.T) {
	svc := newDecisionTestService(nil, 0)
	_, err := svc.GetDecisionRecord(context.Background(), &v1.GetDecisionRecordRequest{DecisionKey: "dk-x"})
	if err == nil {
		t.Fatal("expected NotFound error for unknown decision_key")
	}
}

// TestDecisionRecordService_RoundTrip pins Record → proto 映射全字段
// （confidence/parent 指针、entities ListValue、source_ref/metadata Struct、
// RFC3339 时间戳）。
func TestDecisionRecordService_RoundTrip(t *testing.T) {
	conf := 0.75
	parent := int64(41)
	repo := &fakeDecisionQueryRepo{
		items: []decision.Record{{
			ID: 42, DecisionKey: "dk-1", Category: decision.CategorySystemGuard,
			Scenario: "run 累计 input token 超预算", Reasoning: "run 累计 input 超 150 万",
			Outcome: "tripped", Confidence: &conf,
			ActorType: decision.ActorSystem, ActorKey: "system:token_budget",
			ParentDecisionID: &parent,
			RelatedEntities:  []decision.EntityRef{{Type: "team", Key: "team-9"}},
			SourceRef:        decision.SourceRef{RunID: "run-1"},
			Metadata:         map[string]any{"trigger_rule": "token_budget_tripped", "threshold": 1500000},
			CreatedAt:        "2026-08-26T03:00:00Z", UpdatedAt: "2026-08-26T03:00:01Z",
		}},
		total: 1,
	}
	svc := NewDecisionRecordService(decision.NewQueryUsecase(repo), nil, nil)

	getResp, err := svc.GetDecisionRecord(context.Background(), &v1.GetDecisionRecordRequest{DecisionKey: "dk-1"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	m := getResp.GetRecord()
	if m.GetId() != 42 || m.GetDecisionKey() != "dk-1" || m.GetCategory() != "system_guard" {
		t.Fatalf("base fields: %+v", m)
	}
	if m.GetOutcome() != "tripped" || m.GetActorKey() != "system:token_budget" {
		t.Fatalf("outcome/actor: %+v", m)
	}
	if m.Confidence == nil || m.GetConfidence() != 0.75 {
		t.Fatalf("confidence = %v", m.Confidence)
	}
	if m.ParentDecisionId == nil || m.GetParentDecisionId() != 41 {
		t.Fatalf("parent = %v", m.ParentDecisionId)
	}
	ents := m.GetRelatedEntities().GetValues()
	if len(ents) != 1 || ents[0].GetStructValue().GetFields()["key"].GetStringValue() != "team-9" {
		t.Fatalf("entities = %v", m.GetRelatedEntities())
	}
	if got := m.GetSourceRef().GetFields()["run_id"].GetStringValue(); got != "run-1" {
		t.Fatalf("source_ref.run_id = %q", got)
	}
	if got := m.GetMetadata().GetFields()["trigger_rule"].GetStringValue(); got != "token_budget_tripped" {
		t.Fatalf("metadata.trigger_rule = %q", got)
	}
	if m.GetCreatedAt().AsTime().Format("2006-01-02T15:04:05Z") != "2026-08-26T03:00:00Z" {
		t.Fatalf("created_at = %v", m.GetCreatedAt())
	}

	// List 同路映射 + total/page 回填。
	listResp, err := svc.ListDecisionRecords(context.Background(), &v1.ListDecisionRecordsRequest{Category: "system_guard"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listResp.GetTotal() != 1 || len(listResp.GetItems()) != 1 || listResp.GetItems()[0].GetDecisionKey() != "dk-1" {
		t.Fatalf("list resp = %+v", listResp)
	}
	if listResp.GetPage() != 1 || listResp.GetPageSize() != 20 {
		t.Fatalf("page defaults = %d/%d", listResp.GetPage(), listResp.GetPageSize())
	}
}

// TestDecisionRecordService_GetChain pins the 1.8 chain RPC：root/upstream
// （含 virtual_parent 标记）/downstream 三段响应与 NotFound 路径。
func TestDecisionRecordService_GetChain(t *testing.T) {
	root := decision.Record{
		ID: 11, DecisionKey: "dk-root", Category: decision.CategorySystemGuard,
		Scenario: "s", Reasoning: "r", Outcome: "tripped",
		ActorType: decision.ActorSystem, ActorKey: "system:token_budget",
		SourceRef: decision.SourceRef{RunID: "run-1"}, CreatedAt: "2026-08-26T03:00:00Z",
	}
	repo := &fakeDecisionQueryRepo{
		items: []decision.Record{root},
		// root 无父 → biz 走虚拟父兜底（FindVirtualParentPlanner），标记由 biz 置位。
		planner:    &decision.Record{ID: 5, DecisionKey: "dk-planner", Category: decision.CategoryPlannerOrchestration, Outcome: "selected_dag", ActorType: decision.ActorSystem, ActorKey: "system:task_planner", Scenario: "s"},
		downstream: []decision.Record{{ID: 12, DecisionKey: "dk-child", Category: decision.CategorySystemGuard, Outcome: "blocked", ActorType: decision.ActorSystem, ActorKey: "system:loop_guard", Scenario: "s"}},
	}
	svc := NewDecisionRecordService(decision.NewQueryUsecase(repo), nil, nil)

	resp, err := svc.GetDecisionChain(context.Background(), &v1.GetDecisionChainRequest{DecisionKey: "dk-root"})
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if resp.GetRoot().GetDecisionKey() != "dk-root" {
		t.Fatalf("root = %v", resp.GetRoot())
	}
	if len(resp.GetUpstream()) != 1 || !resp.GetUpstream()[0].GetVirtualParent() {
		t.Fatalf("upstream = %+v", resp.GetUpstream())
	}
	if len(resp.GetDownstream()) != 1 || resp.GetDownstream()[0].GetDecisionKey() != "dk-child" {
		t.Fatalf("downstream = %+v", resp.GetDownstream())
	}

	// 未命中 → NotFound。
	_, err = svc.GetDecisionChain(context.Background(), &v1.GetDecisionChainRequest{DecisionKey: "dk-x"})
	if err == nil {
		t.Fatal("expected NotFound for unknown decision_key")
	}
}

// TestDecisionRecordService_WorkspaceIsolation pins t-dr-3 租户隔离契约：
// List 非系统 caller 派生 VisibleWorkspaces=[callerWS, ""]（共享记录可读），
// 系统 caller 不过滤；Get/GetChain 跨租户 fail-closed 按 NotFound（不透出
// 存在性），共享（''）记录任何租户可见。
func TestDecisionRecordService_WorkspaceIsolation(t *testing.T) {
	mk := func(key, ws string) decision.Record {
		return decision.Record{
			ID: 1, DecisionKey: key, Category: decision.CategorySystemGuard,
			Scenario: "s", Reasoning: "r", Outcome: "tripped",
			ActorType: decision.ActorSystem, ActorKey: "system:guard",
			WorkspaceID: ws, CreatedAt: "2026-08-27T00:00:00Z",
		}
	}
	repo := &fakeDecisionQueryRepo{
		items: []decision.Record{mk("dk-a", "ws-a"), mk("dk-shared", "")},
		total: 2,
	}
	svc := NewDecisionRecordService(decision.NewQueryUsecase(repo), nil, nil)

	// ① List：租户 caller → VisibleWorkspaces=[callerWS, ""]。
	tenantCtx := workspace.WithContext(context.Background(), "ws-a")
	if _, err := svc.ListDecisionRecords(tenantCtx, &v1.ListDecisionRecordsRequest{}); err != nil {
		t.Fatalf("list tenant: %v", err)
	}
	got := repo.lastFilter.VisibleWorkspaces
	if len(got) != 2 || got[0] != "ws-a" || got[1] != "" {
		t.Fatalf("tenant VisibleWorkspaces = %v, want [ws-a '']", got)
	}
	// ② List：系统 caller → 不过滤（nil）。
	sysCtx := workspace.WithSystemWorkspace(context.Background())
	if _, err := svc.ListDecisionRecords(sysCtx, &v1.ListDecisionRecordsRequest{}); err != nil {
		t.Fatalf("list system: %v", err)
	}
	if repo.lastFilter.VisibleWorkspaces != nil {
		t.Fatalf("system VisibleWorkspaces = %v, want nil", repo.lastFilter.VisibleWorkspaces)
	}

	// ③ Get：本租户可见；跨租户 NotFound；共享记录任意租户可见。
	if _, err := svc.GetDecisionRecord(tenantCtx, &v1.GetDecisionRecordRequest{DecisionKey: "dk-a"}); err != nil {
		t.Fatalf("own tenant get: %v", err)
	}
	otherCtx := workspace.WithContext(context.Background(), "ws-b")
	if _, err := svc.GetDecisionRecord(otherCtx, &v1.GetDecisionRecordRequest{DecisionKey: "dk-a"}); err == nil {
		t.Fatal("cross-tenant get must be NotFound")
	}
	if _, err := svc.GetDecisionRecord(otherCtx, &v1.GetDecisionRecordRequest{DecisionKey: "dk-shared"}); err != nil {
		t.Fatalf("shared record must be visible to any tenant: %v", err)
	}

	// ④ GetChain：root 跨租户 → NotFound（链同属一个因果族，校验 root 即覆盖全链）。
	if _, err := svc.GetDecisionChain(tenantCtx, &v1.GetDecisionChainRequest{DecisionKey: "dk-a"}); err != nil {
		t.Fatalf("own tenant chain: %v", err)
	}
	if _, err := svc.GetDecisionChain(otherCtx, &v1.GetDecisionChainRequest{DecisionKey: "dk-a"}); err == nil {
		t.Fatal("cross-tenant chain must be NotFound")
	}
}

// TestDecisionRecordService_ChainNodeVisibility pins t-dr-5 节点级兜底：
// 「同族同 workspace」非 DB 约束——共享（''）root 的虚拟父/downstream 可
// 混入他租户记录（历史数据 + spirit 会话跨部署边界）。upstream 遇首个不
// 可见节点截断，downstream 逐节点剔除；系统 caller 不过滤。
func TestDecisionRecordService_ChainNodeVisibility(t *testing.T) {
	mk := func(id int64, key, ws string) decision.Record {
		return decision.Record{
			ID: id, DecisionKey: key, Category: decision.CategorySystemGuard,
			Scenario: "s", Reasoning: "r", Outcome: "tripped",
			ActorType: decision.ActorSystem, ActorKey: "system:guard",
			WorkspaceID: ws, SourceRef: decision.SourceRef{RunID: "run-1"},
			CreatedAt: "2026-08-27T00:00:00Z",
		}
	}
	repo := &fakeDecisionQueryRepo{
		items: []decision.Record{mk(1, "dk-shared-root", "")},
		// 虚拟父：他租户 planner（root 无父 → biz 走 FindVirtualParentPlanner）。
		planner: &decision.Record{
			ID: 5, DecisionKey: "dk-planner-a", Category: decision.CategoryPlannerOrchestration,
			Outcome: "selected_dag", ActorType: decision.ActorSystem, ActorKey: "system:task_planner",
			Scenario: "s", WorkspaceID: "ws-a", CreatedAt: "2026-08-26T00:00:00Z",
		},
		downstream: []decision.Record{
			mk(2, "dk-down-a", "ws-a"),   // 他租户 → 剔除
			mk(3, "dk-down-shared", ""),  // 共享 → 保留
			mk(4, "dk-down-b", "ws-b"),   // 本租户 → 保留
		},
	}
	svc := NewDecisionRecordService(decision.NewQueryUsecase(repo), nil, nil)

	// ① 租户 caller：虚拟父他租户 → upstream 截断为空；downstream 剔他租户留共享+本租户。
	bCtx := workspace.WithContext(context.Background(), "ws-b")
	resp, err := svc.GetDecisionChain(bCtx, &v1.GetDecisionChainRequest{DecisionKey: "dk-shared-root"})
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(resp.GetUpstream()) != 0 {
		t.Fatalf("cross-tenant virtual parent must be truncated: %+v", resp.GetUpstream())
	}
	if len(resp.GetDownstream()) != 2 ||
		resp.GetDownstream()[0].GetDecisionKey() != "dk-down-shared" ||
		resp.GetDownstream()[1].GetDecisionKey() != "dk-down-b" {
		t.Fatalf("downstream = %+v, want [dk-down-shared dk-down-b]", resp.GetDownstream())
	}

	// ② 系统 caller：不过滤，虚拟父 + 全部 downstream 可见。
	sysResp, err := svc.GetDecisionChain(workspace.WithSystemWorkspace(context.Background()),
		&v1.GetDecisionChainRequest{DecisionKey: "dk-shared-root"})
	if err != nil {
		t.Fatalf("system chain: %v", err)
	}
	if len(sysResp.GetUpstream()) != 1 || !sysResp.GetUpstream()[0].GetVirtualParent() {
		t.Fatalf("system upstream = %+v", sysResp.GetUpstream())
	}
	if len(sysResp.GetDownstream()) != 3 {
		t.Fatalf("system downstream = %+v, want 3", sysResp.GetDownstream())
	}

	// ③ 本租户 caller（ws-a）：虚拟父可见，downstream 剔除 ws-b。
	aResp, err := svc.GetDecisionChain(workspace.WithContext(context.Background(), "ws-a"),
		&v1.GetDecisionChainRequest{DecisionKey: "dk-shared-root"})
	if err != nil {
		t.Fatalf("ws-a chain: %v", err)
	}
	if len(aResp.GetUpstream()) != 1 {
		t.Fatalf("own-tenant virtual parent must stay: %+v", aResp.GetUpstream())
	}
	if len(aResp.GetDownstream()) != 2 ||
		aResp.GetDownstream()[0].GetDecisionKey() != "dk-down-a" ||
		aResp.GetDownstream()[1].GetDecisionKey() != "dk-down-shared" {
		t.Fatalf("ws-a downstream = %+v, want [dk-down-a dk-down-shared]", aResp.GetDownstream())
	}
}

// TestDecisionRecordService_SessionGateStats 钉死 T5 聚合映射 + 四轮审查
// IDOR 校验（2026-08-27）：
//  ① 本租户 caller 查自己会话 → 聚合透传（六类字段映射）。
//  ② 系统 caller → 绕过归属校验。
//  ③ 跨租户 caller → NotFound（不泄露存在性/闸计数侧信道）。
//  ④ 会话不存在 → NotFound；sessUC 未装配 → NotFound（fail-closed）。
//  ⑤ 空 session_id → BadRequest。
func TestDecisionRecordService_SessionGateStats(t *testing.T) {
	repo := &fakeDecisionQueryRepo{}
	sessReader := &fakeSessionWorkspaceReader{byID: map[string]bizsession.Session{
		"sess-a": {ID: "sess-a", WorkspaceID: "ws-a"},
	}}
	svc := NewDecisionRecordService(decision.NewQueryUsecase(repo), sessReader, nil)

	// ① 本租户。
	ownResp, err := svc.GetSessionGateStats(workspace.WithContext(context.Background(), "ws-a"),
		&v1.GetSessionGateStatsRequest{SessionId: "sess-a"})
	if err != nil {
		t.Fatalf("own tenant: %v", err)
	}
	st := ownResp.GetStats()
	if st.GetLoopGuardBlocks() != 3 || st.GetParamRuleDenies() != 1 || !st.GetBudgetTripped() {
		t.Fatalf("stats mapping = %+v", st)
	}

	// ② 系统 caller 绕过。
	if _, err := svc.GetSessionGateStats(workspace.WithSystemWorkspace(context.Background()),
		&v1.GetSessionGateStatsRequest{SessionId: "sess-a"}); err != nil {
		t.Fatalf("system caller must bypass: %v", err)
	}

	// ③ 跨租户 → NotFound。
	if _, err := svc.GetSessionGateStats(workspace.WithContext(context.Background(), "ws-b"),
		&v1.GetSessionGateStatsRequest{SessionId: "sess-a"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("cross-tenant must be NotFound, got %v", err)
	}

	// ④ 不存在 → NotFound；sessUC nil → NotFound。
	if _, err := svc.GetSessionGateStats(workspace.WithContext(context.Background(), "ws-a"),
		&v1.GetSessionGateStatsRequest{SessionId: "sess-missing"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("missing session must be NotFound, got %v", err)
	}
	svcNil := NewDecisionRecordService(decision.NewQueryUsecase(repo), nil, nil)
	if _, err := svcNil.GetSessionGateStats(workspace.WithContext(context.Background(), "ws-a"),
		&v1.GetSessionGateStatsRequest{SessionId: "sess-a"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("nil sessUC must fail-closed NotFound, got %v", err)
	}

	// ⑤ 空 session_id → BadRequest。
	if _, err := svc.GetSessionGateStats(context.Background(), &v1.GetSessionGateStatsRequest{}); err == nil {
		t.Fatal("empty session_id must be rejected")
	}
}

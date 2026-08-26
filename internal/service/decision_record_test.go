package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/decision/v1"
	"aranea-agents/internal/biz/decision"
)

// fakeDecisionQueryRepo 是 service 层的 QueryRepo 测试替身。
type fakeDecisionQueryRepo struct {
	items      []decision.Record
	total      int64
	upstream   []decision.Record
	downstream []decision.Record
	planner    *decision.Record
}

func (f *fakeDecisionQueryRepo) ListRecords(context.Context, decision.ListFilter) ([]decision.Record, int64, error) {
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

func (f *fakeDecisionQueryRepo) FindLatestPlannerByRun(context.Context, string, string, int64) (*decision.Record, error) {
	return f.planner, nil
}

func newDecisionTestService(items []decision.Record, total int64) *DecisionRecordService {
	return NewDecisionRecordService(decision.NewQueryUsecase(&fakeDecisionQueryRepo{items: items, total: total}), nil)
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
	svc := NewDecisionRecordService(decision.NewQueryUsecase(repo), nil)

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
		// root 无父 → biz 走虚拟父兜底（FindLatestPlannerByRun），标记由 biz 置位。
		planner:    &decision.Record{ID: 5, DecisionKey: "dk-planner", Category: decision.CategoryPlannerOrchestration, Outcome: "selected_dag", ActorType: decision.ActorSystem, ActorKey: "system:task_planner", Scenario: "s"},
		downstream: []decision.Record{{ID: 12, DecisionKey: "dk-child", Category: decision.CategorySystemGuard, Outcome: "blocked", ActorType: decision.ActorSystem, ActorKey: "system:loop_guard", Scenario: "s"}},
	}
	svc := NewDecisionRecordService(decision.NewQueryUsecase(repo), nil)

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

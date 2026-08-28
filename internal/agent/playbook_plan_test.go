package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestPlanExpandsNamedPlaybookWithoutLLM(t *testing.T) {
	t.Parallel()
	repo := &stubTaskPlanRepo{}
	impl := NewTaskPlanner(repo, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil)
	AttachPlannerOrganizationReader(impl, &stubOrgReader{nodes: map[string]biz.OrganizationNode{
		"co-1": {
			ID:    "co-1",
			Key:   "acme",
			Level: "company",
			MetadataJSON: `{
			  "playbooks": [{
			    "id": "software_delivery",
			    "authorized_by": "__company_lead_acme__",
			    "stages": [
			      {"id": "design", "domain_path": "设计/视觉"},
			      {"id": "be", "domain_path": "软件/后端", "depends_on": ["design"], "graph_template_id": "tmpl-1", "confirm_before": true, "collection_ids": ["kb-legal"]}
			    ]
			  }]
			}`,
		},
	}})

	plan, err := impl.Plan(context.Background(), biz.PlanInput{
		SpiritSessionID: "sp-pb",
		UserMessage:     "请按 software_delivery 交付本次迭代",
		Mode:            "direct",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || len(plan.SubTasks) != 2 {
		t.Fatalf("playbook stages=%v", plan)
	}
	if plan.SubTasks[1].GraphTemplateID != "tmpl-1" || !plan.SubTasks[1].ConfirmBefore {
		t.Fatalf("graph template/confirm lost: %+v", plan.SubTasks[1])
	}
	if len(plan.SubTasks[1].CollectionIDs) != 1 || plan.SubTasks[1].CollectionIDs[0] != "kb-legal" {
		t.Fatalf("collection ids lost: %+v", plan.SubTasks[1])
	}
	if plan.MemoryHit == nil || plan.MemoryHit.PlaybookID != "software_delivery" {
		t.Fatalf("playbook hit=%+v", plan.MemoryHit)
	}
	if plan.DecomposeReason == "" {
		t.Fatal("expected playbook reason")
	}
}

func TestPlanOrgChainWithoutPlaybookDoesNotDecompose(t *testing.T) {
	t.Parallel()
	repo := &stubTaskPlanRepo{}
	impl := NewTaskPlanner(repo, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil)
	AttachPlannerOrganizationReader(impl, &stubOrgReader{nodes: map[string]biz.OrganizationNode{
		"co-1": {ID: "co-1", Key: "acme", Level: "company", MetadataJSON: `{}`},
	}})

	plan, err := impl.Plan(context.Background(), biz.PlanInput{
		SpiritSessionID: "sp-fill",
		UserMessage:     "这次请按组织链走编制汇报，完成整条软件交付",
		Mode:            "dag",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != biz.StrategyDirect || plan.StrategyReason != biz.PlaybookFillRequiredReason {
		t.Fatalf("want fill-required direct, got strategy=%s reason=%s", plan.Strategy, plan.StrategyReason)
	}
	if len(plan.SubTasks) != 0 {
		t.Fatalf("must not invent stages: %+v", plan.SubTasks)
	}
}

func TestPlanDoesNotAutoPickSolePlaybookOnLightCopy(t *testing.T) {
	t.Parallel()
	repo := &stubTaskPlanRepo{}
	impl := NewTaskPlanner(repo, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil)
	AttachPlannerOrganizationReader(impl, &stubOrgReader{nodes: map[string]biz.OrganizationNode{
		"co-1": {
			ID:    "co-1",
			Key:   "acme",
			Level: "company",
			MetadataJSON: `{
			  "playbooks": [{
			    "id": "software_delivery",
			    "authorized_by": "__company_lead_acme__",
			    "stages": [{"id": "be", "domain_path": "软件/后端"}]
			  }]
			}`,
		},
	}})

	plan, err := impl.Plan(context.Background(), biz.PlanInput{
		SpiritSessionID: "sp-light",
		UserMessage:     "改一句文案",
		Mode:            "direct",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SubTasks) != 0 {
		t.Fatalf("light copy must not expand playbook: %+v", plan.SubTasks)
	}
}

func TestPlan_CrossDeptSubIntentsExpandSolePlaybook(t *testing.T) {
	t.Parallel()
	repo := &stubTaskPlanRepo{}
	impl := NewTaskPlanner(repo, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil)
	AttachPlannerOrganizationReader(impl, &stubOrgReader{nodes: map[string]biz.OrganizationNode{
		"co-1": {
			ID:    "co-1",
			Key:   "acme",
			Level: "company",
			MetadataJSON: `{
			  "playbooks": [{
			    "id": "software_delivery",
			    "authorized_by": "__company_lead_acme__",
			    "stages": [
			      {"id": "design", "domain_path": "设计/视觉"},
			      {"id": "be", "domain_path": "软件/后端", "depends_on": ["design"]}
			    ]
			  }]
			}`,
		},
	}})

	plan, err := impl.Plan(context.Background(), biz.PlanInput{
		SpiritSessionID: "sp-cross",
		UserMessage:     "先调研竞品再出视觉方案",
		IntentArtifact: &biz.IntentArtifact{
			RefinedGoal: "调研竞品并出视觉方案",
			SubIntents: []biz.SubIntent{
				{Goal: "调研竞品", IntentKind: "research"},
				{Goal: "出视觉方案", IntentKind: "creative"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.MemoryHit == nil || plan.MemoryHit.PlaybookID != "software_delivery" {
		t.Fatalf("cross-dept upgrade must expand sole playbook, hit=%+v", plan.MemoryHit)
	}
	if len(plan.SubTasks) != 2 || plan.SubTasks[0].DomainPath != "设计/视觉" {
		t.Fatalf("playbook stages=%+v", plan.SubTasks)
	}
	if plan.Strategy != biz.StrategyDAG {
		t.Fatalf("strategy=%s", plan.Strategy)
	}
}

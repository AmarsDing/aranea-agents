package biz

import (
	"strings"
	"testing"
)

func TestParseAndExpandAuthorizedPlaybook(t *testing.T) {
	t.Parallel()
	meta := `{
	  "playbooks": [{
	    "id": "software_delivery",
	    "authorized_by": "__company_lead_acme__",
	    "stages": [
	      {"id": "design", "department_key": "design", "domain_path": "设计/视觉", "deliverable_names": ["spec"]},
	      {"id": "be", "department_key": "eng", "domain_path": "软件/后端", "depends_on": ["design"], "confirm_before": true, "graph_template_id": "parallel_review", "collection_ids": ["kb-a"]}
	    ]
	  }]
	}`
	pbs := ParseCompanyPlaybooks(meta)
	pb, ok := FindAuthorizedPlaybook(pbs, "software_delivery")
	if !ok {
		t.Fatal("authorized playbook missed")
	}
	steps := ExpandPlaybook(pb)
	if len(steps) != 2 || steps[0].DomainPath != "设计/视觉" {
		t.Fatalf("expand=%+v", steps)
	}
	// F7：step ID 必须带运行级唯一后缀，DependsOn 同步重映射到改名后的 ID。
	if steps[0].ID == "design" || !strings.HasPrefix(steps[0].ID, "design_") {
		t.Fatalf("step id not namespaced: %+v", steps[0])
	}
	if len(steps[1].DependsOn) != 1 || steps[1].DependsOn[0] != steps[0].ID {
		t.Fatalf("depends_on not remapped: %+v", steps[1])
	}
	if !steps[1].ConfirmBefore || steps[1].GraphTemplateID != "parallel_review" {
		t.Fatalf("confirm/template lost: %+v", steps[1])
	}
	if len(steps[1].CollectionIDs) != 1 || steps[1].CollectionIDs[0] != "kb-a" {
		t.Fatalf("collection ids lost: %+v", steps[1])
	}
	if _, ok := FindAuthorizedPlaybook(pbs, "missing"); ok {
		t.Fatal("missing id must not hit")
	}
}

func TestExpandPlaybookUniqueIDsAcrossRuns(t *testing.T) {
	t.Parallel()
	pb := Playbook{
		ID: "software_delivery",
		Stages: []PlaybookStage{
			{ID: "design", DomainPath: "设计/视觉"},
			{ID: "be", DomainPath: "软件/后端", DependsOn: []string{"design"}},
		},
	}
	a := ExpandPlaybook(pb)
	b := ExpandPlaybook(pb)
	// 同一 playbook 两次展开：ID 必须不同（否则 plan_steps_v2 PK 碰撞）。
	for i := range a {
		if a[i].ID == b[i].ID {
			t.Fatalf("run collision on step %d: %s", i, a[i].ID)
		}
	}
	// 每次展开内部：DependsOn 必须指向同次展开的 ID。
	if a[1].DependsOn[0] != a[0].ID || b[1].DependsOn[0] != b[0].ID {
		t.Fatalf("intra-run dep remap broken: a=%+v b=%+v", a, b)
	}
	// 空 stage id 走兜底 id，同样唯一化。
	c := ExpandPlaybook(Playbook{ID: "p", Stages: []PlaybookStage{{DomainPath: "软件/后端"}}})
	if len(c) != 1 || c[0].ID == "pb_p_1" || !strings.HasPrefix(c[0].ID, "pb_p_1_") {
		t.Fatalf("fallback id not namespaced: %+v", c)
	}
}

func TestMergeAndAuthorizePlaybookPreservesOtherMetadata(t *testing.T) {
	t.Parallel()
	n := OrganizationNode{Key: "acme", MetadataJSON: `{"company_lead_agent_id":"lead-1"}`}
	AuthorizePlaybookOnCompany(&n, Playbook{ID: "software_delivery", Stages: []PlaybookStage{{ID: "be"}}})
	if !strings.Contains(n.MetadataJSON, "company_lead_agent_id") {
		t.Fatalf("lost other metadata: %s", n.MetadataJSON)
	}
	pbs := ParseCompanyPlaybooks(n.MetadataJSON)
	if len(pbs) != 1 || pbs[0].AuthorizedBy != CompanyLeadAgentKeyPrefix+"acme__" {
		t.Fatalf("authorized=%+v", pbs)
	}
	AuthorizePlaybookOnCompany(&n, Playbook{ID: "software_delivery", Name: "v2", Stages: []PlaybookStage{{ID: "be"}}})
	if got := ParseCompanyPlaybooks(n.MetadataJSON); len(got) != 1 || got[0].Name != "v2" {
		t.Fatalf("upsert=%+v", got)
	}
}

func TestTryPlaybookForTaskMentionOnly(t *testing.T) {
	t.Parallel()
	meta := `{
	  "playbooks": [{
	    "id": "software_delivery",
	    "name": "软件交付",
	    "authorized_by": "__company_lead_acme__",
	    "stages": [{"id": "be", "domain_path": "软件/后端"}]
	  }]
	}`
	if _, _, ok := TryPlaybookForTask(meta, "改一句文案"); ok {
		t.Fatal("unnamed light task must not auto-pick the only playbook")
	}
	if _, steps, ok := TryPlaybookForTask(meta, "按 software_delivery 走"); !ok || len(steps) != 1 {
		t.Fatal("named playbook must expand")
	}
	if _, steps, ok := TryPlaybookForTask(meta, "走软件交付流程"); !ok || len(steps) != 1 {
		t.Fatal("named playbook by title must expand")
	}
	if _, steps, ok := TrySoleAuthorizedPlaybook(meta); !ok || len(steps) != 1 {
		t.Fatal("sole authorized playbook is for heavy gear only")
	}
}

func TestFindAuthorizedPlaybookRequiresSigner(t *testing.T) {
	t.Parallel()
	if _, ok := FindAuthorizedPlaybook([]Playbook{{ID: "x"}}, "x"); ok {
		t.Fatal("unauthorized playbook must not expand")
	}
}

// RecipeKeysReusable 的 D7-F2 fail-closed 契约（playbook.go 实现为准，本测试只钉契约）：
//   - want == ""（当前任务不受剧本约束）→ 放行，旧条目继续服务；
//   - want != "" 且 entry == ""（旧条目无 provenance）→ 一次性降级：keys 不复用
//     （骨架仍可复用，不删数据）；
//   - 两侧均非空 → 仅指纹完全一致才复用。
func TestConstraintFingerprintAndRecipeKeysFailClosed(t *testing.T) {
	t.Parallel()
	a := ConstraintFingerprint("software_delivery", map[string]string{"sla": "p1", "合规": "等保"})
	b := ConstraintFingerprint("software_delivery", map[string]string{"合规": "等保", "sla": "p1"})
	c := ConstraintFingerprint("software_delivery", map[string]string{"sla": "p2"})
	if a == "" || a != b {
		t.Fatalf("stable hash failed a=%s b=%s", a, b)
	}
	if a == c {
		t.Fatal("different constraints must differ")
	}
	// fail-closed：旧条目空指纹 + 任务带约束指纹 → 一次性降级，keys 不复用。
	if RecipeKeysReusable("", a) {
		t.Fatal("empty entry fingerprint must NOT reuse keys under a constrained task (fail-closed)")
	}
	// 无约束任务放行：want 为空时旧条目（含带指纹的）继续服务。
	if !RecipeKeysReusable(a, "") || !RecipeKeysReusable("", "") {
		t.Fatal("unconstrained task must stay compatible with any entry fingerprint")
	}
	// 指纹一致才复用；不一致不复用。
	if !RecipeKeysReusable(a, a) {
		t.Fatal("identical fingerprints must reuse keys")
	}
	if RecipeKeysReusable(a, c) {
		t.Fatal("mismatched fingerprint must not reuse keys")
	}
}

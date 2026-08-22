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
	      {"id": "be", "department_key": "eng", "domain_path": "软件/后端", "depends_on": ["design"]}
	    ]
	  }]
	}`
	pbs := ParseCompanyPlaybooks(meta)
	pb, ok := FindAuthorizedPlaybook(pbs, "software_delivery")
	if !ok {
		t.Fatal("authorized playbook missed")
	}
	steps := ExpandPlaybook(pb)
	if len(steps) != 2 || steps[0].DomainPath != "设计/视觉" || steps[1].DependsOn[0] != "design" {
		t.Fatalf("expand=%+v", steps)
	}
	if _, ok := FindAuthorizedPlaybook(pbs, "missing"); ok {
		t.Fatal("missing id must not hit")
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

func TestConstraintFingerprintAndRecipeKeys(t *testing.T) {
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
	if !RecipeKeysReusable("", a) || !RecipeKeysReusable(a, "") {
		t.Fatal("legacy empty fingerprint stays compatible")
	}
	if RecipeKeysReusable(a, c) {
		t.Fatal("mismatched fingerprint must not reuse keys")
	}
}

package biz

import "testing"

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

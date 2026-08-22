package biz

import "testing"

func TestIsCompanyLeadAgent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		agent Agent
		want  bool
	}{
		{"variant", Agent{AgentVariant: AgentVariantCompanyLead}, true},
		{"key", Agent{AgentKey: CompanyLeadAgentKeyPrefix + "acme__"}, true},
		{"prefix only", Agent{AgentKey: CompanyLeadAgentKeyPrefix + "acme"}, false},
		{"dept lead is not company", Agent{AgentVariant: "dept_lead"}, false},
		{"worker", Agent{AgentKey: "be"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsCompanyLeadAgent(c.agent); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestCompanyLeadNotAssignable(t *testing.T) {
	t.Parallel()
	a := Agent{
		AgentKey:     CompanyLeadAgentKeyPrefix + "acme__",
		AgentVariant: AgentVariantCompanyLead,
		Status:       "active",
	}
	if IsCatalogAgentAssignable(a) {
		t.Fatal("company_lead must not be catalog-assignable")
	}
	cap := AgentCapability{AgentKey: a.AgentKey, AgentVariant: a.AgentVariant}
	if cap.IsHeuristicAssignable() {
		t.Fatal("company_lead must not be heuristically assignable")
	}
}

func TestCompanyLeadMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	n := OrganizationNode{MetadataJSON: `{"playbooks":[]}`, CompanyLeadAgentID: "lead-1"}
	ApplyCompanyLeadToMetadata(&n)
	n.CompanyLeadAgentID = ""
	HydrateCompanyLeadFromMetadata(&n)
	if n.CompanyLeadAgentID != "lead-1" {
		t.Fatalf("id=%q", n.CompanyLeadAgentID)
	}
}

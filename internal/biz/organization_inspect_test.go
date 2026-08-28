package biz

import "testing"

func TestBuildGovernanceInspectView_DeptLeadScoped(t *testing.T) {
	company := OrganizationNode{ID: "c1", Key: "media", Name: "传媒公司", Level: "company", CompanyLeadAgentID: "gm-1"}
	dept := OrganizationNode{ID: "d1", Key: "mkt", Name: "市场部", Level: "department", ParentID: "c1", DeptLeadAgentID: "lead-1"}
	pos := OrganizationNode{ID: "p1", Key: "copy", Name: "文案岗", Level: "position", ParentID: "d1"}
	other := OrganizationNode{ID: "d2", Key: "eng", Name: "技术部", Level: "department", ParentID: "c1"}
	nodes := []OrganizationNode{company, dept, pos, other}

	view, err := BuildGovernanceInspectView(nodes, Agent{ID: "lead-1", AgentVariant: "dept_lead", PositionID: "d1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if view.ScopeKey != "mkt" {
		t.Fatalf("scope = %q, want mkt", view.ScopeKey)
	}
	keys := map[string]bool{}
	for _, e := range view.Entries {
		keys[e.Key] = true
	}
	if !keys["mkt"] || !keys["copy"] {
		t.Fatalf("dept subtree missing: %+v", view.Entries)
	}
	if keys["eng"] || keys["media"] {
		t.Fatalf("dept lead must not see sibling/company root as entries beyond scope: %+v", view.Entries)
	}
}

func TestBuildGovernanceInspectView_CompanyLeadSeesTree(t *testing.T) {
	company := OrganizationNode{ID: "c1", Key: "media", Name: "传媒公司", Level: "company"}
	office := OrganizationNode{ID: "d0", Key: "media_office", Name: "总经理办公室", Level: "department", ParentID: "c1"}
	gmPos := OrganizationNode{ID: "p0", Key: "media_gm", Name: "总经理", Level: "position", ParentID: "d0"}
	dept := OrganizationNode{ID: "d1", Key: "mkt", Name: "市场部", Level: "department", ParentID: "c1"}
	nodes := []OrganizationNode{company, office, gmPos, dept}

	view, err := BuildGovernanceInspectView(nodes, Agent{
		ID: "gm-1", AgentKey: CompanyLeadAgentKeyPrefix + "media__", AgentVariant: AgentVariantCompanyLead, PositionID: "p0",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if view.ScopeKey != "media" {
		t.Fatalf("scope = %q, want media", view.ScopeKey)
	}
	foundMkt := false
	for _, e := range view.Entries {
		if e.Key == "mkt" {
			foundMkt = true
		}
	}
	if !foundMkt {
		t.Fatalf("GM must see 市场部: %+v", view.Entries)
	}
}

func TestBuildGovernanceInspectView_NonGovernanceRejectedByUsecaseShape(t *testing.T) {
	// Pure helper does not ACL — usecase InspectGovernance does. This locks
	// locateCallerOrgNode miss.
	_, err := BuildGovernanceInspectView(nil, Agent{ID: "x"}, "")
	if err != nil {
		t.Fatalf("empty nodes should return empty view, got %v", err)
	}
}

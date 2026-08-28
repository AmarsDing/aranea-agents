package service

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type stubOrgByID struct {
	nodes map[string]biz.OrganizationNode
}

func (s *stubOrgByID) GetOrgNode(_ context.Context, id string) (biz.OrganizationNode, error) {
	n, ok := s.nodes[id]
	if !ok {
		return biz.OrganizationNode{}, biz.ErrOrgBadRequest("not found")
	}
	return n, nil
}
func (s *stubOrgByID) GetOrgNodeByKey(_ context.Context, _ string) (biz.OrganizationNode, error) {
	return biz.OrganizationNode{}, biz.ErrOrgBadRequest("not found")
}
func (s *stubOrgByID) ListOrgNodes(_ context.Context) ([]biz.OrganizationNode, error) {
	return nil, nil
}
func (s *stubOrgByID) ListOrgNodesByLevel(_ context.Context, _ string) ([]biz.OrganizationNode, error) {
	return nil, nil
}
func (s *stubOrgByID) ListOrgNodesByParentID(_ context.Context, _ string) ([]biz.OrganizationNode, error) {
	return nil, nil
}
func (s *stubOrgByID) ListOrgNodesByIDs(_ context.Context, ids []string) ([]biz.OrganizationNode, error) {
	out := make([]biz.OrganizationNode, 0, len(ids))
	for _, id := range ids {
		if n, ok := s.nodes[id]; ok {
			out = append(out, n)
		}
	}
	return out, nil
}

func TestResolveTeamOrg_StepDepartmentAndCrossDeptIDs(t *testing.T) {
	o := NewRealTeamOrchestrator(loggateway.NewNoop())
	o.SetAgentReader(&stubAgentReaderByKey{byKey: map[string]biz.Agent{
		"copy": {ID: "id-copy", AgentKey: "copy", PositionID: "pos-copy", Status: "active"},
		"be":   {ID: "id-be", AgentKey: "be", PositionID: "pos-be", Status: "active"},
		"lead": {ID: "id-lead", AgentKey: biz.DeptLeadAgentKeyPrefix + "media__", AgentVariant: "dept_lead", PositionID: "dept-media", Status: "active"},
	}})
	o.SetOrganizationReader(&stubOrgByID{nodes: map[string]biz.OrganizationNode{
		"pos-copy":   {ID: "pos-copy", Level: "position", ParentID: "dept-media"},
		"pos-be":     {ID: "pos-be", Level: "position", ParentID: "dept-eng"},
		"dept-media": {ID: "dept-media", Level: "department"},
		"dept-eng":   {ID: "dept-eng", Level: "department"},
	}})

	dept, cross := o.resolveTeamOrg(context.Background(), biz.PlanStep{
		DepartmentID:        "dept-media",
		CrossDeptMemberKeys: []string{"be"},
	}, []string{"copy", "be"})
	if dept != "dept-media" {
		t.Fatalf("DepartmentID=%q", dept)
	}
	if len(cross) != 1 || cross[0] != "id-be" {
		t.Fatalf("cross=%v want [id-be]", cross)
	}
}

func TestResolveTeamOrg_MajorityVoteWhenStepEmpty(t *testing.T) {
	o := NewRealTeamOrchestrator(loggateway.NewNoop())
	o.SetAgentReader(&stubAgentReaderByKey{byKey: map[string]biz.Agent{
		"a": {ID: "1", AgentKey: "a", PositionID: "pos-a"},
		"b": {ID: "2", AgentKey: "b", PositionID: "pos-b"},
		"c": {ID: "3", AgentKey: "c", PositionID: "pos-c"},
	}})
	o.SetOrganizationReader(&stubOrgByID{nodes: map[string]biz.OrganizationNode{
		"pos-a":  {ID: "pos-a", Level: "position", ParentID: "dept-x"},
		"pos-b":  {ID: "pos-b", Level: "position", ParentID: "dept-x"},
		"pos-c":  {ID: "pos-c", Level: "position", ParentID: "dept-y"},
		"dept-x": {ID: "dept-x", Level: "department"},
		"dept-y": {ID: "dept-y", Level: "department"},
	}})
	dept, cross := o.resolveTeamOrg(context.Background(), biz.PlanStep{}, []string{"a", "b", "c"})
	if dept != "dept-x" {
		t.Fatalf("majority dept=%q want dept-x", dept)
	}
	if len(cross) != 1 || cross[0] != "3" {
		t.Fatalf("cross=%v want [3]", cross)
	}
}

func TestResolveTeamOrg_EmptyDepartmentStillAssembles(t *testing.T) {
	o := NewRealTeamOrchestrator(loggateway.NewNoop())
	dept, cross := o.resolveTeamOrg(context.Background(), biz.PlanStep{}, []string{"solo"})
	if dept != "" || len(cross) != 0 {
		t.Fatalf("empty org must not block: dept=%q cross=%v", dept, cross)
	}
}

func TestOrchestrate_EmptyAgentKeysFails(t *testing.T) {
	o := NewRealTeamOrchestrator(loggateway.NewNoop())
	o.SetAssembler(&SpiritTeamAssembler{})
	_, err := o.Orchestrate(context.Background(), biz.PlanStep{ID: "st-empty", Label: "调研"}, biz.TeamStage{SessionID: "sess-1"})
	if err == nil {
		t.Fatal("empty AgentKeys must fail closed")
	}
	if !strings.Contains(err.Error(), "empty agent keys") {
		t.Fatalf("err=%v", err)
	}
}

package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// memOrgRepo is an in-memory OrganizationRepo for usecase-level tests.
type memOrgRepo struct{ nodes map[string]OrganizationNode }

func (r *memOrgRepo) GetOrgNode(_ context.Context, id string) (OrganizationNode, error) {
	n, ok := r.nodes[id]
	if !ok {
		return OrganizationNode{}, ErrNotFound
	}
	return n, nil
}
func (r *memOrgRepo) GetOrgNodeByKey(_ context.Context, key string) (OrganizationNode, error) {
	for _, n := range r.nodes {
		if n.Key == key {
			return n, nil
		}
	}
	return OrganizationNode{}, ErrNotFound
}
func (r *memOrgRepo) GetOrgNodeByKeyAnyState(ctx context.Context, key string) (OrganizationNode, error) {
	return r.GetOrgNodeByKey(ctx, key)
}
func (r *memOrgRepo) ListOrgNodes(context.Context) ([]OrganizationNode, error) {
	out := make([]OrganizationNode, 0, len(r.nodes))
	for _, n := range r.nodes {
		out = append(out, n)
	}
	return out, nil
}
func (r *memOrgRepo) ListOrgNodesByLevel(_ context.Context, level string) ([]OrganizationNode, error) {
	var out []OrganizationNode
	for _, n := range r.nodes {
		if n.Level == level {
			out = append(out, n)
		}
	}
	return out, nil
}
func (r *memOrgRepo) ListOrgNodesByParentID(_ context.Context, parentID string) ([]OrganizationNode, error) {
	var out []OrganizationNode
	for _, n := range r.nodes {
		if n.ParentID == parentID {
			out = append(out, n)
		}
	}
	return out, nil
}
func (r *memOrgRepo) ListOrgNodesByIDs(_ context.Context, ids []string) ([]OrganizationNode, error) {
	var out []OrganizationNode
	for _, id := range ids {
		if n, ok := r.nodes[id]; ok {
			out = append(out, n)
		}
	}
	return out, nil
}
func (r *memOrgRepo) CreateOrgNode(_ context.Context, c OrganizationNode) (OrganizationNode, error) {
	r.nodes[c.ID] = c
	return c, nil
}
func (r *memOrgRepo) UpdateOrgNode(_ context.Context, c OrganizationNode) (OrganizationNode, error) {
	r.nodes[c.ID] = c
	return c, nil
}

// DeleteOrgNode mirrors data.organizationRepo's guards: no system nodes, no nodes with children.
func (r *memOrgRepo) DeleteOrgNode(ctx context.Context, id string) error {
	if r.nodes[id].IsSystem {
		return ErrOrgBadRequest("system preset category cannot be deleted")
	}
	if kids, _ := r.ListOrgNodesByParentID(ctx, id); len(kids) > 0 {
		return ErrOrgBadRequest("node has child nodes")
	}
	return r.ForceDeleteOrgNode(ctx, id)
}
func (r *memOrgRepo) ForceDeleteOrgNode(_ context.Context, id string) error {
	delete(r.nodes, id)
	return nil
}
func (r *memOrgRepo) ReorderOrgNodes(context.Context, []string) error { return nil }

type recordingPositionClearer struct {
	depts     []string
	positions []string
}

func (c *recordingPositionClearer) ClearPositionByDepartment(_ context.Context, deptID string) (int, error) {
	c.depts = append(c.depts, deptID)
	return 0, nil
}
func (c *recordingPositionClearer) ClearPositionByIDs(_ context.Context, ids []string) (int, error) {
	c.positions = append(c.positions, ids...)
	return len(ids), nil
}

func newOrgFixture() (*OrganizationUsecase, *memOrgRepo, *recordingPositionClearer) {
	repo := &memOrgRepo{nodes: map[string]OrganizationNode{
		"c1": {ID: "c1", Key: "acme", Name: "Acme", Level: "company", Status: "active", Enabled: true},
		"c2": {ID: "c2", Key: "beta", Name: "Beta", Level: "company", Status: "active", Enabled: true},
		"d1": {ID: "d1", Key: "acme_rd", Name: "R&D", Level: "department", ParentID: "c1", Status: "active", Enabled: true},
		"p1": {ID: "p1", Key: "acme_rd_dev", Name: "Dev", Level: "position", ParentID: "d1", Status: "active", Enabled: true},
		// What ensureCompanyLeads creates for every company: system office + GM position.
		"o1":  {ID: "o1", Key: CompanyOfficeDeptKey("acme"), Name: "总经理办公室", Level: "department", ParentID: "c1", IsSystem: true},
		"gm1": {ID: "gm1", Key: CompanyLeadPositionKey("acme"), Name: "总经理", Level: "position", ParentID: "o1", IsSystem: true},
	}}
	clear := &recordingPositionClearer{}
	lg := loggateway.NewNoop()
	uc := NewOrganizationUsecase(repo, nil, nil, nil, clear, NewPositionPromptUsecase(repo, lg), nil, lg)
	return uc, repo, clear
}

func TestOrganizationUpdate_KeepsSystemFlagAndOwner(t *testing.T) {
	uc, repo, _ := newOrgFixture()
	repo.nodes["d1"] = OrganizationNode{ID: "d1", Key: "acme_rd", Name: "R&D", Level: "department", ParentID: "c1", Status: "active", OwnerUserID: "u1"}

	out, err := uc.Update(context.Background(), "d1", OrganizationNode{Name: "R&D 2", Level: "department", ParentID: "c1", IsSystem: true, OwnerUserID: "attacker"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.IsSystem || out.OwnerUserID != "u1" || out.Name != "R&D 2" {
		t.Fatalf("got is_system=%v owner=%q name=%q; want false, u1, R&D 2", out.IsSystem, out.OwnerUserID, out.Name)
	}
}

func TestOrganizationUpdate_RejectsLevelChangeWithChildren(t *testing.T) {
	uc, repo, _ := newOrgFixture()
	// Demoting company c1 under c2 would put its departments under a department.
	if _, err := uc.Update(context.Background(), "c1", OrganizationNode{Level: "department", ParentID: "c2"}); err == nil {
		t.Fatal("expected error when changing level of a node with children")
	}
	if repo.nodes["c1"].Level != "company" {
		t.Fatal("c1 must stay a company")
	}
}

func TestOrganizationDelete_CompanyCascades(t *testing.T) {
	uc, repo, clear := newOrgFixture()
	if err := uc.Delete(context.Background(), "c1"); err != nil {
		t.Fatalf("delete company: %v", err)
	}
	for _, id := range []string{"c1", "d1", "p1", "o1", "gm1"} {
		if _, ok := repo.nodes[id]; ok {
			t.Errorf("%s should be deleted with its company", id)
		}
	}
	if _, ok := repo.nodes["c2"]; !ok {
		t.Error("other company must stay")
	}
	if len(clear.depts) != 2 {
		t.Errorf("agents of d1 and the office should be unseated, got %v", clear.depts)
	}
}

func TestOrganizationDelete_SystemNodeRefusedWithoutSideEffects(t *testing.T) {
	uc, repo, clear := newOrgFixture()
	if err := uc.Delete(context.Background(), "o1"); err == nil {
		t.Fatal("deleting a system node directly must be refused")
	}
	if _, ok := repo.nodes["gm1"]; !ok || len(clear.depts) != 0 {
		t.Fatalf("refused delete must not cascade: gm1 present=%v cleared=%v", ok, clear.depts)
	}
}

func TestOrganizationDelete_PositionUnseatsAgents(t *testing.T) {
	uc, repo, clear := newOrgFixture()
	if err := uc.Delete(context.Background(), "p1"); err != nil {
		t.Fatalf("delete position: %v", err)
	}
	if _, ok := repo.nodes["p1"]; ok {
		t.Error("p1 should be deleted")
	}
	if len(clear.positions) != 1 || clear.positions[0] != "p1" {
		t.Errorf("agents on p1 should be unseated, got %v", clear.positions)
	}
}

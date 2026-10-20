package service

import (
	"context"
	"errors"
	"testing"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

type orgScopeOrgReaderStub struct {
	biz.OrganizationReader
	nodes map[string]biz.OrganizationNode
}

func (s orgScopeOrgReaderStub) GetOrgNode(_ context.Context, id string) (biz.OrganizationNode, error) {
	if n, ok := s.nodes[id]; ok {
		return n, nil
	}
	return biz.OrganizationNode{}, errors.New("org node not found")
}

// G-08（ADR-KN-20261020-D/F）：prepareRunContext 装配期 org scope stamp 行为。
func TestPrepareRunContextStampsOrgScope(t *testing.T) {
	nodes := map[string]biz.OrganizationNode{
		"pos1":  {ID: "pos1", Level: "position", ParentID: "dept9"},
		"dept9": {ID: "dept9", Level: "department"},
	}

	t.Run("agent with position stamps {position, department}", func(t *testing.T) {
		o := &ChatOrchestrator{infraDeps: ChatInfraDeps{OrgReader: orgScopeOrgReaderStub{nodes: nodes}}}
		runCtx := o.prepareRunContext(context.Background(), biz.TurnInput{}, biz.Agent{ID: "ag1", PositionID: "pos1"}, chatagent.TRPCBuilderDeps{})
		got := bizknowledge.OrgScopeFromContext(runCtx)
		want := []string{"pos1", "dept9"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("agent without position → no stamp", func(t *testing.T) {
		o := &ChatOrchestrator{infraDeps: ChatInfraDeps{OrgReader: orgScopeOrgReaderStub{nodes: nodes}}}
		runCtx := o.prepareRunContext(context.Background(), biz.TurnInput{}, biz.Agent{ID: "ag2"}, chatagent.TRPCBuilderDeps{})
		if got := bizknowledge.OrgScopeFromContext(runCtx); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("nil OrgReader → no stamp", func(t *testing.T) {
		o := &ChatOrchestrator{}
		runCtx := o.prepareRunContext(context.Background(), biz.TurnInput{}, biz.Agent{ID: "ag1", PositionID: "pos1"}, chatagent.TRPCBuilderDeps{})
		if got := bizknowledge.OrgScopeFromContext(runCtx); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("unresolvable position → fail closed, no stamp", func(t *testing.T) {
		o := &ChatOrchestrator{infraDeps: ChatInfraDeps{OrgReader: orgScopeOrgReaderStub{nodes: nodes}}}
		runCtx := o.prepareRunContext(context.Background(), biz.TurnInput{}, biz.Agent{ID: "ag3", PositionID: "ghost"}, chatagent.TRPCBuilderDeps{})
		if got := bizknowledge.OrgScopeFromContext(runCtx); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})
}

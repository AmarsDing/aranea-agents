package team

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

type orgScopeLookupStub struct {
	biz.AgentRepository
	byID map[string]biz.Agent
}

func (s orgScopeLookupStub) GetAgentByID(_ context.Context, id string) (biz.Agent, error) {
	if a, ok := s.byID[id]; ok {
		return a, nil
	}
	return biz.Agent{}, errors.New("agent not found")
}

func TestResolveTeamOrgScope(t *testing.T) {
	agents := orgScopeLookupStub{byID: map[string]biz.Agent{
		"agA":      {ID: "agA", PositionID: "posA"},
		"agB":      {ID: "agB", PositionID: "posB"},
		"agNoPos":  {ID: "agNoPos"},
		"agDeptLd": {ID: "agDeptLd", PositionID: "deptA"},
	}}
	nodes := map[string]biz.OrganizationNode{
		"posA":  {ID: "posA", Level: "position", ParentID: "deptA"},
		"posB":  {ID: "posB", Level: "position", ParentID: "deptB"},
		"deptA": {ID: "deptA", Level: "department", ParentID: "comp1"},
		"deptB": {ID: "deptB", Level: "department", ParentID: "comp1"},
	}
	getNode := func(_ context.Context, id string) (biz.OrganizationNode, error) {
		if n, ok := nodes[id]; ok {
			return n, nil
		}
		return biz.OrganizationNode{}, errors.New("org node not found")
	}
	disabled := false
	def := Definition{Members: []MemberDef{
		{AgentID: "agA"},
		{AgentID: "agB"},
		{AgentID: "agDeptLd", Enabled: &disabled},
		{AgentID: "agNoPos"},
		{AgentID: "ghost"},
		{AgentID: ""},
	}}
	r := &Runner{
		td: rt.TurnDeps{ReadDeps: rt.TurnReadDeps{Agents: agents}},
		lg: loggateway.NewNoop(),
	}

	t.Run("enabled member union, dedupe, failures excluded", func(t *testing.T) {
		got := r.resolveTeamOrgScope(context.Background(), def, getNode)
		// agA: posA + deptA；agB: posB + deptB；
		// agDeptLd 被禁用（deptA 已由 agA 覆盖，且不应再出现于并集——此处通过去重总长相等体现）；
		// agNoPos / ghost / 空 ID 均跳过。
		want := []string{"posA", "deptA", "posB", "deptB"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("dept lead member included when enabled", func(t *testing.T) {
		enabled := true
		d := Definition{Members: []MemberDef{{AgentID: "agDeptLd", Enabled: &enabled}}}
		got := r.resolveTeamOrgScope(context.Background(), d, getNode)
		want := []string{"deptA"}
		if len(got) != 1 || got[0] != want[0] {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("nil getter → no stamp", func(t *testing.T) {
		if got := r.resolveTeamOrgScope(context.Background(), def, nil); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("shared parent department memoized (C2)", func(t *testing.T) {
		nodes2 := map[string]biz.OrganizationNode{
			"posA":  {ID: "posA", Level: "position", ParentID: "deptA"},
			"posA2": {ID: "posA2", Level: "position", ParentID: "deptA"},
			"deptA": {ID: "deptA", Level: "department"},
		}
		calls := map[string]int{}
		counting := func(_ context.Context, id string) (biz.OrganizationNode, error) {
			calls[id]++
			if n, ok := nodes2[id]; ok {
				return n, nil
			}
			return biz.OrganizationNode{}, errors.New("org node not found")
		}
		dupAgents := orgScopeLookupStub{byID: map[string]biz.Agent{
			"agA":  {ID: "agA", PositionID: "posA"},
			"agA2": {ID: "agA2", PositionID: "posA2"},
		}}
		r2 := &Runner{
			td: rt.TurnDeps{ReadDeps: rt.TurnReadDeps{Agents: dupAgents}},
			lg: loggateway.NewNoop(),
		}
		d := Definition{Members: []MemberDef{{AgentID: "agA"}, {AgentID: "agA2"}}}
		got := r2.resolveTeamOrgScope(context.Background(), d, counting)
		if len(got) != 3 {
			t.Fatalf("got %v, want {posA, deptA, posA2}", got)
		}
		if calls["deptA"] != 1 {
			t.Fatalf("deptA fetched %d times, want 1 (memoized)", calls["deptA"])
		}
	})
}

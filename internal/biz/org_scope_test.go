package biz

import (
	"context"
	"errors"
	"testing"
)

// G-08（ADR-KN-20261020-D/F）：runCtx org scope 解析的分支覆盖。
func TestResolveKnowledgeOrgScope(t *testing.T) {
	nodes := map[string]OrganizationNode{
		"pos1":   {ID: "pos1", Level: "position", ParentID: "deptA"},
		"pos2":   {ID: "pos2", Level: "position", ParentID: "compX"},
		"pos3":   {ID: "pos3", Level: "position"},
		"deptA":  {ID: "deptA", Level: "department", ParentID: "compX"},
		"compX":  {ID: "compX", Level: "company"},
		"deptLd": {ID: "deptLd", Level: "department"},
	}
	getNode := func(allow map[string]OrganizationNode) KnowledgeOrgScopeGetter {
		return func(_ context.Context, id string) (OrganizationNode, error) {
			if n, ok := allow[id]; ok {
				return n, nil
			}
			return OrganizationNode{}, errors.New("not found")
		}
	}

	cases := []struct {
		name string
		get  KnowledgeOrgScopeGetter
		pos  string
		want []string
	}{
		{name: "empty position", get: getNode(nodes), pos: "  ", want: nil},
		{name: "nil getter", get: nil, pos: "pos1", want: nil},
		{name: "node missing → fail closed", get: getNode(nodes), pos: "ghost", want: nil},
		{name: "position + parent department", get: getNode(nodes), pos: "pos1", want: []string{"pos1", "deptA"}},
		{name: "position + parent company → position only", get: getNode(nodes), pos: "pos2", want: []string{"pos2"}},
		{name: "position without parent", get: getNode(nodes), pos: "pos3", want: []string{"pos3"}},
		{name: "dept lead bound at department", get: getNode(nodes), pos: "deptLd", want: []string{"deptLd"}},
		{name: "position trimmed", get: getNode(nodes), pos: " pos1\t", want: []string{"pos1", "deptA"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveKnowledgeOrgScope(context.Background(), tc.get, tc.pos)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

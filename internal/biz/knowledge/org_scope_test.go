package knowledge

import (
	"context"
	"testing"
)

func TestOrgScopeContextRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if ids := OrgScopeFromContext(ctx); len(ids) != 0 {
		t.Fatalf("empty ctx must yield no scope, got %v", ids)
	}
	// 注入规范化：去空白、去重、保序；全空不写入。
	ctx = WithOrgScope(ctx, []string{" dept-a ", "pos-1", "dept-a", "  "})
	got := OrgScopeFromContext(ctx)
	if len(got) != 2 || got[0] != "dept-a" || got[1] != "pos-1" {
		t.Fatalf("scope not normalized: %v", got)
	}
	if ids := OrgScopeFromContext(WithOrgScope(context.Background(), []string{" ", ""})); len(ids) != 0 {
		t.Fatalf("all-blank list must not write key, got %v", ids)
	}
}

func TestOrgScopeIntersects(t *testing.T) {
	t.Parallel()
	if !OrgScopeIntersects([]string{"dept-a", "pos-1"}, []string{"dept-a"}) {
		t.Fatal("overlap must hit")
	}
	if OrgScopeIntersects([]string{"dept-b"}, []string{"dept-a"}) {
		t.Fatal("disjoint sets must miss")
	}
	if OrgScopeIntersects(nil, []string{"dept-a"}) || OrgScopeIntersects([]string{"dept-a"}, nil) {
		t.Fatal("empty side must miss (fail-closed)")
	}
	if !OrgScopeIntersects([]string{" pos-1 "}, []string{"pos-1"}) {
		t.Fatal("whitespace must be tolerated")
	}
}

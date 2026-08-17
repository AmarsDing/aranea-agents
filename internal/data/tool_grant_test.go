package data

import (
	"context"
	"testing"
	"time"

	biztool "aranea-agents/internal/biz/tool"
)

func TestToolGrantRepo_CreateAndHas(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	repo := NewToolGrantRepo(d)
	ctx := context.Background()

	has, err := repo.HasToolGrant(ctx, "agent-1", "bash")
	if err != nil || has {
		t.Fatalf("HasToolGrant before create = (%v,%v), want (false,nil)", has, err)
	}
	if err := repo.CreateToolGrant(ctx, biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash", GrantedBy: "user-1"}); err != nil {
		t.Fatalf("CreateToolGrant: %v", err)
	}
	has, err = repo.HasToolGrant(ctx, "agent-1", "bash")
	if err != nil || !has {
		t.Fatalf("HasToolGrant after create = (%v,%v), want (true,nil)", has, err)
	}
}

func TestToolGrantRepo_CreateIdempotent(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	repo := NewToolGrantRepo(d)
	ctx := context.Background()

	g := biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash", GrantedBy: "user-1"}
	if err := repo.CreateToolGrant(ctx, g); err != nil {
		t.Fatalf("first CreateToolGrant: %v", err)
	}
	// Re-granting the same (agent, tool) must be idempotent, not a
	// unique-constraint error.
	if err := repo.CreateToolGrant(ctx, g); err != nil {
		t.Fatalf("second CreateToolGrant must be idempotent: %v", err)
	}
	grants, err := repo.ListToolGrants(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListToolGrants: %v", err)
	}
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant after idempotent creates, got %d", len(grants))
	}
}

func TestToolGrantRepo_Isolation(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	repo := NewToolGrantRepo(d)
	ctx := context.Background()

	if err := repo.CreateToolGrant(ctx, biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatalf("CreateToolGrant: %v", err)
	}
	for _, tc := range []struct{ agent, tool string }{
		{"agent-2", "bash"},
		{"agent-1", "file_save"},
	} {
		has, err := repo.HasToolGrant(ctx, tc.agent, tc.tool)
		if err != nil || has {
			t.Fatalf("HasToolGrant(%q,%q) = (%v,%v), want (false,nil)", tc.agent, tc.tool, has, err)
		}
	}
}

func TestToolGrantRepo_Delete(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	repo := NewToolGrantRepo(d)
	ctx := context.Background()

	if err := repo.CreateToolGrant(ctx, biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatalf("CreateToolGrant: %v", err)
	}
	if err := repo.DeleteToolGrant(ctx, "agent-1", "bash"); err != nil {
		t.Fatalf("DeleteToolGrant: %v", err)
	}
	has, err := repo.HasToolGrant(ctx, "agent-1", "bash")
	if err != nil || has {
		t.Fatalf("HasToolGrant after delete = (%v,%v), want (false,nil)", has, err)
	}
	// Deleting a non-existent grant must be idempotent.
	if err := repo.DeleteToolGrant(ctx, "agent-1", "bash"); err != nil {
		t.Fatalf("DeleteToolGrant non-existent must be idempotent: %v", err)
	}
}

func TestToolGrantRepo_EmptyKeysRejected(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	repo := NewToolGrantRepo(d)
	ctx := context.Background()

	if err := repo.CreateToolGrant(ctx, biztool.ToolGrant{AgentID: "", ToolKey: "bash"}); err == nil {
		t.Fatal("CreateToolGrant with empty agent_id must fail")
	}
	if err := repo.CreateToolGrant(ctx, biztool.ToolGrant{AgentID: "agent-1", ToolKey: ""}); err == nil {
		t.Fatal("CreateToolGrant with empty tool_key must fail")
	}
	has, err := repo.HasToolGrant(ctx, "", "bash")
	if err != nil || has {
		t.Fatalf("HasToolGrant with empty agent = (%v,%v), want (false,nil)", has, err)
	}
}

// BUG-MON-B: expired rows must be invisible to HasToolGrant, excluded from
// ListToolGrants, and lazily deleted on the list path.
func TestToolGrantRepo_ExpiredGrantFiltered(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	repo := NewToolGrantRepo(d)
	ctx := context.Background()

	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if err := repo.CreateToolGrant(ctx, biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash", GrantedBy: "user-1", ExpiresAt: past}); err != nil {
		t.Fatalf("CreateToolGrant: %v", err)
	}
	has, err := repo.HasToolGrant(ctx, "agent-1", "bash")
	if err != nil || has {
		t.Fatalf("HasToolGrant for expired grant = (%v,%v), want (false,nil)", has, err)
	}
	grants, err := repo.ListToolGrants(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListToolGrants: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("expected 0 live grants, got %d", len(grants))
	}
	client := d.RW().Read(ctx)
	if client == nil {
		t.Fatal("ent client unavailable")
	}
	n, err := client.ToolGrant.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count tool_grants: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected expired row lazily deleted, %d rows remain", n)
	}
}

// BUG-MON-B: re-granting an existing pair must renew expires_at (upsert);
// Ignore() would keep the stale expiry and loop the confirmation prompt.
func TestToolGrantRepo_RegrantRenewsExpiry(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	repo := NewToolGrantRepo(d)
	ctx := context.Background()

	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if err := repo.CreateToolGrant(ctx, biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash", ExpiresAt: past}); err != nil {
		t.Fatalf("CreateToolGrant: %v", err)
	}
	if err := repo.CreateToolGrant(ctx, biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash", ExpiresAt: future}); err != nil {
		t.Fatalf("re-grant: %v", err)
	}
	has, err := repo.HasToolGrant(ctx, "agent-1", "bash")
	if err != nil || !has {
		t.Fatalf("HasToolGrant after renew = (%v,%v), want (true,nil)", has, err)
	}
	grants, err := repo.ListToolGrants(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListToolGrants: %v", err)
	}
	if len(grants) != 1 || grants[0].ExpiresAt != future {
		t.Fatalf("expected single renewed grant with expires_at=%q, got %+v", future, grants)
	}
}

// BUG-MON-B: empty expires_at is the reserved "never expires" form and must
// stay effective (future explicit permanent option).
func TestToolGrantRepo_EmptyExpiresAtNeverExpires(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	repo := NewToolGrantRepo(d)
	ctx := context.Background()

	if err := repo.CreateToolGrant(ctx, biztool.ToolGrant{AgentID: "agent-1", ToolKey: "bash"}); err != nil {
		t.Fatalf("CreateToolGrant: %v", err)
	}
	has, err := repo.HasToolGrant(ctx, "agent-1", "bash")
	if err != nil || !has {
		t.Fatalf("HasToolGrant with empty expires_at = (%v,%v), want (true,nil)", has, err)
	}
}

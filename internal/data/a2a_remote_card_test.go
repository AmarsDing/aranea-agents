package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func setupA2ARawTestRepo(t *testing.T) *a2aRepo {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	d := &Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	if err := EnsureA2ASchema(context.Background(), db, DialectPostgres); err != nil {
		t.Fatalf("ensure a2a schema: %v", err)
	}
	return &a2aRepo{data: d, lg: loggateway.NewNoop()}
}

func TestA2ARepo_UpdateRemoteAgentCard(t *testing.T) {
	repo := setupA2ARawTestRepo(t)
	ctx := context.Background()

	created, err := repo.CreateRemoteAgent(ctx, biz.A2ARemoteAgent{
		Workspace:   "ws-1",
		DisplayName: "Remote A",
		RemoteURL:   "https://a.example.com",
		Enabled:     true,
		OrgID:       "org-a",
		DiscoveredCard: biz.A2AAgentCard{
			AgentID:      "old-card",
			Capabilities: []biz.A2ACapability{{Name: "chat"}},
		},
	})
	if err != nil {
		t.Fatalf("create remote agent: %v", err)
	}

	newCard := biz.A2AAgentCard{
		AgentID:      "new-card",
		DisplayName:  "New Card",
		Capabilities: []biz.A2ACapability{{Name: "chat"}, {Name: "search"}},
	}
	if err := repo.UpdateRemoteAgentCard(ctx, created.ID, newCard); err != nil {
		t.Fatalf("update remote agent card: %v", err)
	}

	got, err := repo.GetRemoteAgent(ctx, created.ID)
	if err != nil {
		t.Fatalf("get remote agent: %v", err)
	}
	if got.DiscoveredCard.AgentID != "new-card" {
		t.Fatalf("card AgentID = %q, want %q", got.DiscoveredCard.AgentID, "new-card")
	}
	if len(got.DiscoveredCard.Capabilities) != 2 {
		t.Fatalf("card capabilities = %d, want 2", len(got.DiscoveredCard.Capabilities))
	}
	// Registry fields preserved: only card_json + updated_at are touched.
	if got.DisplayName != "Remote A" || got.OrgID != "org-a" || !got.Enabled || got.RemoteURL != "https://a.example.com" {
		t.Fatalf("registry fields changed: %+v", got)
	}
}

func TestA2ARepo_UpdateRemoteAgentCardEmptyID(t *testing.T) {
	repo := setupA2ARawTestRepo(t)
	err := repo.UpdateRemoteAgentCard(context.Background(), "  ", biz.A2AAgentCard{})
	if err == nil {
		t.Fatal("expected error for empty id, got nil")
	}
}

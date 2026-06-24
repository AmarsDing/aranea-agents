package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func TestRunTeamCopyOwnershipMigration(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()

	client, _ := testhelper.SetupTestDB(t)
	d := newDataFromClient(client, lg)

	// Create a protected team and a user-made copy that inherited its kind.
	if _, err := client.Team.Create().
		SetID("team-eco").
		SetTeamKey("fox-team-copy-abc123").
		SetDisplayName("Fox Team Copy").
		SetKind("ecosystem_preset").
		SetSource("imported").
		Save(ctx); err != nil {
		t.Fatalf("create copy: %v", err)
	}

	if err := RunTeamCopyOwnershipMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration: %v", err)
	}

	// Re-running should be idempotent.
	if err := RunTeamCopyOwnershipMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration second run: %v", err)
	}

	team, err := client.Team.Get(ctx, "team-eco")
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if team.Kind != "user" {
		t.Errorf("kind = %q, want user", team.Kind)
	}
	if team.Source != "user" {
		t.Errorf("source = %q, want user", team.Source)
	}
}

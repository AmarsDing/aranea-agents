package data

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TestSeedPackBuiltinTemplatesIdempotent verifies that running the builtin pack
// seed multiple times does not create duplicate teams with "-copy-" or " Copy" markers.
func TestSeedPackBuiltinTemplatesIdempotent(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()

	client, _ := testhelper.SetupTestDB(t)

	d := newDataFromClient(client, lg)
	scenarioDir := "../scenario"

	for i := 0; i < 3; i++ {
		if err := SeedPackBuiltinTemplates(ctx, client, d.Dialect(), scenarioDir, lg); err != nil {
			t.Fatalf("startup %d SeedPackBuiltinTemplates: %v", i+1, err)
		}
		if err := SeedPackBuiltinTemplatesV2(ctx, client, d.Dialect(), scenarioDir, lg); err != nil {
			t.Fatalf("startup %d SeedPackBuiltinTemplatesV2: %v", i+1, err)
		}
	}

	rows, err := client.Team.Query().All(ctx)
	if err != nil {
		t.Fatalf("query teams: %v", err)
	}

	copyCount := 0
	keyCount := make(map[string]int)
	for _, team := range rows {
		keyCount[team.TeamKey]++
		if strings.Contains(team.TeamKey, "-copy-") || strings.Contains(team.DisplayName, " Copy") {
			copyCount++
		}
	}
	if copyCount > 0 {
		t.Errorf("found %d teams with copy marker after repeated seeding", copyCount)
		for _, team := range rows {
			t.Logf("team: %s | %s | %s", team.ID, team.TeamKey, team.DisplayName)
		}
	}
	for key, count := range keyCount {
		if count > 1 {
			t.Errorf("team key %q appears %d times", key, count)
		}
	}
}

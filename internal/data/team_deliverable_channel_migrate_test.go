package data

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TestRunTeamDeliverableChannelRepairMigration covers the 2026-08-07 root
// cause: the materialize hook's canonical re-serialization silently dropped
// enable_state_deliverable from persisted definition_json, so DAG teams ran
// without set_deliverable tools and failed the real-deliverable gate.
func TestRunTeamDeliverableChannelRepairMigration(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()

	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, lg)

	mkTeam := func(id, dagNodeID, defJSON string) {
		t.Helper()
		if _, err := client.Team.Create().
			SetID(id).
			SetTeamKey("key-" + id).
			SetDisplayName("Team " + id).
			SetDagNodeID(dagNodeID).
			SetDefinitionJSON(defJSON).
			Save(ctx); err != nil {
			t.Fatalf("create team %s: %v", id, err)
		}
	}

	// A: DAG team missing the flag → must be healed.
	mkTeam("t-dag", "st_1", `{"version":2,"mode":"coordinator","members":[{"agent_id":"a1"},{"agent_id":"a2"}]}`)
	// B: single-member non-DAG team → untouched.
	mkTeam("t-solo", "", `{"version":2,"mode":"sequential","members":[{"agent_id":"a1"}]}`)
	// C: explicit false is a deliberate value → respected, untouched.
	mkTeam("t-explicit", "st_2", `{"version":2,"mode":"coordinator","enable_state_deliverable":false,"members":[{"agent_id":"a1"},{"agent_id":"a2"}]}`)
	// D: multi-member non-DAG team missing the flag → healed (mirrors assembly rule len>1).
	mkTeam("t-multi", "", `{"version":2,"mode":"coordinator","members":[{"agent_id":"a1"},{"agent_id":"a2"},{"agent_id":"a3"}]}`)
	// E: corrupt JSON must not abort the migration.
	mkTeam("t-corrupt", "st_3", `{not-json`)

	if err := RunTeamDeliverableChannelRepairMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration: %v", err)
	}
	// Idempotent re-run.
	if err := RunTeamDeliverableChannelRepairMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration second run: %v", err)
	}

	flagOf := func(id string) (any, bool) {
		t.Helper()
		row, err := client.Team.Get(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(row.DefinitionJSON), &body); err != nil {
			t.Fatalf("unmarshal %s: %v", id, err)
		}
		v, ok := body["enable_state_deliverable"]
		return v, ok
	}

	if v, ok := flagOf("t-dag"); !ok || v != true {
		t.Errorf("t-dag flag = %v (ok=%v), want true", v, ok)
	}
	if _, ok := flagOf("t-solo"); ok {
		t.Error("t-solo must stay untouched")
	}
	if v, ok := flagOf("t-explicit"); !ok || v != false {
		t.Errorf("t-explicit flag = %v (ok=%v), want explicit false preserved", v, ok)
	}
	if v, ok := flagOf("t-multi"); !ok || v != true {
		t.Errorf("t-multi flag = %v (ok=%v), want true", v, ok)
	}
}

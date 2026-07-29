package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// itOpsAgentKeys 列出 it-ops-pack 定义的全部 12 个岗位 Agent key。
var itOpsAgentKeys = []string{
	"alert_handler__general",
	"incident_commander__general",
	"fault_diagnostician__general",
	"log_analyst__general",
	"metric_analyst__general",
	"change_executor__general",
	"runbook_engineer__general",
	"system_inspector__general",
	"network_inspector__general",
	"db_operator__general",
	"compliance_checker__general",
	"postmortem_writer__general",
}

// TestSeedPackItOpsVersionUniqueness guards against seed version colliding with
// DDL/data migration versions that share the schema_migrations table.
// isMigrationApplied matches by version ID only — a collision makes the seed
// silently skip in production while passing in tests (test PG does not run the
// DDL migration registry).
func TestSeedPackItOpsVersionUniqueness(t *testing.T) {
	if SeedPackItOpsV1 <= MigrationMonitorTraceInterruptedBackfill {
		t.Errorf("SeedPackItOpsV1=%d must be greater than the latest DDL migration version %d (schema_migrations versions are globally unique)",
			SeedPackItOpsV1, MigrationMonitorTraceInterruptedBackfill)
	}
	if SeedPackItOpsV1 <= SeedPackAgencyV1 {
		t.Errorf("SeedPackItOpsV1=%d must be greater than SeedPackAgencyV1=%d",
			SeedPackItOpsV1, SeedPackAgencyV1)
	}
}

// TestSeedPackItOpsIdempotent verifies that running the it-ops pack seed
// multiple times does not create duplicate agents or organization nodes.
func TestSeedPackItOpsIdempotent(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()

	client, _ := testhelper.SetupTestPG(t)

	d := newDataFromClient(client, lg)
	scenarioDir := "../scenario"

	for i := 0; i < 3; i++ {
		if err := SeedPackItOps(ctx, client, d.Dialect(), scenarioDir, lg); err != nil {
			t.Fatalf("startup %d SeedPackItOps: %v", i+1, err)
		}
	}

	// 1. 12 个岗位 Agent 全部存在且不重复
	agents, err := client.Agent.Query().All(ctx)
	if err != nil {
		t.Fatalf("query agents: %v", err)
	}
	keyCount := make(map[string]int)
	for _, a := range agents {
		keyCount[a.AgentKey]++
	}
	for _, key := range itOpsAgentKeys {
		if keyCount[key] == 0 {
			t.Errorf("it-ops agent %q not found after seeding", key)
		}
		if keyCount[key] > 1 {
			t.Errorf("it-ops agent %q appears %d times after repeated seeding", key, keyCount[key])
		}
	}

	// 2. 组织节点（1 公司 / 5 部门 / 12 岗位）存在且不重复
	orgs, err := client.Organization.Query().All(ctx)
	if err != nil {
		t.Fatalf("query organizations: %v", err)
	}
	type orgKey struct {
		level string
		key   string
	}
	orgCount := make(map[orgKey]int)
	for _, o := range orgs {
		orgCount[orgKey{o.Level, o.OrgKey}]++
	}

	wantOrgs := []orgKey{
		{"company", "it_ops"},
		{"department", "alert_response"},
		{"department", "diagnostics"},
		{"department", "execution"},
		{"department", "inspection"},
		{"department", "docs"},
		{"position", "alert_handler"},
		{"position", "incident_commander"},
		{"position", "fault_diagnostician"},
		{"position", "log_analyst"},
		{"position", "metric_analyst"},
		{"position", "change_executor"},
		{"position", "runbook_engineer"},
		{"position", "system_inspector"},
		{"position", "network_inspector"},
		{"position", "db_operator"},
		{"position", "compliance_checker"},
		{"position", "postmortem_writer"},
	}
	for _, w := range wantOrgs {
		if orgCount[w] == 0 {
			t.Errorf("organization node %s/%s not found after seeding", w.level, w.key)
		}
		if orgCount[w] > 1 {
			t.Errorf("organization node %s/%s appears %d times after repeated seeding", w.level, w.key, orgCount[w])
		}
	}

	// 3. 每个 Agent 的提示词文件（IDENTITY/MISSION/RULE）已导入
	files, err := client.AgentPromptFile.Query().All(ctx)
	if err != nil {
		t.Fatalf("query agent prompt files: %v", err)
	}
	fileCount := make(map[string]int) // agent_key → file count（通过 agent 反查）
	agentIDToKey := make(map[string]string)
	for _, a := range agents {
		agentIDToKey[a.ID] = a.AgentKey
	}
	for _, f := range files {
		if key, ok := agentIDToKey[f.AgentID]; ok {
			fileCount[key]++
		}
	}
	for _, key := range itOpsAgentKeys {
		if fileCount[key] < 3 {
			t.Errorf("it-ops agent %q has %d prompt files, want >= 3 (IDENTITY/MISSION/RULE)", key, fileCount[key])
		}
	}
}

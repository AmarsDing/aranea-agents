package biz_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/internal/team"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// B10 存量迁移 PG 集成测试（验收：迁移幂等 + preset/custom 双态判定）
// 真实 PG 隔离 schema + 真实 TeamRepo/GraphRepo + 真实 TeamCompilerAdapter +
// 真实 GraphDefinitionUsecase（资产存储）+ 真实 ExecInTx（D1 同事务）。
// JSON 构造助手复用 team_graph_migrate_test.go（同包）。
// ---------------------------------------------------------------------------

type migPGEnv struct {
	teams  *data.TeamRepo
	graphs biz.GraphRepo
	uc     *biz.TeamUsecase
}

func newMigPGEnv(t *testing.T) *migPGEnv {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	d := &data.Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	teams := data.NewTeamRepo(d)
	graphs := data.NewGraphRepo(d)
	defUC := biz.NewGraphDefinitionUsecase(graphs, nil, nil, loggateway.NewNoop())
	compiler := team.NewTeamCompilerAdapter(nil, nil, loggateway.NewNoop())
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:      teams,
		Writer:      teams,
		RunReader:   teams,
		Compiler:    compiler,
		GraphAssets: defUC,
		TxProvider:  d, // D1：物化 + 回写同一事务
		Lg:          loggateway.NewNoop(),
	})
	return &migPGEnv{teams: teams, graphs: graphs, uc: uc}
}

func seedLegacyTeam(t *testing.T, env *migPGEnv, id, key, raw, linkedGraphID string) {
	t.Helper()
	if _, err := env.teams.CreateTeam(context.Background(), biz.Team{
		ID:             id,
		TeamKey:        key,
		DisplayName:    "Legacy " + key,
		DefinitionJSON: raw,
		LinkedGraphID:  linkedGraphID,
	}); err != nil {
		t.Fatalf("seed team %s: %v", id, err)
	}
}

// TestMigrateLegacyEmbeddedGraphs_PG_EndToEnd 覆盖：
//  1. preset 等价 team → source=preset 物化回写（embedded 退役）
//  2. custom 拓扑 team → source=custom 物化（保留用户拓扑节点）
//  3. 已有 linked 的 team 跳过（即使 definition_json 仍含 graph）
//  4. 二次运行幂等（migrated=0）
//  5. 无 embedded graph 的 team 批迁移跳过，运行时惰性兜底物化
func TestMigrateLegacyEmbeddedGraphs_PG_EndToEnd(t *testing.T) {
	env := newMigPGEnv(t)
	ctx := context.Background()

	// Team A：preset 等价 embedded graph（与 mode 模板同生成器产出）
	rawNoGraph := migSpecJSON(t, nil)
	presetGraph := canonicalEmbeddedGraph(t, rawNoGraph)
	rawA := migSpecJSON(t, func(s *biz.OrchestrationSpec) { s.Graph = presetGraph })
	seedLegacyTeam(t, env, "team-pg-a", "pg_a", rawA, "")

	// Team B：custom 拓扑（手改：额外 agent 节点接入边）
	customGraph := canonicalEmbeddedGraph(t, rawNoGraph)
	customGraph.Nodes = append(customGraph.Nodes, biz.EmbeddedGraphNodeSpec{
		ID: "a3", Type: "agent", Label: "A3", AgentID: "a3", Role: "worker",
	})
	for i := range customGraph.Edges {
		if customGraph.Edges[i].Target == "end" {
			customGraph.Edges[i].Target = "a3"
		}
	}
	customGraph.Edges = append(customGraph.Edges, biz.EmbeddedGraphEdgeSpec{ID: "e-a3-end", Source: "a3", Target: "end"})
	rawB := migSpecJSON(t, func(s *biz.OrchestrationSpec) { s.Graph = customGraph })
	seedLegacyTeam(t, env, "team-pg-b", "pg_b", rawB, "")

	// Team C：无 embedded graph（批迁移跳过，留给惰性兜底）
	seedLegacyTeam(t, env, "team-pg-c", "pg_c", rawNoGraph, "")

	// Team D：列值已 linked（即使 definition_json 仍含 graph 也必须跳过）
	rawD := migSpecJSON(t, func(s *biz.OrchestrationSpec) { s.Graph = presetGraph })
	seedLegacyTeam(t, env, "team-pg-d", "pg_d", rawD, "g-preexisting")

	migrated, skipped, failed := env.uc.MigrateLegacyEmbeddedGraphs(ctx)
	if migrated != 2 || failed != 0 {
		t.Fatalf("migrated=%d skipped=%d failed=%d, want 2/≥2/0", migrated, skipped, failed)
	}
	if skipped < 2 {
		t.Fatalf("skipped=%d, want ≥2 (team-pg-c 无 graph + team-pg-d 已 linked)", skipped)
	}

	// Team A：preset 物化回写
	teamA, err := env.teams.GetTeamByID(ctx, "team-pg-a")
	if err != nil {
		t.Fatal(err)
	}
	if teamA.LinkedGraphID == "" {
		t.Fatal("team-pg-a linked_graph_id not backfilled")
	}
	specA := parseSpec(t, teamA.DefinitionJSON)
	if specA.Graph != nil {
		t.Fatal("team-pg-a embedded graph not retired (D.1)")
	}
	if specA.GraphSource() != biz.DefinitionGraphSourcePreset {
		t.Fatalf("team-pg-a source=%q, want preset", specA.GraphSource())
	}
	if specA.LinkedGraphID != teamA.LinkedGraphID {
		t.Fatalf("team-pg-a spec linked=%q ≠ column %q", specA.LinkedGraphID, teamA.LinkedGraphID)
	}
	assetA, err := env.graphs.GetDefinition(ctx, teamA.LinkedGraphID)
	if err != nil {
		t.Fatalf("team-pg-a asset: %v", err)
	}
	if assetA.TeamID != "team-pg-a" {
		t.Fatalf("asset team_id=%q, want team-pg-a", assetA.TeamID)
	}
	if assetA.Metadata[biz.GraphMetadataTeamOwnedKey] != true {
		t.Fatalf("asset metadata owned=%v, want true", assetA.Metadata[biz.GraphMetadataTeamOwnedKey])
	}

	// Team B：custom 物化（保留用户拓扑节点 a3）
	teamB, err := env.teams.GetTeamByID(ctx, "team-pg-b")
	if err != nil {
		t.Fatal(err)
	}
	specB := parseSpec(t, teamB.DefinitionJSON)
	if specB.GraphSource() != biz.DefinitionGraphSourceCustom {
		t.Fatalf("team-pg-b source=%q, want custom", specB.GraphSource())
	}
	assetB, err := env.graphs.GetDefinition(ctx, teamB.LinkedGraphID)
	if err != nil {
		t.Fatalf("team-pg-b asset: %v", err)
	}
	var hasA3 bool
	for _, n := range assetB.Nodes {
		if n.ID == "a3" {
			hasA3 = true
		}
	}
	if !hasA3 {
		t.Fatal("team-pg-b custom topology (node a3) not preserved in asset")
	}

	// Team D：已 linked 跳过，definition_json 原样（未回写）
	teamD, err := env.teams.GetTeamByID(ctx, "team-pg-d")
	if err != nil {
		t.Fatal(err)
	}
	if teamD.LinkedGraphID != "g-preexisting" {
		t.Fatalf("team-pg-d linked=%q, want unchanged g-preexisting", teamD.LinkedGraphID)
	}
	if parseSpec(t, teamD.DefinitionJSON).Graph == nil {
		t.Fatal("team-pg-d must be skipped: embedded graph unexpectedly retired")
	}

	// 幂等：二次运行不再迁移
	migrated2, _, failed2 := env.uc.MigrateLegacyEmbeddedGraphs(ctx)
	if migrated2 != 0 || failed2 != 0 {
		t.Fatalf("second run migrated=%d failed=%d, want 0/0 (idempotent)", migrated2, failed2)
	}

	// Team C：运行时惰性兜底物化
	teamC, err := env.uc.EnsureTeamGraphAsset(ctx, "team-pg-c")
	if err != nil {
		t.Fatalf("EnsureTeamGraphAsset: %v", err)
	}
	if teamC.LinkedGraphID == "" {
		t.Fatal("team-pg-c lazy materialization did not backfill linked_graph_id")
	}
	specC := parseSpec(t, teamC.DefinitionJSON)
	if specC.GraphSource() != biz.DefinitionGraphSourcePreset {
		t.Fatalf("team-pg-c source=%q, want preset (form-derived)", specC.GraphSource())
	}
	if _, err := env.graphs.GetDefinition(ctx, teamC.LinkedGraphID); err != nil {
		t.Fatalf("team-pg-c asset: %v", err)
	}
	// 惰性兜底幂等：重复调用返回已链接 team
	teamC2, err := env.uc.EnsureTeamGraphAsset(ctx, "team-pg-c")
	if err != nil {
		t.Fatal(err)
	}
	if teamC2.LinkedGraphID != teamC.LinkedGraphID {
		t.Fatalf("lazy re-ensure linked=%q, want stable %q", teamC2.LinkedGraphID, teamC.LinkedGraphID)
	}
}

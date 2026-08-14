package biz_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// fakes（B10 存量迁移测试专用；复用真实 TeamCompilerAdapter 保证判定真实性）
// ---------------------------------------------------------------------------

type migTeamRepo struct {
	teams map[string]biz.Team
}

func (r *migTeamRepo) ListTeams(context.Context) ([]biz.Team, error) {
	out := make([]biz.Team, 0, len(r.teams))
	for _, t := range r.teams {
		out = append(out, t)
	}
	return out, nil
}
func (r *migTeamRepo) ListTeamsByStatus(context.Context, string) ([]biz.Team, error) {
	return nil, nil
}
func (r *migTeamRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	t, ok := r.teams[id]
	if !ok {
		return biz.Team{}, fmt.Errorf("team %s not found", id)
	}
	return t, nil
}
func (r *migTeamRepo) GetTeamByKey(context.Context, string) (biz.Team, error) {
	return biz.Team{}, fmt.Errorf("not implemented")
}
func (r *migTeamRepo) ListBySpiritSessionID(context.Context, string) ([]biz.Team, error) {
	return nil, nil
}
func (r *migTeamRepo) ListTeamsByDepartmentID(context.Context, string) ([]biz.Team, error) {
	return nil, nil
}
func (r *migTeamRepo) ListTeamsByWorkspace(context.Context, string) ([]biz.Team, error) {
	return nil, nil
}
func (r *migTeamRepo) CountTeamsByWorkspace(context.Context, string) (int, error) { return 0, nil }
func (r *migTeamRepo) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	r.teams[t.ID] = t
	return t, nil
}
func (r *migTeamRepo) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	r.teams[t.ID] = t
	return t, nil
}
func (r *migTeamRepo) DeleteTeam(context.Context, string) error { return nil }
func (r *migTeamRepo) BatchArchiveTeams(context.Context, []string) (int, error) {
	return 0, nil
}
func (r *migTeamRepo) UpdateTeamWhereStatus(context.Context, string, string, string) (bool, error) {
	return true, nil
}

type migAssetStore struct {
	seq     int
	created map[string]*biz.GraphDefinition
}

func (s *migAssetStore) CreateOwnedGraph(_ context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	s.seq++
	if def.ID == "" {
		def.ID = fmt.Sprintf("g-mig-%d", s.seq)
	}
	if s.created == nil {
		s.created = map[string]*biz.GraphDefinition{}
	}
	s.created[def.ID] = def
	return def, nil
}
func (s *migAssetStore) UpdateOwnedGraph(_ context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	s.created[def.ID] = def
	return def, nil
}
func (s *migAssetStore) DeleteOwnedGraph(_ context.Context, id string) error {
	delete(s.created, id)
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func migSpecJSON(t *testing.T, mutate func(*biz.OrchestrationSpec)) string {
	t.Helper()
	spec := biz.DefaultOrchestrationSpec()
	spec.Mode = "sequential"
	spec.Members = []biz.OrchestrationMember{
		{AgentID: "a1", Role: "worker", Name: "A1", SortOrder: 1},
		{AgentID: "a2", Role: "worker", Name: "A2", SortOrder: 2},
	}
	if mutate != nil {
		mutate(&spec)
	}
	raw, err := biz.OrchestrationSpecToDefinitionJSON(spec)
	if err != nil {
		t.Fatalf("spec json: %v", err)
	}
	return raw
}

// canonicalEmbeddedGraph 用模板生成器产出「preset 等价」的 embedded graph：
// 与表单派生编译走同一生成器，拓扑必然等价。
func canonicalEmbeddedGraph(t *testing.T, rawNoGraph string) *biz.EmbeddedGraphSpec {
	t.Helper()
	def := team.Definition{
		Mode: "sequential",
		Members: []team.MemberDef{
			{AgentID: "a1", Role: "worker"},
			{AgentID: "a2", Role: "worker"},
		},
	}
	genJSON := team.DefinitionGraphSpecJSON(context.Background(), def, rawNoGraph, loggateway.NewNoop())
	if genJSON == "" {
		t.Fatal("DefinitionGraphSpecJSON returned empty")
	}
	var spec biz.EmbeddedGraphSpec
	if err := json.Unmarshal([]byte(genJSON), &spec); err != nil {
		t.Fatalf("unmarshal generated graph: %v", err)
	}
	return &spec
}

func newMigUsecase(repo *migTeamRepo, store *migAssetStore) *biz.TeamUsecase {
	compiler := team.NewTeamCompilerAdapter(nil, nil, loggateway.NewNoop())
	return biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:      repo,
		Writer:      repo,
		Compiler:    compiler,
		GraphAssets: store,
		Lg:          loggateway.NewNoop(),
	})
}

func parseSpec(t *testing.T, raw string) biz.OrchestrationSpec {
	t.Helper()
	spec, err := biz.ParseOrchestrationSpec(raw)
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	return spec
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestMigrateLegacyEmbeddedGraphs_PresetEquivalent(t *testing.T) {
	rawNoGraph := migSpecJSON(t, nil)
	embedded := canonicalEmbeddedGraph(t, rawNoGraph)
	raw := migSpecJSON(t, func(s *biz.OrchestrationSpec) { s.Graph = embedded })

	repo := &migTeamRepo{teams: map[string]biz.Team{
		"team-1": {ID: "team-1", TeamKey: "t1", DisplayName: "Team One", Kind: "user", DefinitionJSON: raw},
	}}
	store := &migAssetStore{}
	uc := newMigUsecase(repo, store)

	migrated, skipped, failed := uc.MigrateLegacyEmbeddedGraphs(context.Background())
	if migrated != 1 || failed != 0 || skipped != 0 {
		t.Fatalf("migrated=%d skipped=%d failed=%d", migrated, skipped, failed)
	}
	got := repo.teams["team-1"]
	if got.LinkedGraphID == "" {
		t.Fatal("linked_graph_id not written back")
	}
	spec := parseSpec(t, got.DefinitionJSON)
	if spec.Graph != nil {
		t.Fatal("embedded graph should be removed after migration")
	}
	if spec.GraphSource() != biz.DefinitionGraphSourcePreset {
		t.Fatalf("source = %q, want preset", spec.GraphSource())
	}
	if spec.LinkedGraphID != got.LinkedGraphID {
		t.Fatalf("spec linked %q != column %q", spec.LinkedGraphID, got.LinkedGraphID)
	}
	asset := store.created[got.LinkedGraphID]
	if asset == nil {
		t.Fatal("graph asset not created")
	}
	if asset.TeamID != "team-1" {
		t.Fatalf("asset team_id = %q", asset.TeamID)
	}
	if got := asset.Metadata[biz.GraphMetadataTeamSourceKey]; got != biz.DefinitionGraphSourcePreset {
		t.Fatalf("asset team_source = %v", got)
	}
	if owned, _ := asset.Metadata[biz.GraphMetadataTeamOwnedKey].(bool); !owned {
		t.Fatal("asset team_owned marker missing")
	}
}

func TestMigrateLegacyEmbeddedGraphs_CustomWhenTopologyDiffers(t *testing.T) {
	rawNoGraph := migSpecJSON(t, nil)
	embedded := canonicalEmbeddedGraph(t, rawNoGraph)
	// 用户手改：插入一个额外 agent 节点并接到边上。
	embedded.Nodes = append(embedded.Nodes, biz.EmbeddedGraphNodeSpec{
		ID: "a3", Type: "agent", Label: "A3", AgentID: "a3", Role: "worker",
	})
	for i := range embedded.Edges {
		if embedded.Edges[i].Target == "end" {
			embedded.Edges[i].Target = "a3"
		}
	}
	embedded.Edges = append(embedded.Edges, biz.EmbeddedGraphEdgeSpec{ID: "e-a3-end", Source: "a3", Target: "end"})
	raw := migSpecJSON(t, func(s *biz.OrchestrationSpec) { s.Graph = embedded })

	repo := &migTeamRepo{teams: map[string]biz.Team{
		"team-1": {ID: "team-1", TeamKey: "t1", DisplayName: "Team One", Kind: "user", DefinitionJSON: raw},
	}}
	store := &migAssetStore{}
	uc := newMigUsecase(repo, store)

	migrated, _, failed := uc.MigrateLegacyEmbeddedGraphs(context.Background())
	if migrated != 1 || failed != 0 {
		t.Fatalf("migrated=%d failed=%d", migrated, failed)
	}
	got := repo.teams["team-1"]
	spec := parseSpec(t, got.DefinitionJSON)
	if spec.GraphSource() != biz.DefinitionGraphSourceCustom {
		t.Fatalf("source = %q, want custom", spec.GraphSource())
	}
	asset := store.created[got.LinkedGraphID]
	if asset == nil {
		t.Fatal("graph asset not created")
	}
	var hasA3 bool
	for _, n := range asset.Nodes {
		if n.ID == "a3" {
			hasA3 = true
		}
	}
	if !hasA3 {
		t.Fatal("custom topology (node a3) not preserved in asset")
	}
	if got := asset.Metadata[biz.GraphMetadataTeamSourceKey]; got != biz.DefinitionGraphSourceCustom {
		t.Fatalf("asset team_source = %v", got)
	}
}

func TestMigrateLegacyEmbeddedGraphs_IdempotentAndContinueOnError(t *testing.T) {
	rawNoGraph := migSpecJSON(t, nil)
	embedded := canonicalEmbeddedGraph(t, rawNoGraph)
	rawOK := migSpecJSON(t, func(s *biz.OrchestrationSpec) { s.Graph = embedded })
	// 坏 team：成员全禁用（表单编译失败）+ embedded 含未知节点类型（embedded
	// 编译失败）→ 双路失败，记入 failed（C-22 语义）。
	rawBad := migSpecJSON(t, func(s *biz.OrchestrationSpec) {
		disabled := false
		s.Members = []biz.OrchestrationMember{{AgentID: "a1", Role: "worker", Name: "A1", SortOrder: 1, EnabledPtr: &disabled}}
		s.Graph = &biz.EmbeddedGraphSpec{
			Version: 1,
			Nodes: []biz.EmbeddedGraphNodeSpec{
				{ID: "start", Type: "start"},
				{ID: "member-1", Type: "agent", AgentID: "a1"},
				{ID: "weird", Type: "magic_box"},
				{ID: "end", Type: "end"},
			},
			Edges: []biz.EmbeddedGraphEdgeSpec{
				{ID: "e1", Source: "start", Target: "member-1"},
				{ID: "e2", Source: "member-1", Target: "end"},
			},
		}
	})

	repo := &migTeamRepo{teams: map[string]biz.Team{
		"team-ok":  {ID: "team-ok", TeamKey: "tok", DisplayName: "OK", Kind: "user", DefinitionJSON: rawOK},
		"team-bad": {ID: "team-bad", TeamKey: "tbad", DisplayName: "Bad", Kind: "user", DefinitionJSON: rawBad},
	}}
	store := &migAssetStore{}
	uc := newMigUsecase(repo, store)

	migrated, _, failed := uc.MigrateLegacyEmbeddedGraphs(context.Background())
	if migrated != 1 || failed != 1 {
		t.Fatalf("first run: migrated=%d failed=%d, want 1/1", migrated, failed)
	}
	// 第二次运行：已迁移的跳过（幂等），坏 team 依旧失败但不重复建资产。
	migrated2, _, failed2 := uc.MigrateLegacyEmbeddedGraphs(context.Background())
	if migrated2 != 0 || failed2 != 1 {
		t.Fatalf("second run: migrated=%d failed=%d, want 0/1", migrated2, failed2)
	}
	if len(store.created) != 1 {
		t.Fatalf("asset count = %d, want 1", len(store.created))
	}
}

func TestEnsureTeamGraphAsset_LazyMaterialize(t *testing.T) {
	// 情形 1：无 embedded graph 的存量 team —— 运行时惰性物化为 preset。
	rawNoGraph := migSpecJSON(t, nil)
	repo := &migTeamRepo{teams: map[string]biz.Team{
		"team-1": {ID: "team-1", TeamKey: "t1", DisplayName: "Team One", Kind: "user", DefinitionJSON: rawNoGraph},
	}}
	store := &migAssetStore{}
	uc := newMigUsecase(repo, store)

	got, err := uc.EnsureTeamGraphAsset(context.Background(), "team-1")
	if err != nil {
		t.Fatalf("EnsureTeamGraphAsset: %v", err)
	}
	if got.LinkedGraphID == "" {
		t.Fatal("lazy materialize did not write linked_graph_id")
	}
	spec := parseSpec(t, got.DefinitionJSON)
	if spec.GraphSource() != biz.DefinitionGraphSourcePreset {
		t.Fatalf("source = %q, want preset", spec.GraphSource())
	}
	if store.created[got.LinkedGraphID] == nil {
		t.Fatal("asset not created on lazy path")
	}
	// 持久化到 repo。
	if repo.teams["team-1"].LinkedGraphID != got.LinkedGraphID {
		t.Fatal("lazy materialize not persisted to repo")
	}

	// 情形 2：已链接 team —— 原样返回，不新建资产。
	linkedRaw := migSpecJSON(t, func(s *biz.OrchestrationSpec) {
		s.LinkedGraphID = "g-existing"
		s.Source = biz.DefinitionGraphSourceLinkedExt
	})
	repo.teams["team-2"] = biz.Team{ID: "team-2", TeamKey: "t2", DisplayName: "Team Two", Kind: "user",
		LinkedGraphID: "g-existing", DefinitionJSON: linkedRaw}
	before := len(store.created)
	got2, err := uc.EnsureTeamGraphAsset(context.Background(), "team-2")
	if err != nil {
		t.Fatalf("EnsureTeamGraphAsset linked: %v", err)
	}
	if got2.LinkedGraphID != "g-existing" || len(store.created) != before {
		t.Fatalf("linked team should be no-op, got linked=%q assets=%d", got2.LinkedGraphID, len(store.created))
	}
}

func TestMigrateLegacyEmbeddedGraphs_SkipsNonLegacyTeams(t *testing.T) {
	// 已链接 team 与无 embedded graph team 都应跳过。
	linkedRaw := migSpecJSON(t, func(s *biz.OrchestrationSpec) { s.LinkedGraphID = "g-1" })
	noGraphRaw := migSpecJSON(t, nil)
	repo := &migTeamRepo{teams: map[string]biz.Team{
		"team-linked":  {ID: "team-linked", TeamKey: "tl", DisplayName: "L", Kind: "user", LinkedGraphID: "g-1", DefinitionJSON: linkedRaw},
		"team-nograph": {ID: "team-nograph", TeamKey: "tn", DisplayName: "N", Kind: "user", DefinitionJSON: noGraphRaw},
		"team-broken":  {ID: "team-broken", TeamKey: "tb", DisplayName: "B", Kind: "user", DefinitionJSON: "{not-json"},
	}}
	store := &migAssetStore{}
	uc := newMigUsecase(repo, store)

	migrated, skipped, failed := uc.MigrateLegacyEmbeddedGraphs(context.Background())
	if migrated != 0 || failed != 0 || skipped != 3 {
		t.Fatalf("migrated=%d skipped=%d failed=%d, want 0/3/0", migrated, skipped, failed)
	}
	if len(store.created) != 0 {
		t.Fatal("no asset should be created")
	}
	// 跳过路径不得改动 definition_json。
	if repo.teams["team-nograph"].DefinitionJSON != noGraphRaw {
		t.Fatal("no-graph team definition_json should be untouched")
	}
	if !strings.Contains(repo.teams["team-broken"].DefinitionJSON, "not-json") {
		t.Fatal("broken team should be untouched")
	}
}

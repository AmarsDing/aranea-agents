package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// --- fakes for Team × Graph save-hook tests（B3/B4/B5/B8） ---

type recTeamWriter struct {
	created   *Team
	updated   *Team
	deletedID string
}

func (s *recTeamWriter) CreateTeam(_ context.Context, t Team) (Team, error) {
	s.created = &t
	return t, nil
}
func (s *recTeamWriter) UpdateTeam(_ context.Context, t Team) (Team, error) {
	s.updated = &t
	return t, nil
}
func (s *recTeamWriter) DeleteTeam(_ context.Context, id string) error {
	s.deletedID = id
	return nil
}
func (s *recTeamWriter) BatchArchiveTeams(_ context.Context, ids []string) (int, error) {
	return len(ids), nil
}
func (s *recTeamWriter) UpdateTeamWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

type fakeGraphStore struct {
	byID            map[string]*GraphDefinition
	nextID          int
	created         []*GraphDefinition
	updated         []*GraphDefinition
	deleted         []string
	failDeleteOwned error // Y11 测试：注入级联删失败
}

func newFakeGraphStore() *fakeGraphStore {
	return &fakeGraphStore{byID: map[string]*GraphDefinition{}}
}

func (s *fakeGraphStore) addExisting(def *GraphDefinition) {
	s.byID[def.ID] = def
}

// TeamGraphAssetStore
func (s *fakeGraphStore) CreateOwnedGraph(_ context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	s.nextID++
	if def.ID == "" {
		def.ID = fmt.Sprintf("g-%d", s.nextID)
	}
	s.byID[def.ID] = def
	s.created = append(s.created, def)
	return def, nil
}
func (s *fakeGraphStore) UpdateOwnedGraph(_ context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	if _, ok := s.byID[def.ID]; !ok {
		return nil, ErrNotFound
	}
	def.Version++
	s.byID[def.ID] = def
	s.updated = append(s.updated, def)
	return def, nil
}
func (s *fakeGraphStore) DeleteOwnedGraph(_ context.Context, id string) error {
	if s.failDeleteOwned != nil {
		return s.failDeleteOwned
	}
	s.deleted = append(s.deleted, id)
	delete(s.byID, id)
	return nil
}

// GraphReader
func (s *fakeGraphStore) GetDefinition(_ context.Context, id string) (*GraphDefinition, error) {
	if d, ok := s.byID[id]; ok {
		return d, nil
	}
	return nil, ErrNotFound
}
func (s *fakeGraphStore) GetDefinitionByName(context.Context, string) (*GraphDefinition, error) {
	return nil, ErrNotFound
}
func (s *fakeGraphStore) ListDefinitions(context.Context, int, string) ([]*GraphDefinition, string, error) {
	return nil, "", nil
}
func (s *fakeGraphStore) ListUserTemplateDefinitions(context.Context, int) ([]*GraphDefinition, error) {
	return nil, nil
}
func (s *fakeGraphStore) ListDefinitionsByWorkspace(context.Context, int, string, string) ([]*GraphDefinition, string, error) {
	return nil, "", nil
}
func (s *fakeGraphStore) ListUserTemplateDefinitionsByWorkspace(context.Context, int, string) ([]*GraphDefinition, error) {
	return nil, nil
}

// GraphWriter（syncGraphTeamID / Delete unbind 路径）
func (s *fakeGraphStore) SaveDefinition(_ context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	return s.CreateOwnedGraph(context.Background(), def)
}
func (s *fakeGraphStore) UpdateDefinition(_ context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	s.byID[def.ID] = def
	return def, nil
}
func (s *fakeGraphStore) DeleteDefinition(_ context.Context, id string) error {
	return s.DeleteOwnedGraph(context.Background(), id)
}
func (s *fakeGraphStore) ReorderGraphs(context.Context, []string) error { return nil }

type fakeTeamCompiler struct {
	cfg GraphBuildConfig
	err error
}

func (f *fakeTeamCompiler) Compile(context.Context, string) (GraphBuildConfig, error) {
	return f.cfg, f.err
}
func (f *fakeTeamCompiler) CompileFromDefinition(TeamDefinition, func(string) string) (GraphBuildConfig, error) {
	return f.cfg, f.err
}

type fakeTxProvider struct{ calls int }

func (f *fakeTxProvider) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	f.calls++
	return fn(ctx)
}

// --- helpers ---

func hookSpecJSON(t *testing.T, mutate func(*OrchestrationSpec)) string {
	t.Helper()
	spec := DefaultOrchestrationSpec()
	spec.Members = []OrchestrationMember{{AgentID: "a1", Role: RoleWorker, Name: "A1", SortOrder: 1}}
	if mutate != nil {
		mutate(&spec)
	}
	raw, err := OrchestrationSpecToDefinitionJSON(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	return raw
}

func ownedGraphDef(id, teamID string) *GraphDefinition {
	return &GraphDefinition{
		ID:      id,
		TeamID:  teamID,
		Version: 1,
		Name:    "owned",
		Metadata: map[string]any{
			GraphMetadataTeamOwnedKey:  true,
			GraphMetadataTeamSourceKey: DefinitionGraphSourcePreset,
		},
	}
}

func newHookUsecase(reader TeamReader, writer TeamWriter, store *fakeGraphStore, compiler TeamCompiler, tx TeamTxProvider) *TeamUsecase {
	return NewTeamUsecase(TeamUsecaseOpts{
		Reader:      reader,
		Writer:      writer,
		RunReader:   &stubTeamRunReader{},
		RunWriter:   &stubTeamRunWriter{},
		StepRepo:    &stubOrchestrationStepRepo{},
		DeadLetter:  &stubTaskDeadLetterRepo{},
		GraphReader: store,
		GraphWriter: store,
		Compiler:    compiler,
		GraphAssets: store,
		TxProvider:  tx,
		Lg:          loggateway.NewNoop(),
	})
}

func simpleBuildConfig() GraphBuildConfig {
	return GraphBuildConfig{
		Nodes: []NodeDef{
			{ID: "start", Type: "start"},
			{ID: "member_1", Type: "agent", AgentName: "a1"},
			{ID: "end", Type: "end"},
		},
		Edges:       []EdgeDef{{From: "start", To: "member_1"}, {From: "member_1", To: "end"}},
		EntryPoint:  "start",
		FinishPoint: "end",
	}
}

func definitionAttrs(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal definition: %v", err)
	}
	return m
}

// --- B3：Create 物化 ---

func TestTeamGraphHook_CreateMaterializesPresetAsset(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	writer := &recTeamWriter{}
	uc := newHookUsecase(&stubTeamReader{}, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, &fakeTxProvider{})

	created, err := uc.Create(context.Background(), Team{
		TeamKey:     "t1",
		DisplayName: "Team One",
		Kind:        "user",
		DefinitionJSON: hookSpecJSON(t, func(s *OrchestrationSpec) {
			s.EnableCheckpoint = true
		}),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 created graph asset, got %d", len(store.created))
	}
	asset := store.created[0]
	if asset.TeamID != created.ID {
		t.Fatalf("asset.TeamID = %q, want team id %q", asset.TeamID, created.ID)
	}
	if !isTeamOwnedGraph(asset) {
		t.Fatal("asset missing team_owned metadata")
	}
	if asset.Metadata[GraphMetadataTeamSourceKey] != DefinitionGraphSourcePreset {
		t.Fatalf("team_source = %v", asset.Metadata[GraphMetadataTeamSourceKey])
	}
	if !asset.EnableCheckpoint {
		t.Fatal("asset.EnableCheckpoint = false, want true (from spec)")
	}
	attrs := definitionAttrs(t, created.DefinitionJSON)
	if attrs["linked_graph_id"] != asset.ID {
		t.Fatalf("linked_graph_id = %v, want %v", attrs["linked_graph_id"], asset.ID)
	}
	if attrs["source"] != DefinitionGraphSourcePreset {
		t.Fatalf("source = %v", attrs["source"])
	}
	if _, hasGraph := attrs["graph"]; hasGraph {
		t.Fatal("definition_json must not carry embedded graph after materialization (D.1)")
	}
	if created.LinkedGraphID != asset.ID {
		t.Fatalf("team.LinkedGraphID = %q", created.LinkedGraphID)
	}
}

// --- 2026-08-07 根因回归：Create 物化链路必须保留交付物通道字段 ---
// 09:24 会话两个 DAG team 落库丢失 enable_state_deliverable → 成员无
// set_deliverable 工具 → 真实交付物闸门判失败 → DAG 下游节点永不派发。

func TestTeamGraphHook_CreatePreservesDeliverableChannelFields(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	writer := &recTeamWriter{}
	uc := newHookUsecase(&stubTeamReader{}, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, &fakeTxProvider{})

	// 用裸 JSON（非 spec 序列化）模拟 SpiritTeamUsecase 装配产物：
	// map 构造的 definition_json 携带 enable_state_deliverable / deliverable_contract / verification_gates。
	raw := `{"version":2,"mode":"coordinator","runtime_engine":"graph","team_graph_runtime":true,` +
		`"members":[{"agent_id":"a1","role":"synthesizer","name":"S","enabled":true,"sort_order":1}],` +
		`"enable_state_deliverable":true,` +
		`"deliverable_contract":{"entries":[{"topic":"root_cause","required":true}]},` +
		`"verification_gates":[{"gate_type":"tool_assertion","description":"d","max_retries":3,"tool":"skill_list"}]` +
		`}`
	created, err := uc.Create(context.Background(), Team{
		TeamKey:        "t-deliv",
		DisplayName:    "Deliverable Team",
		Kind:           "user",
		DefinitionJSON: raw,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	attrs := definitionAttrs(t, created.DefinitionJSON)
	if attrs["enable_state_deliverable"] != true {
		t.Fatalf("enable_state_deliverable dropped by materialize hook: %v", attrs["enable_state_deliverable"])
	}
	contract, ok := attrs["deliverable_contract"].(map[string]any)
	if !ok {
		t.Fatalf("deliverable_contract dropped by materialize hook: %v", attrs["deliverable_contract"])
	}
	if entries, ok := contract["entries"].([]any); !ok || len(entries) != 1 {
		t.Fatalf("deliverable_contract.entries mangled: %v", contract["entries"])
	}
	if gates, ok := attrs["verification_gates"].([]any); !ok || len(gates) != 1 {
		t.Fatalf("verification_gates dropped by materialize hook: %v", attrs["verification_gates"])
	}
}

// --- B3/D1：物化失败 = 保存失败（无残留） ---

func TestTeamGraphHook_CreateMaterializeFailureAbortsSave(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	writer := &recTeamWriter{}
	tx := &fakeTxProvider{}
	uc := newHookUsecase(&stubTeamReader{}, writer, store,
		&fakeTeamCompiler{err: apierror.BadRequest("TEAM", "compile graph: no enabled members")}, tx)

	_, err := uc.Create(context.Background(), Team{
		TeamKey:        "t2",
		DisplayName:    "Team Two",
		Kind:           "user",
		DefinitionJSON: hookSpecJSON(t, nil),
	})
	if err == nil {
		t.Fatal("expected materialize failure to abort save")
	}
	if writer.created != nil {
		t.Fatal("team must not be persisted when materialization fails (D1)")
	}
	if len(store.created) != 0 {
		t.Fatal("no graph asset should be persisted on failure")
	}
	if tx.calls != 1 {
		t.Fatalf("expected save wrapped in one transaction, got %d", tx.calls)
	}
}

// --- B3：preset 幂等保存（仅改名）跳过重建 ---

func TestTeamGraphHook_UpdatePresetIdempotentSkipsRebuild(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	prev := Team{
		ID:             "team-1",
		TeamKey:        "t1",
		DisplayName:    "Old Name",
		Kind:           "user",
		Status:         TeamStatusPending,
		LinkedGraphID:  "g-1",
		DefinitionJSON: hookSpecJSON(t, func(s *OrchestrationSpec) { s.LinkedGraphID = "g-1"; s.Source = DefinitionGraphSourcePreset }),
	}
	writer := &recTeamWriter{}
	uc := newHookUsecase(&stubTeamReader{team: prev}, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, nil)

	updated, err := uc.Update(context.Background(), "team-1", Team{DisplayName: "New Name"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(store.created) != 0 || len(store.updated) != 0 {
		t.Fatalf("idempotent save must skip rebuild: created=%d updated=%d", len(store.created), len(store.updated))
	}
	if updated.LinkedGraphID != "g-1" {
		t.Fatalf("LinkedGraphID = %q", updated.LinkedGraphID)
	}
}

// --- B3：preset 拓扑变更原地重建（graph_id 连续） ---

func TestTeamGraphHook_UpdatePresetTopologyChangeRebuildsInPlace(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	prev := Team{
		ID:             "team-1",
		TeamKey:        "t1",
		DisplayName:    "Team",
		Kind:           "user",
		Status:         TeamStatusPending,
		LinkedGraphID:  "g-1",
		DefinitionJSON: hookSpecJSON(t, func(s *OrchestrationSpec) { s.LinkedGraphID = "g-1"; s.Source = DefinitionGraphSourcePreset }),
	}
	writer := &recTeamWriter{}
	uc := newHookUsecase(&stubTeamReader{team: prev}, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, nil)

	patchJSON := hookSpecJSON(t, func(s *OrchestrationSpec) {
		s.Members = append(s.Members, OrchestrationMember{AgentID: "a2", Role: RoleWorker, Name: "A2", SortOrder: 2})
	})
	updated, err := uc.Update(context.Background(), "team-1", Team{DefinitionJSON: patchJSON})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(store.updated) != 1 {
		t.Fatalf("expected in-place UpdateGraph, got created=%d updated=%d", len(store.created), len(store.updated))
	}
	if store.updated[0].ID != "g-1" {
		t.Fatalf("rebuild must keep same graph id, got %q", store.updated[0].ID)
	}
	if updated.LinkedGraphID != "g-1" {
		t.Fatalf("LinkedGraphID = %q", updated.LinkedGraphID)
	}
	attrs := definitionAttrs(t, updated.DefinitionJSON)
	if attrs["source"] != DefinitionGraphSourcePreset {
		t.Fatalf("source = %v", attrs["source"])
	}
}

// --- B4：custom 未触碰拓扑 → 保留资产 ---

func TestTeamGraphHook_CustomUnchangedKeepsAsset(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	owned := ownedGraphDef("g-1", "team-1")
	owned.Metadata[GraphMetadataTeamSourceKey] = DefinitionGraphSourceCustom
	store.addExisting(owned)
	prev := Team{
		ID:            "team-1",
		TeamKey:       "t1",
		DisplayName:   "Team",
		Kind:          "user",
		Status:        TeamStatusPending,
		LinkedGraphID: "g-1",
		DefinitionJSON: hookSpecJSON(t, func(s *OrchestrationSpec) {
			s.LinkedGraphID = "g-1"
			s.Source = DefinitionGraphSourceCustom
		}),
	}
	writer := &recTeamWriter{}
	uc := newHookUsecase(&stubTeamReader{team: prev}, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, nil)

	patchJSON := hookSpecJSON(t, func(s *OrchestrationSpec) {
		s.LinkedGraphID = "g-1"
		s.Source = DefinitionGraphSourceCustom
	})
	updated, err := uc.Update(context.Background(), "team-1", Team{DefinitionJSON: patchJSON, DisplayName: "Renamed"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(store.created) != 0 || len(store.updated) != 0 {
		t.Fatalf("custom unchanged must not rebuild: created=%d updated=%d", len(store.created), len(store.updated))
	}
	attrs := definitionAttrs(t, updated.DefinitionJSON)
	if attrs["source"] != DefinitionGraphSourceCustom {
		t.Fatalf("source = %v, want custom preserved", attrs["source"])
	}
}

// --- B4：custom + 拓扑变更 → 按 preset 重建（前端已确认覆盖） ---

func TestTeamGraphHook_CustomTopologyChangeRebuildsAsPreset(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	owned := ownedGraphDef("g-1", "team-1")
	owned.Metadata[GraphMetadataTeamSourceKey] = DefinitionGraphSourceCustom
	store.addExisting(owned)
	prev := Team{
		ID:            "team-1",
		TeamKey:       "t1",
		DisplayName:   "Team",
		Kind:          "user",
		Status:        TeamStatusPending,
		LinkedGraphID: "g-1",
		DefinitionJSON: hookSpecJSON(t, func(s *OrchestrationSpec) {
			s.LinkedGraphID = "g-1"
			s.Source = DefinitionGraphSourceCustom
		}),
	}
	writer := &recTeamWriter{}
	uc := newHookUsecase(&stubTeamReader{team: prev}, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, nil)

	patchJSON := hookSpecJSON(t, func(s *OrchestrationSpec) {
		s.LinkedGraphID = "g-1"
		s.Source = DefinitionGraphSourceCustom
		s.Mode = TeamModeParallel // 拓扑字段变更
	})
	updated, err := uc.Update(context.Background(), "team-1", Team{DefinitionJSON: patchJSON})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(store.updated) != 1 || store.updated[0].ID != "g-1" {
		t.Fatalf("expected in-place rebuild of g-1, got %+v", store.updated)
	}
	attrs := definitionAttrs(t, updated.DefinitionJSON)
	if attrs["source"] != DefinitionGraphSourcePreset {
		t.Fatalf("source = %v, want preset after rebuild (B4)", attrs["source"])
	}
}

// --- B4 回归：物化路径经 guard 装配的真实 GraphDefinitionUsecase 不得被镜像为 custom ---
// 生产装配：TeamUsecase.graphAssets = 真实 defUC，且 defUC.SetTeamGraphGuard(teamUC)。
// 物化（B4 重建）若走带 guard 的 UpdateGraph，team_source 会被 OnTeamOwnedGraphSaved
// 误镜像为 custom（2026-08-01 V3 运行时验证发现）。

func TestTeamGraphHook_MaterializeThroughGuardKeepsPresetSource(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	owned := ownedGraphDef("g-1", "team-1")
	owned.Metadata[GraphMetadataTeamSourceKey] = DefinitionGraphSourceCustom
	owned.Nodes = []NodeDef{{ID: "member_1", Type: "agent", AgentName: "a1"}}
	store.addExisting(owned)
	prev := Team{
		ID:            "team-1",
		TeamKey:       "t1",
		DisplayName:   "Team",
		Kind:          "user",
		Status:        TeamStatusPending,
		LinkedGraphID: "g-1",
		DefinitionJSON: hookSpecJSON(t, func(s *OrchestrationSpec) {
			s.LinkedGraphID = "g-1"
			s.Source = DefinitionGraphSourceCustom
		}),
	}
	writer := &recTeamWriter{}
	teamUC := NewTeamUsecase(TeamUsecaseOpts{
		Reader:      &stubTeamReader{team: prev},
		Writer:      writer,
		RunReader:   &stubTeamRunReader{},
		RunWriter:   &stubTeamRunWriter{},
		StepRepo:    &stubOrchestrationStepRepo{},
		DeadLetter:  &stubTaskDeadLetterRepo{},
		GraphReader: store,
		GraphWriter: store,
		Compiler:    &fakeTeamCompiler{cfg: simpleBuildConfig()},
		Lg:          loggateway.NewNoop(),
	})
	defUC := newGuardDefUC(store, teamUC)
	teamUC.graphAssets = defUC // 生产同款装配：物化走真实 defUC（带 guard）

	patchJSON := hookSpecJSON(t, func(s *OrchestrationSpec) {
		s.LinkedGraphID = "g-1"
		s.Source = DefinitionGraphSourceCustom
		s.Mode = TeamModeParallel // 拓扑字段变更 → 触发按 preset 重建
	})
	if _, err := teamUC.Update(context.Background(), "team-1", Team{DefinitionJSON: patchJSON}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	saved := store.byID["g-1"]
	if saved == nil {
		t.Fatal("g-1 missing after rebuild")
	}
	if got := saved.Metadata[GraphMetadataTeamSourceKey]; got != DefinitionGraphSourcePreset {
		t.Fatalf("team_source = %v, want preset（物化路径不得被 guard 镜像为 custom）", got)
	}
	if writer.updated == nil {
		t.Fatal("team not persisted")
	}
	attrs := definitionAttrs(t, writer.updated.DefinitionJSON)
	if attrs["source"] != DefinitionGraphSourcePreset {
		t.Fatalf("team definition source = %v, want preset", attrs["source"])
	}
}

// --- B8：换绑校验 + D2 旧 owned 图删除 ---

func TestTeamGraphHook_BindExternalGraph(t *testing.T) {
	t.Parallel()

	t.Run("target not found", func(t *testing.T) {
		store := newFakeGraphStore()
		prev := Team{ID: "team-1", TeamKey: "t1", DisplayName: "T", Kind: "user", Status: TeamStatusPending,
			DefinitionJSON: hookSpecJSON(t, nil)}
		uc := newHookUsecase(&stubTeamReader{team: prev}, &recTeamWriter{}, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, nil)
		patchJSON := hookSpecJSON(t, func(s *OrchestrationSpec) {
			s.Source = DefinitionGraphSourceLinkedExt
			s.LinkedGraphID = "g-missing"
		})
		_, err := uc.Update(context.Background(), "team-1", Team{DefinitionJSON: patchJSON})
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not-found error, got %v", err)
		}
	})

	t.Run("reject graph owned by another team", func(t *testing.T) {
		store := newFakeGraphStore()
		store.addExisting(ownedGraphDef("g-other", "team-OTHER"))
		prev := Team{ID: "team-1", TeamKey: "t1", DisplayName: "T", Kind: "user", Status: TeamStatusPending,
			DefinitionJSON: hookSpecJSON(t, nil)}
		uc := newHookUsecase(&stubTeamReader{team: prev}, &recTeamWriter{}, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, nil)
		patchJSON := hookSpecJSON(t, func(s *OrchestrationSpec) {
			s.Source = DefinitionGraphSourceLinkedExt
			s.LinkedGraphID = "g-other"
		})
		_, err := uc.Update(context.Background(), "team-1", Team{DefinitionJSON: patchJSON})
		if err == nil {
			t.Fatal("expected conflict when linking another team's owned graph")
		}
		if e, ok := apierror.From(err); !ok || e.Code != apierror.CodeConflict {
			t.Fatalf("expected Conflict, got %v", err)
		}
	})

	t.Run("rebind deletes old owned graph (D2)", func(t *testing.T) {
		store := newFakeGraphStore()
		store.addExisting(ownedGraphDef("g-old", "team-1"))
		store.addExisting(&GraphDefinition{ID: "g-ext", Name: "independent"})
		prev := Team{ID: "team-1", TeamKey: "t1", DisplayName: "T", Kind: "user", Status: TeamStatusPending,
			LinkedGraphID: "g-old",
			DefinitionJSON: hookSpecJSON(t, func(s *OrchestrationSpec) {
				s.LinkedGraphID = "g-old"
				s.Source = DefinitionGraphSourcePreset
			})}
		uc := newHookUsecase(&stubTeamReader{team: prev}, &recTeamWriter{}, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, nil)
		patchJSON := hookSpecJSON(t, func(s *OrchestrationSpec) {
			s.Source = DefinitionGraphSourceLinkedExt
			s.LinkedGraphID = "g-ext"
		})
		updated, err := uc.Update(context.Background(), "team-1", Team{DefinitionJSON: patchJSON})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if len(store.deleted) != 1 || store.deleted[0] != "g-old" {
			t.Fatalf("D2: old owned graph must be deleted, deleted=%v", store.deleted)
		}
		if _, stillThere := store.byID["g-ext"]; !stillThere {
			t.Fatal("external graph must survive rebinding")
		}
		if updated.LinkedGraphID != "g-ext" {
			t.Fatalf("LinkedGraphID = %q", updated.LinkedGraphID)
		}
		attrs := definitionAttrs(t, updated.DefinitionJSON)
		if attrs["source"] != DefinitionGraphSourceLinkedExt {
			t.Fatalf("source = %v", attrs["source"])
		}
	})
}

// --- B5：删 team——owned 级联删 / external 只解绑 ---

func TestTeamGraphHook_DeleteTeamOwnedCascade(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	reader := &stubTeamReader{team: Team{ID: "team-1", Kind: "user", LinkedGraphID: "g-1"}}
	writer := &recTeamWriter{}
	uc := newHookUsecase(reader, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, nil)

	if err := uc.Delete(context.Background(), "team-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "g-1" {
		t.Fatalf("owned graph must be cascade-deleted, deleted=%v", store.deleted)
	}
	if writer.deletedID != "team-1" {
		t.Fatalf("team not deleted: %q", writer.deletedID)
	}
}

func TestTeamGraphHook_DeleteTeamExternalUnbindOnly(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	// external 图：无 team_owned metadata，但 ORG-11b 回填了 team_id。
	store.addExisting(&GraphDefinition{ID: "g-ext", Name: "independent", TeamID: "team-1"})
	reader := &stubTeamReader{team: Team{ID: "team-1", Kind: "user", LinkedGraphID: "g-ext"}}
	writer := &recTeamWriter{}
	uc := newHookUsecase(reader, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, nil)

	if err := uc.Delete(context.Background(), "team-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("external graph must NOT be deleted, deleted=%v", store.deleted)
	}
	survivor := store.byID["g-ext"]
	if survivor == nil {
		t.Fatal("external graph must survive team deletion")
	}
	if survivor.TeamID != "" {
		t.Fatalf("external graph must be unbound, TeamID = %q", survivor.TeamID)
	}
	if writer.deletedID != "team-1" {
		t.Fatalf("team not deleted: %q", writer.deletedID)
	}
}

// --- Y11：DeleteTeam 级联（owned graph + team 行）原子化 ---

// TestTeamGraphHook_DeleteTeamCascadeAtomicOnFailure (Y11)：owned 图级联删
// 失败时整个删除必须中止——不得出现「图已删/删失败但 team 行已删」的半完成
// 态。此前级联删错误被 best-effort 吞掉（Warn），team 照常删除，留下的
// linked_graph_id 悬空或 owned 图孤儿均无事务兜底。
func TestTeamGraphHook_DeleteTeamCascadeAtomicOnFailure(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	store.failDeleteOwned = errors.New("graph delete blocked")
	reader := &stubTeamReader{team: Team{ID: "team-1", Kind: "user", LinkedGraphID: "g-1"}}
	writer := &recTeamWriter{}
	tx := &fakeTxProvider{}
	uc := newHookUsecase(reader, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, tx)

	err := uc.Delete(context.Background(), "team-1")
	if err == nil {
		t.Fatal("级联删失败必须使 Delete 返回错误")
	}
	if writer.deletedID != "" {
		t.Fatalf("级联删失败时 team 行不得删除（原子性），deletedID=%q", writer.deletedID)
	}
	if tx.calls != 1 {
		t.Fatalf("级联 + team 删除须在同一事务内执行，ExecInTx calls=%d", tx.calls)
	}
}

// TestTeamGraphHook_DeleteTeamCascadeSameTx (Y11)：成功路径上级联删与 team
// 行删除必须包裹在同一个 ExecInTx 中（D1 对称：保存侧物化+回写已同事务）。
func TestTeamGraphHook_DeleteTeamCascadeSameTx(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	reader := &stubTeamReader{team: Team{ID: "team-1", Kind: "user", LinkedGraphID: "g-1"}}
	writer := &recTeamWriter{}
	tx := &fakeTxProvider{}
	uc := newHookUsecase(reader, writer, store, &fakeTeamCompiler{cfg: simpleBuildConfig()}, tx)

	if err := uc.Delete(context.Background(), "team-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if tx.calls != 1 {
		t.Fatalf("级联 + team 删除须在同一事务内执行，ExecInTx calls=%d", tx.calls)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "g-1" {
		t.Fatalf("owned graph must be cascade-deleted, deleted=%v", store.deleted)
	}
	if writer.deletedID != "team-1" {
		t.Fatalf("team not deleted: %q", writer.deletedID)
	}
}

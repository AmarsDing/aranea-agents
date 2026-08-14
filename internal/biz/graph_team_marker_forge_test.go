package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// Y10：team_owned/team_source/team_id 是 Team 生命周期的权威标记（物化/
// 迁移/换绑/ORG-11b 回填），公共 Graph CRUD 入口（Create/Import/Update）
// 不得允许调用方自声明——否则可伪造 owned 关系，触发 B6 反向同步改写
// 受害 team 的 DefinitionJSON（跨工作区写）或 B7 删除保护错位。

type fakeDefFactory struct{}

func (fakeDefFactory) Visualize(context.Context, GraphBuildConfig) (*GraphVisualization, error) {
	return &GraphVisualization{}, nil
}
func (fakeDefFactory) Validate(context.Context, GraphBuildConfig) (*GraphValidationResult, error) {
	return &GraphValidationResult{}, nil
}
func (fakeDefFactory) ListTemplates() []GraphTemplateRef { return nil }
func (fakeDefFactory) GetTemplate(string) (GraphTemplateRef, bool) {
	return GraphTemplateRef{}, false
}
func (fakeDefFactory) TemplateToDef(GraphTemplateRef, string, string) *GraphDefinition {
	return &GraphDefinition{}
}

func newForgeDefUC(store *fakeGraphStore) *GraphDefinitionUsecase {
	return NewGraphDefinitionUsecase(store, fakeDefFactory{}, nil, loggateway.NewNoop())
}

func forgedMarkers() map[string]any {
	return map[string]any{
		GraphMetadataTeamOwnedKey:  true,
		GraphMetadataTeamSourceKey: DefinitionGraphSourcePreset,
	}
}

func TestCreateGraph_StripsForgedTeamMarkers(t *testing.T) {
	store := newFakeGraphStore()
	uc := newForgeDefUC(store)
	saved, err := uc.CreateGraph(context.Background(), &GraphDefinition{
		Name:     "forged",
		TeamID:   "victim-team",
		Metadata: forgedMarkers(),
	})
	if err != nil {
		t.Fatalf("CreateGraph: %v", err)
	}
	if saved.TeamID != "" {
		t.Fatalf("TeamID=%q want 清空（不可自声明属主）", saved.TeamID)
	}
	if isTeamOwnedGraph(saved) {
		t.Fatalf("team_owned 标记未被剥离: %v", saved.Metadata)
	}
	if _, ok := saved.Metadata[GraphMetadataTeamSourceKey]; ok {
		t.Fatalf("team_source 标记未被剥离: %v", saved.Metadata)
	}
}

func TestImportGraph_StripsForgedTeamMarkers(t *testing.T) {
	store := newFakeGraphStore()
	uc := newForgeDefUC(store)
	raw := []byte(`{
		"name":"imported","team_id":"victim-team",
		"entry_point":"n1","finish_point":"n1",
		"nodes":[{"id":"n1","type":"agent","agent_name":"a"}],
		"metadata":{"team_owned":true,"team_source":"preset"}
	}`)
	saved, err := uc.ImportGraph(context.Background(), raw, "", "", "ws-1")
	if err != nil {
		t.Fatalf("ImportGraph: %v", err)
	}
	if saved.TeamID != "" {
		t.Fatalf("TeamID=%q want 清空", saved.TeamID)
	}
	if isTeamOwnedGraph(saved) {
		t.Fatalf("导入的 team_owned 标记未被剥离: %v", saved.Metadata)
	}
	if _, ok := saved.Metadata[GraphMetadataTeamSourceKey]; ok {
		t.Fatalf("导入的 team_source 标记未被剥离: %v", saved.Metadata)
	}
	if saved.WorkspaceID != "ws-1" {
		t.Fatalf("WorkspaceID=%q want ws-1", saved.WorkspaceID)
	}
}

func TestUpdateGraph_CannotSelfDeclareTeamMarkers(t *testing.T) {
	store := newFakeGraphStore()
	store.addExisting(&GraphDefinition{ID: "g-1", Name: "plain", Version: 1})
	uc := newForgeDefUC(store)
	saved, err := uc.UpdateGraph(context.Background(), &GraphDefinition{
		ID:       "g-1",
		Name:     "plain",
		TeamID:   "victim-team",
		Metadata: forgedMarkers(),
	})
	if err != nil {
		t.Fatalf("UpdateGraph: %v", err)
	}
	if saved.TeamID != "" {
		t.Fatalf("TeamID=%q want 空（previous 无属主）", saved.TeamID)
	}
	if isTeamOwnedGraph(saved) {
		t.Fatalf("update 自声明 team_owned 未被剥离: %v", saved.Metadata)
	}
}

func TestUpdateGraph_PreservesORG11bBackfilledTeamID(t *testing.T) {
	store := newFakeGraphStore()
	// external 链接图：无 team_owned，但 ORG-11b 已回填 team_id。
	store.addExisting(&GraphDefinition{ID: "g-2", Name: "linked", TeamID: "team-ext", Version: 1})
	uc := newForgeDefUC(store)
	saved, err := uc.UpdateGraph(context.Background(), &GraphDefinition{
		ID:      "g-2",
		Name:    "linked",
		TeamID:  "other-team", // 编辑器/调用方试图改写（或清空）team_id
	})
	if err != nil {
		t.Fatalf("UpdateGraph: %v", err)
	}
	if saved.TeamID != "team-ext" {
		t.Fatalf("TeamID=%q want team-ext（DB 现状权威，team_id 只能经 Team 侧改变）", saved.TeamID)
	}
}

func TestUpdateGraph_OwnedMarkersAuthoritativeFromDB(t *testing.T) {
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-3", "team-1"))
	uc := newForgeDefUC(store) // guard=nil：标记恢复不应依赖 guard 装配
	saved, err := uc.UpdateGraph(context.Background(), &GraphDefinition{
		ID:      "g-3",
		Name:    "owned",
		TeamID:  "team-2", // 试图换绑到他队
	})
	if err != nil {
		t.Fatalf("UpdateGraph: %v", err)
	}
	if saved.TeamID != "team-1" {
		t.Fatalf("TeamID=%q want team-1（owned 关系只能经 Team 保存钩子改变）", saved.TeamID)
	}
	if !isTeamOwnedGraph(saved) {
		t.Fatal("owned 图的 team_owned 标记丢失")
	}
}

// 编译期接口断言：fakeDefFactory 满足 GraphDefinitionFactory。
var _ GraphDefinitionFactory = fakeDefFactory{}

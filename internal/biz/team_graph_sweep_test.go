package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// --- fakes for SweepTerminalAutoTeamGraphs tests ---

type sweepTeamReader struct {
	teams []Team
	err   error
}

func (s *sweepTeamReader) ListTeams(context.Context) ([]Team, error) { return s.teams, s.err }
func (s *sweepTeamReader) ListTeamsByStatus(context.Context, string) ([]Team, error) {
	return nil, nil
}
func (s *sweepTeamReader) GetTeamByID(_ context.Context, id string) (Team, error) {
	for _, tm := range s.teams {
		if tm.ID == id {
			return tm, nil
		}
	}
	return Team{}, ErrNotFound
}
func (s *sweepTeamReader) GetTeamByKey(context.Context, string) (Team, error) {
	return Team{}, ErrNotFound
}
func (s *sweepTeamReader) ListBySpiritSessionID(context.Context, string) ([]Team, error) {
	return nil, nil
}
func (s *sweepTeamReader) ListTeamsByDepartmentID(context.Context, string) ([]Team, error) {
	return nil, nil
}
func (s *sweepTeamReader) ListTeamsByWorkspace(context.Context, string) ([]Team, error) {
	return nil, nil
}
func (s *sweepTeamReader) CountTeamsByWorkspace(context.Context, string) (int, error) {
	return 0, nil
}

func TestSweepTerminalAutoTeamGraphs_DeletesTerminalAutoOwned(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	teams := []Team{
		// 命中：终态 + auto + 超龄 + owned 图
		{ID: "t1", AutoCreated: true, Status: TeamStatusCompleted, UpdatedAt: old, LinkedGraphID: "g1"},
		// 跳过：手动团队
		{ID: "t2", AutoCreated: false, Status: TeamStatusCompleted, UpdatedAt: old, LinkedGraphID: "g2"},
		// 跳过：非终态
		{ID: "t3", AutoCreated: true, Status: TeamStatusRunning, UpdatedAt: old, LinkedGraphID: "g3"},
		// 跳过：未超龄（未来时间，避免与 cutoff=now 的微秒级竞态）
		{ID: "t4", AutoCreated: true, Status: TeamStatusFailed, UpdatedAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), LinkedGraphID: "g4"},
		// 跳过：linked_external（team_owned 标记缺失）
		{ID: "t5", AutoCreated: true, Status: TeamStatusCompleted, UpdatedAt: old, LinkedGraphID: "g5"},
		// 跳过：图 team_id 不匹配（他队图）
		{ID: "t6", AutoCreated: true, Status: TeamStatusCompleted, UpdatedAt: old, LinkedGraphID: "g6"},
		// 跳过：图已不存在（幂等）
		{ID: "t7", AutoCreated: true, Status: TeamStatusCancelled, UpdatedAt: old, LinkedGraphID: "g-gone"},
		// 命中：partial_failure 也在终态口径内
		{ID: "t8", AutoCreated: true, Status: TeamStatusPartialFailure, UpdatedAt: old, LinkedGraphID: "g8"},
		// 命中：archived 是归档沉降点
		{ID: "t9", AutoCreated: true, Status: TeamStatusArchived, UpdatedAt: old, LinkedGraphID: "g9"},
		// 跳过：interrupted（防 resume 边缘场景）
		{ID: "t10", AutoCreated: true, Status: TeamStatusInterrupted, UpdatedAt: old, LinkedGraphID: "g10"},
	}
	graphs := newFakeGraphStore()
	graphs.addExisting(ownedGraphDef("g1", "t1"))
	graphs.addExisting(ownedGraphDef("g2", "t2"))
	graphs.addExisting(ownedGraphDef("g3", "t3"))
	graphs.addExisting(ownedGraphDef("g4", "t4"))
	graphs.addExisting(&GraphDefinition{ID: "g5", TeamID: "t5", Metadata: map[string]any{}}) // external：无 owned 标记
	graphs.addExisting(ownedGraphDef("g6", "other-team"))
	graphs.addExisting(ownedGraphDef("g8", "t8"))
	graphs.addExisting(ownedGraphDef("g9", "t9"))
	graphs.addExisting(ownedGraphDef("g10", "t10"))

	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:      &sweepTeamReader{teams: teams},
		GraphReader: graphs,
		GraphAssets: graphs,
		Lg:          loggateway.NewNoop(),
	})
	swept, err := uc.SweepTerminalAutoTeamGraphs(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	if swept != 3 {
		t.Fatalf("swept = %d, want 3 (t1, t8, t9)", swept)
	}
	for _, id := range []string{"g2", "g3", "g4", "g5", "g6", "g10"} {
		if _, ok := graphs.byID[id]; !ok {
			t.Errorf("graph %s must survive sweep", id)
		}
	}
	for _, id := range []string{"g1", "g8", "g9"} {
		if _, ok := graphs.byID[id]; ok {
			t.Errorf("graph %s must be swept", id)
		}
	}
}

func TestSweepTerminalAutoTeamGraphs_SkipsUnparseableTime(t *testing.T) {
	teams := []Team{
		{ID: "t1", AutoCreated: true, Status: TeamStatusCompleted, UpdatedAt: "not-a-time", LinkedGraphID: "g1"},
	}
	graphs := newFakeGraphStore()
	graphs.addExisting(ownedGraphDef("g1", "t1"))
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:      &sweepTeamReader{teams: teams},
		GraphReader: graphs,
		GraphAssets: graphs,
		Lg:          loggateway.NewNoop(),
	})
	swept, err := uc.SweepTerminalAutoTeamGraphs(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	if swept != 0 {
		t.Fatalf("swept = %d, want 0（删除不可逆，时间不可解析必须保守跳过）", swept)
	}
}

func TestSweepTerminalAutoTeamGraphs_ContinuesOnDeleteError(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	teams := []Team{
		{ID: "t1", AutoCreated: true, Status: TeamStatusCompleted, UpdatedAt: old, LinkedGraphID: "g1"},
		{ID: "t2", AutoCreated: true, Status: TeamStatusCompleted, UpdatedAt: old, LinkedGraphID: "g2"},
	}
	graphs := newFakeGraphStore()
	graphs.addExisting(ownedGraphDef("g1", "t1"))
	graphs.addExisting(ownedGraphDef("g2", "t2"))
	failStore := &sweepFlakyStore{fakeGraphStore: graphs, failIDs: map[string]bool{"g1": true}}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:      &sweepTeamReader{teams: teams},
		GraphReader: graphs,
		GraphAssets: failStore,
		Lg:          loggateway.NewNoop(),
	})
	swept, err := uc.SweepTerminalAutoTeamGraphs(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("sweep must not fail on single delete error: %v", err)
	}
	if swept != 1 {
		t.Fatalf("swept = %d, want 1（g1 失败不阻塞 g2）", swept)
	}
}

// sweepFlakyStore 包装 fakeGraphStore，对指定 id 注入删除失败。
type sweepFlakyStore struct {
	*fakeGraphStore
	failIDs map[string]bool
}

func (s *sweepFlakyStore) DeleteOwnedGraph(ctx context.Context, id string) error {
	if s.failIDs[id] {
		return errors.New("injected delete failure")
	}
	return s.fakeGraphStore.DeleteOwnedGraph(ctx, id)
}

func TestMaterializeTeamGraphDefinition_AutoCreatedTag(t *testing.T) {
	cfg := GraphBuildConfig{EntryPoint: "n1"}
	auto := MaterializeTeamGraphDefinition(Team{ID: "t1", DisplayName: "auto", AutoCreated: true}, cfg, nil, DefinitionGraphSourcePreset)
	if auto.Metadata[GraphMetadataTeamAutoCreatedKey] != true {
		t.Error("auto-created team 的物化图必须打 team_auto_created 标记")
	}
	manual := MaterializeTeamGraphDefinition(Team{ID: "t2", DisplayName: "manual"}, cfg, nil, DefinitionGraphSourcePreset)
	if _, ok := manual.Metadata[GraphMetadataTeamAutoCreatedKey]; ok {
		t.Error("手动团队的物化图不得打 team_auto_created 标记")
	}
	// 更新路径：existing 残留的标记必须以属主现状为准（手动团队更新后标记被清除）。
	stale := &GraphDefinition{ID: "g1", Metadata: map[string]any{GraphMetadataTeamAutoCreatedKey: true}}
	updated := MaterializeTeamGraphDefinition(Team{ID: "t3", DisplayName: "manual2"}, cfg, stale, DefinitionGraphSourcePreset)
	if _, ok := updated.Metadata[GraphMetadataTeamAutoCreatedKey]; ok {
		t.Error("existing 残留的 team_auto_created 必须被属主现状覆盖")
	}
}

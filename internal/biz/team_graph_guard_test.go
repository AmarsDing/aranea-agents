package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// --- fakes for Team × Graph guard tests（B6 反向同步 / B7 删除保护） ---

type activeRunReader struct{ stubTeamRunReader }

func (activeRunReader) HasActiveTeamRun(context.Context, string) (bool, error) { return true, nil }

type fakeLinkedGraphReader struct {
	refs    []Team
	queried string
}

func (f *fakeLinkedGraphReader) ListTeamsByLinkedGraphID(_ context.Context, graphID string) ([]Team, error) {
	f.queried = graphID
	return f.refs, nil
}

func fakeAgentIDResolver(mapping map[string]string) TeamAgentIDResolver {
	return func(context.Context) func(string) (string, bool) {
		return func(agentKey string) (string, bool) {
			id, ok := mapping[agentKey]
			return id, ok
		}
	}
}

func newGuardTeamUsecase(reader TeamReader, writer TeamWriter, runReader TeamRunReader, linked TeamLinkedGraphReader, resolver TeamAgentIDResolver) *TeamUsecase {
	return NewTeamUsecase(TeamUsecaseOpts{
		Reader:          reader,
		Writer:          writer,
		RunReader:       runReader,
		RunWriter:       &stubTeamRunWriter{},
		StepRepo:        &stubOrchestrationStepRepo{},
		DeadLetter:      &stubTaskDeadLetterRepo{},
		LinkedReader:    linked,
		AgentIDResolver: resolver,
		Lg:              loggateway.NewNoop(),
	})
}

func newGuardDefUC(store *fakeGraphStore, guard TeamGraphGuard) *GraphDefinitionUsecase {
	uc := NewGraphDefinitionUsecase(store, nil, nil, loggateway.NewNoop())
	if guard != nil {
		uc.SetTeamGraphGuard(guard)
	}
	return uc
}

func teamOwnedDefForSave(id, teamID string) *GraphDefinition {
	return &GraphDefinition{
		ID:          id,
		Name:        "edited",
		EntryPoint:  "member_1",
		FinishPoint: "member_1",
		Nodes: []NodeDef{
			{ID: "member_1", Type: "agent", AgentName: "key-a1", Description: "Alpha", Instruction: "do a", RequiredRole: RoleWorker},
			{ID: "member_2", Type: "agent", AgentName: "key-a2", Description: "Beta", Instruction: "do b"},
		},
		Edges: []EdgeDef{{From: "member_1", To: "member_2"}},
	}
}

// --- B6 反向同步 ---

func TestTeamGraphGuard_SaveOwnedGraphReverseSyncsTeam(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	team := Team{ID: "team-1", TeamKey: "t1", DisplayName: "Team One", Kind: "user",
		DefinitionJSON: hookSpecJSON(t, func(s *OrchestrationSpec) { s.LinkedGraphID = "g-1" })}
	writer := &recTeamWriter{}
	teamUC := newGuardTeamUsecase(&stubTeamReader{team: team}, writer, &stubTeamRunReader{},
		nil, fakeAgentIDResolver(map[string]string{"key-a1": "a1", "key-a2": "a2"}))
	defUC := newGuardDefUC(store, teamUC)

	saved, err := defUC.UpdateGraph(context.Background(), teamOwnedDefForSave("g-1", "team-1"))
	if err != nil {
		t.Fatalf("UpdateGraph: %v", err)
	}
	// 图已保存，team_source 镜像为 custom。
	if got := saved.Metadata[GraphMetadataTeamSourceKey]; got != DefinitionGraphSourceCustom {
		t.Fatalf("team_source = %v, want custom", got)
	}
	if owned, _ := saved.Metadata[GraphMetadataTeamOwnedKey].(bool); !owned {
		t.Fatal("team_owned marker lost on save")
	}
	// team 回写：source=custom + members 从图派生（key → id 反查）。
	if writer.updated == nil {
		t.Fatal("team not updated after owned graph save")
	}
	spec, err := ParseOrchestrationSpec(writer.updated.DefinitionJSON)
	if err != nil {
		t.Fatalf("parse synced definition: %v", err)
	}
	if spec.GraphSource() != DefinitionGraphSourceCustom {
		t.Fatalf("team source = %q, want custom", spec.GraphSource())
	}
	if spec.LinkedGraphID != "g-1" || writer.updated.LinkedGraphID != "g-1" {
		t.Fatalf("linked_graph_id = %q / %q", spec.LinkedGraphID, writer.updated.LinkedGraphID)
	}
	if len(spec.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(spec.Members))
	}
	m := spec.Members[0]
	if m.AgentID != "a1" || m.Name != "Alpha" || m.TaskPrompt != "do a" || m.Role != RoleWorker || m.SortOrder != 1 {
		t.Fatalf("member[0] = %+v", m)
	}
	if spec.Members[1].AgentID != "a2" || spec.Members[1].Role != RoleWorker {
		t.Fatalf("member[1] = %+v", spec.Members[1])
	}
}

func TestTeamGraphGuard_SaveOwnedGraphRejectsActiveRun(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	team := Team{ID: "team-1", TeamKey: "t1", DisplayName: "Team One", Kind: "user",
		DefinitionJSON: hookSpecJSON(t, nil)}
	writer := &recTeamWriter{}
	teamUC := newGuardTeamUsecase(&stubTeamReader{team: team}, writer, &activeRunReader{}, nil, nil)
	defUC := newGuardDefUC(store, teamUC)

	_, err := defUC.UpdateGraph(context.Background(), teamOwnedDefForSave("g-1", "team-1"))
	if err == nil {
		t.Fatal("expected active-run rejection")
	}
	e, ok := apierror.From(err)
	if !ok || e.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict, got %v", err)
	}
	if len(store.updated) != 0 {
		t.Fatal("graph must not be persisted while owner has an active run")
	}
	if writer.updated != nil {
		t.Fatal("team must not be touched on rejection")
	}
}

func TestTeamGraphGuard_SaveOwnedGraphSkipsUnresolvableAgentKey(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	team := Team{ID: "team-1", TeamKey: "t1", DisplayName: "Team One", Kind: "user",
		DefinitionJSON: hookSpecJSON(t, nil)}
	writer := &recTeamWriter{}
	// key-a2 无法解析 → 该节点跳过，其余同步。
	teamUC := newGuardTeamUsecase(&stubTeamReader{team: team}, writer, &stubTeamRunReader{},
		nil, fakeAgentIDResolver(map[string]string{"key-a1": "a1"}))
	defUC := newGuardDefUC(store, teamUC)

	if _, err := defUC.UpdateGraph(context.Background(), teamOwnedDefForSave("g-1", "team-1")); err != nil {
		t.Fatalf("UpdateGraph: %v", err)
	}
	spec, err := ParseOrchestrationSpec(writer.updated.DefinitionJSON)
	if err != nil {
		t.Fatalf("parse synced definition: %v", err)
	}
	if len(spec.Members) != 1 || spec.Members[0].AgentID != "a1" {
		t.Fatalf("members = %+v, want single a1", spec.Members)
	}
}

func TestTeamGraphGuard_SaveOwnedGraphRestoresStrippedMarkers(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	team := Team{ID: "team-1", TeamKey: "t1", DisplayName: "Team One", Kind: "user",
		DefinitionJSON: hookSpecJSON(t, nil)}
	writer := &recTeamWriter{}
	teamUC := newGuardTeamUsecase(&stubTeamReader{team: team}, writer, &stubTeamRunReader{}, nil, nil)
	defUC := newGuardDefUC(store, teamUC)

	// 编辑器提交：无 metadata、无 team_id（不感知服务端标记）。
	incoming := teamOwnedDefForSave("g-1", "")
	saved, err := defUC.UpdateGraph(context.Background(), incoming)
	if err != nil {
		t.Fatalf("UpdateGraph: %v", err)
	}
	if owned, _ := saved.Metadata[GraphMetadataTeamOwnedKey].(bool); !owned {
		t.Fatal("team_owned marker not restored")
	}
	if saved.TeamID != "team-1" {
		t.Fatalf("TeamID = %q, want team-1", saved.TeamID)
	}
	if writer.updated == nil {
		t.Fatal("reverse sync must run even when editor strips markers")
	}
}

func TestTeamGraphGuard_SaveIndependentGraphSkipsSync(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(&GraphDefinition{ID: "g-9", Name: "independent", Version: 1})
	writer := &recTeamWriter{}
	teamUC := newGuardTeamUsecase(&stubTeamReader{}, writer, &stubTeamRunReader{}, nil, nil)
	defUC := newGuardDefUC(store, teamUC)

	if _, err := defUC.UpdateGraph(context.Background(), &GraphDefinition{ID: "g-9", Name: "independent-v2"}); err != nil {
		t.Fatalf("UpdateGraph: %v", err)
	}
	if writer.updated != nil {
		t.Fatal("independent graph save must not touch any team")
	}
}

func TestTeamGraphGuard_SaveOwnedGraphOrphanOwnerSavesWithoutSync(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-gone"))
	writer := &recTeamWriter{}
	// stubTeamReader 对未知 ID 返回 ErrNotFound → 孤儿 owned 资产不阻断保存。
	teamUC := newGuardTeamUsecase(&stubTeamReader{}, writer, &stubTeamRunReader{}, nil, nil)
	defUC := newGuardDefUC(store, teamUC)

	if _, err := defUC.UpdateGraph(context.Background(), teamOwnedDefForSave("g-1", "team-gone")); err != nil {
		t.Fatalf("UpdateGraph: %v", err)
	}
	if writer.updated != nil {
		t.Fatal("orphan owned graph must not trigger team writeback")
	}
}

// --- B7 删除保护 ---

func TestTeamGraphGuard_DeleteOwnedGraphRejectedWhileOwnerExists(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	team := Team{ID: "team-1", TeamKey: "t1", DisplayName: "Team One", Kind: "user"}
	teamUC := newGuardTeamUsecase(&stubTeamReader{team: team}, &recTeamWriter{}, &stubTeamRunReader{}, nil, nil)
	defUC := newGuardDefUC(store, teamUC)

	err := defUC.DeleteGraph(context.Background(), "g-1")
	if err == nil {
		t.Fatal("expected delete rejection for team-owned graph")
	}
	e, ok := apierror.From(err)
	if !ok || e.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict, got %v", err)
	}
	if len(store.deleted) != 0 {
		t.Fatal("graph must not be deleted")
	}
}

func TestTeamGraphGuard_DeleteExternallyLinkedGraphRejected(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(&GraphDefinition{ID: "g-2", Name: "shared-graph", Version: 1})
	linked := &fakeLinkedGraphReader{refs: []Team{
		{ID: "team-1", TeamKey: "t1", DisplayName: "Team One"},
		{ID: "team-2", TeamKey: "t2", DisplayName: "Team Two"},
	}}
	teamUC := newGuardTeamUsecase(&stubTeamReader{}, &recTeamWriter{}, &stubTeamRunReader{}, linked, nil)
	defUC := newGuardDefUC(store, teamUC)

	err := defUC.DeleteGraph(context.Background(), "g-2")
	if err == nil {
		t.Fatal("expected delete rejection for externally linked graph")
	}
	e, ok := apierror.From(err)
	if !ok || e.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict, got %v", err)
	}
	if linked.queried != "g-2" {
		t.Fatalf("linked reader queried %q", linked.queried)
	}
	if len(store.deleted) != 0 {
		t.Fatal("graph must not be deleted")
	}
}

func TestTeamGraphGuard_DeleteOrphanOwnedGraphAllowed(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-gone"))
	// 属主已不存在 + 无 external 引用 → 允许删除（孤儿清理）。
	teamUC := newGuardTeamUsecase(&stubTeamReader{}, &recTeamWriter{}, &stubTeamRunReader{},
		&fakeLinkedGraphReader{}, nil)
	defUC := newGuardDefUC(store, teamUC)

	if err := defUC.DeleteGraph(context.Background(), "g-1"); err != nil {
		t.Fatalf("DeleteGraph: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "g-1" {
		t.Fatalf("deleted = %v", store.deleted)
	}
}

func TestTeamGraphGuard_DeleteUnreferencedIndependentGraphAllowed(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(&GraphDefinition{ID: "g-3", Name: "free", Version: 1})
	teamUC := newGuardTeamUsecase(&stubTeamReader{}, &recTeamWriter{}, &stubTeamRunReader{},
		&fakeLinkedGraphReader{}, nil)
	defUC := newGuardDefUC(store, teamUC)

	if err := defUC.DeleteGraph(context.Background(), "g-3"); err != nil {
		t.Fatalf("DeleteGraph: %v", err)
	}
}

func TestTeamGraphGuard_DeleteOwnedGraphBypassesProtection(t *testing.T) {
	t.Parallel()
	store := newFakeGraphStore()
	store.addExisting(ownedGraphDef("g-1", "team-1"))
	team := Team{ID: "team-1", TeamKey: "t1", DisplayName: "Team One", Kind: "user"}
	teamUC := newGuardTeamUsecase(&stubTeamReader{team: team}, &recTeamWriter{}, &stubTeamRunReader{}, nil, nil)
	defUC := newGuardDefUC(store, teamUC)

	// B5/D2 级联路径：属主 team 存在也可删除（归属校验已由调用方完成）。
	if err := defUC.DeleteOwnedGraph(context.Background(), "g-1"); err != nil {
		t.Fatalf("DeleteOwnedGraph: %v", err)
	}
	if len(store.deleted) != 1 {
		t.Fatalf("deleted = %v", store.deleted)
	}
}

// --- DeriveMembersFromGraphNodes 共享函数 ---

func TestDeriveMembersFromGraphNodes(t *testing.T) {
	t.Parallel()
	nodes := []NodeDef{
		{ID: "start", Type: "start"},
		{ID: "n1", Type: "agent", AgentName: "key-a", Description: "Alpha", Instruction: "do a", RequiredRole: RoleCoordinator},
		{ID: "n2", Type: "function", FuncRef: "fn"},
		{ID: "n3", Type: "agent", AgentName: "key-b"},
		{ID: "n4", Type: "agent", AgentName: "key-missing"},
		{ID: "n5", Type: "agent"}, // 无 AgentName → 跳过
	}
	resolve := func(key string) (string, bool) {
		switch key {
		case "key-a":
			return "id-a", true
		case "key-b":
			return "id-b", true
		default:
			return "", false
		}
	}
	members, skipped := DeriveMembersFromGraphNodes(nodes, resolve)
	if len(members) != 2 {
		t.Fatalf("members = %+v", members)
	}
	if members[0].AgentID != "id-a" || members[0].Role != RoleCoordinator || members[0].SortOrder != 1 {
		t.Fatalf("members[0] = %+v", members[0])
	}
	if members[1].AgentID != "id-b" || members[1].Role != RoleWorker || members[1].Name != "Agent" {
		t.Fatalf("members[1] = %+v", members[1])
	}
	if len(skipped) != 1 || skipped[0] != "key-missing" {
		t.Fatalf("skipped = %v", skipped)
	}
	// nil resolver：key 原样保留。
	members, skipped = DeriveMembersFromGraphNodes(nodes, nil)
	if len(members) != 3 || len(skipped) != 0 {
		t.Fatalf("nil resolver: members=%v skipped=%v", members, skipped)
	}
	if members[0].AgentID != "key-a" {
		t.Fatalf("nil resolver member[0].AgentID = %q", members[0].AgentID)
	}
}

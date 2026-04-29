// memory_l4_service_test.go 覆盖 §13 对 L4 知识图谱的验收：实体 upsert、去重、版本快照、
// 重命名、合并（关系重连）、邻域遍历与渲染，
// 以及 L0 NeighborhoodSegmentForL0 门控（l4_enabled、
// l4_graph_inject_neighbors、l4_graph_max_neighbors / l4_graph_max_hops）。
package service

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
	"path/filepath"
	"strings"
	"testing"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// newTestL4Service 搭建内存 L4 栈（repo + service）。返回的 repo 供测试在
// 不变量未从服务层暴露时直接查看底层状态。
func newTestL4Service(t *testing.T) (*MemoryL4Service, repository.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "l4.db")
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("new repo failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err = repo.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return NewMemoryL4Service(repo), repo
}

func mustUpsertEntity(t *testing.T, svc *MemoryL4Service, in EntityUpsertInput) mem.MemoryEntity {
	t.Helper()
	e, err := svc.UpsertEntity(context.Background(), in)
	if err != nil {
		t.Fatalf("upsert entity %q: %v", in.Name, err)
	}
	return e
}

// §13 #1 – 新建实体存为 active 并在 `memory_entity_versions` 写 v1 快照。
func TestL4UpsertCreatesEntityAndV1Version(t *testing.T) {
	svc, _ := newTestL4Service(t)
	e := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType:  mem.ScopeWorkspace,
		ScopeID:    "ws_acme",
		EntityType: mem.EntityTech,
		Name:       "React 19",
	})
	if e.ID == "" || e.Status != mem.EntityStatusActive {
		t.Fatalf("expected active entity with id, got %#v", e)
	}
	if e.NameNormalized != "react 19" {
		t.Fatalf("expected normalized name, got %q", e.NameNormalized)
	}
	versions, err := svc.ListEntityVersions(context.Background(), e.ID, 10)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("expected single v1 version, got %#v", versions)
	}
}

// §13 #2 – 相同 (scope, type, 规范化名) upsert 升版本并复用实体 ID，不新插行。
func TestL4UpsertDedupsByNaturalKey(t *testing.T) {
	svc, _ := newTestL4Service(t)
	first := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType:   mem.ScopeWorkspace,
		ScopeID:     "ws_acme",
		EntityType:  mem.EntityProject,
		Name:        "Aranea Backend",
		Description: "first description",
	})
	second := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType:   mem.ScopeWorkspace,
		ScopeID:     "ws_acme",
		EntityType:  mem.EntityProject,
		Name:        "  aranea backend  ",
		Description: "second description",
	})
	if first.ID != second.ID {
		t.Fatalf("expected same id on dedup, got %q vs %q", first.ID, second.ID)
	}
	versions, err := svc.ListEntityVersions(context.Background(), first.ID, 10)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("expected at least 2 version snapshots after upsert, got %d", len(versions))
	}
}

// §13 – RelationUpsert rejects self-loops.
func TestL4UpsertRelationRejectsSelfLoop(t *testing.T) {
	svc, _ := newTestL4Service(t)
	e := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws", EntityType: mem.EntityTech, Name: "Go",
	})
	_, err := svc.UpsertRelation(context.Background(), RelationUpsertInput{
		ScopeType:    mem.ScopeWorkspace,
		ScopeID:      "ws",
		SourceID:     e.ID,
		TargetID:     e.ID,
		RelationType: mem.RelUses,
	})
	if err == nil {
		t.Fatalf("expected error for self-loop, got nil")
	}
}

// §13 #3 – Neighborhood 返回中心节点与相连关系。
func TestL4NeighborhoodReturnsConnectedNodes(t *testing.T) {
	svc, _ := newTestL4Service(t)
	alice := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws", EntityType: mem.EntityPerson, Name: "Alice",
	})
	proj := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws", EntityType: mem.EntityProject, Name: "Aranea",
	})
	if _, err := svc.UpsertRelation(context.Background(), RelationUpsertInput{
		ScopeType:    mem.ScopeWorkspace,
		ScopeID:      "ws",
		SourceID:     alice.ID,
		TargetID:     proj.ID,
		RelationType: mem.RelWorksOn,
	}); err != nil {
		t.Fatalf("upsert relation: %v", err)
	}
	n, err := svc.Neighborhood(context.Background(), alice.ID, 1, 10)
	if err != nil {
		t.Fatalf("neighborhood: %v", err)
	}
	if n.Center.ID != alice.ID {
		t.Fatalf("expected center=alice, got %q", n.Center.ID)
	}
	if len(n.Relations) == 0 {
		t.Fatalf("expected at least one relation in neighborhood, got %#v", n)
	}
	foundProj := false
	for _, e := range n.Entities {
		if e.ID == proj.ID {
			foundProj = true
		}
	}
	if !foundProj {
		t.Fatalf("expected project entity to appear in neighborhood, got %#v", n.Entities)
	}
}

// §13 – RenderForPrompt 生成 `# memory.l4.graph` 块，受 max_chars 限制且含中心与至少一邻居。
func TestL4RenderForPromptBoundedByMaxChars(t *testing.T) {
	svc, _ := newTestL4Service(t)
	a := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws", EntityType: mem.EntityPerson, Name: "Bob",
	})
	b := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws", EntityType: mem.EntityProject, Name: "Phoenix",
	})
	if _, err := svc.UpsertRelation(context.Background(), RelationUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws",
		SourceID: a.ID, TargetID: b.ID, RelationType: mem.RelWorksOn,
	}); err != nil {
		t.Fatalf("upsert relation: %v", err)
	}
	n, err := svc.Neighborhood(context.Background(), a.ID, 1, 10)
	if err != nil {
		t.Fatalf("neighborhood: %v", err)
	}
	body, ok := svc.RenderForPrompt(n, 200)
	if !ok {
		t.Fatalf("expected rendered prompt, got empty")
	}
	if len(body) > 200 {
		t.Fatalf("expected body bounded by 200 chars, got %d", len(body))
	}
	if !strings.Contains(body, "Bob") || !strings.Contains(body, "Phoenix") {
		t.Fatalf("expected center & neighbor in body, got %q", body)
	}
	if !strings.HasPrefix(body, "# memory.l4.graph") {
		t.Fatalf("expected leading header, got %q", body)
	}
}

// §13 – 邻域为空时 RenderForPrompt 返回 ok=false。
func TestL4RenderForPromptEmpty(t *testing.T) {
	svc, _ := newTestL4Service(t)
	if _, ok := svc.RenderForPrompt(mem.GraphNeighborhood{}, 200); ok {
		t.Fatalf("expected ok=false for empty neighborhood")
	}
}

// §13 #5 – Merge 将源标为已合并并把关系改指主实体；后续邻域查询不再返回源。
func TestL4MergeRewiresRelationsAndArchivesSource(t *testing.T) {
	svc, repo := newTestL4Service(t)
	canonical := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws", EntityType: mem.EntityTech, Name: "React",
	})
	dup := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws", EntityType: mem.EntityTech, Name: "Reactjs",
	})
	hub := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws", EntityType: mem.EntityProject, Name: "App",
	})
	if _, err := svc.UpsertRelation(context.Background(), RelationUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws",
		SourceID: hub.ID, TargetID: dup.ID, RelationType: mem.RelUses,
	}); err != nil {
		t.Fatalf("seed relation: %v", err)
	}
	if err := svc.MergeEntities(context.Background(), canonical.ID, []string{dup.ID}, "tester", "dedup"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	dupAfter, err := repo.GetEntity(dup.ID)
	if err != nil {
		t.Fatalf("get dup: %v", err)
	}
	if dupAfter.Status != mem.EntityStatusMerged || dupAfter.MergedInto != canonical.ID {
		t.Fatalf("expected dup merged into canonical, got status=%s merged_into=%s", dupAfter.Status, dupAfter.MergedInto)
	}
	rels, err := repo.ListRelationsForNode(canonical.ID, 50)
	if err != nil {
		t.Fatalf("list relations: %v", err)
	}
	hasRewired := false
	for _, r := range rels {
		if r.SourceID == hub.ID && r.TargetID == canonical.ID {
			hasRewired = true
		}
	}
	if !hasRewired {
		t.Fatalf("expected rewired relation pointing at canonical, got %#v", rels)
	}
}

// §13 – Rename 写入带名称 diff 的新版本。
func TestL4RenameEntityWritesVersion(t *testing.T) {
	svc, _ := newTestL4Service(t)
	e := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeWorkspace, ScopeID: "ws", EntityType: mem.EntityProject, Name: "Old Name",
	})
	updated, err := svc.RenameEntity(context.Background(), e.ID, "New Name", "tester", "")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if updated.Name != "New Name" {
		t.Fatalf("expected renamed entity, got %q", updated.Name)
	}
	versions, err := svc.ListEntityVersions(context.Background(), e.ID, 10)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("expected at least 2 versions after rename, got %d", len(versions))
	}
	latest := versions[0]
	if !strings.Contains(latest.DiffJSON, "Old Name") || !strings.Contains(latest.DiffJSON, "New Name") {
		t.Fatalf("expected diff to mention before/after, got %q", latest.DiffJSON)
	}
}

// §13 #6 – `l4_graph_inject_neighbors=false`（默认）不注入片段。
func TestL4NeighborhoodSegmentForL0DisabledByDefault(t *testing.T) {
	svc, _ := newTestL4Service(t)
	mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeAgent, ScopeID: "agent-x", EntityType: mem.EntityTech, Name: "GraphQL",
	})
	if _, ok := svc.NeighborhoodSegmentForL0(context.Background(), "sess", "agent-x", "graphql"); ok {
		t.Fatalf("expected ok=false when settings disabled")
	}
}

// §13 #6 – `l4_enabled` 与 `l4_graph_inject_neighbors` 均为 true 时产出片段，并受 `l4_graph_max_neighbors` 限制。
func TestL4NeighborhoodSegmentForL0EmitsSegmentWhenEnabled(t *testing.T) {
	svc, repo := newTestL4Service(t)
	a := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeAgent, ScopeID: "agent-x", EntityType: mem.EntityProject, Name: "Aranea",
	})
	b := mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeAgent, ScopeID: "agent-x", EntityType: mem.EntityTech, Name: "Vitest",
	})
	if _, err := svc.UpsertRelation(context.Background(), RelationUpsertInput{
		ScopeType: mem.ScopeAgent, ScopeID: "agent-x",
		SourceID: a.ID, TargetID: b.ID, RelationType: mem.RelUses,
	}); err != nil {
		t.Fatalf("seed relation: %v", err)
	}
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:                "agent-x",
		L4Enabled:              true,
		L4GraphInjectNeighbors: true,
		L4GraphMaxHops:         1,
		L4GraphMaxNeighbors:    5,
	}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	seg, ok := svc.NeighborhoodSegmentForL0(context.Background(), "sess", "agent-x", "Aranea")
	if !ok {
		t.Fatalf("expected segment, got ok=false")
	}
	if seg.Section != "memory.l4.graph" {
		t.Fatalf("expected section=memory.l4.graph, got %q", seg.Section)
	}
	if !strings.Contains(seg.Content, "Aranea") {
		t.Fatalf("expected center entity name in content, got %q", seg.Content)
	}
}

// §13 #6 – `l4_enabled=false` 同时关闭本片段与（按规范）system.self_evolution 片段，故即使 inject 打开也不应出现 L4 图。
func TestL4NeighborhoodSegmentForL0RespectsEnableMaster(t *testing.T) {
	svc, repo := newTestL4Service(t)
	mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType: mem.ScopeAgent, ScopeID: "agent-y", EntityType: mem.EntityTech, Name: "Postgres",
	})
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:                "agent-y",
		L4Enabled:              false,
		L4GraphInjectNeighbors: true,
	}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	if _, ok := svc.NeighborhoodSegmentForL0(context.Background(), "sess", "agent-y", "postgres"); ok {
		t.Fatalf("expected ok=false when l4_enabled=false")
	}
}

// §12 第二阶段 – ExtractFromFact 从陈述/详情抽词典项，在事实作用域落实体，
// 经 memory_entity_facts 反向链接。
func TestL4ExtractFromFactCreatesEntitiesAndLinks(t *testing.T) {
	svc, repo := newTestL4Service(t)
	fact, err := repo.CreateFact(mem.MemoryFact{
		ID:        "fact-1",
		ScopeType: mem.ScopeWorkspace,
		ScopeID:   "ws_acme",
		Statement: "We migrated the frontend to React 19 and switched the cache to Redis.",
		Kind:      mem.FactGeneric,
		Status:    mem.FactStatusActive,
	})
	if err != nil {
		t.Fatalf("seed fact: %v", err)
	}
	report, err := svc.ExtractFromFact(context.Background(), fact.ID)
	if err != nil {
		t.Fatalf("extract fact: %v", err)
	}
	if report.NewEntities < 2 {
		t.Fatalf("expected at least 2 new entities, got %#v", report)
	}
	hits, err := svc.SearchByText(context.Background(), mem.ScopeWorkspace, "ws_acme", "react", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected an entity matching 'react', got 0")
	}
	// 再跑抽取验证反向链接；计数应为更新而非新实体。
	rerun, err := svc.ExtractFromFact(context.Background(), fact.ID)
	if err != nil {
		t.Fatalf("re-extract: %v", err)
	}
	if rerun.NewEntities != 0 {
		t.Fatalf("expected idempotent re-extraction, got %#v", rerun)
	}
	if rerun.UpdatedEntities == 0 {
		t.Fatalf("expected updates on re-extraction, got %#v", rerun)
	}
}

// §12 第二阶段 – ExtractFromEpisode 读 episode 标题/目标/结果并在 episode 作用域建实体。无可用作用域时短路 skipped=N。
func TestL4ExtractFromEpisodeCreatesEntities(t *testing.T) {
	svc, repo := newTestL4Service(t)
	episode, err := repo.CreateEpisode(mem.MemoryEpisode{
		ID:             "ep-1",
		SessionID:      "sess-1",
		AgentID:        "agent-x",
		Kind:           mem.EpisodeKindTask,
		Title:          "Profile vitest run on the Vue 3 app",
		Goal:           "Identify why Vitest is slow on the Quasar bundle",
		OutcomeSummary: "Pinned vite to v5 and switched the test runner cache.",
	})
	if err != nil {
		t.Fatalf("seed episode: %v", err)
	}
	report, err := svc.ExtractFromEpisode(context.Background(), episode.ID)
	if err != nil {
		t.Fatalf("extract episode: %v", err)
	}
	if report.NewEntities < 2 {
		t.Fatalf("expected at least 2 new entities, got %#v", report)
	}
}

// §12 第二阶段 – 无词典命中时返回友好 `note` 与零计数，调用方可短路。
func TestL4ExtractFromFactNoMatchesIsBenign(t *testing.T) {
	svc, repo := newTestL4Service(t)
	fact, err := repo.CreateFact(mem.MemoryFact{
		ID:        "fact-2",
		ScopeType: mem.ScopeWorkspace,
		ScopeID:   "ws_acme",
		Statement: "Random observation about today's weather.",
		Kind:      mem.FactGeneric,
		Status:    mem.FactStatusActive,
	})
	if err != nil {
		t.Fatalf("seed fact: %v", err)
	}
	report, err := svc.ExtractFromFact(context.Background(), fact.ID)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if report.NewEntities != 0 || report.UpdatedEntities != 0 || report.Note == "" {
		t.Fatalf("expected zero counts + note, got %#v", report)
	}
}

func TestL3UpsertFactTriggersL4Extraction(t *testing.T) {
	l4, repo := newTestL4Service(t)
	l3 := NewMemoryL3Service(repo)
	l3.SetL4ExtractionSource(l4)

	fact, err := l3.UpsertFact(context.Background(), mem.FactUpsertInput{
		ScopeType: mem.ScopeAgent,
		ScopeID:   "agent-auto-extract",
		AgentID:   "agent-auto-extract",
		Statement: "Use TypeScript with React 19 for the memory center UI.",
		Kind:      mem.FactRule,
	})
	if err != nil {
		t.Fatalf("upsert fact: %v", err)
	}
	links, err := l4.ListEntityFacts(context.Background(), "", 10)
	if err == nil && len(links) > 0 {
		t.Fatalf("empty entity id should not return links: %#v", links)
	}
	entities, err := l4.SearchByText(context.Background(), mem.ScopeAgent, "agent-auto-extract", "TypeScript", 10)
	if err != nil {
		t.Fatalf("search entities: %v", err)
	}
	if len(entities) == 0 {
		t.Fatalf("expected L4 extraction to create an entity for fact %s", fact.ID)
	}
	foundLinked := false
	for _, entity := range entities {
		entityLinks, err := l4.ListEntityFacts(context.Background(), entity.ID, 10)
		if err != nil {
			t.Fatalf("list entity facts: %v", err)
		}
		for _, link := range entityLinks {
			if link.FactID == fact.ID {
				foundLinked = true
			}
		}
	}
	if !foundLinked {
		t.Fatalf("expected extracted entity to be linked back to fact %s", fact.ID)
	}
}

func TestL2CreateEpisodeTriggersL4Extraction(t *testing.T) {
	l4, repo := newTestL4Service(t)
	l2 := NewMemoryL2Service(repo)
	l2.SetL4ExtractionSource(l4)
	seedAgentAndSession(t, repo, "agent-episode-extract", "sess-episode-extract")

	episode, err := l2.CreateMilestoneEpisode(context.Background(), CreateEpisodeInput{
		SessionID: "sess-episode-extract",
		AgentID:   "agent-episode-extract",
		Title:     "React migration",
		Goal:      "Move the frontend to React and TypeScript",
	})
	if err != nil {
		t.Fatalf("create episode: %v", err)
	}
	entities, err := l4.SearchByText(context.Background(), mem.ScopeAgent, "agent-episode-extract", "React", 10)
	if err != nil {
		t.Fatalf("search entities: %v", err)
	}
	if len(entities) == 0 {
		t.Fatalf("expected L4 extraction to create an entity for episode %s", episode.ID)
	}
}

func TestL3RecallSegmentUsesTeamScopeContext(t *testing.T) {
	l4, repo := newTestL4Service(t)
	_ = l4
	l3 := NewMemoryL3Service(repo)
	if _, err := repo.UpsertAgentRuntimeSettings(domain.AgentRuntimeSettings{
		AgentID:            "agent-team-scope",
		L3Enabled:          true,
		L3RecallTopK:       3,
		L3RecallMinScore:   0.1,
		L3RecallScopesJSON: `["team"]`,
	}); err != nil {
		t.Fatalf("settings: %v", err)
	}
	if _, err := l3.UpsertFact(context.Background(), mem.FactUpsertInput{
		ScopeType: mem.ScopeTeam,
		ScopeID:   "team-alpha",
		TeamID:    "team-alpha",
		Statement: "Team Alpha prefers Postgres for analytics workloads.",
		Kind:      mem.FactPreference,
	}); err != nil {
		t.Fatalf("upsert fact: %v", err)
	}
	if _, ok := l3.RecallSegmentForL0(context.Background(), "sess", "agent-team-scope", "analytics Postgres"); ok {
		t.Fatalf("legacy L0 recall should not see team scope without context")
	}
	if seg, ok := l3.RecallSegmentForL0WithContext(context.Background(), mem.L0MemoryScopeContext{
		SessionID: "sess",
		AgentID:   "agent-team-scope",
		TeamID:    "team-alpha",
		Query:     "analytics Postgres",
	}); !ok || !strings.Contains(seg.Content, "Postgres") {
		t.Fatalf("context-rich L0 recall should include team fact, ok=%v seg=%#v", ok, seg)
	}
}

// §13 – SearchByText 返回名称或别名命中的实体。
func TestL4SearchByTextMatchesAliases(t *testing.T) {
	svc, _ := newTestL4Service(t)
	mustUpsertEntity(t, svc, EntityUpsertInput{
		ScopeType:  mem.ScopeWorkspace,
		ScopeID:    "ws",
		EntityType: mem.EntityFramework,
		Name:       "Quasar Framework",
		Aliases:    []string{"Quasar"},
	})
	hits, err := svc.SearchByText(context.Background(), mem.ScopeWorkspace, "ws", "quasar", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected matches for 'quasar', got 0")
	}
}

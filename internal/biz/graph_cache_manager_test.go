package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── stubs ───────────────────────────────────────────────────────────────────

type stubCompiledTeamRepo struct {
	ct         *CompiledTeam
	err        error
	gotTeamID  string
	gotGraphID string
	calls      int
}

func (r *stubCompiledTeamRepo) Save(context.Context, string, string, string, *CompiledTeam) error {
	return nil
}
func (r *stubCompiledTeamRepo) Load(_ context.Context, teamID, graphID string) (*CompiledTeam, error) {
	return r.ct, r.err
}
func (r *stubCompiledTeamRepo) LoadForSession(_ context.Context, teamID, graphID, _ string) (*CompiledTeam, error) {
	r.calls++
	r.gotTeamID = teamID
	r.gotGraphID = graphID
	return r.ct, r.err
}
func (r *stubCompiledTeamRepo) Delete(context.Context, string, string) error { return nil }

type stubGraphDefProvider struct {
	def *GraphDefinition
	err error
}

func (p *stubGraphDefProvider) GetGraph(context.Context, string) (*GraphDefinition, error) {
	return p.def, p.err
}

type stubTeamDefProvider struct {
	json string
	err  error
}

func (p *stubTeamDefProvider) GetTeamDefinitionJSON(context.Context, string) (string, error) {
	return p.json, p.err
}

func newCacheTestExec(id, graphID string) *GraphExecution {
	return NewGraphExecution(context.Background(), id, graphID, "sess-1", string(GraphExecRunning))
}

func newCacheManager(repo CompiledTeamRepo, defProv GraphDefinitionProvider, teamProv TeamDefinitionJSONProvider) *GraphCacheManager {
	return NewGraphCacheManager(repo, defProv, teamProv, loggateway.NewNoop())
}

// ── F2: legacy composite graph id ───────────────────────────────────────────

func TestCompiledTeamIDFromGraphID(t *testing.T) {
	cases := map[string]string{
		"team:t-1":            "t-1",
		"team:t-1:run-9":      "t-1",
		"9cba5f30-uuid-asset": "", // linked asset id carries no inline team
		"team:":               "",
		"":                    "",
	}
	for in, want := range cases {
		if got := compiledTeamIDFromGraphID(in); got != want {
			t.Errorf("compiledTeamIDFromGraphID(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestBuildConfigForExecution_LegacyCompositeGraphID_UsesInlineTeamID(t *testing.T) {
	ct := NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil)
	repo := &stubCompiledTeamRepo{ct: ct}
	cm := newCacheManager(repo, nil, nil)

	exec := newCacheTestExec("exec-1", "team:t-1:run-9")
	got, err := cm.BuildConfigForExecution(context.Background(), exec)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != ct {
		t.Fatal("expected the persisted compiled team")
	}
	if repo.gotTeamID != "t-1" {
		t.Fatalf("LoadForSession teamID=%q, want %q (run id must not leak into the team id)", repo.gotTeamID, "t-1")
	}
	if repo.gotGraphID != "team:t-1:run-9" {
		t.Fatalf("LoadForSession graphID=%q, want the full composite id", repo.gotGraphID)
	}
}

// ── F1: stale definition hash must fall through to re-resolve ───────────────

func TestBuildConfigForExecution_StaleHash_ResolvesFromDefinition(t *testing.T) {
	stale := NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil)
	stale.DefinitionHash = ComputeDefinitionHash("old-definition-json")
	repo := &stubCompiledTeamRepo{ct: stale}
	teamProv := &stubTeamDefProvider{json: "new-definition-json"} // hash differs
	def := &GraphDefinition{ID: "team:t-1", Name: "rebuilt"}
	cm := newCacheManager(repo, &stubGraphDefProvider{def: def}, teamProv)

	exec := newCacheTestExec("exec-1", "team:t-1")
	got, err := cm.BuildConfigForExecution(context.Background(), exec)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got == stale {
		t.Fatal("stale compiled team returned despite hash mismatch (F1 regression)")
	}
	if got.DefinitionHash != "" {
		t.Fatalf("definition-resolved team must not inherit the stale hash, got %q", got.DefinitionHash)
	}
	cached, ok := cm.GetTeamBuildConfig(exec.ID)
	if !ok || cached != got {
		t.Fatal("re-resolved team must be cached to avoid repeated repo-load/rebuild churn (终审-7)")
	}
	if cached == stale {
		t.Fatal("stale entry must never be cached")
	}
}

func TestBuildConfigForExecution_StaleHashUnresolvableTeam_Conflict(t *testing.T) {
	// 终审-1：合成 team id 的执行在定义不可解析（团队被删/GetGraph 报错）时，
	// 不得泄露底层 NotFound——返回确定性 Conflict，与 Y4 恢复一致性守卫同语义。
	stale := NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil)
	stale.DefinitionHash = ComputeDefinitionHash("old-definition-json")
	repo := &stubCompiledTeamRepo{ct: stale}
	teamProv := &stubTeamDefProvider{json: "new-definition-json"}
	cm := newCacheManager(repo, &stubGraphDefProvider{err: errors.New("definition not found")}, teamProv)

	exec := newCacheTestExec("exec-1", "team:t-1:run-9")
	_, err := cm.BuildConfigForExecution(context.Background(), exec)
	if err == nil {
		t.Fatal("expected Conflict for an unresolvable team definition")
	}
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("err=%v (%T), want apierror Conflict", err, err)
	}
}

func TestBuildConfigForExecution_FreshHash_UsesPersistedTeam(t *testing.T) {
	ct := NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil)
	ct.DefinitionHash = ComputeDefinitionHash("current-json")
	repo := &stubCompiledTeamRepo{ct: ct}
	teamProv := &stubTeamDefProvider{json: "current-json"}
	cm := newCacheManager(repo, nil, teamProv)

	exec := newCacheTestExec("exec-1", "team:t-1")
	got, err := cm.BuildConfigForExecution(context.Background(), exec)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != ct {
		t.Fatal("expected persisted compiled team on fresh hash")
	}
	if cached, ok := cm.GetTeamBuildConfig(exec.ID); !ok || cached != ct {
		t.Fatal("fresh entry should be cached")
	}
}

func TestBuildConfigForExecution_HashCheckUnavailable_UsesCached(t *testing.T) {
	ct := NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil)
	ct.DefinitionHash = "anything"
	repo := &stubCompiledTeamRepo{ct: ct}
	teamProv := &stubTeamDefProvider{err: errors.New("team def gone")}
	cm := newCacheManager(repo, nil, teamProv)

	exec := newCacheTestExec("exec-1", "team:t-1")
	got, err := cm.BuildConfigForExecution(context.Background(), exec)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != ct {
		t.Fatal("degraded hash check should use the cached entry as-is")
	}
}

// ── F3: linked assets derive the team from the definition ───────────────────

func TestBuildConfigForExecution_LinkedGraph_DerivesTeamFromDefinition(t *testing.T) {
	ct := NewCompiledTeam(GraphBuildConfig{}, nil, nil, nil)
	repo := &stubCompiledTeamRepo{ct: ct}
	def := &GraphDefinition{ID: "asset-1", TeamID: "t-9"}
	cm := newCacheManager(repo, &stubGraphDefProvider{def: def}, nil)

	exec := newCacheTestExec("exec-1", "asset-1")
	got, err := cm.BuildConfigForExecution(context.Background(), exec)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != ct {
		t.Fatal("expected persisted compiled team for the owned linked graph")
	}
	if repo.gotTeamID != "t-9" {
		t.Fatalf("LoadForSession teamID=%q, want the definition's owner team %q", repo.gotTeamID, "t-9")
	}
}

func TestBuildConfigForExecution_ExternalLinkedGraph_ResolvesFromDefinition(t *testing.T) {
	// Externally-linked graphs (no owner team) have no readable compiled_teams
	// row — repo is skipped and the definition re-resolves.
	repo := &stubCompiledTeamRepo{ct: nil, err: errors.New("no row")}
	def := &GraphDefinition{ID: "asset-ext"}
	cm := newCacheManager(repo, &stubGraphDefProvider{def: def}, nil)

	exec := newCacheTestExec("exec-1", "asset-ext")
	got, err := cm.BuildConfigForExecution(context.Background(), exec)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got == nil {
		t.Fatal("expected a definition-resolved compiled team")
	}
}

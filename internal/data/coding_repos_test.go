package data

import (
	"context"
	"testing"

	bridge "aranea-agents/internal/biz/agentbridge"
	"aranea-agents/pkg/apierror"
)

// ---------- AgentRepo ----------

func TestCodingAgentRepo_UpsertPreservesIdentity(t *testing.T) {
	d := newTestDataPG(t)
	r := NewCodingAgentRepo(d)
	ctx := context.Background()

	first := &bridge.CodingAgent{
		Workspace:   "default",
		AgentKey:    "codebuddy",
		DisplayName: "CodeBuddy",
		Command:     "codebuddy",
		Args:        []string{"--acp"},
		Env:         map[string]string{"FOO": "bar"},
		Enabled:     true,
	}
	if err := r.Upsert(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.ID == "" {
		t.Fatal("expected generated id after upsert")
	}
	if first.CreatedAt == "" || first.UpdatedAt == "" {
		t.Fatalf("expected timestamps populated: %+v", first)
	}

	// Re-upsert same (workspace, agent_key): identity preserved, mutable fields updated.
	second := &bridge.CodingAgent{
		Workspace:   "default",
		AgentKey:    "codebuddy",
		DisplayName: "CodeBuddy v2",
		Command:     "codebuddy2",
		Enabled:     false,
	}
	if err := r.Upsert(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("re-upsert changed id: %q -> %q", first.ID, second.ID)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("re-upsert changed created_at: %q -> %q", first.CreatedAt, second.CreatedAt)
	}

	got, err := r.GetByKey(ctx, "default", "codebuddy")
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if got.DisplayName != "CodeBuddy v2" || got.Command != "codebuddy2" || got.Enabled {
		t.Fatalf("mutable fields not updated: %+v", got)
	}
	if len(got.Args) != 1 || got.Args[0] != "--acp" {
		t.Fatalf("args lost on upsert (not in update set is fine, but must persist): %+v", got)
	}
}

func TestCodingAgentRepo_GetByKeyNotFoundAndList(t *testing.T) {
	d := newTestDataPG(t)
	r := NewCodingAgentRepo(d)
	ctx := context.Background()

	_, err := r.GetByKey(ctx, "default", "ghost")
	if err == nil {
		t.Fatal("expected NotFound for missing key")
	}
	if apiErrCode(t, err) != apierror.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %v", err)
	}

	for _, key := range []string{"a", "b"} {
		if err := r.Upsert(ctx, &bridge.CodingAgent{
			Workspace: "default", AgentKey: key, DisplayName: key, Command: key,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Upsert(ctx, &bridge.CodingAgent{
		Workspace: "other", AgentKey: "c", DisplayName: "c", Command: "c",
	}); err != nil {
		t.Fatal(err)
	}

	list, err := r.List(ctx, "default")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2 (workspace scoped)", len(list))
	}
}

func TestCodingAgentRepo_UpdateProbe(t *testing.T) {
	d := newTestDataPG(t)
	r := NewCodingAgentRepo(d)
	ctx := context.Background()

	a := &bridge.CodingAgent{Workspace: "default", AgentKey: "codex", DisplayName: "Codex", Command: "npx"}
	if err := r.Upsert(ctx, a); err != nil {
		t.Fatal(err)
	}
	if a.LastProbeOK {
		t.Fatal("default last_probe_ok must be false")
	}

	if err := r.UpdateProbe(ctx, a.ID, true, ""); err != nil {
		t.Fatalf("UpdateProbe ok: %v", err)
	}
	got, err := r.GetByKey(ctx, "default", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastProbeOK || got.LastProbeError != "" {
		t.Fatalf("probe not updated: %+v", got)
	}

	if err := r.UpdateProbe(ctx, a.ID, false, "spawn: executable not found"); err != nil {
		t.Fatalf("UpdateProbe fail: %v", err)
	}
	got, _ = r.GetByKey(ctx, "default", "codex")
	if got.LastProbeOK || got.LastProbeError == "" {
		t.Fatalf("probe failure not recorded: %+v", got)
	}

	if err := r.UpdateProbe(ctx, "missing-id", true, ""); err == nil {
		t.Fatal("expected NotFound updating probe of missing id")
	}
}

// ---------- ProjectRepo ----------

func TestCodingProjectRepo_UpsertGetListDelete(t *testing.T) {
	d := newTestDataPG(t)
	r := NewCodingProjectRepo(d)
	ctx := context.Background()

	p := &bridge.CodingProject{
		Workspace: "default", Name: "aranea-agents", Path: `F:\aranea-agents`, Description: "主仓",
	}
	if err := r.Upsert(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected generated id")
	}

	got, err := r.GetByName(ctx, "default", "aranea-agents")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Path != `F:\aranea-agents` {
		t.Fatalf("path = %q", got.Path)
	}

	// Re-upsert same name: identity preserved, path updated.
	again := &bridge.CodingProject{Workspace: "default", Name: "aranea-agents", Path: `F:\moved`}
	if err := r.Upsert(ctx, again); err != nil {
		t.Fatal(err)
	}
	if again.ID != p.ID {
		t.Fatalf("re-upsert changed id: %q -> %q", p.ID, again.ID)
	}

	if _, err := r.GetByName(ctx, "default", "ghost"); err == nil {
		t.Fatal("expected NotFound for missing name")
	}

	list, err := r.List(ctx, "default")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v/%v, want 1 item", list, err)
	}

	if err := r.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetByName(ctx, "default", "aranea-agents"); err == nil {
		t.Fatal("expected NotFound after delete")
	}
	if err := r.Delete(ctx, p.ID); err == nil {
		t.Fatal("expected NotFound deleting twice")
	}
}

func TestCodingProjectRepo_MatchRanking(t *testing.T) {
	d := newTestDataPG(t)
	r := NewCodingProjectRepo(d)
	ctx := context.Background()

	seed := []struct{ name, path string }{
		{"aranea-agents", `F:\aranea-agents`},
		{"aranea-web", `F:\aranea-web`},
		{"my-aranea-notes", `F:\notes`},
		{"unrelated", `F:\other`},
	}
	for _, s := range seed {
		if err := r.Upsert(ctx, &bridge.CodingProject{
			Workspace: "default", Name: s.name, Path: s.path,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Other workspace must never leak into match results.
	if err := r.Upsert(ctx, &bridge.CodingProject{
		Workspace: "other", Name: "aranea-agents", Path: `F:\other-ws`,
	}); err != nil {
		t.Fatal(err)
	}

	// Exact match wins alone.
	got, err := r.Match(ctx, "default", "aranea-agents")
	if err != nil {
		t.Fatalf("Match exact: %v", err)
	}
	if len(got) != 1 || got[0].Name != "aranea-agents" {
		t.Fatalf("exact match = %+v", got)
	}

	// No exact: prefix matches rank before substring matches.
	got, err = r.Match(ctx, "default", "aranea")
	if err != nil {
		t.Fatalf("Match prefix: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("prefix+substring candidates = %d, want 3 (%+v)", len(got), got)
	}
	if got[0].Name != "aranea-agents" || got[1].Name != "aranea-web" {
		t.Fatalf("prefix matches must rank first: %+v", got)
	}
	if got[2].Name != "my-aranea-notes" {
		t.Fatalf("substring match must rank last: %+v", got)
	}

	// Zero candidates.
	got, err = r.Match(ctx, "default", "nothing-here")
	if err != nil {
		t.Fatalf("Match zero: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("zero candidates = %+v", got)
	}
}

// ---------- TaskRepo ----------

func TestCodingTaskRepo_CreateGetList(t *testing.T) {
	d := newTestDataPG(t)
	r := NewCodingTaskRepo(d)
	ctx := context.Background()

	task := &bridge.CodingTask{
		Workspace: "default", SessionID: "sess-1", AgentID: "a-1", ProjectID: "p-1",
		Prompt: "修复登录页样式", Status: bridge.StatusDispatched,
	}
	if err := r.Create(ctx, task); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.ID == "" || task.CreatedAt == "" {
		t.Fatalf("expected id+created_at populated: %+v", task)
	}

	got, err := r.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != bridge.StatusDispatched || got.Prompt != "修复登录页样式" {
		t.Fatalf("got = %+v", got)
	}

	if _, err := r.Get(ctx, "missing"); err == nil {
		t.Fatal("expected NotFound for missing task")
	}

	// Second task in another session; ListBySession is session-scoped, newest first.
	task2 := &bridge.CodingTask{
		Workspace: "default", SessionID: "sess-1", AgentID: "a-1", ProjectID: "p-1",
		Prompt: "第二个任务", Status: bridge.StatusRunning,
	}
	if err := r.Create(ctx, task2); err != nil {
		t.Fatal(err)
	}
	task3 := &bridge.CodingTask{
		Workspace: "default", SessionID: "sess-2", AgentID: "a-1", ProjectID: "p-1",
		Prompt: "别的会话", Status: bridge.StatusRunning,
	}
	if err := r.Create(ctx, task3); err != nil {
		t.Fatal(err)
	}

	list, err := r.ListBySession(ctx, "sess-1", 10)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}

	limited, err := r.ListBySession(ctx, "sess-1", 1)
	if err != nil || len(limited) != 1 {
		t.Fatalf("limit = %v/%v, want 1", limited, err)
	}
}

func TestCodingTaskRepo_UpdateStatusCAS(t *testing.T) {
	d := newTestDataPG(t)
	r := NewCodingTaskRepo(d)
	ctx := context.Background()

	task := &bridge.CodingTask{
		Workspace: "default", SessionID: "s", AgentID: "a", ProjectID: "p",
		Prompt: "x", Status: bridge.StatusDispatched,
	}
	if err := r.Create(ctx, task); err != nil {
		t.Fatal(err)
	}

	summary := "完成：改了 3 个文件"
	completedAt := "2026-08-12T10:00:00Z"
	acpSess := "acp-123"
	patch := bridge.TaskPatch{
		ACPSessionID: &acpSess,
		Summary:      &summary,
		CompletedAt:  &completedAt,
	}
	if err := r.UpdateStatus(ctx, task.ID, bridge.StatusDispatched, bridge.StatusRunning, patch); err != nil {
		t.Fatalf("UpdateStatus CAS ok: %v", err)
	}
	got, err := r.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != bridge.StatusRunning || got.Summary != summary ||
		got.CompletedAt != completedAt || got.ACPSessionID != acpSess {
		t.Fatalf("patch not applied: %+v", got)
	}
	if got.UpdatedAt == "" {
		t.Fatal("updated_at must be refreshed")
	}

	// Stale from-state must conflict (lost-update guard).
	err = r.UpdateStatus(ctx, task.ID, bridge.StatusDispatched, bridge.StatusFailed, bridge.TaskPatch{})
	if err == nil {
		t.Fatal("expected conflict on stale from-state")
	}
	if apiErrCode(t, err) != apierror.CodeConflict {
		t.Fatalf("expected CodeConflict, got %v", err)
	}

	// Progress counter patch applies independently.
	n := 7
	if err := r.UpdateStatus(ctx, task.ID, bridge.StatusRunning, bridge.StatusRunning,
		bridge.TaskPatch{ProgressCount: &n}); err != nil {
		t.Fatalf("same-state noop patch: %v", err)
	}
	got, _ = r.Get(ctx, task.ID)
	if got.ProgressCount != 7 || got.Status != bridge.StatusRunning {
		t.Fatalf("progress patch = %+v", got)
	}
}

func TestCodingTaskRepo_ListActive(t *testing.T) {
	d := newTestDataPG(t)
	r := NewCodingTaskRepo(d)
	ctx := context.Background()

	statuses := []bridge.TaskStatus{
		bridge.StatusDispatched, bridge.StatusRunning, bridge.StatusAwaitingApproval,
		bridge.StatusCancelling, bridge.StatusDone, bridge.StatusFailed, bridge.StatusCancelled,
	}
	for i, st := range statuses {
		tk := &bridge.CodingTask{
			Workspace: "default", SessionID: "s", AgentID: "a", ProjectID: "p",
			Prompt: string(rune('a' + i)), Status: st,
		}
		if err := r.Create(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}

	active, err := r.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 4 {
		t.Fatalf("active len = %d, want 4 (non-terminal only)", len(active))
	}
	for _, tk := range active {
		if tk.Status.IsTerminal() {
			t.Fatalf("terminal task leaked into active list: %+v", tk)
		}
	}
}

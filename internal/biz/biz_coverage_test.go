package biz_test

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

// --- Team usecase tests ---

type memTeamRepoB struct {
	items map[string]biz.Team
}

func newMemTeamRepoB() *memTeamRepoB {
	return &memTeamRepoB{items: make(map[string]biz.Team)}
}

func (m *memTeamRepoB) ListTeams(_ context.Context) ([]biz.Team, error) {
	out := make([]biz.Team, 0, len(m.items))
	for _, t := range m.items {
		out = append(out, t)
	}
	return out, nil
}
func (m *memTeamRepoB) ListTeamsByStatus(_ context.Context, status string) ([]biz.Team, error) {
	out := make([]biz.Team, 0)
	for _, t := range m.items {
		if t.Status == status {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *memTeamRepoB) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	t, ok := m.items[id]
	if !ok {
		return biz.Team{}, fmt.Errorf("not found: %s", id)
	}
	return t, nil
}
func (m *memTeamRepoB) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	if t.ID == "" {
		t.ID = fmt.Sprintf("tid-%d", len(m.items)+1)
	}
	m.items[t.ID] = t
	return t, nil
}
func (m *memTeamRepoB) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	m.items[t.ID] = t
	return t, nil
}
func (m *memTeamRepoB) DeleteTeam(_ context.Context, id string) error {
	delete(m.items, id)
	return nil
}
func (m *memTeamRepoB) BatchArchiveTeams(_ context.Context, ids []string) (int, error) {
	n := 0
	for _, id := range ids {
		if t, ok := m.items[id]; ok {
			t.Status = biz.TeamStatusArchived
			m.items[id] = t
			n++
		}
	}
	return n, nil
}
func (m *memTeamRepoB) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (m *memTeamRepoB) GetTeamByKey(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, nil
}
func (m *memTeamRepoB) ListTeamsByDepartmentID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (m *memTeamRepoB) ListTeamsByWorkspace(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (m *memTeamRepoB) ListTeamRuns(_ context.Context, _ string, _ int) ([]biz.TeamRunRecord, error) {
	return nil, nil
}
func (m *memTeamRepoB) ListTeamRunsByTeamIDs(_ context.Context, _ []string, _ int) (map[string][]biz.TeamRunRecord, error) {
	return nil, nil
}
func (m *memTeamRepoB) HasActiveTeamRun(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *memTeamRepoB) GetTeamRunByID(_ context.Context, id string) (biz.TeamRunRecord, error) {
	return biz.TeamRunRecord{}, fmt.Errorf("team run not found: %s", id)
}
func (m *memTeamRepoB) ListTeamRunSteps(_ context.Context, _ string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (m *memTeamRepoB) CreateTeamRun(_ context.Context, r biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	return r, nil
}
func (m *memTeamRepoB) UpdateTeamRun(_ context.Context, _ biz.TeamRunRecord) error { return nil }
func (m *memTeamRepoB) UpdateTeamWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (m *memTeamRepoB) UpdateTeamRunWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (m *memTeamRepoB) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error { return nil }
func (m *memTeamRepoB) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error {
	return nil
}
func (m *memTeamRepoB) UpdateTeamRunTraceID(_ context.Context, _, _ string) error { return nil }
func (m *memTeamRepoB) BatchCreateOrchestrationSteps(_ context.Context, _ []biz.OrchestrationStep) error {
	return nil
}
func (m *memTeamRepoB) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (m *memTeamRepoB) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error {
	return nil
}
func (m *memTeamRepoB) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (m *memTeamRepoB) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}
func (m *memTeamRepoB) CreateTeamRunStep(_ context.Context, s biz.TeamRunStep) (biz.TeamRunStep, error) {
	return s, nil
}

func TestTeamUsecase_CreateAndList(t *testing.T) {
	repo := newMemTeamRepoB()
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()})
	ctx := context.Background()

	team, err := uc.Create(ctx, biz.Team{TeamKey: "alpha", DisplayName: "Alpha Team"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if team.Status != biz.TeamStatusPending {
		t.Errorf("default status should be pending, got %s", team.Status)
	}

	items, err := uc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1, got %d", len(items))
	}
}

func TestTeamUsecase_Create_Validation(t *testing.T) {
	repo := newMemTeamRepoB()
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()})
	ctx := context.Background()

	_, err := uc.Create(ctx, biz.Team{TeamKey: "", DisplayName: "X"})
	if err == nil {
		t.Error("expected error for empty team_key")
	}

	_, err = uc.Create(ctx, biz.Team{TeamKey: "k", DisplayName: ""})
	if err == nil {
		t.Error("expected error for empty display_name")
	}
}

func TestTeamUsecase_Delete_DefaultBlocked(t *testing.T) {
	repo := newMemTeamRepoB()
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()})
	ctx := context.Background()

	team, _ := uc.Create(ctx, biz.Team{TeamKey: "default", DisplayName: "Default"})
	// Mark as default directly in repo
	t2 := repo.items[team.ID]
	t2.IsDefault = true
	repo.items[team.ID] = t2

	err := uc.Delete(ctx, team.ID)
	if err == nil {
		t.Error("expected conflict error when deleting default team")
	}
}

func TestTeamUsecase_Update(t *testing.T) {
	repo := newMemTeamRepoB()
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()})
	ctx := context.Background()

	team, _ := uc.Create(ctx, biz.Team{TeamKey: "t1", DisplayName: "Original"})
	updated, err := uc.Update(ctx, team.ID, biz.Team{DisplayName: "Renamed"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.DisplayName != "Renamed" {
		t.Errorf("expected Renamed, got %s", updated.DisplayName)
	}
}

func TestTeamUsecase_Duplicate(t *testing.T) {
	repo := newMemTeamRepoB()
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()})
	ctx := context.Background()

	team, _ := uc.Create(ctx, biz.Team{TeamKey: "orig", DisplayName: "Orig"})
	copy, err := uc.Duplicate(ctx, team.ID)
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if copy.ID == team.ID {
		t.Error("duplicate should have different ID")
	}
	if copy.IsDefault {
		t.Error("duplicate should not be default")
	}
}

func TestTeamUsecase_Duplicate_BuiltinBecomesUserOwned(t *testing.T) {
	repo := newMemTeamRepoB()
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()})
	ctx := context.Background()

	builtin, err := uc.Create(ctx, biz.Team{TeamKey: "eco", DisplayName: "Eco", Kind: "ecosystem_preset", Source: "imported"})
	if err != nil {
		t.Fatalf("create builtin: %v", err)
	}
	copy, err := uc.Duplicate(ctx, builtin.ID)
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if copy.Kind != "user" {
		t.Errorf("duplicate kind = %q, want user", copy.Kind)
	}
	if copy.Source != "user" {
		t.Errorf("duplicate source = %q, want user", copy.Source)
	}
	if err := uc.Delete(ctx, copy.ID); err != nil {
		t.Errorf("delete duplicated copy: %v", err)
	}
}

// --- CronUsecase tests ---

type memCronRepoB struct {
	tasks map[string]biz.CronTask
}

func newMemCronRepoB() *memCronRepoB {
	return &memCronRepoB{tasks: make(map[string]biz.CronTask)}
}

func (m *memCronRepoB) ListCronTasks(_ context.Context) ([]biz.CronTask, error) {
	out := make([]biz.CronTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out, nil
}
func (m *memCronRepoB) GetCronTask(_ context.Context, id string) (biz.CronTask, error) {
	t, ok := m.tasks[id]
	if !ok {
		return biz.CronTask{}, fmt.Errorf("not found")
	}
	return t, nil
}
func (m *memCronRepoB) CreateCronTask(_ context.Context, t biz.CronTask) (biz.CronTask, error) {
	m.tasks[t.ID] = t
	return t, nil
}
func (m *memCronRepoB) UpdateCronTask(_ context.Context, t biz.CronTask) (biz.CronTask, error) {
	m.tasks[t.ID] = t
	return t, nil
}
func (m *memCronRepoB) DeleteCronTask(_ context.Context, id string) error {
	delete(m.tasks, id)
	return nil
}
func (m *memCronRepoB) GetCronTaskRun(_ context.Context, id string) (biz.CronTaskRun, error) {
	return biz.CronTaskRun{ID: id}, nil
}
func (m *memCronRepoB) ListCronTaskRuns(_ context.Context, _ biz.CronTaskRunQuery) ([]biz.CronTaskRun, error) {
	return nil, nil
}
func (m *memCronRepoB) InsertCronTaskRun(_ context.Context, _ biz.CronTaskRunInput) error {
	return nil
}
func (m *memCronRepoB) UpdateCronTaskRun(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func TestCronUsecase_CreateGetDelete(t *testing.T) {
	uc := biz.NewCronUsecase(newMemCronRepoB(), nil)
	ctx := context.Background()

	task, err := uc.CreateTask(ctx, biz.CronTask{TaskKey: "daily", Name: "Daily Job"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.Status != "active" {
		t.Errorf("default status: %s", task.Status)
	}
	if task.ID == "" {
		t.Error("ID should be assigned")
	}

	got, err := uc.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TaskKey != "daily" {
		t.Errorf("task_key mismatch: %s", got.TaskKey)
	}

	err = uc.DeleteTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestCronUsecase_Create_Validation(t *testing.T) {
	uc := biz.NewCronUsecase(newMemCronRepoB(), nil)
	ctx := context.Background()

	_, err := uc.CreateTask(ctx, biz.CronTask{Name: "no-key"})
	if err == nil {
		t.Error("expected error for missing task_key")
	}

	_, err = uc.CreateTask(ctx, biz.CronTask{TaskKey: "no-name"})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestCronUsecase_Get_EmptyID(t *testing.T) {
	uc := biz.NewCronUsecase(newMemCronRepoB(), nil)
	ctx := context.Background()
	_, err := uc.GetTask(ctx, "")
	if err == nil {
		t.Error("expected error for empty id")
	}
}

// --- PluginUsecase tests ---

type memPluginRepoB struct {
	items map[string]biz.Plugin
}

func newMemPluginRepoB() *memPluginRepoB {
	return &memPluginRepoB{items: make(map[string]biz.Plugin)}
}

func (m *memPluginRepoB) SearchPlugins(_ context.Context, q biz.PluginListQuery) (biz.PluginListResult, error) {
	out := make([]biz.Plugin, 0, len(m.items))
	for _, p := range m.items {
		out = append(out, p)
	}
	return biz.PluginListResult{Items: out, Total: len(out)}, nil
}
func (m *memPluginRepoB) GetPlugin(_ context.Context, id string) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("not found")
	}
	return p, nil
}

func (m *memPluginRepoB) GetByKey(_ context.Context, key string) (biz.Plugin, error) {
	for _, p := range m.items {
		if p.Key == key {
			return p, nil
		}
	}
	return biz.Plugin{}, shared.ErrNotFound
}

func (m *memPluginRepoB) CreatePlugin(_ context.Context, p biz.Plugin) (biz.Plugin, error) {
	m.items[p.ID] = p
	return p, nil
}
func (m *memPluginRepoB) UpdatePluginEnabled(_ context.Context, id string, enabled bool) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("not found")
	}
	p.Enabled = enabled
	m.items[id] = p
	return p, nil
}
func (m *memPluginRepoB) UpdatePluginConfig(_ context.Context, id string, configJSON string) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("not found")
	}
	p.ConfigJSON = configJSON
	m.items[id] = p
	return p, nil
}
func (m *memPluginRepoB) UpdateSortOrder(_ context.Context, id string, sortOrder int) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("not found")
	}
	p.SortOrder = sortOrder
	m.items[id] = p
	return p, nil
}
func (m *memPluginRepoB) UpdatePluginScope(_ context.Context, id string, scope string) (biz.Plugin, error) {
	p, ok := m.items[id]
	if !ok {
		return biz.Plugin{}, fmt.Errorf("not found")
	}
	p.Scope = scope
	m.items[id] = p
	return p, nil
}

func (m *memPluginRepoB) IncrementStats(_ context.Context, pluginKey string, delta biz.PluginStatUpdate) error {
	for id, p := range m.items {
		if p.Key == pluginKey {
			p.InvokeCount += delta.InvokeCount
			p.BlockCount += delta.BlockDelta
			p.ErrorCount += delta.ErrorDelta
			if delta.LastStatus != "" {
				p.LastStatus = delta.LastStatus
			}
			m.items[id] = p
			return nil
		}
	}
	return nil
}

func TestPluginUsecase_ListAndToggle(t *testing.T) {
	repo := newMemPluginRepoB()
	repo.items["p1"] = biz.Plugin{ID: "p1", Enabled: false}
	uc := biz.NewPluginUsecase(repo, nil, nil)
	ctx := context.Background()

	result, err := uc.List(ctx, biz.PluginListQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1, got %d", result.Total)
	}

	toggled, err := uc.ToggleEnabled(ctx, "p1", true)
	if err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if !toggled.Enabled {
		t.Error("expected enabled=true")
	}
}

func TestPluginUsecase_Toggle_EmptyID(t *testing.T) {
	uc := biz.NewPluginUsecase(newMemPluginRepoB(), nil, nil)
	ctx := context.Background()
	_, err := uc.ToggleEnabled(ctx, "", true)
	if err == nil {
		t.Error("expected error for empty id")
	}
}

func TestPluginUsecase_UpdateConfig(t *testing.T) {
	repo := newMemPluginRepoB()
	repo.items["p2"] = biz.Plugin{ID: "p2"}
	uc := biz.NewPluginUsecase(repo, nil, nil)
	ctx := context.Background()

	out, err := uc.UpdateConfig(ctx, "p2", `{"key":"val"}`)
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if out.ConfigJSON != `{"key":"val"}` {
		t.Errorf("config mismatch: %s", out.ConfigJSON)
	}
}

func TestPluginUsecase_UpdateConfig_SchemaValidation(t *testing.T) {
	repo := newMemPluginRepoB()
	repo.items["audit"] = biz.Plugin{
		ID:               "audit",
		ConfigSchemaJSON: `{"type":"object","properties":{"max_content_length":{"type":"integer"}}}`,
	}
	uc := biz.NewPluginUsecase(repo, nil, nil)
	ctx := context.Background()

	_, err := uc.UpdateConfig(ctx, "audit", `{"max_content_length":"bad"}`)
	if err == nil {
		t.Fatal("expected schema validation error")
	}

	out, err := uc.UpdateConfig(ctx, "audit", `{"max_content_length":100}`)
	if err != nil {
		t.Fatalf("valid config: %v", err)
	}
	if out.ConfigJSON != `{"max_content_length":100}` {
		t.Errorf("config mismatch: %s", out.ConfigJSON)
	}
}

// --- ArtifactUsecase tests ---

type memArtifactRepoB struct {
	items map[string]biz.Artifact
	data  map[string][]byte
}

func newMemArtifactRepoB() *memArtifactRepoB {
	return &memArtifactRepoB{items: make(map[string]biz.Artifact), data: make(map[string][]byte)}
}

func (m *memArtifactRepoB) Save(_ context.Context, sessionID, name, mimeType string, data []byte) (biz.Artifact, error) {
	id := biz.NewArtifactID()
	version := 1
	for _, existing := range m.items {
		if existing.SessionID == sessionID && existing.Name == name && existing.Version >= version {
			version = existing.Version + 1
		}
	}
	a := biz.Artifact{ID: id, SessionID: sessionID, Name: name, MimeType: mimeType, Size: int64(len(data)), Version: version}
	m.items[id] = a
	m.data[id] = data
	return a, nil
}
func (m *memArtifactRepoB) Load(_ context.Context, id string, _ int) (biz.Artifact, []byte, error) {
	a, ok := m.items[id]
	if !ok {
		return biz.Artifact{}, nil, fmt.Errorf("not found")
	}
	return a, m.data[id], nil
}
func (m *memArtifactRepoB) LoadMeta(_ context.Context, id string, _ int) (biz.Artifact, error) {
	a, ok := m.items[id]
	if !ok {
		return biz.Artifact{}, fmt.Errorf("not found")
	}
	return a, nil
}
func (m *memArtifactRepoB) LoadMetas(_ context.Context, ids []string, _ int) ([]biz.Artifact, error) {
	out := make([]biz.Artifact, 0, len(ids))
	for _, id := range ids {
		if a, ok := m.items[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}
func (m *memArtifactRepoB) List(_ context.Context, sessionID string, _, _ int) ([]biz.Artifact, int, error) {
	var out []biz.Artifact
	for _, a := range m.items {
		if a.SessionID == sessionID {
			out = append(out, a)
		}
	}
	return out, len(out), nil
}
func (m *memArtifactRepoB) Delete(_ context.Context, id string) error {
	delete(m.items, id)
	delete(m.data, id)
	return nil
}
func (m *memArtifactRepoB) DeleteVersion(_ context.Context, sessionID, name string, version int) error {
	for id, a := range m.items {
		if a.SessionID == sessionID && a.Name == name && a.Version == version {
			delete(m.items, id)
			delete(m.data, id)
			return nil
		}
	}
	return fmt.Errorf("artifact: version %d of %q not found in session %q", version, name, sessionID)
}
func (m *memArtifactRepoB) ListBySessionAndName(_ context.Context, sessionID, name string) ([]biz.Artifact, error) {
	var out []biz.Artifact
	for _, a := range m.items {
		if a.SessionID == sessionID && a.Name == name {
			out = append(out, a)
		}
	}
	return out, nil
}

func TestArtifactUsecase_SaveLoadDeleteList(t *testing.T) {
	uc := biz.NewArtifactUsecase(newMemArtifactRepoB(), nil)
	ctx := context.Background()
	payload := []byte("artifact data")

	saved, err := uc.Save(ctx, "sess-1", "test.txt", "text/plain", payload)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.ID == "" {
		t.Error("expected non-empty ID")
	}

	meta, data, err := uc.Load(ctx, saved.ID, 0)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(data) != string(payload) {
		t.Errorf("data mismatch: %q", data)
	}
	if meta.Name != "test.txt" {
		t.Errorf("name mismatch: %s", meta.Name)
	}

	items, total, err := uc.List(ctx, "sess-1", 10, 0, "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Errorf("expected 1, got %d", total)
	}

	if err = uc.Delete(ctx, saved.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, _, err = uc.Load(ctx, saved.ID, 0)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestArtifactUsecase_ListVersions(t *testing.T) {
	uc := biz.NewArtifactUsecase(newMemArtifactRepoB(), nil)
	ctx := context.Background()

	_, _ = uc.Save(ctx, "sess-2", "file.txt", "text/plain", []byte("v1"))
	_, _ = uc.Save(ctx, "sess-2", "file.txt", "text/plain", []byte("v2"))

	versions, err := uc.ListVersions(ctx, "sess-2", "file.txt")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}
}

// DAT-01 / ART-04: Deleting one ID must remove all sibling versions sharing
// session+name. Prior behavior left orphan version files on disk.
func TestArtifactUsecase_DeleteRemovesAllVersions(t *testing.T) {
	uc := biz.NewArtifactUsecase(newMemArtifactRepoB(), nil)
	ctx := context.Background()

	v1, err := uc.Save(ctx, "sess-3", "doc.txt", "text/plain", []byte("v1"))
	if err != nil {
		t.Fatalf("save v1: %v", err)
	}
	v2, err := uc.Save(ctx, "sess-3", "doc.txt", "text/plain", []byte("v2"))
	if err != nil {
		t.Fatalf("save v2: %v", err)
	}
	v3, err := uc.Save(ctx, "sess-3", "doc.txt", "text/plain", []byte("v3"))
	if err != nil {
		t.Fatalf("save v3: %v", err)
	}
	other, err := uc.Save(ctx, "sess-3", "other.txt", "text/plain", []byte("o"))
	if err != nil {
		t.Fatalf("save other: %v", err)
	}

	// Deleting via the middle version's ID should still wipe v1, v2, v3.
	if err := uc.Delete(ctx, v2.ID); err != nil {
		t.Fatalf("delete v2: %v", err)
	}

	for _, id := range []string{v1.ID, v2.ID, v3.ID} {
		if _, _, err := uc.Load(ctx, id, 0); err == nil {
			t.Errorf("expected version %s to be deleted", id)
		}
	}
	// Sibling artifact under same session but different name MUST survive.
	if _, _, err := uc.Load(ctx, other.ID, 0); err != nil {
		t.Errorf("unrelated artifact should remain: %v", err)
	}
}

// TestArtifactUsecase_DeleteVersionRemovesSingleVersion verifies that
// DeleteVersion removes exactly one version while leaving siblings intact.
func TestArtifactUsecase_DeleteVersionRemovesSingleVersion(t *testing.T) {
	uc := biz.NewArtifactUsecase(newMemArtifactRepoB(), nil)
	ctx := context.Background()

	v1, _ := uc.Save(ctx, "sess-4", "log.txt", "text/plain", []byte("v1"))
	v2, _ := uc.Save(ctx, "sess-4", "log.txt", "text/plain", []byte("v2"))
	v3, _ := uc.Save(ctx, "sess-4", "log.txt", "text/plain", []byte("v3"))

	// Delete only v2 (using v1.ID as the handle; any version ID resolves the name).
	if err := uc.DeleteVersion(ctx, v1.ID, v2.Version); err != nil {
		t.Fatalf("DeleteVersion: %v", err)
	}

	// v2 must be gone.
	if _, _, err := uc.Load(ctx, v2.ID, 0); err == nil {
		t.Error("v2 should have been deleted")
	}
	// v1 and v3 must survive.
	if _, _, err := uc.Load(ctx, v1.ID, 0); err != nil {
		t.Errorf("v1 should remain: %v", err)
	}
	if _, _, err := uc.Load(ctx, v3.ID, 0); err != nil {
		t.Errorf("v3 should remain: %v", err)
	}

	// Deleting a non-existent version must return an error.
	if err := uc.DeleteVersion(ctx, v1.ID, 99); err == nil {
		t.Error("expected error for non-existent version 99")
	}
}

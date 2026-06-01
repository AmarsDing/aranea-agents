package modelregistry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubMigrateBackend struct {
	completedRules []string
	failedRules    []string
	stats          ApplyMigrationStats
	errors         []string
}

func (s *stubMigrateBackend) MigrateProviderBindings(_ context.Context, _, _ string) (ApplyMigrationStats, error) {
	return s.stats, nil
}

func (s *stubMigrateBackend) BatchMigrateProviderBindings(_ context.Context, rules []ProviderMigrationRule, skipRules []string) BatchMigrationResult {
	skip := map[string]bool{}
	for _, r := range skipRules {
		skip[r] = true
	}
	var completed []string
	var failed []string
	for _, rule := range rules {
		if skip[rule.Legacy] {
			continue
		}
		if len(s.failedRules) > 0 {
			found := false
			for _, f := range s.failedRules {
				if f == rule.Legacy {
					found = true
					break
				}
			}
			if found {
				failed = append(failed, rule.Legacy)
				continue
			}
		}
		completed = append(completed, rule.Legacy)
	}
	return BatchMigrationResult{
		CompletedRules: completed,
		FailedRules:    failed,
		Stats:          s.stats,
		Errors:         s.errors,
	}
}

type stubApplyReader struct {
	rows []ApplyRow
	err  error
}

func (s *stubApplyReader) ListProviderModels(_ context.Context) ([]ApplyRow, error) {
	return s.rows, s.err
}

func (s *stubApplyReader) CountProviderBindings(_ context.Context, _ string) (ApplyMigrationStats, error) {
	return ApplyMigrationStats{}, nil
}

type stubApplyWriter struct {
	batchApplyCalled bool
	lastPatches      []ApplyRow
	lastPricing      []PricingUpsert
	result           BatchApplyResult
}

func (s *stubApplyWriter) SaveProviderModel(_ context.Context, _ ApplyRow) error {
	return nil
}

func (s *stubApplyWriter) UpsertModelPricing(_ context.Context, _, _ string, _ MicroPricing, _ string) error {
	return nil
}

func (s *stubApplyWriter) BatchApply(_ context.Context, patches []ApplyRow, pricing []PricingUpsert) BatchApplyResult {
	s.batchApplyCalled = true
	s.lastPatches = patches
	s.lastPricing = pricing
	return s.result
}

func TestFetchPhase_Timeout(t *testing.T) {
	p := NewFetchPhase()
	if p.Timeout() != 120*time.Second {
		t.Fatalf("expected 120s, got %v", p.Timeout())
	}
}

func TestFetchPhase_Name(t *testing.T) {
	p := NewFetchPhase()
	if p.Name() != "fetch" {
		t.Fatalf("expected fetch, got %q", p.Name())
	}
}

func TestMigratePhase_Timeout(t *testing.T) {
	p := NewMigratePhase(nil)
	if p.Timeout() != 300*time.Second {
		t.Fatalf("expected 300s, got %v", p.Timeout())
	}
}

func TestMigratePhase_WithCheckpoint(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	allRules := ListProviderMigrationRules()
	if len(allRules) < 2 {
		t.Skip("need at least 2 migration rules")
	}
	completedRule := allRules[0].Legacy
	cp := MigrationCheckpoint{CompletedRules: []string{completedRule}}
	if err := st.SaveMigrationCheckpoint(cp); err != nil {
		t.Fatal(err)
	}

	backend := &stubMigrateBackend{
		stats: ApplyMigrationStats{Agents: 5},
	}
	p := NewMigratePhase(backend)
	pc := &PhaseContext{
		Ctx:   context.Background(),
		Store: st,
	}
	result := p.Run(pc)
	if result.Status != PhaseSucceeded {
		t.Fatalf("expected succeeded, got %q errors=%v", result.Status, result.Errors)
	}
	for _, c := range backend.completedRules {
		if c == completedRule {
			t.Fatalf("completed rule %q should have been skipped", completedRule)
		}
	}
}

func TestMigratePhase_PartialFailure(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	allRules := ListProviderMigrationRules()
	if len(allRules) < 2 {
		t.Skip("need at least 2 migration rules")
	}
	failedRule := allRules[0].Legacy

	backend := &stubMigrateBackend{
		failedRules: []string{failedRule},
		stats:       ApplyMigrationStats{Agents: 3},
		errors:      []string{"migrate " + failedRule + ": error"},
	}
	p := NewMigratePhase(backend)
	pc := &PhaseContext{
		Ctx:   context.Background(),
		Store: st,
	}
	result := p.Run(pc)
	if result.Status != PhaseSucceeded {
		t.Fatalf("expected succeeded with partial failure, got %q", result.Status)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected errors from partial failure")
	}
}

func TestMigratePhase_AllFail(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	allRules := ListProviderMigrationRules()
	failedRules := make([]string, len(allRules))
	for i, r := range allRules {
		failedRules[i] = r.Legacy
	}

	backend := &stubMigrateBackend{
		failedRules: failedRules,
		errors:      []string{"all failed"},
	}
	p := NewMigratePhase(backend)
	pc := &PhaseContext{
		Ctx:   context.Background(),
		Store: st,
	}
	result := p.Run(pc)
	if result.Status != PhaseFailed {
		t.Fatalf("expected failed, got %q", result.Status)
	}
}

func TestApplyPhase_SkipEmptyDirectory(t *testing.T) {
	reader := &stubApplyReader{}
	writer := &stubApplyWriter{}
	p := NewApplyPhase(reader, writer)
	pc := &PhaseContext{
		Ctx:       context.Background(),
		Directory: nil,
		Policy:    Policy{AutoApply: "metadata_and_pricing"},
	}
	result := p.Run(pc)
	if result.Status != PhaseSkipped {
		t.Fatalf("expected skipped, got %q", result.Status)
	}
}

func TestMigratePhase_LoadCheckpointError(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := os.MkdirAll(st.MigrationCheckpointPath(), 0o755); err != nil {
		t.Fatal(err)
	}

	backend := &stubMigrateBackend{stats: ApplyMigrationStats{Agents: 1}}
	p := NewMigratePhase(backend)
	pc := &PhaseContext{
		Ctx:   context.Background(),
		Store: st,
	}
	result := p.Run(pc)
	if result.Status != PhaseFailed {
		t.Fatalf("expected failed, got %q", result.Status)
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "load checkpoint") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error containing 'load checkpoint', got %v", result.Errors)
	}
}

func TestMigratePhase_SaveCheckpointError(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)

	if err := st.SaveMigrationCheckpoint(MigrationCheckpoint{}); err != nil {
		t.Fatal(err)
	}
	cpPath := st.MigrationCheckpointPath()
	if err := os.Chmod(cpPath, 0o444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(cpPath, 0o644)

	backend := &stubMigrateBackend{stats: ApplyMigrationStats{Agents: 1}}
	p := NewMigratePhase(backend)
	pc := &PhaseContext{
		Ctx:   context.Background(),
		Store: st,
	}
	result := p.Run(pc)
	if result.Status != PhaseSucceeded {
		t.Fatalf("expected succeeded, got %q errors=%v", result.Status, result.Errors)
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "save checkpoint") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error containing 'save checkpoint', got %v", result.Errors)
	}
}

func TestStore_MigrationCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)

	cp := MigrationCheckpoint{
		CompletedRules: []string{"gemini->google", "aliyun-qwen->alibaba-cn"},
	}
	if err := st.SaveMigrationCheckpoint(cp); err != nil {
		t.Fatal(err)
	}

	loaded, err := st.LoadMigrationCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.CompletedRules) != len(cp.CompletedRules) {
		t.Fatalf("expected %d completed rules, got %d", len(cp.CompletedRules), len(loaded.CompletedRules))
	}
	for i, r := range cp.CompletedRules {
		if loaded.CompletedRules[i] != r {
			t.Fatalf("rule[%d]: expected %q, got %q", i, r, loaded.CompletedRules[i])
		}
	}
}

func TestStore_LoadMigrationCheckpoint_NoFile(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)

	cp, err := st.LoadMigrationCheckpoint()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(cp.CompletedRules) != 0 {
		t.Fatalf("expected empty completed rules, got %v", cp.CompletedRules)
	}
}

func TestStore_LoadMigrationCheckpoint_BadJSON(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)

	if err := os.MkdirAll(filepath.Dir(st.MigrationCheckpointPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(st.MigrationCheckpointPath(), []byte("{{{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	cp, err := st.LoadMigrationCheckpoint()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(cp.CompletedRules) != 0 {
		t.Fatalf("expected empty completed rules, got %v", cp.CompletedRules)
	}
}

func TestNewCheckpoint(t *testing.T) {
	rules := []string{"gemini->google", "aliyun-qwen->alibaba-cn"}
	cp := NewCheckpoint(rules)
	if cp == nil {
		t.Fatal("expected non-nil checkpoint")
	}
	if len(cp.CompletedRules) != len(rules) {
		t.Fatalf("expected %d rules, got %d", len(rules), len(cp.CompletedRules))
	}
	for i, r := range rules {
		if cp.CompletedRules[i] != r {
			t.Fatalf("rule[%d]: expected %q, got %q", i, r, cp.CompletedRules[i])
		}
	}
}

func TestApplyPhase_SkipNonePolicy(t *testing.T) {
	reader := &stubApplyReader{}
	writer := &stubApplyWriter{}
	p := NewApplyPhase(reader, writer)
	pc := &PhaseContext{
		Ctx:       context.Background(),
		Directory: Directory{"openai": {ID: "openai", Models: map[string]Model{"gpt-4o": {ID: "gpt-4o"}}}},
		Policy:    Policy{AutoApply: "none"},
	}
	result := p.Run(pc)
	if result.Status != PhaseSkipped {
		t.Fatalf("expected skipped, got %q", result.Status)
	}
}

func TestApplyPhase_BatchApply(t *testing.T) {
	reader := &stubApplyReader{
		rows: []ApplyRow{{
			ID:           "1",
			Key:          "openai:gpt-4o",
			Provider:     "openai",
			Model:        "gpt-4o",
			Enabled:      true,
			ConfigJSON:   `{}`,
			MetadataJSON: `{}`,
		}},
	}
	writer := &stubApplyWriter{
		result: BatchApplyResult{RowsUpdated: 1},
	}
	p := NewApplyPhase(reader, writer)
	cat := Directory{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Env:  []string{"OPENAI_API_KEY"},
			Models: map[string]Model{
				"gpt-4o": {
					ID:          "gpt-4o",
					Name:        "GPT-4o",
					ReleaseDate: "2024-05-13",
					Cost:        &ModelCost{Input: 2.5, Output: 10},
					Limit:       ModelLimit{Context: 128000, Output: 4096},
				},
			},
		},
	}
	pc := &PhaseContext{
		Ctx:       context.Background(),
		Directory: cat,
		Policy:    Policy{AutoApply: "metadata_and_pricing"},
	}
	result := p.Run(pc)
	if result.Status != PhaseSucceeded {
		t.Fatalf("expected succeeded, got %q errors=%v", result.Status, result.Errors)
	}
	if !writer.batchApplyCalled {
		t.Fatal("expected BatchApply to be called")
	}
	if len(writer.lastPatches) == 0 {
		t.Fatal("expected patches to be passed to BatchApply")
	}
}

func TestLogoPhase_SkipEmptyDirectory(t *testing.T) {
	p := NewLogoPhase()
	pc := &PhaseContext{
		Ctx:       context.Background(),
		Directory: nil,
	}
	result := p.Run(pc)
	if result.Status != PhaseSkipped {
		t.Fatalf("expected skipped, got %q", result.Status)
	}
}

func TestLogoPhase_Name(t *testing.T) {
	p := NewLogoPhase()
	if p.Name() != "logos" {
		t.Fatalf("expected logos, got %q", p.Name())
	}
}

func TestPhaseFromCtx(t *testing.T) {
	pc := &PhaseContext{
		Ctx:   context.Background(),
		Store: NewStore(t.TempDir()),
	}
	ctx := WithPhaseCtx(context.Background(), pc)
	got := PhaseFromCtx(ctx)
	if got != pc {
		t.Fatal("PhaseFromCtx roundtrip failed")
	}
	empty := PhaseFromCtx(context.Background())
	if empty != nil {
		t.Fatal("expected nil from empty context")
	}
}

func TestFetchPhase_RunSuccess(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := st.SavePolicy(DefaultPolicy()); err != nil {
		t.Fatal(err)
	}
	cat := Directory{"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{"gpt-4o": {ID: "gpt-4o"}}}}
	meta := Meta{
		SyncedAt:      time.Now().UTC().Format(time.RFC3339),
		ETag:          `"old"`,
		SourceURL:     DefaultPolicy().SourceURL,
		ProviderCount: 1,
		ModelCount:    1,
	}
	if err := st.SaveDirectory(cat, meta); err != nil {
		t.Fatal(err)
	}

	oldHook := catalogFetchHook
	defer func() { catalogFetchHook = oldHook }()
	catalogFetchHook = func(_ context.Context, _, _ string) (FetchResult, error) {
		newCat := Directory{
			"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{
				"gpt-4o":  {ID: "gpt-4o", Name: "GPT-4o"},
				"gpt-4.1": {ID: "gpt-4.1", Name: "GPT-4.1"},
			}},
		}
		body, _ := json.Marshal(newCat)
		return FetchResult{Body: body, ETag: `"new"`}, nil
	}

	p := NewFetchPhase()
	pc := &PhaseContext{Ctx: context.Background(), Store: st}
	result := p.Run(pc)
	if result.Status != PhaseSucceeded {
		t.Fatalf("expected succeeded, got %q errors=%v", result.Status, result.Errors)
	}
	if result.Stats["providers"] != 1 {
		t.Fatalf("expected 1 provider, got %d", result.Stats["providers"])
	}
}

func TestFetchPhase_RunNotModified(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	if err := st.SavePolicy(DefaultPolicy()); err != nil {
		t.Fatal(err)
	}
	cat := Directory{"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{}}}
	meta := Meta{
		SyncedAt:      time.Now().UTC().Format(time.RFC3339),
		ETag:          `"cached"`,
		SourceURL:     DefaultPolicy().SourceURL,
		ProviderCount: 1,
	}
	if err := st.SaveDirectory(cat, meta); err != nil {
		t.Fatal(err)
	}

	oldHook := catalogFetchHook
	defer func() { catalogFetchHook = oldHook }()
	catalogFetchHook = func(_ context.Context, _, _ string) (FetchResult, error) {
		return FetchResult{NotModified: true, ETag: `"cached"`}, nil
	}

	p := NewFetchPhase()
	pc := &PhaseContext{Ctx: context.Background(), Store: st}
	result := p.Run(pc)
	if result.Status != PhaseSkipped {
		t.Fatalf("expected skipped, got %q", result.Status)
	}
}

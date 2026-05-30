package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"aranea-agents/internal/modelregistry"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

type mockStoreProvider struct {
	store *modelregistry.Store
	err   error
}

func (m *mockStoreProvider) Store(ctx context.Context) (*modelregistry.Store, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.store, nil
}

type mockApplyBackend struct{}

func (m *mockApplyBackend) ListProviderModels(ctx context.Context) ([]modelregistry.ApplyRow, error) {
	return nil, nil
}

func (m *mockApplyBackend) SaveProviderModel(ctx context.Context, row modelregistry.ApplyRow) error {
	return nil
}

func (m *mockApplyBackend) UpsertModelPricing(ctx context.Context, provider, model string, micro modelregistry.MicroPricing, source string) error {
	return nil
}

func (m *mockApplyBackend) CountProviderBindings(ctx context.Context, provider string) (modelregistry.ApplyMigrationStats, error) {
	return modelregistry.ApplyMigrationStats{}, nil
}

func (m *mockApplyBackend) MigrateProviderBindings(ctx context.Context, from, to string) (modelregistry.ApplyMigrationStats, error) {
	return modelregistry.ApplyMigrationStats{}, nil
}

func (m *mockApplyBackend) BatchMigrateProviderBindings(ctx context.Context, rules []modelregistry.ProviderMigrationRule, skipRules []string) modelregistry.BatchMigrationResult {
	return modelregistry.BatchMigrationResult{}
}

func (m *mockApplyBackend) BatchApply(ctx context.Context, patches []modelregistry.ApplyRow, pricing []modelregistry.PricingUpsert) modelregistry.BatchApplyResult {
	return modelregistry.BatchApplyResult{}
}

func setupTempStore(t *testing.T) (*modelregistry.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store := modelregistry.NewStore(dir)
	policy := modelregistry.DefaultPolicy()
	if err := store.SavePolicy(policy); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}
	dirData := modelregistry.Directory{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]modelregistry.Model{
				"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o"},
			},
		},
	}
	meta := modelregistry.Meta{
		SyncedAt:      time.Now().UTC().Format(time.RFC3339),
		ProviderCount: 1,
		ModelCount:    1,
	}
	if err := store.SaveDirectory(dirData, meta); err != nil {
		t.Fatalf("SaveDirectory: %v", err)
	}
	return store, dir
}

func TestBuildModelRegistrySyncAgent(t *testing.T) {
	store, _ := setupTempStore(t)
	sp := &mockStoreProvider{store: store}
	backend := &mockApplyBackend{}

	agent, err := BuildModelRegistrySyncAgent(sp, backend)
	if err != nil {
		t.Fatalf("BuildModelRegistrySyncAgent: %v", err)
	}
	if agent == nil {
		t.Fatal("agent is nil")
	}
	if agent.Info().Name != "model-registry-sync" {
		t.Fatalf("expected name model-registry-sync, got %s", agent.Info().Name)
	}
	if len(agent.Tools()) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(agent.Tools()))
	}
	if agent.SubAgents() != nil {
		t.Fatal("expected nil SubAgents")
	}
}

func TestModelRegistrySyncAgent_Info(t *testing.T) {
	store, _ := setupTempStore(t)
	sp := &mockStoreProvider{store: store}
	backend := &mockApplyBackend{}

	agent, _ := BuildModelRegistrySyncAgent(sp, backend)
	info := agent.Info()
	if info.Name != "model-registry-sync" {
		t.Fatalf("expected name model-registry-sync, got %s", info.Name)
	}
	if info.Description == "" {
		t.Fatal("expected non-empty description")
	}
}

func TestModelRegistrySyncAgent_Tools(t *testing.T) {
	store, _ := setupTempStore(t)
	sp := &mockStoreProvider{store: store}
	backend := &mockApplyBackend{}

	agent, _ := BuildModelRegistrySyncAgent(sp, backend)
	tools := agent.Tools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}

	expectedNames := []string{
		"fetch_model_directory",
		"migrate_provider_bindings",
		"apply_model_directory",
		"sync_provider_logos",
	}
	for i, expected := range expectedNames {
		decl := tools[i].Declaration()
		if decl.Name != expected {
			t.Fatalf("tool[%d]: expected name %s, got %s", i, expected, decl.Name)
		}
	}
}

func TestModelRegistrySyncAgent_SubAgents(t *testing.T) {
	store, _ := setupTempStore(t)
	sp := &mockStoreProvider{store: store}
	backend := &mockApplyBackend{}

	agent, _ := BuildModelRegistrySyncAgent(sp, backend)
	if agent.SubAgents() != nil {
		t.Fatal("expected nil SubAgents")
	}
	if agent.FindSubAgent("any") != nil {
		t.Fatal("expected nil FindSubAgent")
	}
}

func TestModelRegistrySyncAgent_Run_StoreError(t *testing.T) {
	sp := &mockStoreProvider{err: errors.New("store unavailable")}
	backend := &mockApplyBackend{}

	agent, _ := BuildModelRegistrySyncAgent(sp, backend)
	ch, err := agent.Run(context.Background(), &trpcagent.Invocation{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var events []*trpcevent.Event
	for evt := range ch {
		events = append(events, evt)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event on store error")
	}

	foundError := false
	for _, evt := range events {
		if evt.Object == "error" && evt.Error != nil {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatal("expected an error event when store provider fails")
	}
}

func TestModelRegistrySyncAgent_Run_EventsFlow(t *testing.T) {
	store, dir := setupTempStore(t)
	_ = dir

	sp := &mockStoreProvider{store: store}
	backend := &mockApplyBackend{}

	agent, _ := BuildModelRegistrySyncAgent(sp, backend)
	ch, err := agent.Run(context.Background(), &trpcagent.Invocation{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var events []*trpcevent.Event
	for evt := range ch {
		events = append(events, evt)
	}
	if len(events) == 0 {
		t.Fatal("expected events from Run")
	}

	var phaseStarts []string
	var phaseResults []string
	var hasCompletion bool
	for _, evt := range events {
		if evt.Tag == "phase_start" {
			phase, ok := evt.Extensions["phase"]
			if ok {
				var name string
				_ = json.Unmarshal(phase, &name)
				phaseStarts = append(phaseStarts, name)
			}
		}
		if evt.Tag == "phase_succeeded" || evt.Tag == "phase_failed" || evt.Tag == "phase_skipped" {
			phase, ok := evt.Extensions["phase"]
			if ok {
				var name string
				_ = json.Unmarshal(phase, &name)
				phaseResults = append(phaseResults, name)
			}
		}
		if evt.Done && evt.Object == "runner.completion" {
			hasCompletion = true
		}
	}

	if len(phaseStarts) == 0 {
		t.Fatal("expected at least one phase_start event")
	}
	if len(phaseResults) == 0 {
		t.Fatal("expected at least one phase result event")
	}
	if !hasCompletion {
		t.Fatal("expected a completion event")
	}

	expectedPhases := []string{"fetch", "migrate", "apply"}
	for _, ep := range expectedPhases {
		found := false
		for _, ps := range phaseStarts {
			if ps == ep {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected phase_start for %q, got %v", ep, phaseStarts)
		}
	}
}

func TestModelRegistrySyncAgent_ImplementsAgent(t *testing.T) {
	store, _ := setupTempStore(t)
	sp := &mockStoreProvider{store: store}
	backend := &mockApplyBackend{}

	var _ trpcagent.Agent = (*ModelRegistrySyncAgent)(nil)
	agent, _ := BuildModelRegistrySyncAgent(sp, backend)
	_ = agent
}

func TestModelRegistrySyncAgent_Run_StoreDirNotExists(t *testing.T) {
	dir := t.TempDir()
	emptyDir := filepath.Join(dir, "nonexistent")
	store := modelregistry.NewStore(emptyDir)
	sp := &mockStoreProvider{store: store}
	backend := &mockApplyBackend{}

	agent, _ := BuildModelRegistrySyncAgent(sp, backend)
	ch, err := agent.Run(context.Background(), &trpcagent.Invocation{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var events []*trpcevent.Event
	for evt := range ch {
		events = append(events, evt)
	}
	if len(events) == 0 {
		t.Fatal("expected events from Run")
	}

	var hasCompletion bool
	for _, evt := range events {
		if evt.Done && evt.Object == "runner.completion" {
			hasCompletion = true
		}
	}
	if !hasCompletion {
		t.Fatal("expected a completion event even when store has no saved data")
	}
}

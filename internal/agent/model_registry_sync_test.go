package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// requireModelsDevReachable 网络依赖测试离线优雅跳过：models.dev 在本机须代理方可达，
// 测试进程不走代理时直连重试约 21s 后恒败。3s 探测不可达即 Skip；
// 联网环境（CI/已注入代理）仍执行真实端到端同步路径。
func requireModelsDevReachable(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, modelregistry.DefaultPolicy().SourceURL, nil)
	if err != nil {
		t.Skipf("probe request build failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("models.dev unreachable (%v), skipping network-dependent test", err)
	}
	_ = resp.Body.Close()
}

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
	store := modelregistry.NewStore(dir, loggateway.NewNoop())
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

	agent, err := BuildModelRegistrySyncAgent(sp, backend, loggateway.NewNoop())
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

	agent, _ := BuildModelRegistrySyncAgent(sp, backend, loggateway.NewNoop())
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

	agent, _ := BuildModelRegistrySyncAgent(sp, backend, loggateway.NewNoop())
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

	agent, _ := BuildModelRegistrySyncAgent(sp, backend, loggateway.NewNoop())
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

	agent, _ := BuildModelRegistrySyncAgent(sp, backend, loggateway.NewNoop())
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
	requireModelsDevReachable(t)
	store, dir := setupTempStore(t)
	_ = dir

	sp := &mockStoreProvider{store: store}
	backend := &mockApplyBackend{}

	agent, _ := BuildModelRegistrySyncAgent(sp, backend, loggateway.NewNoop())
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
	}

	if len(phaseStarts) == 0 {
		t.Fatal("expected at least one phase_start event")
	}
	if len(phaseResults) == 0 {
		t.Fatal("expected at least one phase result event")
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
	agent, _ := BuildModelRegistrySyncAgent(sp, backend, loggateway.NewNoop())
	_ = agent
}

func TestModelRegistrySyncAgent_Run_StoreDirNotExists(t *testing.T) {
	dir := t.TempDir()
	emptyDir := filepath.Join(dir, "nonexistent")
	store := modelregistry.NewStore(emptyDir, loggateway.NewNoop())
	sp := &mockStoreProvider{store: store}
	backend := &mockApplyBackend{}

	agent, _ := BuildModelRegistrySyncAgent(sp, backend, loggateway.NewNoop())
	ch, err := agent.Run(context.Background(), &trpcagent.Invocation{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var events []*trpcevent.Event
	for evt := range ch {
		events = append(events, evt)
	}
	if len(events) == 0 {
		t.Fatal("expected events from Run even when store has no saved data")
	}

	hasPhaseEvent := false
	for _, evt := range events {
		if evt.Tag == "phase_start" || evt.Tag == "phase_succeeded" || evt.Tag == "phase_failed" || evt.Tag == "phase_skipped" {
			hasPhaseEvent = true
		}
	}
	if !hasPhaseEvent {
		t.Fatal("expected at least one phase event even when store has no saved data")
	}
}

package agent

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// --- Fakes ---

// fakeFactoryModel implements trpcmodel.Model for AgentFactory tests.
type fakeFactoryModel struct {
	responses []*trpcmodel.Response
	err       error // function-level error (returned by GenerateContent)
}

func (m *fakeFactoryModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan *trpcmodel.Response, len(m.responses))
	for _, r := range m.responses {
		ch <- r
	}
	close(ch)
	return ch, nil
}

func (m *fakeFactoryModel) Info() trpcmodel.Info {
	return trpcmodel.Info{Name: "fake-factory-model"}
}

// fakeFactoryAgentStore implements biz.AgentReader + biz.AgentWriter for tests.
type fakeFactoryAgentStore struct {
	mu     sync.Mutex
	agents map[string]biz.Agent // keyed by AgentKey
}

func newFakeFactoryAgentStore() *fakeFactoryAgentStore {
	return &fakeFactoryAgentStore{agents: make(map[string]biz.Agent)}
}

func (s *fakeFactoryAgentStore) CreateAgent(_ context.Context, a biz.Agent) (biz.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.agents[a.AgentKey]; exists {
		return biz.Agent{}, shared.ErrAgentKeyConflict
	}
	s.agents[a.AgentKey] = a
	return a, nil
}

func (s *fakeFactoryAgentStore) GetAgentByAgentKey(_ context.Context, agentKey string) (biz.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[agentKey]
	if !ok {
		return biz.Agent{}, shared.ErrNotFound
	}
	return a, nil
}

// Unused AgentReader methods
func (s *fakeFactoryAgentStore) SearchAgents(_ context.Context, _ biz.AgentListQuery) (biz.AgentListResult, error) {
	return biz.AgentListResult{}, nil
}
func (s *fakeFactoryAgentStore) GetAgentByID(_ context.Context, _ string) (biz.Agent, error) {
	return biz.Agent{}, shared.ErrNotFound
}
func (s *fakeFactoryAgentStore) ListExtrasForAgents(_ context.Context, _ []string) (map[string]biz.AgentListExtras, error) {
	return nil, nil
}
func (s *fakeFactoryAgentStore) ListAgentsByIDs(_ context.Context, _ []string) ([]biz.Agent, error) {
	return nil, nil
}

// Unused AgentWriter methods
func (s *fakeFactoryAgentStore) UpdateAgent(_ context.Context, a biz.Agent) (biz.Agent, error) {
	return a, nil
}
func (s *fakeFactoryAgentStore) DeleteAgent(_ context.Context, _ string) error { return nil }
func (s *fakeFactoryAgentStore) ToggleFavorite(_ context.Context, _ string) (biz.Agent, error) {
	return biz.Agent{}, nil
}

// fakeTemplateRepo implements biz.AgentTemplateRepo.
type fakeTemplateRepo struct {
	templates []biz.AgentTemplate
	err       error
}

func (r *fakeTemplateRepo) ListAgentTemplates(_ context.Context) ([]biz.AgentTemplate, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.templates, nil
}

// factoryCaptureBus captures published envelopes for assertions.
type factoryCaptureBus struct {
	mu        sync.Mutex
	published []contract.Envelope
}

func (b *factoryCaptureBus) Publish(_ context.Context, env contract.Envelope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, env)
}

func (b *factoryCaptureBus) Subscribe(_ contract.SubscribeOptions) (<-chan contract.Envelope, func()) {
	return nil, func() {}
}

func (b *factoryCaptureBus) DropCount() uint64 { return 0 }

func (b *factoryCaptureBus) getPublished() []contract.Envelope {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]contract.Envelope, len(b.published))
	copy(out, b.published)
	return out
}

// --- Helpers ---

func defaultFactoryTemplates() []biz.AgentTemplate {
	return []biz.AgentTemplate{
		{Key: "programmer", Label: "程序员", DisplayName: "研发助手", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "资深研发工程师，关注架构、代码质量、测试与可维护性。"},
		{Key: "writer", Label: "写手", DisplayName: "写作助手", Provider: "openrouter", Model: "gpt-4.1-mini", Description: "擅长品牌文案、结构化写作、润色与多语种表达。"},
	}
}

func validFactoryJSONResponse() string {
	return `{"display_name":"Go 后端助手","description":"擅长 Go 后端开发","provider":"openrouter","model":"gpt-4.1-mini","system_prompt":"You are a Go backend expert."}`
}

func newTestAgentFactory(model trpcmodel.Model, store *fakeFactoryAgentStore, templates []biz.AgentTemplate, bus *factoryCaptureBus) *AgentFactoryImpl {
	return &AgentFactoryImpl{
		llm:          model,
		agentWriter:  store,
		agentReader:  store,
		templateRepo: &fakeTemplateRepo{templates: templates},
		bus:          bus,
		lg:           loggateway.NewNoop().With(loggateway.Domain("agent_factory")),
	}
}

// --- Tests ---

func TestAgentFactory_EnsureAgent_CreateNew(t *testing.T) {
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)

	profile := biz.TaskProfile{
		Domain:               "engineering",
		TaskDescription:      "Write a Go REST API",
		RequiredCapabilities: []string{"go-backend"},
	}

	agentKey, err := factory.EnsureAgent(context.Background(), profile)
	if err != nil {
		t.Fatalf("EnsureAgent failed: %v", err)
	}
	if agentKey == "" {
		t.Fatal("agentKey is empty")
	}

	// Verify agent was persisted
	agent, err := store.GetAgentByAgentKey(context.Background(), agentKey)
	if err != nil {
		t.Fatalf("GetAgentByAgentKey failed: %v", err)
	}
	if agent.Source != "system" {
		t.Errorf("Source=%q want %q", agent.Source, "system")
	}
	if agent.DisplayName != "Go 后端助手" {
		t.Errorf("DisplayName=%q want %q", agent.DisplayName, "Go 后端助手")
	}
	if agent.Status != string(biz.AgentStatusActive) {
		t.Errorf("Status=%q want %q", agent.Status, biz.AgentStatusActive)
	}

	// Verify event was published
	published := bus.getPublished()
	if len(published) != 1 {
		t.Fatalf("published=%d want 1", len(published))
	}
	env := published[0]
	if env.Type != contract.EnvelopeTypeAgentCreated {
		t.Errorf("event type=%q want %q", env.Type, contract.EnvelopeTypeAgentCreated)
	}
	if env.Metadata["agent_key"] != agentKey {
		t.Errorf("metadata agent_key=%v want %q", env.Metadata["agent_key"], agentKey)
	}
	if env.Metadata["source"] != "system" {
		t.Errorf("metadata source=%v want %q", env.Metadata["source"], "system")
	}
}

func TestAgentFactory_EnsureAgent_Idempotent(t *testing.T) {
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)

	profile := biz.TaskProfile{
		Domain:               "engineering",
		TaskDescription:      "Write a Go REST API",
		RequiredCapabilities: []string{"go-backend"},
	}

	// First call creates the agent
	key1, err := factory.EnsureAgent(context.Background(), profile)
	if err != nil {
		t.Fatalf("first EnsureAgent failed: %v", err)
	}

	// Second call with same profile should return same key (idempotent)
	key2, err := factory.EnsureAgent(context.Background(), profile)
	if err != nil {
		t.Fatalf("second EnsureAgent failed: %v", err)
	}
	if key1 != key2 {
		t.Errorf("idempotency broken: key1=%q key2=%q", key1, key2)
	}

	// Only one event should be published (first call only)
	published := bus.getPublished()
	if len(published) != 1 {
		t.Errorf("published=%d want 1 (idempotent call should not publish)", len(published))
	}
}

func TestAgentFactory_EnsureAgent_LLMFailure(t *testing.T) {
	model := &fakeFactoryModel{
		err: errors.New("LLM service unavailable"),
	}
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)

	profile := biz.TaskProfile{
		Domain:          "engineering",
		TaskDescription: "Write a Go REST API",
	}

	_, err := factory.EnsureAgent(context.Background(), profile)
	if err == nil {
		t.Fatal("expected error for LLM failure, got nil")
	}

	// Verify no agent was persisted
	if len(store.agents) != 0 {
		t.Errorf("agents=%d want 0 (no agent should be persisted on LLM failure)", len(store.agents))
	}

	// Verify no event was published
	published := bus.getPublished()
	if len(published) != 0 {
		t.Errorf("published=%d want 0 (no event on LLM failure)", len(published))
	}
}

func TestAgentFactory_EnsureAgent_InvalidJSON(t *testing.T) {
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: "this is not JSON"}}}},
		},
	}
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)

	profile := biz.TaskProfile{
		Domain:               "engineering",
		TaskDescription:      "Write a Go REST API",
		RequiredCapabilities: []string{"go-backend"},
	}

	// Invalid JSON should fall back to default definition, not error
	agentKey, err := factory.EnsureAgent(context.Background(), profile)
	if err != nil {
		t.Fatalf("EnsureAgent failed on invalid JSON (should use fallback): %v", err)
	}
	if agentKey == "" {
		t.Fatal("agentKey is empty")
	}

	// Verify agent was persisted with fallback display name
	agent, err := store.GetAgentByAgentKey(context.Background(), agentKey)
	if err != nil {
		t.Fatalf("GetAgentByAgentKey failed: %v", err)
	}
	if agent.Source != "system" {
		t.Errorf("Source=%q want %q", agent.Source, "system")
	}
	// Fallback display name should contain the domain
	if agent.DisplayName == "" {
		t.Error("DisplayName is empty (fallback should provide a default)")
	}
}

func TestAgentFactory_EnsureAgent_NilLLM(t *testing.T) {
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	// Pass nil model — AgentFactory should return Internal error
	factory := newTestAgentFactory(nil, store, defaultFactoryTemplates(), bus)

	profile := biz.TaskProfile{
		Domain:          "engineering",
		TaskDescription: "Write a Go REST API",
	}

	_, err := factory.EnsureAgent(context.Background(), profile)
	if err == nil {
		t.Fatal("expected error for nil LLM, got nil")
	}

	// Verify it's an apierror.Internal
	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeInternal {
		t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeInternal)
	}
}

func TestAgentFactory_EnsureAgent_EmptyTaskDescription(t *testing.T) {
	model := &fakeFactoryModel{}
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)

	profile := biz.TaskProfile{
		Domain:          "engineering",
		TaskDescription: "", // empty
	}

	_, err := factory.EnsureAgent(context.Background(), profile)
	if err == nil {
		t.Fatal("expected error for empty task description, got nil")
	}

	apiErr, ok := err.(*apierror.Error)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeBadRequest {
		t.Errorf("Code=%q want %q", apiErr.Code, apierror.CodeBadRequest)
	}
}

func TestAgentFactory_GenerateAgentDefinition_StreamingDelta(t *testing.T) {
	// Test that streaming delta content is concatenated correctly
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: `{"display_name":"`}}}},
			{Choices: []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: `Streamed`}}}},
			{Choices: []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: ` Agent","description":"delta","provider":"openrouter","model":"gpt-4.1-mini","system_prompt":""}`}}}},
		},
	}
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)

	profile := biz.TaskProfile{
		Domain:               "engineering",
		TaskDescription:      "Stream test",
		RequiredCapabilities: []string{"go-backend"},
	}

	agentKey, err := factory.EnsureAgent(context.Background(), profile)
	if err != nil {
		t.Fatalf("EnsureAgent failed: %v", err)
	}

	agent, err := store.GetAgentByAgentKey(context.Background(), agentKey)
	if err != nil {
		t.Fatalf("GetAgentByAgentKey failed: %v", err)
	}
	if agent.DisplayName != "Streamed Agent" {
		t.Errorf("DisplayName=%q want %q (streaming delta concatenation)", agent.DisplayName, "Streamed Agent")
	}
}

func TestAgentFactory_BuildDynamicAgentKey_Deterministic(t *testing.T) {
	factory := &AgentFactoryImpl{
		lg: loggateway.NewNoop().With(loggateway.Domain("agent_factory")),
	}

	profile1 := biz.TaskProfile{
		Domain:               "engineering",
		TaskDescription:      "Write a Go REST API",
		RequiredCapabilities: []string{"go-backend"},
		PreferredTools:       []string{"read_file"},
		PreferredModel:       "gpt-4.1-mini",
	}
	profile2 := profile1 // identical

	key1 := factory.buildDynamicAgentKey(profile1)
	key2 := factory.buildDynamicAgentKey(profile2)

	if key1 != key2 {
		t.Errorf("deterministic key broken: key1=%q key2=%q", key1, key2)
	}

	// Different profile should produce different key
	profile3 := profile1
	profile3.TaskDescription = "Different task"
	key3 := factory.buildDynamicAgentKey(profile3)
	if key1 == key3 {
		t.Errorf("different profiles produced same key: %q", key1)
	}

	// Key should have "factory-" prefix
	if key1[:8] != "factory-" {
		t.Errorf("key prefix=%q want %q", key1[:8], "factory-")
	}
}

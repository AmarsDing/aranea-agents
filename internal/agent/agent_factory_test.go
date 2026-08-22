package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]biz.Agent, 0, len(s.agents))
	for _, a := range s.agents {
		items = append(items, a)
	}
	return biz.AgentListResult{Items: items, Total: len(items)}, nil
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

// factoryCaptureBus captures published v2 Events for assertions.
type factoryCaptureBus struct {
	mu        sync.Mutex
	published []biz.Event
}

func (b *factoryCaptureBus) Publish(_ context.Context, ev biz.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, ev)
}

func (b *factoryCaptureBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return nil, func() {}
}

func (b *factoryCaptureBus) getPublished() []biz.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.Event, len(b.published))
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
		eventBus:     bus,
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
	ev := published[0]
	if ev.EventKind() != biz.EventKindSystemNotice {
		t.Errorf("EventKind=%q want %q", ev.EventKind(), biz.EventKindSystemNotice)
	}
	notice, ok := ev.(*biz.SystemNoticeEvent)
	if !ok {
		t.Fatalf("expected *biz.SystemNoticeEvent, got %T", ev)
	}
	if notice.Meta["event_type"] != "agent_created" {
		t.Errorf("event_type=%v want %q", notice.Meta["event_type"], "agent_created")
	}
	if notice.Meta["agent_key"] != agentKey {
		t.Errorf("metadata agent_key=%v want %q", notice.Meta["agent_key"], agentKey)
	}
	if notice.Meta["source"] != "system" {
		t.Errorf("metadata source=%v want %q", notice.Meta["source"], "system")
	}
}

func TestAgentFactory_EnsureAgent_OccupiesExistingPosition(t *testing.T) {
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	store := newFakeFactoryAgentStore()
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), &factoryCaptureBus{})
	org := &stubOrgReader{nodes: sampleOrgTree()}
	factory.SetOrganizationReader(org)

	_, err := factory.EnsureAgent(context.Background(), biz.TaskProfile{
		Domain:          "engineering",
		DomainPath:      "软件/后端",
		TaskDescription: "Write a Go REST API",
		DepartmentID:    "dept-eng",
	})
	if err != nil {
		t.Fatal(err)
	}
	var created biz.Agent
	for _, a := range store.agents {
		created = a
	}
	if created.PositionID != "pos-other-eng" {
		t.Fatalf("PositionID=%q want pos-other-eng (其他岗优先)", created.PositionID)
	}
	if org.creates != 0 {
		t.Fatalf("Factory must not create org nodes, creates=%d", org.creates)
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

// --- P-ORCH fakes ---

// fakeFactoryEmitter implements biz.ActivityEmitter for confirmation tests.
type fakeFactoryEmitter struct {
	mu              sync.Mutex
	confirmParams   []biz.ActivityConfirmParams
	confirmResults  []factoryConfirmResult
	confirmTimeouts []string
	idCounter       int
}

type factoryConfirmResult struct {
	activityID string
	approved   bool
}

func (e *fakeFactoryEmitter) EmitNotice(_ context.Context, _, _ string) error { return nil }

func (e *fakeFactoryEmitter) EmitConfirmRequest(_ context.Context, p biz.ActivityConfirmParams) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.idCounter++
	e.confirmParams = append(e.confirmParams, p)
	return fmt.Sprintf("confirm-%d", e.idCounter), nil
}

func (e *fakeFactoryEmitter) EmitConfirmResult(_ context.Context, activityID string, approved bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.confirmResults = append(e.confirmResults, factoryConfirmResult{activityID: activityID, approved: approved})
	return nil
}

func (e *fakeFactoryEmitter) EmitConfirmTimeout(_ context.Context, activityID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.confirmTimeouts = append(e.confirmTimeouts, activityID)
	return nil
}

// progressPhases extracts orchestration_progress phases from captured events.
func progressPhases(events []biz.Event) []string {
	var phases []string
	for _, ev := range events {
		notice, ok := ev.(*biz.SystemNoticeEvent)
		if !ok || notice.NoticeType != "orchestration_progress" {
			continue
		}
		phase, _ := notice.Meta["phase"].(string)
		phases = append(phases, phase)
	}
	return phases
}

// --- P-ORCH tests ---

func TestAgentFactory_EnsureAgent_PublishesProgressEvents(t *testing.T) {
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
		SpiritSessionID:      "sess-orch-1",
	}

	agentKey, err := factory.EnsureAgent(context.Background(), profile)
	if err != nil {
		t.Fatalf("EnsureAgent failed: %v", err)
	}

	phases := progressPhases(bus.getPublished())
	if len(phases) != 2 {
		t.Fatalf("orchestration_progress events=%d want 2 (creating_agent + agent_created), got %v", len(phases), phases)
	}
	if phases[0] != "creating_agent" {
		t.Errorf("phases[0]=%q want %q", phases[0], "creating_agent")
	}
	if phases[1] != "agent_created" {
		t.Errorf("phases[1]=%q want %q", phases[1], "agent_created")
	}

	// agent_created event must carry the real display name + agent key and
	// be routed to the spirit session.
	for _, ev := range bus.getPublished() {
		notice, ok := ev.(*biz.SystemNoticeEvent)
		if !ok || notice.NoticeType != "orchestration_progress" {
			continue
		}
		if notice.Meta["phase"] != "agent_created" {
			continue
		}
		if notice.Meta["agent_name"] != "Go 后端助手" {
			t.Errorf("agent_name=%v want %q", notice.Meta["agent_name"], "Go 后端助手")
		}
		if notice.Meta["agent_key"] != agentKey {
			t.Errorf("agent_key=%v want %q", notice.Meta["agent_key"], agentKey)
		}
		if notice.SpiritSessionID() != "sess-orch-1" {
			t.Errorf("sessionID=%q want %q", notice.SpiritSessionID(), "sess-orch-1")
		}
	}
}

func TestAgentFactory_EnsureAgent_ConfirmApproved(t *testing.T) {
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)
	emitter := &fakeFactoryEmitter{}

	replyFn := serviceawaitreply.ReplyFunc(func(_ context.Context) (string, error) {
		return "approved", nil
	})
	ctx := serviceawaitreply.WithReplyFunc(context.Background(), replyFn)
	ctx = biz.WithActivityEmitter(ctx, emitter)

	profile := biz.TaskProfile{
		Domain:               "engineering",
		TaskDescription:      "Write a Go REST API",
		RequiredCapabilities: []string{"go-backend"},
		SpiritSessionID:      "sess-orch-2",
	}

	agentKey, err := factory.EnsureAgent(ctx, profile)
	if err != nil {
		t.Fatalf("EnsureAgent failed: %v", err)
	}
	if agentKey == "" {
		t.Fatal("agentKey is empty")
	}

	// Confirm request must have been emitted with the proposal.
	if len(emitter.confirmParams) != 1 {
		t.Fatalf("confirmRequests=%d want 1", len(emitter.confirmParams))
	}
	if emitter.confirmParams[0].ToolName != "agent_factory" {
		t.Errorf("ToolName=%q want %q", emitter.confirmParams[0].ToolName, "agent_factory")
	}
	if emitter.confirmParams[0].ToolArguments == "" {
		t.Error("ToolArguments is empty (proposal JSON expected)")
	}

	// Confirm result must report approved=true.
	if len(emitter.confirmResults) != 1 {
		t.Fatalf("confirmResults=%d want 1", len(emitter.confirmResults))
	}
	if !emitter.confirmResults[0].approved {
		t.Error("confirmResults[0].approved=false want true")
	}

	// Agent must be persisted after approval.
	if _, err := store.GetAgentByAgentKey(context.Background(), agentKey); err != nil {
		t.Fatalf("agent not persisted after approval: %v", err)
	}
}

func TestAgentFactory_EnsureAgent_ConfirmDenied(t *testing.T) {
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)
	emitter := &fakeFactoryEmitter{}

	replyFn := serviceawaitreply.ReplyFunc(func(_ context.Context) (string, error) {
		return "denied", nil
	})
	ctx := serviceawaitreply.WithReplyFunc(context.Background(), replyFn)
	ctx = biz.WithActivityEmitter(ctx, emitter)

	profile := biz.TaskProfile{
		Domain:          "engineering",
		TaskDescription: "Write a Go REST API",
	}

	_, err := factory.EnsureAgent(ctx, profile)
	if err == nil {
		t.Fatal("expected error for denied confirmation, got nil")
	}

	// Confirm result must report approved=false.
	if len(emitter.confirmResults) != 1 {
		t.Fatalf("confirmResults=%d want 1", len(emitter.confirmResults))
	}
	if emitter.confirmResults[0].approved {
		t.Error("confirmResults[0].approved=true want false")
	}

	// No agent may be persisted after denial.
	if len(store.agents) != 0 {
		t.Errorf("agents=%d want 0 (no agent on denial)", len(store.agents))
	}

	// No agent_created progress event may be published after denial.
	for _, phase := range progressPhases(bus.getPublished()) {
		if phase == "agent_created" {
			t.Error("agent_created progress event published after denial")
		}
	}
}

func TestAgentFactory_EnsureAgent_ConfirmReplyError(t *testing.T) {
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	store := newFakeFactoryAgentStore()
	bus := &factoryCaptureBus{}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)
	emitter := &fakeFactoryEmitter{}

	replyFn := serviceawaitreply.ReplyFunc(func(_ context.Context) (string, error) {
		return "", errors.New("confirmation timeout")
	})
	ctx := serviceawaitreply.WithReplyFunc(context.Background(), replyFn)
	ctx = biz.WithActivityEmitter(ctx, emitter)

	profile := biz.TaskProfile{
		Domain:          "engineering",
		TaskDescription: "Write a Go REST API",
	}

	_, err := factory.EnsureAgent(ctx, profile)
	if err == nil {
		t.Fatal("expected error for reply failure, got nil")
	}

	// No agent may be persisted after reply error.
	if len(store.agents) != 0 {
		t.Errorf("agents=%d want 0 (no agent on reply error)", len(store.agents))
	}
}

// --- Task 6: key 修正 + 出生登记 + 同域复用 ---

func TestAgentFactory_BuildDynamicAgentKey_DomainDerived(t *testing.T) {
	factory := &AgentFactoryImpl{lg: loggateway.NewNoop()}
	// 同域同模型、任务文本不同 → 同一 key（"写诗"/"写散文"复用同一 Agent）
	p1 := biz.TaskProfile{DomainPath: "创作/文学", TaskDescription: "写一首诗", PreferredModel: "gpt-4.1-mini"}
	p2 := biz.TaskProfile{DomainPath: "创作/文学", TaskDescription: "写一篇散文", PreferredModel: "gpt-4.1-mini"}
	if k1, k2 := factory.buildDynamicAgentKey(p1), factory.buildDynamicAgentKey(p2); k1 != k2 {
		t.Errorf("same domain+model must derive same key: %q vs %q", k1, k2)
	}
	// 模型不同 → key 不同
	p3 := p1
	p3.PreferredModel = "gpt-4.1"
	if k1, k3 := factory.buildDynamicAgentKey(p1), factory.buildDynamicAgentKey(p3); k1 == k3 {
		t.Error("different model must derive different key")
	}
	// DomainPath 为空 → 旧行为（任务文本参与哈希）
	p4 := biz.TaskProfile{Domain: "engineering", TaskDescription: "task A"}
	p5 := biz.TaskProfile{Domain: "engineering", TaskDescription: "task B"}
	if k4, k5 := factory.buildDynamicAgentKey(p4), factory.buildDynamicAgentKey(p5); k4 == k5 {
		t.Error("legacy path: different task text must derive different key")
	}
}

func TestAgentFactory_ParseAgentDefinition_MissionDomain(t *testing.T) {
	factory := &AgentFactoryImpl{lg: loggateway.NewNoop()}
	text := `{"display_name":"文学写手","description":"擅长诗歌","provider":"openrouter","model":"gpt-4.1-mini","system_prompt":"...","mission_statement":"中文诗歌与散文创作写手","domain_path":"创作/诗歌"}`
	def, err := factory.parseAgentDefinition(text, biz.TaskProfile{Domain: "创作", TaskDescription: "写诗"}, biz.AgentTemplate{})
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if def.MissionStatement != "中文诗歌与散文创作写手" {
		t.Errorf("MissionStatement=%q", def.MissionStatement)
	}
	if def.DomainPath != "创作" {
		t.Errorf("DomainPath=%q want %q（词表外二级域归并一级域）", def.DomainPath, "创作")
	}
}

func TestAgentFactory_ParseAgentDefinition_MissionFallback(t *testing.T) {
	factory := &AgentFactoryImpl{lg: loggateway.NewNoop()}
	text := `{"display_name":"x","description":"通用描述","provider":"p","model":"m"}`
	def, err := factory.parseAgentDefinition(text, biz.TaskProfile{TaskDescription: "t"}, biz.AgentTemplate{})
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if def.MissionStatement != "通用描述" {
		t.Errorf("mission must fallback to description, got %q", def.MissionStatement)
	}
}

// embedFunc 适配器，测试用 fake embedder。
type embedFunc func(ctx context.Context, texts []string) ([][]float32, error)

func (f embedFunc) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return f(ctx, texts)
}
func (f embedFunc) Dim() int { return 3 }

func TestAgentFactory_EnsureAgent_SameDomainMissionReuse(t *testing.T) {
	store := newFakeFactoryAgentStore()
	// 预置同域 Agent（使命与"写诗"高度相似 → embedding 相同向量）
	store.agents["factory-existing"] = biz.Agent{
		AgentKey: "factory-existing", DisplayName: "文学写手", Status: "active",
		DomainPath: "创作/文学", MissionStatement: "中文诗歌散文创作",
	}
	bus := &factoryCaptureBus{}
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)
	factory.embedder = embedFunc(func(_ context.Context, texts []string) ([][]float32, error) {
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{1, 0, 0} // 全部同向 → cosine = 1.0 ≥ 0.85
		}
		return out, nil
	})

	key, err := factory.EnsureAgent(context.Background(), biz.TaskProfile{
		DomainPath: "创作/文学", TaskDescription: "写一首春天的诗",
	})
	if err != nil {
		t.Fatalf("EnsureAgent err: %v", err)
	}
	if key != "factory-existing" {
		t.Errorf("key=%q want factory-existing（同域使命相似复用）", key)
	}
	if len(store.agents) != 1 {
		t.Errorf("agents=%d want 1（未创建新 Agent）", len(store.agents))
	}
	if len(bus.getPublished()) != 0 {
		t.Errorf("published=%d want 0（复用不发事件）", len(bus.getPublished()))
	}
}

func TestAgentFactory_EnsureAgent_DomainReuseBelowThreshold(t *testing.T) {
	store := newFakeFactoryAgentStore()
	store.agents["factory-existing"] = biz.Agent{
		AgentKey: "factory-existing", DisplayName: "科幻写手", Status: "active",
		DomainPath: "创作/文学", MissionStatement: "硬科幻小说创作",
	}
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), &factoryCaptureBus{})
	factory.embedder = embedFunc(func(_ context.Context, texts []string) ([][]float32, error) {
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{1, 0, 0}
		}
		if len(out) > 1 {
			out[1] = []float32{0, 1, 0} // 正交 → cosine = 0 < 0.85
		}
		return out, nil
	})

	key, err := factory.EnsureAgent(context.Background(), biz.TaskProfile{
		DomainPath: "创作/文学", TaskDescription: "写一首春天的诗",
	})
	if err != nil {
		t.Fatalf("EnsureAgent err: %v", err)
	}
	if key == "factory-existing" {
		t.Error("similarity below 0.85 must create new agent")
	}
	if len(store.agents) != 2 {
		t.Errorf("agents=%d want 2（新建落库）", len(store.agents))
	}
	// 新 Agent 出生登记 mission/domain
	created, _ := store.GetAgentByAgentKey(context.Background(), key)
	if created.DomainPath != "创作" {
		t.Errorf("created.DomainPath=%q want %q（definition 归一化落库）", created.DomainPath, "创作")
	}
	if created.MissionStatement == "" {
		t.Error("created.MissionStatement must not be empty（出生登记）")
	}
}

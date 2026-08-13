package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ── stubs ────────────────────────────────────────────────────────────────────

// orchStubStore implements biz.UnifiedEvolutionStore with recording for tests.
type orchStubStore struct {
	mu            sync.Mutex
	pending       map[string]bool // key: targetType/targetID
	expireCalls   int
	created       []biz.UnifiedEvolutionSuggestion
	latestByKey   map[string]*biz.UnifiedEvolutionSuggestion // key: targetType/targetID/actionType
	createErr     error
	hasPendingErr error
}

func newOrchStubStore() *orchStubStore {
	return &orchStubStore{
		pending:     make(map[string]bool),
		latestByKey: make(map[string]*biz.UnifiedEvolutionSuggestion),
	}
}

func storeKey(targetType, targetID string) string { return targetType + "/" + targetID }
func actionKey(targetType, targetID, action string) string {
	return targetType + "/" + targetID + "/" + action
}

func (s *orchStubStore) HasPendingForTarget(_ context.Context, targetType, targetID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending[storeKey(targetType, targetID)], s.hasPendingErr
}

func (s *orchStubStore) GetLatestByTarget(_ context.Context, _, _ string) (*biz.UnifiedEvolutionSuggestion, error) {
	return nil, nil
}

func (s *orchStubStore) GetLatestByTargetAndAction(_ context.Context, targetType, targetID, actionType string) (*biz.UnifiedEvolutionSuggestion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latestByKey[actionKey(targetType, targetID, actionType)], nil
}

func (s *orchStubStore) GetByID(_ context.Context, _ string) (*biz.UnifiedEvolutionSuggestion, error) {
	return nil, nil
}

func (s *orchStubStore) ListByTarget(_ context.Context, _, _, _ string, _, _ int) ([]biz.UnifiedEvolutionSuggestion, error) {
	return nil, nil
}

func (s *orchStubStore) CountByTarget(_ context.Context, _, _, _ string) (int, error) {
	return 0, nil
}

func (s *orchStubStore) ListByTargetAndAction(_ context.Context, _, _, _, _ string, _, _ int) ([]biz.UnifiedEvolutionSuggestion, error) {
	return nil, nil
}

func (s *orchStubStore) CountByTargetAndAction(_ context.Context, _, _, _, _ string) (int, error) {
	return 0, nil
}

func (s *orchStubStore) Create(_ context.Context, suggestion biz.UnifiedEvolutionSuggestion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createErr != nil {
		return s.createErr
	}
	s.created = append(s.created, suggestion)
	s.pending[storeKey(string(suggestion.TargetType), suggestion.TargetID)] = true
	return nil
}

func (s *orchStubStore) UpdateStatus(_ context.Context, _, _, _, _ string) error { return nil }
func (s *orchStubStore) UpdateStatusCAS(_ context.Context, _ string, _ []string, _, _, _ string) (bool, error) {
	return true, nil
}
func (s *orchStubStore) UpdateDraftBody(_ context.Context, _, _ string) error { return nil }
func (s *orchStubStore) UpdateLifecycleStatus(_ context.Context, _, _ string) error {
	return nil
}
func (s *orchStubStore) UpdateSandboxResult(_ context.Context, _ string, _ bool, _ json.RawMessage) error {
	return nil
}

func (s *orchStubStore) UpdateMetadataKey(_ context.Context, _, _, _ string) error { return nil }

func (s *orchStubStore) ExpireOlderThan(_ context.Context, _ time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireCalls++
	return 0, nil
}

func (s *orchStubStore) createdCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.created)
}

func (s *orchStubStore) expireCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.expireCalls
}

// orchStubTrigger records Check calls and returns fixed suggestions.
type orchStubTrigger struct {
	mu          sync.Mutex
	targetType  biz.EvolutionTargetType
	actionType  biz.EvolutionActionType
	source      string
	checkedIDs  []string
	suggestions []biz.UnifiedEvolutionSuggestion
	checkErr    error
}

func (t *orchStubTrigger) TargetType() biz.EvolutionTargetType { return t.targetType }
func (t *orchStubTrigger) ActionType() biz.EvolutionActionType { return t.actionType }
func (t *orchStubTrigger) TriggerSource() string               { return t.source }

func (t *orchStubTrigger) Check(_ context.Context, targetID string) ([]biz.UnifiedEvolutionSuggestion, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.checkedIDs = append(t.checkedIDs, targetID)
	if t.checkErr != nil {
		return nil, t.checkErr
	}
	var out []biz.UnifiedEvolutionSuggestion
	for _, s := range t.suggestions {
		s.TargetID = targetID
		out = append(out, s)
	}
	return out, nil
}

func (t *orchStubTrigger) checkedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.checkedIDs)
}

func (t *orchStubTrigger) wasChecked(targetID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range t.checkedIDs {
		if id == targetID {
			return true
		}
	}
	return false
}

// orchStubSkillReader implements biz.SkillQueryReader with paged results.
type orchStubSkillReader struct {
	pages [][]biz.Skill
	err   error
	calls int
}

func (r *orchStubSkillReader) SearchSkills(_ context.Context, q biz.SkillListQuery) (biz.SkillListResult, error) {
	r.calls++
	if r.err != nil {
		return biz.SkillListResult{}, r.err
	}
	idx := q.Offset / q.Limit
	if idx >= len(r.pages) {
		return biz.SkillListResult{}, nil
	}
	return biz.SkillListResult{Items: r.pages[idx]}, nil
}

func (r *orchStubSkillReader) SearchSkillInvocations(_ context.Context, _ biz.SkillRunQuery) (biz.SkillRunResult, error) {
	return biz.SkillRunResult{}, nil
}
func (r *orchStubSkillReader) ListSkillVersions(_ context.Context, _ biz.SkillVersionListQuery) (biz.SkillVersionListResult, error) {
	return biz.SkillVersionListResult{}, nil
}
func (r *orchStubSkillReader) ListSkillSimilaritySources(_ context.Context) ([]biz.SkillSimilaritySource, error) {
	return nil, nil
}
func (r *orchStubSkillReader) ListRegisteredSlugs(_ context.Context) ([]string, error) {
	return nil, nil
}

// orchStubAgentLister implements EvolutionAgentLister.
type orchStubAgentLister struct {
	agents       []biz.Agent
	settings     map[string]biz.AgentRuntimeSettings
	settingsErr  map[string]error
	searchErr    error
	searchedKeys []string
}

func (l *orchStubAgentLister) SearchAgents(_ context.Context, q biz.AgentListQuery) (biz.AgentListResult, error) {
	if l.searchErr != nil {
		return biz.AgentListResult{}, l.searchErr
	}
	if q.Offset >= len(l.agents) {
		return biz.AgentListResult{}, nil
	}
	end := q.Offset + q.Limit
	if end > len(l.agents) {
		end = len(l.agents)
	}
	return biz.AgentListResult{Items: l.agents[q.Offset:end]}, nil
}

func (l *orchStubAgentLister) GetAgentRuntimeSettings(_ context.Context, agentID string) (biz.AgentRuntimeSettings, error) {
	if err, ok := l.settingsErr[agentID]; ok {
		return biz.AgentRuntimeSettings{}, err
	}
	return l.settings[agentID], nil
}

// drafterSpy records DraftPending calls (EVO-20 post-pass wiring).
type drafterSpy struct {
	mu    sync.Mutex
	calls []string
}

func (s *drafterSpy) DraftPending(_ context.Context, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, agentID)
	return nil
}

func (s *drafterSpy) wasCalled(agentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.calls {
		if id == agentID {
			return true
		}
	}
	return false
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newTestOrchestrator(store *orchStubStore, triggers ...biz.EvolutionTrigger) *biz.SkillEvolutionOrchestrator {
	orch := biz.NewSkillEvolutionOrchestrator(store, store, store, loggateway.NewNoop())
	for _, tr := range triggers {
		orch.RegisterTrigger(tr)
	}
	return orch
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestNewEvolutionOrchestratorWorker_Defaults(t *testing.T) {
	w := NewEvolutionOrchestratorWorker(0, nil, nil, nil, nil, loggateway.NewNoop())
	if w.interval != 2*time.Hour {
		t.Errorf("expected default interval 2h, got %s", w.interval)
	}
	w2 := NewEvolutionOrchestratorWorker(5*time.Minute, nil, nil, nil, nil, loggateway.NewNoop())
	if w2.interval != 5*time.Minute {
		t.Errorf("expected interval 5m, got %s", w2.interval)
	}
}

func TestEvolutionOrchestratorWorker_Start_NilOrch(t *testing.T) {
	w := NewEvolutionOrchestratorWorker(time.Minute, nil, nil, nil, nil, loggateway.NewNoop())
	done := make(chan struct{})
	go func() {
		w.Start(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start with nil orchestrator should return immediately")
	}
}

func TestEvolutionOrchestratorWorker_ScanSkills(t *testing.T) {
	store := newOrchStubStore()
	trigger := &orchStubTrigger{
		targetType: biz.EvolutionTargetSkill,
		actionType: biz.EvolutionActionImprove,
		source:     "health",
		suggestions: []biz.UnifiedEvolutionSuggestion{
			{
				TargetType: biz.EvolutionTargetSkill,
				ActionType: biz.EvolutionActionImprove,
				Status:     "pending",
			},
		},
	}
	orch := newTestOrchestrator(store, trigger)
	reader := &orchStubSkillReader{
		pages: [][]biz.Skill{
			{{ID: "sk-1"}, {ID: "sk-2"}},
		},
	}
	w := NewEvolutionOrchestratorWorker(time.Minute, orch, nil, reader, nil, loggateway.NewNoop())

	if err := w.scanSkills(context.Background()); err != nil {
		t.Fatalf("scanSkills: %v", err)
	}
	if !trigger.wasChecked("sk-1") || !trigger.wasChecked("sk-2") {
		t.Errorf("expected trigger checked for both skills, got %v", trigger.checkedIDs)
	}
	if store.createdCount() != 2 {
		t.Errorf("expected 2 suggestions created, got %d", store.createdCount())
	}
}

func TestEvolutionOrchestratorWorker_ScanSkills_ErrorPropagates(t *testing.T) {
	store := newOrchStubStore()
	orch := newTestOrchestrator(store)
	reader := &orchStubSkillReader{err: errors.New("db down")}
	w := NewEvolutionOrchestratorWorker(time.Minute, orch, nil, reader, nil, loggateway.NewNoop())

	if err := w.scanSkills(context.Background()); err == nil {
		t.Fatal("expected search error to propagate")
	}
}

func TestEvolutionOrchestratorWorker_ScanSkills_SkipsPending(t *testing.T) {
	store := newOrchStubStore()
	store.pending[storeKey(string(biz.EvolutionTargetSkill), "sk-1")] = true
	trigger := &orchStubTrigger{
		targetType: biz.EvolutionTargetSkill,
		actionType: biz.EvolutionActionImprove,
		source:     "health",
		suggestions: []biz.UnifiedEvolutionSuggestion{
			{TargetType: biz.EvolutionTargetSkill, ActionType: biz.EvolutionActionImprove, Status: "pending"},
		},
	}
	orch := newTestOrchestrator(store, trigger)
	reader := &orchStubSkillReader{pages: [][]biz.Skill{{{ID: "sk-1"}, {ID: "sk-2"}}}}
	w := NewEvolutionOrchestratorWorker(time.Minute, orch, nil, reader, nil, loggateway.NewNoop())

	if err := w.scanSkills(context.Background()); err != nil {
		t.Fatalf("scanSkills: %v", err)
	}
	if trigger.wasChecked("sk-1") {
		t.Error("sk-1 has pending suggestion — trigger should not run for it")
	}
	if !trigger.wasChecked("sk-2") {
		t.Error("sk-2 should have been checked")
	}
}

func TestEvolutionOrchestratorWorker_ScanAgents_OnlyOptedIn(t *testing.T) {
	store := newOrchStubStore()
	trigger := &orchStubTrigger{
		targetType: biz.EvolutionTargetAgent,
		actionType: biz.EvolutionActionCreate,
		source:     "pattern",
		suggestions: []biz.UnifiedEvolutionSuggestion{
			{TargetType: biz.EvolutionTargetAgent, ActionType: biz.EvolutionActionCreate, Status: "pending"},
		},
	}
	orch := newTestOrchestrator(store, trigger)
	lister := &orchStubAgentLister{
		agents: []biz.Agent{{ID: "ag-1"}, {ID: "ag-2"}, {ID: "ag-3"}},
		settings: map[string]biz.AgentRuntimeSettings{
			"ag-1": {EvolutionSkillEvolve: true},
			"ag-2": {EvolutionSkillEvolve: false},
			"ag-3": {EvolutionSkillEvolve: true},
		},
	}
	w := NewEvolutionOrchestratorWorker(time.Minute, orch, lister, nil, nil, loggateway.NewNoop())

	if err := w.scanAgents(context.Background()); err != nil {
		t.Fatalf("scanAgents: %v", err)
	}
	if !trigger.wasChecked("ag-1") {
		t.Error("ag-1 (opted-in) should be checked")
	}
	if trigger.wasChecked("ag-2") {
		t.Error("ag-2 (opted-out) should NOT be checked")
	}
	if !trigger.wasChecked("ag-3") {
		t.Error("ag-3 (opted-in) should be checked")
	}
	if store.createdCount() != 2 {
		t.Errorf("expected 2 suggestions created, got %d", store.createdCount())
	}
}

func TestEvolutionOrchestratorWorker_ScanAgents_SettingsErrorSkips(t *testing.T) {
	store := newOrchStubStore()
	trigger := &orchStubTrigger{
		targetType: biz.EvolutionTargetAgent,
		actionType: biz.EvolutionActionCreate,
		source:     "pattern",
		suggestions: []biz.UnifiedEvolutionSuggestion{
			{TargetType: biz.EvolutionTargetAgent, ActionType: biz.EvolutionActionCreate, Status: "pending"},
		},
	}
	orch := newTestOrchestrator(store, trigger)
	lister := &orchStubAgentLister{
		agents: []biz.Agent{{ID: "ag-1"}, {ID: "ag-2"}},
		settings: map[string]biz.AgentRuntimeSettings{
			"ag-2": {EvolutionSkillEvolve: true},
		},
		settingsErr: map[string]error{"ag-1": errors.New("not found")},
	}
	w := NewEvolutionOrchestratorWorker(time.Minute, orch, lister, nil, nil, loggateway.NewNoop())

	if err := w.scanAgents(context.Background()); err != nil {
		t.Fatalf("scanAgents: %v", err)
	}
	if trigger.wasChecked("ag-1") {
		t.Error("ag-1 (settings error) should be skipped")
	}
	if !trigger.wasChecked("ag-2") {
		t.Error("ag-2 should be checked")
	}
}

func TestEvolutionOrchestratorWorker_ScanAgents_SearchErrorPropagates(t *testing.T) {
	store := newOrchStubStore()
	orch := newTestOrchestrator(store)
	lister := &orchStubAgentLister{searchErr: errors.New("db down")}
	w := NewEvolutionOrchestratorWorker(time.Minute, orch, lister, nil, nil, loggateway.NewNoop())

	if err := w.scanAgents(context.Background()); err == nil {
		t.Fatal("expected search error to propagate")
	}
}

func TestEvolutionOrchestratorWorker_RunOnce_ExpiresThenScans(t *testing.T) {
	store := newOrchStubStore()
	skillTrigger := &orchStubTrigger{
		targetType: biz.EvolutionTargetSkill,
		actionType: biz.EvolutionActionImprove,
		source:     "health",
		suggestions: []biz.UnifiedEvolutionSuggestion{
			{TargetType: biz.EvolutionTargetSkill, ActionType: biz.EvolutionActionImprove, Status: "pending"},
		},
	}
	orch := newTestOrchestrator(store, skillTrigger)
	reader := &orchStubSkillReader{pages: [][]biz.Skill{{{ID: "sk-1"}}}}
	lister := &orchStubAgentLister{
		agents: []biz.Agent{{ID: "ag-1"}},
		settings: map[string]biz.AgentRuntimeSettings{
			"ag-1": {EvolutionSkillEvolve: true},
		},
	}
	w := NewEvolutionOrchestratorWorker(time.Minute, orch, lister, reader, nil, loggateway.NewNoop())

	w.runOnce(context.Background())

	// runOnce dispatches through safego.Go (async); poll for completion.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if store.expireCallCount() >= 1 && store.createdCount() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if store.expireCallCount() == 0 {
		t.Error("ExpirePending should be called once per tick")
	}
	if !skillTrigger.wasChecked("sk-1") {
		t.Error("skill scan should run during tick")
	}
}

// EVO-20: the drafter post-pass runs once per L3-opted-in agent after the
// trigger pass; L1-only agents are skipped (their pipelines never produce
// evolve_agent suggestions, so drafting would be a wasted query).
func TestEvolutionOrchestratorWorker_ScanAgents_RunsDrafterForL3OptedIn(t *testing.T) {
	store := newOrchStubStore()
	orch := newTestOrchestrator(store)
	lister := &orchStubAgentLister{
		agents: []biz.Agent{{ID: "ag-l1"}, {ID: "ag-l3"}, {ID: "ag-evo"}, {ID: "ag-off"}},
		settings: map[string]biz.AgentRuntimeSettings{
			"ag-l1":  {EvolutionSkillEvolve: true},
			"ag-l3":  {EvolutionSuggestionsEnabled: true},
			"ag-evo": {EvoEnabled: true},
			"ag-off": {},
		},
	}
	spy := &drafterSpy{}
	w := NewEvolutionOrchestratorWorker(time.Minute, orch, lister, nil, spy, loggateway.NewNoop())

	if err := w.scanAgents(context.Background()); err != nil {
		t.Fatalf("scanAgents: %v", err)
	}
	if spy.wasCalled("ag-l1") {
		t.Error("L1-only agent should not run the drafter")
	}
	if !spy.wasCalled("ag-l3") {
		t.Error("L3 (EvolutionSuggestionsEnabled) agent should run the drafter")
	}
	if !spy.wasCalled("ag-evo") {
		t.Error("L3 (EvoEnabled) agent should run the drafter")
	}
	if spy.wasCalled("ag-off") {
		t.Error("opted-out agent should not run the drafter")
	}
}

// EVO-20: nil drafter keeps the worker fully functional (feature disabled).
func TestEvolutionOrchestratorWorker_ScanAgents_NilDrafterOK(t *testing.T) {
	store := newOrchStubStore()
	orch := newTestOrchestrator(store)
	lister := &orchStubAgentLister{
		agents:   []biz.Agent{{ID: "ag-1"}},
		settings: map[string]biz.AgentRuntimeSettings{"ag-1": {EvoEnabled: true}},
	}
	w := NewEvolutionOrchestratorWorker(time.Minute, orch, lister, nil, nil, loggateway.NewNoop())
	if err := w.scanAgents(context.Background()); err != nil {
		t.Fatalf("scanAgents with nil drafter: %v", err)
	}
}

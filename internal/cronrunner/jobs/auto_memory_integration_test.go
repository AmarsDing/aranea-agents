package jobs

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	sessionsess "aranea-agents/internal/biz/session"
	"aranea-agents/internal/conf"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/pkg/loggateway"
)

type memoryTestAgentRepo struct {
	ids map[string]struct{}
}

func newMemoryEnabledAgentsUC(ids ...string) *biz.AgentUsecase {
	repo := &memoryTestAgentRepo{ids: make(map[string]struct{}, len(ids))}
	for _, id := range ids {
		repo.ids[id] = struct{}{}
	}
	return biz.NewAgentUsecase(biz.AgentUsecaseDeps{Reader: repo, Writer: repo, Settings: repo, Files: repo, Position: repo, Tx: repo, Lg: loggateway.NewNoop()})
}

func (r *memoryTestAgentRepo) SearchAgents(context.Context, biz.AgentListQuery) (biz.AgentListResult, error) {
	return biz.AgentListResult{}, nil
}
func (r *memoryTestAgentRepo) ListExtrasForAgents(context.Context, []string) (map[string]biz.AgentListExtras, error) {
	return map[string]biz.AgentListExtras{}, nil
}
func (r *memoryTestAgentRepo) ListAgentCreators(context.Context) ([]biz.AgentCreator, error) {
	return nil, nil
}
func (r *memoryTestAgentRepo) GetAgentByID(_ context.Context, id string) (biz.Agent, error) {
	if _, ok := r.ids[id]; !ok {
		return biz.Agent{}, sql.ErrNoRows
	}
	settings := biz.DefaultAgentRuntimeSettings()
	settings.AgentID = id
	return biz.Agent{ID: id, AgentKey: id, Settings: &settings}, nil
}
func (r *memoryTestAgentRepo) GetAgentByAgentKey(context.Context, string) (biz.Agent, error) {
	return biz.Agent{}, sql.ErrNoRows
}
func (r *memoryTestAgentRepo) CreateAgent(context.Context, biz.Agent) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (r *memoryTestAgentRepo) UpdateAgent(context.Context, biz.Agent) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (r *memoryTestAgentRepo) DeleteAgent(context.Context, string) error { return nil }
func (r *memoryTestAgentRepo) GetAgentRuntimeSettings(_ context.Context, id string) (biz.AgentRuntimeSettings, error) {
	if _, ok := r.ids[id]; !ok {
		return biz.AgentRuntimeSettings{}, sql.ErrNoRows
	}
	return biz.AgentRuntimeSettings{
		AgentID:          id,
		MemoryEnabled:    true,
		L3Enabled:        true,
		L2EpisodeEnabled: true,
		L2RecallEnabled:  true,
	}, nil
}
func (r *memoryTestAgentRepo) UpsertAgentRuntimeSettings(context.Context, biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{}, nil
}
func (r *memoryTestAgentRepo) ListAgentPromptFiles(context.Context, string) ([]biz.AgentPromptFile, error) {
	return nil, nil
}
func (r *memoryTestAgentRepo) ReplaceAgentPromptFiles(context.Context, string, []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	return nil, nil
}
func (r *memoryTestAgentRepo) CreateAgentPromptFile(context.Context, biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	return biz.AgentPromptFile{}, nil
}
func (r *memoryTestAgentRepo) UpdateAgentPromptFile(context.Context, biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	return biz.AgentPromptFile{}, nil
}
func (r *memoryTestAgentRepo) DeleteAgentPromptFile(context.Context, string, string) error {
	return nil
}
func (r *memoryTestAgentRepo) ExecInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (r *memoryTestAgentRepo) ReorderAgents(context.Context, []string) error { return nil }
func (r *memoryTestAgentRepo) ClearPositionByDepartment(context.Context, string) (int, error) { return 0, nil }
func (r *memoryTestAgentRepo) CountAgentsByProviderAndModel(context.Context, string, string) (int, error) {
	return 0, nil
}
func (r *memoryTestAgentRepo) CreateAgentAtomic(_ context.Context, a biz.Agent, _ []biz.AgentPromptFile, _ biz.AgentRuntimeSettings) (biz.Agent, error) {
	return a, nil
}
func (r *memoryTestAgentRepo) UpdateAgentAtomic(_ context.Context, a biz.Agent, _ []biz.AgentPromptFile, _ *biz.AgentRuntimeSettings) (biz.Agent, error) {
	return a, nil
}
func (r *memoryTestAgentRepo) ToggleFavorite(context.Context, string) (biz.Agent, error) {
	return biz.Agent{}, nil
}

type fakeConsolidationWriter struct {
	mu      sync.Mutex
	facts   []biz.MemoryFactWrite
	episode *biz.EpisodeWrite
}

func (f *fakeConsolidationWriter) UpsertFactsAndEpisodeBatch(_ context.Context, facts []biz.MemoryFactWrite, ep *biz.EpisodeWrite) (*biz.ConsolidationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.facts = append(f.facts, facts...)
	if ep != nil {
		f.episode = ep
	}
	return &biz.ConsolidationResult{
		FactRows:     make([][]byte, len(facts)),
		FactsWritten: len(facts),
	}, nil
}

func (f *fakeConsolidationWriter) getFacts() []biz.MemoryFactWrite {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]biz.MemoryFactWrite, len(f.facts))
	copy(cp, f.facts)
	return cp
}

func (f *fakeConsolidationWriter) getEpisode() *biz.EpisodeWrite {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.episode
}

func TestAutoMemoryWorker_ExtractChain(t *testing.T) {
	writer := &fakeConsolidationWriter{}
	ctx := context.Background()

	const (
		sessID  = "sess-int-1"
		agentID = "agent-int-1"
		userID  = "user-int-1"
		msgID   = "msg-u-1"
	)

	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: sessID, AgentID: agentID, UserID: userID},
		msgs: []sessionsess.ChatMessage{{
			ID: msgID, SessionID: sessID, Role: "user", ContentMarkdown: "I prefer dark mode",
		}},
	}
	sessionsUC := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil)
	agentsUC := newMemoryEnabledAgentsUC(agentID)
	q := memtrpc.NewMemoryJobQueue(&conf.Runtime{}, 4, 0, loggateway.NewNoop())
	w, err := NewAutoMemoryWorker(AutoMemoryWorkerConfig{
		RuntimeConf:  &conf.Runtime{},
		Interval:     0,
		Sessions:     sessionsUC,
		Agents:       agentsUC,
		Writer:       writer,
		Consolidator: biz.NewHeuristicConsolidator(),
		Queue:        q,
		Logger:       loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewAutoMemoryWorker: %v", err)
	}

	req := memtrpc.AutoMemoryJobRequest{SessionID: sessID, UserID: userID, AppName: agentID}
	if err := w.extract(ctx, req); err != nil {
		t.Fatalf("extract: %v", err)
	}

	facts := writer.getFacts()
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].SourceKind != "auto_memory" {
		t.Fatalf("source_kind=%q want auto_memory", facts[0].SourceKind)
	}
	if facts[0].SourceMessageID != msgID {
		t.Fatalf("source_message_id=%q want %q", facts[0].SourceMessageID, msgID)
	}

	ep := writer.getEpisode()
	if ep == nil {
		t.Fatal("expected episode, got nil")
	}
	if ep.ConsolidatedL3 != 1 {
		t.Fatalf("consolidated_l3=%d want 1", ep.ConsolidatedL3)
	}
}

func TestAutoMemoryWorker_DrainUsesInjectedQueue(t *testing.T) {
	writer := &fakeConsolidationWriter{}
	ctx := context.Background()

	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: "sess-q-1", AgentID: "agent-q-1", UserID: "user-q-1"},
		msgs: []sessionsess.ChatMessage{{
			ID: "m1", SessionID: "sess-q-1", Role: "user", ContentMarkdown: "My name is Alice",
		}},
	}
	sessionsUC := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil)
	agentsUC := newMemoryEnabledAgentsUC("agent-q-1")
	q := memtrpc.NewMemoryJobQueue(&conf.Runtime{}, 4, 0, loggateway.NewNoop())
	w, err := NewAutoMemoryWorker(AutoMemoryWorkerConfig{
		RuntimeConf:  &conf.Runtime{},
		Interval:     0,
		Sessions:     sessionsUC,
		Agents:       agentsUC,
		Writer:       writer,
		Consolidator: biz.NewHeuristicConsolidator(),
		Queue:        q,
		Logger:       loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewAutoMemoryWorker: %v", err)
	}

	q.Enqueue(memtrpc.AutoMemoryJobRequest{SessionID: "sess-q-1", UserID: "user-q-1", AppName: "agent-q-1"})
	deadline := time.Now().Add(2 * time.Second)
	for {
		w.drain(ctx)
		facts := writer.getFacts()
		if len(facts) >= 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected fact after queue drain, got %d", len(facts))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

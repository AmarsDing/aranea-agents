package biz

import (
	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
	"context"
	"sync"
	"testing"
	"time"
)

func TestHandleTeamRejection_IllegalTransition_ReturnsError(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	teamRepo.items["t1"] = Team{ID: "t1", Status: TeamStatusCompleted, DisplayName: "done"}
	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, Lg: loggateway.NewNoop()})
	uc := NewSpiritTeamUsecase(teamUC, nil, nil, loggateway.NewNoop())
	_, err := uc.HandleTeamRejection(context.Background(), "t1", ReworkTracker{MaxRetries: 3}, "quality")
	if err == nil {
		t.Fatal("expected error when completed team cannot rework to pending")
	}
}

type keyedSpiritAgentResolver struct {
	id  string
	key string
}

func (m *keyedSpiritAgentResolver) List(_ context.Context, q AgentListQuery) (AgentListResult, error) {
	if q.Keyword != "" && q.Keyword != m.key {
		return AgentListResult{}, nil
	}
	return AgentListResult{Items: []Agent{{ID: m.id, AgentKey: m.key}}, Total: 1}, nil
}

type recordingAllDoneNotifier struct {
	ids []string
}

func (n *recordingAllDoneNotifier) NotifyAllTeamsCompleted(_ context.Context, spiritSessionID string) {
	n.ids = append(n.ids, spiritSessionID)
}

func TestPollTeamCompletions_FiltersBySpiritAgent(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.items["sp-run"] = Session{
		ID: "sp-run", AgentID: "agent_spirit", Status: string(session.SessionStatusRunning),
	}
	sessionRepo.items["chat-run"] = Session{
		ID: "chat-run", AgentID: "agent_chat", Status: string(session.SessionStatusRunning),
	}
	teamRepo.items["t-sp"] = Team{ID: "t-sp", SpiritSessionID: "sp-run", Status: TeamStatusCompleted}
	teamRepo.items["t-chat"] = Team{ID: "t-chat", SpiritSessionID: "chat-run", Status: TeamStatusCompleted}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &keyedSpiritAgentResolver{id: "agent_spirit", key: SpiritAgentKey}, loggateway.NewNoop())
	n := &recordingAllDoneNotifier{}
	uc.SetAllTeamsCompletedNotifier(n)
	uc.orchestration.pollTeamCompletions(context.Background())
	if len(n.ids) != 1 || n.ids[0] != "sp-run" {
		t.Fatalf("notified %v, want [sp-run] (chat sessions must not be scanned)", n.ids)
	}
}

func TestPollTeamCompletions_NoSpiritAgent_SkipsScan(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.items["sp-run"] = Session{
		ID: "sp-run", AgentID: "agent_spirit", Status: string(session.SessionStatusRunning),
	}
	teamRepo.items["t-sp"] = Team{ID: "t-sp", SpiritSessionID: "sp-run", Status: TeamStatusCompleted}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &keyedSpiritAgentResolver{id: "agent_spirit", key: "not-spirit"}, loggateway.NewNoop())
	n := &recordingAllDoneNotifier{}
	uc.SetAllTeamsCompletedNotifier(n)
	uc.orchestration.pollTeamCompletions(context.Background())
	if len(n.ids) != 0 {
		t.Fatalf("notified %v, want none when spirit agent cannot be resolved", n.ids)
	}
}

func TestAssembleTeam_DoesNotRegisterTimeout(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.seedSpirit("spirit-1")
	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(&memSpiritTransactor{}))

	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-1",
		TaskDescription: "pending timeout must not start at assemble",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       false,
	})
	if err != nil {
		t.Fatalf("AssembleTeam: %v", err)
	}
	if _, loaded := uc.orchestration.timeouts.timers.Load(result.Team.ID); loaded {
		t.Fatal("team timeout must be registered at StartTeamTurn, not AssembleTeam")
	}
}

type recordingTimeoutHandler struct {
	mu    sync.Mutex
	calls []string
}

func (h *recordingTimeoutHandler) HandleTeamTimeout(_ context.Context, _, teamID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, teamID)
}

func TestRegisterTeamTimeout_Pending_SkipsFail(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	teamRepo.items["t-pending"] = Team{
		ID: "t-pending", Status: TeamStatusPending, SpiritSessionID: "spirit-1",
	}
	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, Lg: loggateway.NewNoop()})
	orch := &SpiritOrchestration{
		teamUC:   teamUC,
		timeouts: &teamTimeoutRegistry{},
		lg:       loggateway.NewNoop(),
	}
	handler := &recordingTimeoutHandler{}
	orch.SetTimeoutHandler(handler)
	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-pending")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, loaded := orch.timeouts.timers.Load("t-pending"); !loaded {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)

	got, err := teamUC.Get(context.Background(), "t-pending")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != TeamStatusPending {
		t.Fatalf("pending team status=%q, timeout must not fail it", got.Status)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.calls) != 0 {
		t.Fatalf("timeout handler called for pending team: %v", handler.calls)
	}
}

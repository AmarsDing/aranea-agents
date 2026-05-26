package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

type stubTeamRepo struct {
	team biz.Team
}

func (s stubTeamRepo) ListTeams(context.Context) ([]biz.Team, error) { return nil, nil }
func (s stubTeamRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	if id != s.team.ID {
		return biz.Team{}, nil
	}
	return s.team, nil
}
func (s stubTeamRepo) CreateTeam(context.Context, biz.Team) (biz.Team, error) { return biz.Team{}, nil }
func (s stubTeamRepo) UpdateTeam(context.Context, biz.Team) (biz.Team, error) { return biz.Team{}, nil }
func (s stubTeamRepo) DeleteTeam(context.Context, string) error               { return nil }
func (s stubTeamRepo) ListTeamRuns(context.Context, string, int) ([]biz.TeamRun, error) {
	return nil, nil
}
func (s stubTeamRepo) GetTeamRunByID(context.Context, string) (biz.TeamRun, error) {
	return biz.TeamRun{}, nil
}
func (s stubTeamRepo) ListTeamRunSteps(context.Context, string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (s stubTeamRepo) CreateTeamRun(context.Context, biz.TeamRun) (biz.TeamRun, error) {
	return biz.TeamRun{}, nil
}
func (s stubTeamRepo) UpdateTeamRun(context.Context, biz.TeamRun) error { return nil }
func (s stubTeamRepo) UpdateTeamRunGraphExecutionID(context.Context, string, string) error {
	return nil
}
func (s stubTeamRepo) UpdateTeamRunTraceID(context.Context, string, string) error { return nil }
func (s stubTeamRepo) BatchCreateOrchestrationSteps(context.Context, []biz.OrchestrationStep) error {
	return nil
}
func (s stubTeamRepo) ListOrchestrationSteps(context.Context, string, string, int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (s stubTeamRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error { return nil }
func (s stubTeamRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (s stubTeamRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}
func (s stubTeamRepo) UpdateTeamRunSummaryJSON(context.Context, string, string) error { return nil }
func (s stubTeamRepo) CreateTeamRunStep(context.Context, biz.TeamRunStep) (biz.TeamRunStep, error) {
	return biz.TeamRunStep{}, nil
}

type channelTestAgentRepo struct {
	key string
}

func (s channelTestAgentRepo) SearchAgents(context.Context, biz.AgentListQuery) (biz.AgentListResult, error) {
	return biz.AgentListResult{}, nil
}
func (s channelTestAgentRepo) GetAgentByID(context.Context, string) (biz.Agent, error) {
	return biz.Agent{AgentKey: s.key}, nil
}
func (s channelTestAgentRepo) GetAgentByAgentKey(context.Context, string) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (s channelTestAgentRepo) CreateAgent(context.Context, biz.Agent) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (s channelTestAgentRepo) UpdateAgent(context.Context, biz.Agent) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (s channelTestAgentRepo) DeleteAgent(context.Context, string) error { return nil }
func (s channelTestAgentRepo) GetAgentRuntimeSettings(context.Context, string) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{}, nil
}
func (s channelTestAgentRepo) UpsertAgentRuntimeSettings(context.Context, biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{}, nil
}
func (s channelTestAgentRepo) ListAgentPromptFiles(context.Context, string) ([]biz.AgentPromptFile, error) {
	return nil, nil
}
func (s channelTestAgentRepo) ReplaceAgentPromptFiles(context.Context, string, []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	return nil, nil
}
func (s channelTestAgentRepo) CreateAgentPromptFile(context.Context, biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	return biz.AgentPromptFile{}, nil
}
func (s channelTestAgentRepo) UpdateAgentPromptFile(context.Context, biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	return biz.AgentPromptFile{}, nil
}
func (s channelTestAgentRepo) DeleteAgentPromptFile(context.Context, string, string) error { return nil }
func (s channelTestAgentRepo) ListExtrasForAgents(context.Context, []string) (map[string]biz.AgentListExtras, error) {
	return map[string]biz.AgentListExtras{}, nil
}
func (s channelTestAgentRepo) ListAgentCreators(context.Context) ([]biz.AgentCreator, error) {
	return nil, nil
}
func (s channelTestAgentRepo) ExecInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type stubGraphExecutor struct {
	lastGraphID string
	lastSession string
	lastCfg     biz.GraphBuildConfig
}

func (s *stubGraphExecutor) ExecuteGraphByID(_ context.Context, graphID, _ string, _ map[string]any) (string, error) {
	s.lastGraphID = graphID
	return "exec-graph-1", nil
}

func (s *stubGraphExecutor) ExecuteGraphBuildConfig(_ context.Context, graphID, sessionID string, cfg biz.GraphBuildConfig, _ map[string]any) (string, error) {
	s.lastGraphID = graphID
	s.lastSession = sessionID
	s.lastCfg = cfg
	return "exec-team-1", nil
}

func TestExecuteAsyncGraphTarget_teamGraph(t *testing.T) {
	defJSON := `{"version":1,"mode":"sequential","linked_graph_id":"linked-g-1","members":[{"agent_id":"agent-1","sort_order":1}]}`
	exec := &stubGraphExecutor{}
	h := &ChannelIngress{
		teams: stubTeamRepo{team: biz.Team{
			ID:             "team-42",
			DefinitionJSON: defJSON,
		}},
		agents: channelTestAgentRepo{key: "worker-key"},
		graphs: exec,
	}
	target := biz.ChannelAsyncGraphTarget{TargetType: "team_graph", TeamID: "team-42"}
	tt, gid, asyncID, err := h.executeAsyncGraphTarget(context.Background(), target, "sess-99", map[string]any{"input": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if tt != "team_graph" || gid != "linked-g-1" || asyncID != "exec-team-1" {
		t.Fatalf("got type=%q graph=%q async=%q", tt, gid, asyncID)
	}
	if exec.lastSession != "sess-99" {
		t.Fatalf("session=%q", exec.lastSession)
	}
	if len(exec.lastCfg.Nodes) == 0 {
		t.Fatal("expected compiled graph nodes")
	}
	if exec.lastCfg.Nodes[0].AgentName != "worker-key" {
		t.Fatalf("agent_name=%q want worker-key", exec.lastCfg.Nodes[0].AgentName)
	}
}

func TestExecuteAsyncGraphTarget_teamGraphFallbackGraphID(t *testing.T) {
	defJSON := `{"version":1,"mode":"sequential","members":[{"agent_id":"agent-1","sort_order":1}]}`
	exec := &stubGraphExecutor{}
	h := &ChannelIngress{
		teams: stubTeamRepo{team: biz.Team{
			ID:             "team-7",
			DefinitionJSON: defJSON,
		}},
		agents: channelTestAgentRepo{key: "k1"},
		graphs: exec,
	}
	target := biz.ChannelAsyncGraphTarget{TargetType: "team_graph", TeamID: "team-7"}
	_, gid, _, err := h.executeAsyncGraphTarget(context.Background(), target, "sess-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if gid != "team-team-7" {
		t.Fatalf("graph id=%q want team-team-7", gid)
	}
}

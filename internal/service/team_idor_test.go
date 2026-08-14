package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// newIDORTeamService wires a TeamService with tenant-scoped teams/runs/sessions
// for N5 IDOR tests.
func newIDORTeamService() *TeamService {
	repo := &cancelTeamRunRepo{
		teamByID: map[string]biz.Team{
			"team-private-a": {ID: "team-private-a", TeamKey: "a", WorkspaceID: "ws-a"},
			"team-shared":    {ID: "team-shared", TeamKey: "s", WorkspaceID: ""},
		},
		runs: map[string]biz.TeamRunRecord{
			"run-a1": {ID: "run-a1", TeamID: "team-private-a", SessionID: "sess-a1", Status: biz.TeamRunStatusRunning},
			"run-s1": {ID: "run-s1", TeamID: "team-shared", SessionID: "sess-s1", Status: biz.TeamRunStatusPaused},
		},
		deadLetters: map[string]biz.TaskDeadLetter{
			"dl-a1": {ID: "dl-a1", TeamID: "team-private-a", Status: biz.TaskDeadLetterStatusPending},
		},
	}
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo,
		StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop(),
	})
	sessionUC := biz.NewSessionUsecase(&f10SessionRepo{sessions: []biz.Session{
		{ID: "spirit-a", WorkspaceID: "ws-a"},
		{ID: "spirit-shared", WorkspaceID: ""},
	}}, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	bus := event.NewV2Bus()
	return NewTeamService(uc, nil, nil, sessionUC, nil, &testRunRegistry{}, bus, loggateway.NewNoop(), nil, nil, nil, nil, nil, nil, nil, nil)
}

// TestTeamService_IDOR covers N5: the previously unguarded TeamService RPCs
// must enforce workspace access (team → workspace / run → team → workspace /
// spirit session → workspace).
func TestTeamService_IDOR(t *testing.T) {
	svc := newIDORTeamService()
	tenantB := wsCtx("ws-b")
	ctx := context.Background()

	t.Run("PauseTeamRun_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.PauseTeamRun(tenantB, &v1.PauseTeamRunRequest{Id: "run-a1"})
		assertNotFound(t, err, "PauseTeamRun")
	})

	t.Run("UnpauseTeamRun_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.UnpauseTeamRun(tenantB, &v1.UnpauseTeamRunRequest{Id: "run-a1"})
		assertNotFound(t, err, "UnpauseTeamRun")
	})

	t.Run("InjectTeamMessage_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.InjectTeamMessage(tenantB, &v1.InjectTeamMessageRequest{TeamId: "team-private-a", Message: "hi"})
		assertNotFound(t, err, "InjectTeamMessage")
	})

	t.Run("ResumeTeamRunExecution_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ResumeTeamRunExecution(tenantB, &v1.ResumeTeamRunExecutionRequest{RunId: "run-a1"})
		assertNotFound(t, err, "ResumeTeamRunExecution")
	})

	t.Run("ListTaskDeadLetters_ByTeam_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListTaskDeadLetters(tenantB, &v1.ListTaskDeadLettersRequest{TeamId: "team-private-a"})
		assertNotFound(t, err, "ListTaskDeadLetters(team_id)")
	})

	t.Run("ListTaskDeadLetters_BySession_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListTaskDeadLetters(tenantB, &v1.ListTaskDeadLettersRequest{SessionId: "spirit-a"})
		assertNotFound(t, err, "ListTaskDeadLetters(session_id)")
	})

	t.Run("ResolveTaskDeadLetter_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ResolveTaskDeadLetter(tenantB, &v1.ResolveTaskDeadLetterRequest{Id: "dl-a1"})
		assertNotFound(t, err, "ResolveTaskDeadLetter")
	})

	t.Run("ListSpiritTeams_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ListSpiritTeams(tenantB, &v1.ListSpiritTeamsRequest{SpiritSessionId: "spirit-a"})
		assertNotFound(t, err, "ListSpiritTeams")
	})

	t.Run("SynthesizeResults_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.SynthesizeResults(tenantB, &v1.SynthesizeResultsRequest{SpiritSessionId: "spirit-a"})
		assertNotFound(t, err, "SynthesizeResults")
	})

	t.Run("ArchiveTeam_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.ArchiveTeam(tenantB, &v1.ArchiveTeamRequest{TeamId: "team-private-a"})
		assertNotFound(t, err, "ArchiveTeam")
	})

	t.Run("RetryTeam_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.RetryTeam(tenantB, &v1.RetryTeamRequest{TeamId: "team-private-a"})
		assertNotFound(t, err, "RetryTeam")
	})

	t.Run("GetTeamRunObservatory_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.GetTeamRunObservatory(tenantB, &v1.GetTeamRunObservatoryRequest{RunId: "run-a1"})
		assertNotFound(t, err, "GetTeamRunObservatory")
	})

	t.Run("GetTeamRunObservatoryTimeline_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.GetTeamRunObservatoryTimeline(tenantB, &v1.GetTeamRunObservatoryTimelineRequest{RunId: "run-a1"})
		assertNotFound(t, err, "GetTeamRunObservatoryTimeline")
	})

	t.Run("CompileTeamGraph_CrossTenant_Denied", func(t *testing.T) {
		_, err := svc.CompileTeamGraph(tenantB, &v1.CompileTeamGraphRequest{TeamId: "team-private-a"})
		assertNotFound(t, err, "CompileTeamGraph")
	})

	t.Run("PauseTeamRun_OwnerWorkspace_Allowed", func(t *testing.T) {
		resp, err := svc.PauseTeamRun(wsCtx("ws-a"), &v1.PauseTeamRunRequest{Id: "run-a1"})
		if err != nil {
			t.Fatalf("owner workspace should be allowed: %v", err)
		}
		if resp.GetStatus() != biz.TeamRunStatusPaused {
			t.Fatalf("status=%q want %q", resp.GetStatus(), biz.TeamRunStatusPaused)
		}
	})

	t.Run("ListSpiritTeams_OwnerWorkspace_Allowed", func(t *testing.T) {
		if _, err := svc.ListSpiritTeams(wsCtx("ws-a"), &v1.ListSpiritTeamsRequest{SpiritSessionId: "spirit-a"}); err != nil {
			t.Fatalf("owner workspace should be allowed: %v", err)
		}
	})

	t.Run("ListTaskDeadLetters_OwnerWorkspace_Allowed", func(t *testing.T) {
		if _, err := svc.ListTaskDeadLetters(wsCtx("ws-a"), &v1.ListTaskDeadLettersRequest{TeamId: "team-private-a"}); err != nil {
			t.Fatalf("owner workspace should be allowed: %v", err)
		}
	})

	t.Run("PauseTeamRun_NoWorkspaceContext_SharedTeamRun_Denied", func(t *testing.T) {
		// Mutating a shared team run as a non-system caller must fail closed.
		_, err := svc.UnpauseTeamRun(ctx, &v1.UnpauseTeamRunRequest{Id: "run-s1"})
		assertNotFound(t, err, "UnpauseTeamRun(shared, default-ws caller)")
	})
}

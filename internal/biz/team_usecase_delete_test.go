package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// --- stubs for TeamUsecase Delete tests ---

type stubTeamReader struct {
	team Team
}

func (s *stubTeamReader) ListTeams(context.Context) ([]Team, error)            { return nil, nil }
func (s *stubTeamReader) ListTeamsByStatus(context.Context, string) ([]Team, error) { return nil, nil }
func (s *stubTeamReader) GetTeamByID(_ context.Context, id string) (Team, error) {
	if s.team.ID != id {
		return Team{}, ErrNotFound
	}
	return s.team, nil
}
func (s *stubTeamReader) GetTeamByKey(context.Context, string) (Team, error) { return Team{}, ErrNotFound }
func (s *stubTeamReader) ListBySpiritSessionID(context.Context, string) ([]Team, error) { return nil, nil }
func (s *stubTeamReader) ListTeamsByDepartmentID(context.Context, string) ([]Team, error) { return nil, nil }

type stubTeamWriter struct {
	deletedID string
}

func (s *stubTeamWriter) CreateTeam(context.Context, Team) (Team, error) { return Team{}, nil }
func (s *stubTeamWriter) UpdateTeam(context.Context, Team) (Team, error) { return Team{}, nil }
func (s *stubTeamWriter) DeleteTeam(_ context.Context, id string) error {
	s.deletedID = id
	return nil
}
func (s *stubTeamWriter) BatchArchiveTeams(_ context.Context, ids []string) (int, error) {
	return len(ids), nil
}

type stubTeamRunReader struct{}

func (s *stubTeamRunReader) ListTeamRuns(context.Context, string, int) ([]TeamRun, error) { return nil, nil }
func (s *stubTeamRunReader) ListTeamRunsByTeamIDs(context.Context, []string, int) (map[string][]TeamRun, error) {
	return nil, nil
}
func (s *stubTeamRunReader) HasActiveTeamRun(context.Context, string) (bool, error)       { return false, nil }
func (s *stubTeamRunReader) GetTeamRunByID(context.Context, string) (TeamRun, error)      { return TeamRun{}, nil }
func (s *stubTeamRunReader) ListTeamRunSteps(context.Context, string) ([]TeamRunStep, error) {
	return nil, nil
}

type stubTeamRunWriter struct{}

func (s *stubTeamRunWriter) CreateTeamRun(context.Context, TeamRun) (TeamRun, error)          { return TeamRun{}, nil }
func (s *stubTeamRunWriter) UpdateTeamRun(context.Context, TeamRun) error                      { return nil }
func (s *stubTeamRunWriter) UpdateTeamRunGraphExecutionID(context.Context, string, string) error { return nil }
func (s *stubTeamRunWriter) UpdateTeamRunTraceID(context.Context, string, string) error        { return nil }
func (s *stubTeamRunWriter) UpdateTeamRunSummaryJSON(context.Context, string, string) error    { return nil }
func (s *stubTeamRunWriter) CreateTeamRunStep(context.Context, TeamRunStep) (TeamRunStep, error) { return TeamRunStep{}, nil }

type stubOrchestrationStepRepo struct{}

func (s *stubOrchestrationStepRepo) BatchCreateOrchestrationSteps(context.Context, []OrchestrationStep) error {
	return nil
}
func (s *stubOrchestrationStepRepo) ListOrchestrationSteps(context.Context, string, string, int) ([]OrchestrationStep, error) {
	return nil, nil
}

type stubTaskDeadLetterRepo struct{}

func (s *stubTaskDeadLetterRepo) CreateTaskDeadLetter(context.Context, TaskDeadLetter) error        { return nil }
func (s *stubTaskDeadLetterRepo) ListTaskDeadLetters(context.Context, TaskDeadLetterListFilter) ([]TaskDeadLetter, error) {
	return nil, nil
}
func (s *stubTaskDeadLetterRepo) ResolveTaskDeadLetter(context.Context, string) (TaskDeadLetter, error) {
	return TaskDeadLetter{}, nil
}

// --- tests ---

func TestTeamUsecase_DeleteRejectsSystemBuiltin(t *testing.T) {
	t.Parallel()
	reader := &stubTeamReader{team: Team{ID: "team-1", Kind: "system_builtin"}}
	writer := &stubTeamWriter{}
	uc := NewTeamUsecase(reader, writer, &stubTeamRunReader{}, &stubTeamRunWriter{}, &stubOrchestrationStepRepo{}, &stubTaskDeadLetterRepo{}, nil, nil, nil, nil, loggateway.NewNoop())
	err := uc.Delete(context.Background(), "team-1")
	if err == nil {
		t.Fatal("expected error when deleting system_builtin team")
	}
	e, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if e.Code != apierror.CodeForbidden {
		t.Fatalf("expected code %s, got %s", apierror.CodeForbidden, e.Code)
	}
	if e.Domain != "TEAM" {
		t.Fatalf("expected domain TEAM, got %s", e.Domain)
	}
	if writer.deletedID != "" {
		t.Fatal("expected team not to be deleted")
	}
}

func TestTeamUsecase_DeleteAllowsUserTeam(t *testing.T) {
	t.Parallel()
	reader := &stubTeamReader{team: Team{ID: "team-2", Kind: "user"}}
	writer := &stubTeamWriter{}
	uc := NewTeamUsecase(reader, writer, &stubTeamRunReader{}, &stubTeamRunWriter{}, &stubOrchestrationStepRepo{}, &stubTaskDeadLetterRepo{}, nil, nil, nil, nil, loggateway.NewNoop())
	err := uc.Delete(context.Background(), "team-2")
	if err != nil {
		t.Fatalf("expected no error when deleting user team, got %v", err)
	}
	if writer.deletedID != "team-2" {
		t.Fatalf("expected team-2 to be deleted, got %q", writer.deletedID)
	}
}

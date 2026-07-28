package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// stubTeamDeliverablePort implements SpiritTeamDeliverablePort for tool tests.
type stubTeamDeliverablePort struct {
	teams   []biz.Team
	listErr error

	out biz.UpstreamDeliverableContent
	err error

	gotReaderSessionID string
	gotTeamID          string
	gotMaxChars        int
	readCalls          int
}

func (s *stubTeamDeliverablePort) ListAllTeams(_ context.Context, _ string) ([]biz.Team, error) {
	return s.teams, s.listErr
}

func (s *stubTeamDeliverablePort) ReadUpstreamDeliverable(_ context.Context, readerSessionID, teamID string, maxChars int) (biz.UpstreamDeliverableContent, error) {
	s.readCalls++
	s.gotReaderSessionID = readerSessionID
	s.gotTeamID = teamID
	s.gotMaxChars = maxChars
	return s.out, s.err
}

func spiritInvocationCtx(sessionID string) context.Context {
	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: sessionID}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func TestGetTeamDeliverableTool_Declaration(t *testing.T) {
	tl := NewGetTeamDeliverableTool(&stubTeamDeliverablePort{})
	decl := tl.Declaration()
	if decl.Name != "get_team_deliverable" {
		t.Fatalf("name=%q want get_team_deliverable", decl.Name)
	}
	if !strings.Contains(decl.Description, "交付物") {
		t.Fatalf("description should explain deliverable retrieval, got %q", decl.Description)
	}
}

// F6 (Phase 11): empty team_id lists this spirit session's teams so the
// spirit can pick one — no more read_session_history archaeology.
func TestGetTeamDeliverableTool_Call_EmptyTeamID_ListsTeams(t *testing.T) {
	port := &stubTeamDeliverablePort{teams: []biz.Team{
		{ID: "t-1", DisplayName: "安装 xlsx", Status: "completed", TaskDescription: "安装 xlsx skill"},
		{ID: "t-2", DisplayName: "调研报告", Status: "running", TaskDescription: "调研"},
	}}
	tl := NewGetTeamDeliverableTool(port)

	res, err := tl.Call(spiritInvocationCtx("sess-spirit"), []byte(`{}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	out, ok := res.(GetTeamDeliverableOutput)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if len(out.Teams) != 2 {
		t.Fatalf("expected 2 teams, got %+v", out.Teams)
	}
	if out.Teams[0].TeamID != "t-1" || out.Teams[0].TeamName != "安装 xlsx" || out.Teams[0].Status != "completed" || out.Teams[0].Task == "" {
		t.Fatalf("team brief mapping mismatch: %+v", out.Teams[0])
	}
	if port.readCalls != 0 {
		t.Fatalf("listing must not trigger deliverable read, got %d reads", port.readCalls)
	}
}

func TestGetTeamDeliverableTool_Call_HappyPath(t *testing.T) {
	port := &stubTeamDeliverablePort{
		teams: []biz.Team{{ID: "t-1", DisplayName: "安装 xlsx", Status: "completed"}},
		out: biz.UpstreamDeliverableContent{
			Content:   `{"status":"success","detail":"installed"}`,
			SizeChars: 40,
			TeamID:    "t-1",
		},
	}
	tl := NewGetTeamDeliverableTool(port)

	res, err := tl.Call(spiritInvocationCtx("sess-spirit"), []byte(`{"team_id":"t-1","max_chars":8000}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	out := res.(GetTeamDeliverableOutput)
	if out.Content == "" || out.SizeChars != 40 || out.Error != "" {
		t.Fatalf("output mismatch: %+v", out)
	}
	if out.TeamName != "安装 xlsx" || out.Status != "completed" {
		t.Fatalf("team context mismatch: %+v", out)
	}
	// Spirit reads with empty readerSessionID → contract validation exempted.
	if port.gotReaderSessionID != "" {
		t.Fatalf("reader session id must be empty for spirit reads, got %q", port.gotReaderSessionID)
	}
	if port.gotTeamID != "t-1" || port.gotMaxChars != 8000 {
		t.Fatalf("read args mismatch: team=%q max=%d", port.gotTeamID, port.gotMaxChars)
	}
}

// F6: read failures (team not completed / no deliverable) surface as the
// structured error field, never as a tool-level error — the spirit LLM can
// read the reason and recover.
func TestGetTeamDeliverableTool_Call_ReadError_StructuredField(t *testing.T) {
	port := &stubTeamDeliverablePort{
		teams: []biz.Team{{ID: "t-2", DisplayName: "调研报告", Status: "running"}},
		err:   errors.New("upstream team t-2 has not completed yet (status=running)"),
	}
	tl := NewGetTeamDeliverableTool(port)

	res, err := tl.Call(spiritInvocationCtx("sess-spirit"), []byte(`{"team_id":"t-2"}`))
	if err != nil {
		t.Fatalf("read failure must NOT be a tool error, got %v", err)
	}
	out := res.(GetTeamDeliverableOutput)
	if out.Error == "" || !strings.Contains(out.Error, "not completed") {
		t.Fatalf("error field should carry the reason, got %+v", out)
	}
	if out.Status != "running" || out.TeamName != "调研报告" || out.Content != "" {
		t.Fatalf("context fields mismatch: %+v", out)
	}
}

// Without a spirit invocation context the tool cannot scope the team list —
// this IS a tool-level error (consistent with other spirit tools).
func TestGetTeamDeliverableTool_Call_NoInvocation_Errors(t *testing.T) {
	tl := NewGetTeamDeliverableTool(&stubTeamDeliverablePort{})
	if _, err := tl.Call(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("missing spirit session id should error")
	}
}

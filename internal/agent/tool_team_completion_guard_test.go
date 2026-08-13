package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type fakeTeamCompletionChecker struct {
	result biz.AllTeamsCompletedResult
}

func (f fakeTeamCompletionChecker) CheckAllTeamsCompleted(context.Context, string) biz.AllTeamsCompletedResult {
	return f.result
}

func spiritInvocationCtx() context.Context {
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "spirit-s1"}}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

// WP-2a: synthesize_results must be blocked deterministically while teams are
// still running (production: 124 failures, 71.7% of its calls, all from the
// same CONFLICT). The guard already covers get_team_deliverable; synthesis
// has the same precondition.
func TestTeamCompletionGuard_BlocksSynthesizeResultsWhenTeamsRunning(t *testing.T) {
	hook := newTeamCompletionGuardBeforeHook(fakeTeamCompletionChecker{result: biz.AllTeamsCompletedResult{
		AllDone: false, TotalTeams: 3, CompletedTeams: 1,
	}}, nil)
	res, err := hook.HandleBeforeTool(spiritInvocationCtx(), &trpctool.BeforeToolArgs{ToolName: "synthesize_results"})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res == nil || res.CustomResult == nil {
		t.Fatal("expected guard to block synthesize_results with a guidance result, got nil")
	}
	guidance, _ := res.CustomResult.(string)
	if !strings.Contains(guidance, "1/3") {
		t.Fatalf("guidance must carry structured progress (1/3), got %q", guidance)
	}
}

func TestTeamCompletionGuard_AllowsSynthesizeResultsWhenAllDone(t *testing.T) {
	hook := newTeamCompletionGuardBeforeHook(fakeTeamCompletionChecker{result: biz.AllTeamsCompletedResult{
		AllDone: true, TotalTeams: 2, CompletedTeams: 2,
	}}, nil)
	res, err := hook.HandleBeforeTool(spiritInvocationCtx(), &trpctool.BeforeToolArgs{ToolName: "synthesize_results"})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res == nil || res.CustomResult != nil {
		t.Fatalf("all teams done must pass through, got %+v", res)
	}
}

func TestTeamCompletionGuard_BlocksGetTeamDeliverable(t *testing.T) {
	hook := newTeamCompletionGuardBeforeHook(fakeTeamCompletionChecker{result: biz.AllTeamsCompletedResult{
		AllDone: false, TotalTeams: 2, CompletedTeams: 0,
	}}, nil)
	res, err := hook.HandleBeforeTool(spiritInvocationCtx(), &trpctool.BeforeToolArgs{ToolName: "get_team_deliverable"})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res == nil || res.CustomResult == nil {
		t.Fatal("expected guard to block get_team_deliverable while teams running")
	}
}

func TestTeamCompletionGuard_IgnoresUnrelatedTools(t *testing.T) {
	hook := newTeamCompletionGuardBeforeHook(fakeTeamCompletionChecker{result: biz.AllTeamsCompletedResult{
		AllDone: false, TotalTeams: 2, CompletedTeams: 0,
	}}, nil)
	res, err := hook.HandleBeforeTool(spiritInvocationCtx(), &trpctool.BeforeToolArgs{ToolName: "read_file"})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res == nil || res.CustomResult != nil {
		t.Fatalf("unrelated tools must pass through, got %+v", res)
	}
}

package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

func TestProjectStateCueFromInvocation_NoStateReturnsEmpty(t *testing.T) {
	// 非团队运行（无 RuntimeState / session 也无 key）→ 零开销空切片。
	if got := ProjectStateCueFromInvocation(nil, projectStateCueBudgetRunes); got != "" {
		t.Fatalf("nil invocation must yield empty cue, got %q", got)
	}
	inv := trpcagent.NewInvocation()
	if got := ProjectStateCueFromInvocation(inv, projectStateCueBudgetRunes); got != "" {
		t.Fatalf("no project_state must yield empty cue, got %q", got)
	}
}

func TestProjectStateCueFromInvocation_RuntimeStateAuthoritative(t *testing.T) {
	var ps biz.TeamProjectState
	ps.SetActiveRequests([]string{"需求评审"})
	ps.RollChange("leader", "拆解了任务")

	// RuntimeState 携带空 map 时即为权威：session 里的旧状态不得漏进来。
	stale := biz.TeamProjectState{}
	stale.RecordMilestone("上个 run 的旧里程碑")
	staleBytes, _ := json.Marshal(stale.ToMap())
	sess := session.NewSession("app", "user", "sess")
	sess.SetState(biz.ProjectStateKey, staleBytes)

	inv := trpcagent.NewInvocation()
	inv.Session = sess
	inv.RunOptions.RuntimeState = map[string]any{biz.ProjectStateKey: map[string]any{}}
	if got := ProjectStateCueFromInvocation(inv, projectStateCueBudgetRunes); got != "" {
		t.Fatalf("empty authoritative RuntimeState must suppress stale session state, got %q", got)
	}

	// RuntimeState 携带真实状态时渲染切片。
	inv.RunOptions.RuntimeState[biz.ProjectStateKey] = ps.ToMap()
	got := ProjectStateCueFromInvocation(inv, projectStateCueBudgetRunes)
	if !strings.Contains(got, "需求评审") || !strings.Contains(got, "拆解了任务") {
		t.Fatalf("cue missing expected entries: %q", got)
	}
}

func TestProjectStateCueFromInvocation_SessionFallbackWithinBudget(t *testing.T) {
	var ps biz.TeamProjectState
	ps.SetActiveRequests([]string{"req-1"})
	for i := 0; i < biz.ProjectStateMaxRecent; i++ {
		ps.RollChange("m", strings.Repeat("变更", 60))
	}
	ps.SetDecisionDigest(strings.Repeat("决策", 300))
	b, _ := json.Marshal(ps.ToMap())

	sess := session.NewSession("app", "user", "sess")
	sess.SetState(biz.ProjectStateKey, b)
	inv := trpcagent.NewInvocation()
	inv.Session = sess

	got := ProjectStateCueFromInvocation(inv, projectStateCueBudgetRunes)
	if got == "" {
		t.Fatal("session-seeded project_state must render a cue")
	}
	if n := len([]rune(got)); n > projectStateCueBudgetRunes {
		t.Fatalf("cue runes=%d exceed budget %d", n, projectStateCueBudgetRunes)
	}
	if !strings.Contains(got, "req-1") {
		t.Fatalf("active requests must survive budget slicing, got %q", got)
	}
}

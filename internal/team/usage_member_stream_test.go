package team

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
)

// memberStreamRepoStub implements biz.TeamRunReader + biz.TeamRunWriter for
// recordGraphMemberUsageFromResult / fallback-suppression tests.
type memberStreamRepoStub struct {
	steps []biz.TeamRunStep
	runs  map[string]biz.TeamRunRecord
}

func newMemberStreamRepo() *memberStreamRepoStub {
	return &memberStreamRepoStub{runs: map[string]biz.TeamRunRecord{}}
}

func (s *memberStreamRepoStub) ListTeamRuns(context.Context, string, int) ([]biz.TeamRunRecord, error) {
	return nil, nil
}
func (s *memberStreamRepoStub) ListTeamRunsByTeamIDs(context.Context, []string, int) (map[string][]biz.TeamRunRecord, error) {
	return nil, nil
}
func (s *memberStreamRepoStub) HasActiveTeamRun(context.Context, string) (bool, error) {
	return false, nil
}
func (s *memberStreamRepoStub) GetTeamRunByID(_ context.Context, id string) (biz.TeamRunRecord, error) {
	if r, ok := s.runs[id]; ok {
		return r, nil
	}
	return biz.TeamRunRecord{}, biz.ErrNotFound
}
func (s *memberStreamRepoStub) ListTeamRunSteps(_ context.Context, runID string) ([]biz.TeamRunStep, error) {
	out := make([]biz.TeamRunStep, 0, len(s.steps))
	for _, st := range s.steps {
		if st.RunID == runID {
			out = append(out, st)
		}
	}
	return out, nil
}
func (s *memberStreamRepoStub) CreateTeamRun(_ context.Context, r biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	s.runs[r.ID] = r
	return r, nil
}
func (s *memberStreamRepoStub) UpdateTeamRun(_ context.Context, r biz.TeamRunRecord) error {
	s.runs[r.ID] = r
	return nil
}
func (s *memberStreamRepoStub) UpdateTeamRunGraphExecutionID(context.Context, string, string) error {
	return nil
}
func (s *memberStreamRepoStub) UpdateTeamRunTraceID(context.Context, string, string) error { return nil }
func (s *memberStreamRepoStub) UpdateTeamRunSummaryJSON(context.Context, string, string) error {
	return nil
}
func (s *memberStreamRepoStub) CreateTeamRunStep(_ context.Context, st biz.TeamRunStep) (biz.TeamRunStep, error) {
	s.steps = append(s.steps, st)
	return st, nil
}
func (s *memberStreamRepoStub) UpdateTeamRunWhereStatus(context.Context, string, string, string) (bool, error) {
	return true, nil
}

// memberStreamAgentsStub resolves per-member agents with DISTINCT provider/model
// per agent id so tests can assert per-member pricing coordinates.
type memberStreamAgentsStub struct {
	biz.AgentRepository
}

func (memberStreamAgentsStub) GetAgentByID(_ context.Context, id string) (biz.Agent, error) {
	return biz.Agent{
		ID:          id,
		AgentKey:    "key-" + id,
		DisplayName: id,
		Provider:    "prov-" + id,
		Model:       "model-" + id,
	}, nil
}

func newMemberStreamRunner(usage biz.TeamUsageQuerier, sessions biz.SessionTurnManager, repo *memberStreamRepoStub) *Runner {
	return &Runner{
		usage:     usage,
		runReader: repo,
		runWriter: repo,
		td: rt.TurnDeps{
			ReadDeps: rt.TurnReadDeps{Agents: memberStreamAgentsStub{}},
			Sessions: sessions,
		},
	}
}

const memberStreamDefJSON = `{"version":1,"mode":"sequential","members":[` +
	`{"agent_id":"agent-a","role":"anchor","sort_order":10},` +
	`{"agent_id":"agent-b","role":"worker","sort_order":20}]}`

func memberStreamFinishInput(run biz.TeamRunRecord, result agent.EventStreamResult, promptTok, completionTok int) TeamRunFinishInput {
	return TeamRunFinishInput{
		Run:            run,
		TeamID:         "team-1",
		DefinitionJSON: memberStreamDefJSON,
		Content:        "hello",
		AssistantMsg: biz.ChatMessage{
			Role: "assistant", ContentMarkdown: "done", Status: biz.TeamMemberStepStatusOK,
			CreatedAt: "2026-01-01T00:00:00Z",
		},
		Result:        result,
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		UsageSource:   agent.UsageSourceStreaming,
		Prov:          "anchor-prov",
		Mod:           "anchor-mod",
		DialogMode:    "default",
		GraphExecID:   "gexec-1",
		AnchorMem:     MemberDef{AgentID: "agent-a", Role: "anchor", SortOrder: 10},
		AnchorAg:      biz.Agent{ID: "agent-a", AgentKey: "key-agent-a", Provider: "prov-agent-a", Model: "model-agent-a"},
	}
}

func memberStreamResult(usage map[string]agent.MemberTokenUsage, cachedTok int) agent.EventStreamResult {
	return agent.EventStreamResult{MemberUsage: usage, CachedTok: cachedTok}
}

// P2-1b (2026-08-19): graph watch 健康路径的计费归因恢复——genuine 成员行按
// MemberUsage 落库（成员各自 prov/mod + step 链接 + member_level_stream 标记），
// run 全量与 Σ MemberUsage 的余额归因 anchor（stream_anchor_remainder），全部
// 跳过 session 累加（team_turn 行是唯一累加源）。
func TestRecordGraphMemberUsageFromResult(t *testing.T) {
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1", TeamID: "team-1"}

	t.Run("genuine member rows plus anchor remainder", func(t *testing.T) {
		repo := newMemberStreamRepo()
		repo.runs[run.ID] = run
		repo.steps = append(repo.steps,
			biz.TeamRunStep{ID: "step-a", RunID: run.ID, AgentKey: "key-agent-a"},
			biz.TeamRunStep{ID: "step-b", RunID: run.ID, AgentKey: "key-agent-b"},
		)
		usage := &fakeTeamUsage{}
		sessions := &fakeMetricSessions{}
		r := newMemberStreamRunner(usage, sessions, repo)

		result := memberStreamResult(map[string]agent.MemberTokenUsage{
			"key-agent-a": {PromptTokens: 100, CompletionTokens: 50, CachedTokens: 10},
			"key-agent-b": {PromptTokens: 200, CompletionTokens: 80, CachedTokens: 20},
		}, 45)
		in := memberStreamFinishInput(run, result, 400, 200)

		wrote := r.recordGraphMemberUsageFromResult(context.Background(), in)
		if !wrote {
			t.Fatal("wrote=false want true")
		}
		if len(usage.events) != 3 {
			t.Fatalf("events=%d want 3 (2 genuine + 1 remainder)", len(usage.events))
		}

		evA := usage.events[0]
		if evA.AgentKey != "key-agent-a" || evA.InputTokens != 100 || evA.OutputTokens != 50 || evA.CachedInputTokens != 10 {
			t.Fatalf("member-a row = %+v", evA)
		}
		if evA.ProviderCode != "prov-agent-a" || evA.ModelAPIID != "model-agent-a" {
			t.Fatalf("member-a pricing coords = %s/%s, want per-member agent's", evA.ProviderCode, evA.ModelAPIID)
		}
		if evA.MessageID != "step-a" {
			t.Fatalf("member-a MessageID = %q, want linked step-a", evA.MessageID)
		}
		if !strings.Contains(evA.MetadataJSON, biz.UsageAttributionMemberLevelStream) {
			t.Fatalf("member-a missing member_level_stream marker: %s", evA.MetadataJSON)
		}
		if !strings.Contains(evA.MetadataJSON, agent.UsageSourceStreaming) {
			t.Fatalf("member-a missing streaming source marker: %s", evA.MetadataJSON)
		}

		evB := usage.events[1]
		if evB.AgentKey != "key-agent-b" || evB.InputTokens != 200 || evB.OutputTokens != 80 || evB.CachedInputTokens != 20 {
			t.Fatalf("member-b row = %+v", evB)
		}
		if evB.MessageID != "step-b" {
			t.Fatalf("member-b MessageID = %q, want linked step-b", evB.MessageID)
		}

		evRem := usage.events[2]
		if evRem.AgentKey != "key-agent-a" || evRem.InputTokens != 100 || evRem.OutputTokens != 70 || evRem.CachedInputTokens != 15 {
			t.Fatalf("remainder row = %+v, want anchor 100/70 cached 15", evRem)
		}
		if !strings.Contains(evRem.MetadataJSON, biz.UsageAttributionStreamAnchorRemainder) {
			t.Fatalf("remainder missing stream_anchor_remainder marker: %s", evRem.MetadataJSON)
		}

		// 成员行合计必须等于 run 全量（= team_turn 行），计费聚合口径自洽。
		sumIn := evA.InputTokens + evB.InputTokens + evRem.InputTokens
		sumOut := evA.OutputTokens + evB.OutputTokens + evRem.OutputTokens
		if sumIn != 400 || sumOut != 200 {
			t.Fatalf("member rows sum %d/%d, want run totals 400/200", sumIn, sumOut)
		}
		if len(sessions.deltas) != 0 {
			t.Fatalf("completion-path member rows must not accumulate session metrics, got %d deltas", len(sessions.deltas))
		}
	})

	t.Run("exact coverage writes no remainder row", func(t *testing.T) {
		repo := newMemberStreamRepo()
		repo.runs[run.ID] = run
		usage := &fakeTeamUsage{}
		r := newMemberStreamRunner(usage, nil, repo)

		result := memberStreamResult(map[string]agent.MemberTokenUsage{
			"key-agent-a": {PromptTokens: 100, CompletionTokens: 50},
			"key-agent-b": {PromptTokens: 200, CompletionTokens: 80},
		}, 0)
		in := memberStreamFinishInput(run, result, 300, 130)

		if !r.recordGraphMemberUsageFromResult(context.Background(), in) {
			t.Fatal("wrote=false want true")
		}
		if len(usage.events) != 2 {
			t.Fatalf("events=%d want 2 (no remainder)", len(usage.events))
		}
		// 无 step 可链接时 MessageID 回退 run.ID（与 team_turn 行同约定）。
		if usage.events[0].MessageID != run.ID {
			t.Fatalf("MessageID = %q, want run-scoped fallback %q", usage.events[0].MessageID, run.ID)
		}
	})

	t.Run("empty member usage writes nothing", func(t *testing.T) {
		repo := newMemberStreamRepo()
		usage := &fakeTeamUsage{}
		r := newMemberStreamRunner(usage, nil, repo)

		in := memberStreamFinishInput(run, agent.EventStreamResult{}, 300, 130)
		if r.recordGraphMemberUsageFromResult(context.Background(), in) {
			t.Fatal("wrote=true want false without MemberUsage")
		}
		if len(usage.events) != 0 {
			t.Fatalf("events=%d want 0", len(usage.events))
		}
	})

	t.Run("duplicate agent ids share one member usage entry", func(t *testing.T) {
		repo := newMemberStreamRepo()
		usage := &fakeTeamUsage{}
		r := newMemberStreamRunner(usage, nil, repo)

		dupDef := `{"version":1,"mode":"sequential","members":[` +
			`{"agent_id":"agent-a","role":"anchor","sort_order":10},` +
			`{"agent_id":"agent-a","role":"worker","sort_order":20}]}`
		result := memberStreamResult(map[string]agent.MemberTokenUsage{
			"key-agent-a": {PromptTokens: 100, CompletionTokens: 50},
		}, 0)
		in := memberStreamFinishInput(run, result, 100, 50)
		in.DefinitionJSON = dupDef

		if !r.recordGraphMemberUsageFromResult(context.Background(), in) {
			t.Fatal("wrote=false want true")
		}
		if len(usage.events) != 1 {
			t.Fatalf("events=%d want 1 (duplicate agent keys consume the entry once)", len(usage.events))
		}
	})
}

// P2-1b 双计守卫：stream 成员行已落库时，anchor-fallback 的 usage 行必须被抑制
// （step 仍落库做展示，token 清零使 recordMemberUsage 自然跳过）。
func TestGraphRunStepsFallback_SuppressUsageRow(t *testing.T) {
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1", TeamID: "team-1"}
	result := memberStreamResult(map[string]agent.MemberTokenUsage{
		"key-agent-a": {PromptTokens: 100, CompletionTokens: 50},
		"key-agent-b": {PromptTokens: 200, CompletionTokens: 80},
	}, 0)

	t.Run("suppressed fallback persists display-only step", func(t *testing.T) {
		repo := newMemberStreamRepo()
		repo.runs[run.ID] = run
		usage := &fakeTeamUsage{}
		r := newMemberStreamRunner(usage, nil, repo)

		in := memberStreamFinishInput(run, result, 300, 130)
		fromStream := r.recordGraphMemberUsageFromResult(context.Background(), in)
		if !fromStream {
			t.Fatal("stream rows must be written first")
		}
		r.finalizeGraphRunStepsFallback(context.Background(), in, fromStream)

		if len(repo.steps) != 1 {
			t.Fatalf("steps=%d want 1 (display-only fallback step)", len(repo.steps))
		}
		if repo.steps[0].TokenIn != 0 || repo.steps[0].TokenOut != 0 {
			t.Fatalf("suppressed fallback step tokens = %d/%d, want 0/0", repo.steps[0].TokenIn, repo.steps[0].TokenOut)
		}
		if len(usage.events) != 2 {
			t.Fatalf("events=%d want 2 (stream rows only, no run-level fallback row)", len(usage.events))
		}
	})

	t.Run("unsuppressed fallback records run-level row", func(t *testing.T) {
		repo := newMemberStreamRepo()
		repo.runs[run.ID] = run
		usage := &fakeTeamUsage{}
		r := newMemberStreamRunner(usage, nil, repo)

		in := memberStreamFinishInput(run, agent.EventStreamResult{}, 300, 130)
		r.finalizeGraphRunStepsFallback(context.Background(), in, false)

		if len(usage.events) != 1 {
			t.Fatalf("events=%d want 1 (run-level fallback row)", len(usage.events))
		}
		ev := usage.events[0]
		if ev.InputTokens != 300 || ev.OutputTokens != 130 {
			t.Fatalf("fallback row tokens = %d/%d, want run totals 300/130", ev.InputTokens, ev.OutputTokens)
		}
		if !strings.Contains(ev.MetadataJSON, biz.UsageAttributionRunLevelAnchorFallback) {
			t.Fatalf("fallback row missing run_level_anchor_fallback marker: %s", ev.MetadataJSON)
		}
	})
}

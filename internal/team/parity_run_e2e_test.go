package team

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
)

// parityMemRepo captures team_run_steps and supports minimal TeamRepository for E2E harness.
type parityMemRepo struct {
	steps []biz.TeamRunStep
	runs  map[string]biz.TeamRun
}

func newParityMemRepo() *parityMemRepo {
	return &parityMemRepo{runs: map[string]biz.TeamRun{}}
}

func (m *parityMemRepo) ListTeams(context.Context) ([]biz.Team, error) { return nil, nil }
func (m *parityMemRepo) GetTeamByID(context.Context, string) (biz.Team, error) {
	return biz.Team{}, biz.ErrNotFound
}
func (m *parityMemRepo) CreateTeam(context.Context, biz.Team) (biz.Team, error) { return biz.Team{}, nil }
func (m *parityMemRepo) UpdateTeam(context.Context, biz.Team) (biz.Team, error) { return biz.Team{}, nil }
func (m *parityMemRepo) DeleteTeam(context.Context, string) error               { return nil }
func (m *parityMemRepo) ListTeamRuns(context.Context, string, int) ([]biz.TeamRun, error) {
	return nil, nil
}
func (m *parityMemRepo) HasActiveTeamRun(context.Context, string) (bool, error) {
	return false, nil
}
func (m *parityMemRepo) GetTeamRunByID(_ context.Context, id string) (biz.TeamRun, error) {
	if r, ok := m.runs[id]; ok {
		return r, nil
	}
	return biz.TeamRun{}, biz.ErrNotFound
}
func (m *parityMemRepo) ListTeamRunSteps(_ context.Context, runID string) ([]biz.TeamRunStep, error) {
	out := make([]biz.TeamRunStep, 0)
	for _, s := range m.steps {
		if s.RunID == runID {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out, nil
}
func (m *parityMemRepo) CreateTeamRun(_ context.Context, r biz.TeamRun) (biz.TeamRun, error) {
	m.runs[r.ID] = r
	return r, nil
}
func (m *parityMemRepo) UpdateTeamRun(_ context.Context, r biz.TeamRun) error {
	m.runs[r.ID] = r
	return nil
}
func (m *parityMemRepo) UpdateTeamRunGraphExecutionID(context.Context, string, string) error { return nil }
func (m *parityMemRepo) UpdateTeamRunTraceID(context.Context, string, string) error           { return nil }
func (m *parityMemRepo) UpdateTeamRunSummaryJSON(context.Context, string, string) error      { return nil }
func (m *parityMemRepo) CreateTeamRunStep(_ context.Context, step biz.TeamRunStep) (biz.TeamRunStep, error) {
	m.steps = append(m.steps, step)
	return step, nil
}
func (m *parityMemRepo) BatchCreateOrchestrationSteps(context.Context, []biz.OrchestrationStep) error {
	return nil
}
func (m *parityMemRepo) ListOrchestrationSteps(context.Context, string, string, int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (m *parityMemRepo) CreateTaskDeadLetter(context.Context, biz.TaskDeadLetter) error { return nil }
func (m *parityMemRepo) ListTaskDeadLetters(context.Context, biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (m *parityMemRepo) ResolveTaskDeadLetter(context.Context, string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}

type parityStubAgents struct {
	biz.AgentRepository
}

func (parityStubAgents) GetAgentByID(_ context.Context, id string) (biz.Agent, error) {
	return biz.Agent{
		ID:          id,
		AgentKey:    compileAgentKey(id),
		DisplayName: id,
		Provider:    "stub",
		Model:       "stub-model",
	}, nil
}

func newParityTestRunner(repo *parityMemRepo, bus event.Bus) *Runner {
	return &Runner{
		teams: repo,
		td: rt.TurnDeps{
			Catalog: rt.Catalog{Agents: parityStubAgents{}},
			Pipeline: rt.EventPipeline{Bus: bus},
		},
	}
}

func stubStreamResultFromOutcomes(outcomes []parityMemberOutcome, reply string) agent.EventStreamResult {
	result := agent.EventStreamResult{
		MemberUsage:     make(map[string]agent.MemberTokenUsage),
		MemberToolCalls: make(map[string]int),
	}
	result.Reply.WriteString(reply)
	result.PromptTok = 0
	result.CompletionTok = 0
	result.HasContent = reply != ""
	for _, o := range outcomes {
		key := compileAgentKey(o.AgentID)
		result.MemberUsage[key] = agent.MemberTokenUsage{
			PromptTokens:     o.TokenIn,
			CompletionTokens: o.TokenOut,
		}
		result.MemberToolCalls[key] = o.ToolCallCount
		result.PromptTok += o.TokenIn
		result.CompletionTok += o.TokenOut
	}
	return result
}

func parityAssistantMsg(sessionID, markdown string) biz.ChatMessage {
	return biz.ChatMessage{
		SessionID:       sessionID,
		Role:            "assistant",
		ContentMarkdown: markdown,
		Status:          "ok",
		CreatedAt:       "2026-05-23T00:00:00Z",
	}
}

type parityPathOutcome struct {
	steps     []biz.TeamRunStep
	envelopes []event.Envelope
}

func runNativePathHarness(t *testing.T, def Definition, outcomes []parityMemberOutcome) parityPathOutcome {
	t.Helper()
	repo := newParityMemRepo()
	bus := event.NewBus()
	ch, unsub := bus.Subscribe(event.SubscribeOptions{BufferSize: 64})
	defer unsub()

	runner := newParityTestRunner(repo, bus)
	run := parityRunBase(def.Mode)
	repo.runs[run.ID] = run

	reply := fmt.Sprintf("team reply %s", def.Mode)
	result := stubStreamResultFromOutcomes(outcomes, reply)
	asst := parityAssistantMsg(run.SessionID, reply)
	finishIn := TeamRunFinishInput{
		Run:            run,
		TeamID:         run.TeamID,
		DefinitionJSON: definitionJSONFromDef(def),
		Content:        "hello parity",
		AssistantMsg:   asst,
		Result:         result,
		PromptTok:      result.PromptTok,
		CompletionTok:  result.CompletionTok,
		GraphExecID:    "",
	}
	runner.persistNativeBulkMemberSteps(context.Background(), finishIn, EnabledMembers(def))

	envs := drainEnvelopes(ch)
	steps, _ := repo.ListTeamRunSteps(context.Background(), run.ID)
	return parityPathOutcome{steps: steps, envelopes: envs}
}

func runGraphPathHarness(t *testing.T, def Definition, outcomes []parityMemberOutcome) parityPathOutcome {
	t.Helper()
	repo := newParityMemRepo()
	bus := event.NewBus()
	ch, unsub := bus.Subscribe(event.SubscribeOptions{BufferSize: 64})
	defer unsub()

	runner := newParityTestRunner(repo, bus)
	run := parityRunBase(def.Mode)
	run.GraphExecutionID = "graph-exec-parity"
	repo.runs[run.ID] = run

	reply := fmt.Sprintf("team reply %s", def.Mode)
	result := stubStreamResultFromOutcomes(outcomes, reply)
	asst := parityAssistantMsg(run.SessionID, reply)
	finishIn := TeamRunFinishInput{
		Run:            run,
		TeamID:         run.TeamID,
		DefinitionJSON: definitionJSONFromDef(def),
		Content:        "hello parity",
		AssistantMsg:   asst,
		Result:         result,
		PromptTok:      result.PromptTok,
		CompletionTok:  result.CompletionTok,
		GraphExecID:    run.GraphExecutionID,
	}
	runner.persistGraphMemberStepsFromResultTestOnly(context.Background(), finishIn, def)

	envs := drainEnvelopes(ch)
	steps, _ := repo.ListTeamRunSteps(context.Background(), run.ID)
	return parityPathOutcome{steps: steps, envelopes: envs}
}

func definitionJSONFromDef(def Definition) string {
	b, err := json.Marshal(def)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func drainEnvelopes(ch <-chan event.Envelope) []event.Envelope {
	var out []event.Envelope
	for {
		select {
		case env, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, env)
		default:
			return out
		}
	}
}

func stepFingerprint(steps []biz.TeamRunStep) string {
	parts := make([]string, 0, len(steps))
	for _, s := range steps {
		parts = append(parts, fmt.Sprintf("%s:%d:%d:%d:%s", s.AgentKey, s.ToolCallCount, s.TokenIn, s.TokenOut, s.Status))
	}
	sort.Strings(parts)
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:8])
}

func envelopeTypeHash(envs []event.Envelope, skip map[event.EnvelopeType]struct{}) string {
	types := make([]string, 0, len(envs))
	for _, e := range envs {
		if skip != nil {
			if _, ok := skip[e.Type]; ok {
				continue
			}
		}
		types = append(types, string(e.Type))
	}
	sort.Strings(types)
	h := sha256.Sum256([]byte(strings.Join(types, ",")))
	return hex.EncodeToString(h[:8])
}

// TestParityRunE2E_stubStreamAllModes exercises Native vs Graph step persistence with stub EventStreamResult.
func TestParityRunE2E_stubStreamAllModes(t *testing.T) {
	modes := []string{"sequential", "parallel", "coordinator", "critic_loop", "adaptive", "swarm"}

	for _, mode := range modes {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			def := parityFixture(mode)
			outcomes := parityMemberOutcomes(def)

			native := runNativePathHarness(t, def, outcomes)
			graph := runGraphPathHarness(t, def, outcomes)

			if len(native.steps) != len(graph.steps) {
				t.Fatalf("step count native=%d graph=%d", len(native.steps), len(graph.steps))
			}
			nativeFP := stepFingerprint(native.steps)
			graphFP := stepFingerprint(graph.steps)
			if nativeFP != graphFP {
				t.Fatalf("step fingerprint native=%s graph=%s", nativeFP, graphFP)
			}

			// Harness uses persistStep on both paths → WS envelope sequences should match.
			nativeWS := envelopeTypeHash(native.envelopes, nil)
			graphWS := envelopeTypeHash(graph.envelopes, nil)
			if nativeWS != graphWS {
				t.Fatalf("WS type hash native=%s graph=%s", nativeWS, graphWS)
			}

			nativeSteps := envelopeTypeSetFromEnvs(native.envelopes, envelopeTypeSet(nativeOnlyEnvelopeTypes))
			if len(nativeSteps) == 0 {
				t.Fatal("expected team_step envelopes on native path")
			}
			t.Logf("mode=%s steps=%d fp=%s native_envs=%d graph_envs=%d",
				mode, len(native.steps), nativeFP, len(native.envelopes), len(graph.envelopes))
		})
	}
}

func envelopeTypeSetFromEnvs(envs []event.Envelope, want map[event.EnvelopeType]struct{}) map[event.EnvelopeType]int {
	out := make(map[event.EnvelopeType]int)
	for _, e := range envs {
		if _, ok := want[e.Type]; ok {
			out[e.Type]++
		}
	}
	return out
}

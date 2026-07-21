package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// Mocks for deliverable-passing tests (P0-② / P0-③)
// ---------------------------------------------------------------------------

type deliverableTeamRepo struct {
	items       map[string]Team
	updateCalls int
}

func newDeliverableTeamRepo() *deliverableTeamRepo {
	return &deliverableTeamRepo{items: make(map[string]Team)}
}

func (m *deliverableTeamRepo) Create(_ context.Context, in Team) (Team, error) {
	if in.ID == "" {
		in.ID = fmt.Sprintf("tid-%d", len(m.items)+1)
	}
	m.items[in.ID] = in
	return in, nil
}
func (m *deliverableTeamRepo) Get(_ context.Context, id string) (Team, error) {
	t, ok := m.items[id]
	if !ok {
		return Team{}, fmt.Errorf("not found: %s", id)
	}
	return t, nil
}
func (m *deliverableTeamRepo) Update(_ context.Context, id string, patch Team) (Team, error) {
	m.updateCalls++
	cur, ok := m.items[id]
	if !ok {
		return Team{}, fmt.Errorf("not found: %s", id)
	}
	// Mirror TeamUsecase.Update passthrough semantics: only whitelisted fields
	// are applied. DeliverablesOutput must be one of them (P0-② fix).
	if patch.DeliverablesOutput != "" {
		cur.DeliverablesOutput = patch.DeliverablesOutput
	}
	m.items[id] = cur
	return cur, nil
}
func (m *deliverableTeamRepo) TransitionStatus(_ context.Context, id string, newStatus string) (Team, error) {
	cur, ok := m.items[id]
	if !ok {
		return Team{}, fmt.Errorf("not found: %s", id)
	}
	cur.Status = newStatus
	m.items[id] = cur
	return cur, nil
}
func (m *deliverableTeamRepo) ListBySpiritSessionID(_ context.Context, spiritSessionID string) ([]Team, error) {
	var out []Team
	for _, t := range m.items {
		if t.SpiritSessionID == spiritSessionID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *deliverableTeamRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (m *deliverableTeamRepo) ListRuns(_ context.Context, _ string, _ int) ([]TeamRunRecord, error) {
	return nil, nil
}

type deliverableSessionAccessor struct {
	sessionsByTeam map[string]Session
	extraByTeam    map[string][]Session // additional sessions returned by Search (e.g. member sessions)
	messages       map[string][]ChatMessage // sessionID → messages
}

func newDeliverableSessionAccessor() *deliverableSessionAccessor {
	return &deliverableSessionAccessor{
		sessionsByTeam: make(map[string]Session),
		extraByTeam:    make(map[string][]Session),
		messages:       make(map[string][]ChatMessage),
	}
}

func (m *deliverableSessionAccessor) Get(_ context.Context, id string) (Session, error) {
	for _, s := range m.sessionsByTeam {
		if s.ID == id {
			return s, nil
		}
	}
	return Session{}, fmt.Errorf("not found: %s", id)
}
func (m *deliverableSessionAccessor) Create(_ context.Context, in Session) (Session, error) {
	return in, nil
}
func (m *deliverableSessionAccessor) Search(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
	// Extras first: production ordering is not guaranteed (member sessions share
	// the same team_id), so callers must identify the team session by
	// SessionType rather than position.
	var items []Session
	items = append(items, m.extraByTeam[q.TeamID]...)
	if s, ok := m.sessionsByTeam[q.TeamID]; ok {
		items = append(items, s)
	}
	if len(items) == 0 {
		return SessionListResult{}, nil
	}
	return SessionListResult{Items: items}, nil
}
func (m *deliverableSessionAccessor) ListMessagesRecent(_ context.Context, sessionID string, _ int) ([]ChatMessage, error) {
	return m.messages[sessionID], nil
}
func (m *deliverableSessionAccessor) ListChildSessions(_ context.Context, _ string) ([]Session, error) {
	return nil, nil
}

type deliverableAgentResolver struct{}

func (deliverableAgentResolver) List(_ context.Context, _ AgentListQuery) (AgentListResult, error) {
	return AgentListResult{}, nil
}

// deliverableStepReader stubs SpiritStepReader: exact session_id semantics.
type deliverableStepReader struct {
	stepsBySession map[string][]Step // sessionID → steps (chronological)
}

func newDeliverableStepReader() *deliverableStepReader {
	return &deliverableStepReader{stepsBySession: make(map[string][]Step)}
}

func (m *deliverableStepReader) ListStepsBySessionID(_ context.Context, sessionID string) ([]Step, error) {
	return m.stepsBySession[sessionID], nil
}

func newDeliverableUsecase(teams *deliverableTeamRepo, sessions *deliverableSessionAccessor) *SpiritTeamUsecase {
	return NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop())
}

func newDeliverableUsecaseWithSteps(teams *deliverableTeamRepo, sessions *deliverableSessionAccessor, steps *deliverableStepReader) *SpiritTeamUsecase {
	return NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop(), WithSpiritStepReader(steps))
}

// seedCompletedTeam creates a completed team with a reply step in its session.
func seedCompletedTeam(teams *deliverableTeamRepo, sessions *deliverableSessionAccessor, id, spiritSessionID, dagNodeID, name, assistantContent string) Team {
	return seedCompletedTeamWithSteps(teams, sessions, nil, id, spiritSessionID, dagNodeID, name, assistantContent)
}

// seedCompletedTeamWithSteps seeds the team session and — when steps is
// non-nil — a completed reply step carrying assistantContent (the production
// data source for ExtractTeamOutput). The legacy ChatMessage seeding is kept
// for the no-stepReader fallback path.
func seedCompletedTeamWithSteps(teams *deliverableTeamRepo, sessions *deliverableSessionAccessor, steps *deliverableStepReader, id, spiritSessionID, dagNodeID, name, assistantContent string) Team {
	t := Team{
		ID:              id,
		SpiritSessionID: spiritSessionID,
		DagNodeID:       dagNodeID,
		DisplayName:     name,
		Status:          TeamStatusCompleted,
	}
	teams.items[id] = t
	sessID := "sess-" + id
	sessions.sessionsByTeam[id] = Session{ID: sessID, TeamID: id, ParentSessionID: spiritSessionID, SessionType: string(SessionTypeTeam)}
	if assistantContent != "" {
		sessions.messages[sessID] = []ChatMessage{
			{Role: "user", ContentMarkdown: "任务"},
			{Role: "assistant", ContentMarkdown: assistantContent},
		}
		if steps != nil {
			steps.stepsBySession[sessID] = []Step{
				{ID: "step-thinking-" + id, SessionID: sessID, Kind: StepKindThinking, Content: "思考中", Status: StepStatusCompleted},
				{ID: "step-reply-" + id, SessionID: sessID, Kind: StepKindReply, Content: assistantContent, Status: StepStatusCompleted},
			}
		}
	}
	return t
}

// ---------------------------------------------------------------------------
// P0-② WriteDeliverablesToSession: persist to DeliverablesOutput (not ParallelConfigJSON)
// ---------------------------------------------------------------------------

func TestWriteDeliverablesToSession_PersistsToDeliverablesOutput(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	team := seedCompletedTeam(teams, sessions, "t1", "sp1", "st_1", "分析团队", "分析完成\n- 发现A")
	team.ParallelConfigJSON = `{"max_teams":3}`
	teams.items["t1"] = team

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	stored := teams.items["t1"]
	if stored.DeliverablesOutput == "" || stored.DeliverablesOutput == "{}" {
		t.Fatalf("DeliverablesOutput should be written, got %q", stored.DeliverablesOutput)
	}
	var outputs map[string]string
	if err := json.Unmarshal([]byte(stored.DeliverablesOutput), &outputs); err != nil {
		t.Fatalf("DeliverablesOutput must be a JSON object: %v", err)
	}
	if !strings.Contains(outputs["st_1"], "分析完成") {
		t.Fatalf("DeliverablesOutput[st_1] should contain team summary, got %q", outputs["st_1"])
	}
	// ParallelConfigJSON must NOT be overloaded with deliverable output keys.
	if strings.Contains(stored.ParallelConfigJSON, "deliverable_output_") {
		t.Fatalf("ParallelConfigJSON must not carry deliverable outputs, got %q", stored.ParallelConfigJSON)
	}
}

// Corrupt cached JSON must not block the write: the cache is rebuilt fresh
// (error path coverage for the tolerant-unmarshal branch).
func TestWriteDeliverablesToSession_CorruptCache_Rebuilds(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	team := seedCompletedTeam(teams, sessions, "t1", "sp1", "st_1", "分析团队", "新成果")
	team.DeliverablesOutput = "{corrupt-json"
	teams.items["t1"] = team

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	stored := teams.items["t1"]
	var outputs map[string]string
	if err := json.Unmarshal([]byte(stored.DeliverablesOutput), &outputs); err != nil {
		t.Fatalf("rebuilt DeliverablesOutput must be valid JSON, got %q: %v", stored.DeliverablesOutput, err)
	}
	if outputs["st_1"] != "新成果" {
		t.Fatalf("rebuilt cache should contain the new summary, got %q", outputs["st_1"])
	}
}

func TestWriteDeliverablesToSession_NoDagNode_NoOp(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	seedCompletedTeam(teams, sessions, "t1", "sp1", "", "团队", "输出")
	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	if teams.updateCalls != 0 {
		t.Fatalf("no DagNodeID → no update expected, got %d updates", teams.updateCalls)
	}
}

func TestWriteDeliverablesToSession_NoAssistantMessage_NoOp(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	seedCompletedTeam(teams, sessions, "t1", "sp1", "st_1", "团队", "")
	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	if teams.updateCalls != 0 {
		t.Fatalf("no assistant message → no update expected, got %d updates", teams.updateCalls)
	}
}

func TestReadDeliverableOutput_FromDeliverablesOutput(t *testing.T) {
	u := newDeliverableUsecase(newDeliverableTeamRepo(), newDeliverableSessionAccessor())
	cases := []struct {
		name string
		team Team
		want string
	}{
		{"present", Team{DagNodeID: "st_1", DeliverablesOutput: `{"st_1":"上游成果"}`}, "上游成果"},
		{"empty", Team{DagNodeID: "st_1"}, ""},
		{"empty object", Team{DagNodeID: "st_1", DeliverablesOutput: "{}"}, ""},
		{"invalid json", Team{DagNodeID: "st_1", DeliverablesOutput: "{bad"}, ""},
		{"missing key", Team{DagNodeID: "st_2", DeliverablesOutput: `{"st_1":"x"}`}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := u.readDeliverableOutput(tc.team); got != tc.want {
				t.Fatalf("readDeliverableOutput = %q, want %q", got, tc.want)
			}
		})
	}
}

// RecordTeamCompletion must persist deliverable output BEFORE downstream
// scheduling reads it (P0-② wiring).
func TestRecordTeamCompletion_PersistsDeliverableOutput(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	team := seedCompletedTeam(teams, sessions, "t1", "sp1", "st_1", "分析团队", "分析成果")

	u.RecordTeamCompletion(context.Background(), team, 100)

	stored := teams.items["t1"]
	var outputs map[string]string
	if err := json.Unmarshal([]byte(stored.DeliverablesOutput), &outputs); err != nil || outputs["st_1"] == "" {
		t.Fatalf("RecordTeamCompletion should persist deliverable output, got %q", stored.DeliverablesOutput)
	}
}

// ---------------------------------------------------------------------------
// P0-②/P0-③ InjectUpstreamDeliverables: read persisted output for downstream input
// ---------------------------------------------------------------------------

func TestInjectUpstreamDeliverables_ReadsPersistedOutput(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	upstream := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "上游团队", "不应使用此消息")
	upstream.DeliverablesOutput = `{"st_1":"持久化的上游成果"}`
	teams.items["t-up"] = upstream

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if !strings.Contains(prefix, "上游团队") || !strings.Contains(prefix, "持久化的上游成果") {
		t.Fatalf("prefix should contain upstream team name and persisted output, got %q", prefix)
	}
}

func TestInjectUpstreamDeliverables_NoDependsOn_Empty(t *testing.T) {
	u := newDeliverableUsecase(newDeliverableTeamRepo(), newDeliverableSessionAccessor())
	if got := u.InjectUpstreamDeliverables(context.Background(), Team{SpiritSessionID: "sp1"}); got != "" {
		t.Fatalf("no DependsOn → empty prefix, got %q", got)
	}
}

func TestInjectUpstreamDeliverables_FallbackExtract(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	// Upstream completed but DeliverablesOutput not yet written → fallback to extraction.
	seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "上游团队", "即时提取的成果")

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if !strings.Contains(prefix, "即时提取的成果") {
		t.Fatalf("fallback extraction should supply upstream output, got %q", prefix)
	}
}

// ---------------------------------------------------------------------------
// P0-③b ScheduleDependentTeams: activate action carries task description
// with upstream deliverable prefix.
// ---------------------------------------------------------------------------

func TestScheduleDependentTeams_ActivateCarriesDeliverablePrefix(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	upstream := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "上游团队", "上游成果内容")
	upstream.DeliverablesOutput = `{"st_1":"上游成果内容"}`
	teams.items["t-up"] = upstream

	teams.items["t-down"] = Team{
		ID:              "t-down",
		SpiritSessionID: "sp1",
		DagNodeID:       "st_2",
		DisplayName:     "下游团队",
		TaskDescription: "下游原始任务",
		Status:          TeamStatusPending,
		DependsOn:       []string{"st_1"},
	}

	actions := u.ScheduleDependentTeams(context.Background(), "sp1", upstream)
	if len(actions) != 1 || actions[0].Action != "activate" {
		t.Fatalf("expected 1 activate action, got %+v", actions)
	}
	desc := actions[0].TaskDescription
	if !strings.Contains(desc, "上游成果内容") {
		t.Fatalf("activate TaskDescription should carry upstream deliverable prefix, got %q", desc)
	}
	if !strings.Contains(desc, "下游原始任务") {
		t.Fatalf("activate TaskDescription should retain original task description, got %q", desc)
	}
}

func TestScheduleDependentTeams_FailActionUnchanged(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	failed := Team{ID: "t-up", SpiritSessionID: "sp1", DagNodeID: "st_1", DisplayName: "上游团队", Status: TeamStatusFailed}
	teams.items["t-up"] = failed
	teams.items["t-down"] = Team{
		ID: "t-down", SpiritSessionID: "sp1", DagNodeID: "st_2", DisplayName: "下游团队",
		TaskDescription: "下游原始任务", Status: TeamStatusPending, DependsOn: []string{"st_1"},
	}

	actions := u.ScheduleDependentTeams(context.Background(), "sp1", failed)
	if len(actions) != 1 || actions[0].Action != "fail" {
		t.Fatalf("expected 1 fail action, got %+v", actions)
	}
}

// ---------------------------------------------------------------------------
// ExtractTeamOutput: StepV2Reader (exact session_id) as primary data source
// ---------------------------------------------------------------------------

// Production data source: the team session's final completed reply step.
// ChatMessage fallback must not shadow the step reader when both exist.
func TestExtractTeamOutput_PrefersStepReader(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	seedCompletedTeamWithSteps(teams, sessions, steps, "t1", "sp1", "st_1", "分析团队", "步骤成果\n- 发现A")
	// Divergent legacy message content — must be ignored in favor of the step.
	sessions.messages["sess-t1"] = []ChatMessage{
		{Role: "assistant", ContentMarkdown: "消息成果（不应使用）"},
	}

	summary, keyFindings, err := u.ExtractTeamOutput(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ExtractTeamOutput: %v", err)
	}
	if !strings.Contains(summary, "步骤成果") {
		t.Fatalf("summary should come from reply step, got %q", summary)
	}
	if !strings.Contains(keyFindings, "发现A") {
		t.Fatalf("keyFindings should extract bullet lines from reply step, got %q", keyFindings)
	}
}

// Non-reply steps (task/thinking/action) must not be treated as output;
// when no completed reply step exists, fall back to legacy messages.
func TestExtractTeamOutput_NoReplyStep_FallsBackToMessages(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	seedCompletedTeamWithSteps(teams, sessions, steps, "t1", "sp1", "st_1", "分析团队", "消息成果")
	// Replace steps: thinking step only, no reply.
	steps.stepsBySession["sess-t1"] = []Step{
		{ID: "step-thinking-t1", SessionID: "sess-t1", Kind: StepKindThinking, Content: "思考中", Status: StepStatusCompleted},
	}

	summary, _, err := u.ExtractTeamOutput(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ExtractTeamOutput: %v", err)
	}
	if !strings.Contains(summary, "消息成果") {
		t.Fatalf("summary should fall back to messages, got %q", summary)
	}
}

// Member sessions share the same team_id; the extractor must identify the
// team main session by SessionType and never read a member session's steps.
func TestExtractTeamOutput_PicksTeamSessionAmongMemberSessions(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	team := seedCompletedTeamWithSteps(teams, sessions, steps, "t1", "sp1", "st_1", "分析团队", "团队主会话成果")
	// Member session returned by Search BEFORE the team session (extras-first).
	memberSessID := "sess-member-t1"
	sessions.extraByTeam["t1"] = []Session{
		{ID: memberSessID, TeamID: "t1", ParentSessionID: "sess-t1", SessionType: string(SessionTypeAgent)},
	}
	steps.stepsBySession[memberSessID] = []Step{
		{ID: "step-member-reply", SessionID: memberSessID, Kind: StepKindReply, Content: "成员个人成果", Status: StepStatusCompleted},
	}
	sessions.messages[memberSessID] = []ChatMessage{
		{Role: "assistant", ContentMarkdown: "成员个人成果"},
	}

	summary, _, err := u.ExtractTeamOutput(context.Background(), team.ID)
	if err != nil {
		t.Fatalf("ExtractTeamOutput: %v", err)
	}
	if !strings.Contains(summary, "团队主会话成果") {
		t.Fatalf("summary should come from the team main session, got %q", summary)
	}
}

// Without a wired step reader (tests/CLI), the legacy message path still works.
func TestExtractTeamOutput_NoStepReader_UsesMessages(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	seedCompletedTeam(teams, sessions, "t1", "sp1", "st_1", "分析团队", "消息成果")

	summary, _, err := u.ExtractTeamOutput(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ExtractTeamOutput: %v", err)
	}
	if !strings.Contains(summary, "消息成果") {
		t.Fatalf("summary should come from messages, got %q", summary)
	}
}

// Running (unfinished) reply steps must not be treated as final output.
func TestExtractTeamOutput_RunningReplyStep_NotUsed(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	seedCompletedTeamWithSteps(teams, sessions, steps, "t1", "sp1", "st_1", "分析团队", "消息成果")
	steps.stepsBySession["sess-t1"] = []Step{
		{ID: "step-reply-t1", SessionID: "sess-t1", Kind: StepKindReply, Content: "未完成的回复", Status: StepStatusRunning},
	}

	summary, _, err := u.ExtractTeamOutput(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ExtractTeamOutput: %v", err)
	}
	if strings.Contains(summary, "未完成的回复") {
		t.Fatalf("running reply step must not be used, got %q", summary)
	}
	if !strings.Contains(summary, "消息成果") {
		t.Fatalf("summary should fall back to messages, got %q", summary)
	}
}

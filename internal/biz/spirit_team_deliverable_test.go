package biz

import (
	"context"
	"encoding/json"
	"errors"
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
	extraByTeam    map[string][]Session     // additional sessions returned by Search (e.g. member sessions)
	messages       map[string][]ChatMessage // sessionID → messages
	children       []Session                // returned by ListChildSessions (B.10.17 token aggregation tests)
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
	return m.children, nil
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
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "分析完成\n- 发现A"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`, "")
	team.ParallelConfigJSON = `{"max_teams":3}`
	teams.items["t1"] = team

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	stored := teams.items["t1"]
	if stored.DeliverablesOutput == "" || stored.DeliverablesOutput == "{}" {
		t.Fatalf("DeliverablesOutput should be written, got %q", stored.DeliverablesOutput)
	}
	refs := ParseDeliverableRefs(stored.DeliverablesOutput)
	ref, ok := refs["st_1"]
	if !ok {
		t.Fatalf("DeliverablesOutput[st_1] missing, got %q", stored.DeliverablesOutput)
	}
	if !strings.Contains(ref.Summary, "分析完成") {
		t.Fatalf("DeliverablesOutput[st_1].summary should contain team summary, got %q", ref.Summary)
	}
	// ParallelConfigJSON must NOT be overloaded with deliverable output keys.
	if strings.Contains(stored.ParallelConfigJSON, "deliverable_output_") {
		t.Fatalf("ParallelConfigJSON must not carry deliverable outputs, got %q", stored.ParallelConfigJSON)
	}
}

// P2: the written value must be a DeliverableRef envelope carrying metadata
// for downstream full-text retrieval decisions.
func TestWriteDeliverablesToSession_WritesDeliverableRefMetadata(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	// Content longer than MaxSummaryLen (500 runes) so the summary truncates.
	longContent := strings.Repeat("长", 600) + "\n- 关键发现一\n- 关键发现二"
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": longContent}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`, "")

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	refs := ParseDeliverableRefs(teams.items["t1"].DeliverablesOutput)
	ref, ok := refs["st_1"]
	if !ok {
		t.Fatalf("st_1 ref missing, got %q", teams.items["t1"].DeliverablesOutput)
	}
	if ref.TeamID != "t1" {
		t.Fatalf("team_id should be the owning team, got %q", ref.TeamID)
	}
	if ref.TeamSessionID != "sess-t1" {
		t.Fatalf("team_session_id should be the team main session, got %q", ref.TeamSessionID)
	}
	fullSize := len([]rune(longContent))
	if ref.SizeChars != fullSize {
		t.Fatalf("size_chars should be the FULL content size %d, got %d", fullSize, ref.SizeChars)
	}
	if !ref.Truncated {
		t.Fatalf("content longer than MaxSummaryLen must mark truncated=true")
	}
	if len([]rune(ref.Summary)) > MaxSummaryLen {
		t.Fatalf("summary must be truncated to MaxSummaryLen, got %d runes", len([]rune(ref.Summary)))
	}
	if !strings.Contains(ref.KeyFindings, "关键发现一") {
		t.Fatalf("key_findings should extract bullet lines, got %q", ref.KeyFindings)
	}
}

// Short content fits the summary budget: truncated=false and summary is the
// full content.
func TestWriteDeliverablesToSession_ShortContent_NotTruncated(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "短成果"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`, "")
	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	ref := ParseDeliverableRefs(teams.items["t1"].DeliverablesOutput)["st_1"]
	if ref.Truncated {
		t.Fatalf("short content must not be marked truncated")
	}
	if ref.Summary != "短成果" || ref.SizeChars != len([]rune("短成果")) {
		t.Fatalf("summary/size mismatch: %+v", ref)
	}
}

// A legacy plain-string value for ANOTHER node in the same cache must survive
// a P2 write (mixed map coexistence).
func TestWriteDeliverablesToSession_PreservesLegacyEntries(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "新成果"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`, "")
	team.DeliverablesOutput = `{"st_0":"旧节点摘要"}`
	teams.items["t1"] = team

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	refs := ParseDeliverableRefs(teams.items["t1"].DeliverablesOutput)
	if refs["st_0"].Summary != "旧节点摘要" {
		t.Fatalf("legacy entry st_0 must be preserved, got %q", teams.items["t1"].DeliverablesOutput)
	}
	if refs["st_1"].Summary != "新成果" || refs["st_1"].TeamID != "t1" {
		t.Fatalf("new P2 envelope missing, got %q", teams.items["t1"].DeliverablesOutput)
	}
}

// Corrupt cached JSON must not block the write: the cache is rebuilt fresh
// (error path coverage for the tolerant-unmarshal branch).
func TestWriteDeliverablesToSession_CorruptCache_Rebuilds(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "新成果"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`, "")
	team.DeliverablesOutput = "{corrupt-json"
	teams.items["t1"] = team

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	stored := teams.items["t1"]
	refs := ParseDeliverableRefs(stored.DeliverablesOutput)
	if len(refs) == 0 {
		t.Fatalf("rebuilt DeliverablesOutput must be valid, got %q", stored.DeliverablesOutput)
	}
	if refs["st_1"].Summary != "新成果" {
		t.Fatalf("rebuilt cache should contain the new summary, got %q", refs["st_1"].Summary)
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

func TestReadDeliverableRef_DualMode(t *testing.T) {
	u := newDeliverableUsecase(newDeliverableTeamRepo(), newDeliverableSessionAccessor())
	cases := []struct {
		name        string
		team        Team
		wantSummary string
		wantOK      bool
	}{
		{"p2 envelope", Team{DagNodeID: "st_1", DeliverablesOutput: `{"st_1":{"summary":"信封摘要","team_id":"t1","team_session_id":"sess-t1","size_chars":100,"truncated":true}}`}, "信封摘要", true},
		{"legacy string", Team{DagNodeID: "st_1", DeliverablesOutput: `{"st_1":"旧摘要"}`}, "旧摘要", true},
		{"missing key", Team{DagNodeID: "st_2", DeliverablesOutput: `{"st_1":{"summary":"x"}}`}, "", false},
		{"corrupt", Team{DagNodeID: "st_1", DeliverablesOutput: "{bad"}, "", false},
		{"no dag node", Team{DeliverablesOutput: `{"st_1":"x"}`}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, ok := u.readDeliverableRef(tc.team)
			if ok != tc.wantOK || ref.Summary != tc.wantSummary {
				t.Fatalf("readDeliverableRef = (%+v, %v), want summary %q ok %v", ref, ok, tc.wantSummary, tc.wantOK)
			}
		})
	}
}

// RecordTeamCompletion must persist deliverable output BEFORE downstream
// scheduling reads it (P0-② wiring).
func TestRecordTeamCompletion_PersistsDeliverableOutput(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "分析成果"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)
	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`, "")

	u.RecordTeamCompletion(context.Background(), team, 100)

	stored := teams.items["t1"]
	refs := ParseDeliverableRefs(stored.DeliverablesOutput)
	if refs["st_1"].Summary == "" {
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

// ---------------------------------------------------------------------------
// P2 产物引用化：截断时附加 read_upstream_deliverable 取全文指引
// ---------------------------------------------------------------------------

// Persisted P2 envelope with truncated=true → the injection prefix must tell
// the downstream team how to retrieve the full text.
func TestInjectUpstreamDeliverables_TruncatedRef_AppendsFullTextGuidance(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	upstream := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "调研团队", "不应使用此消息")
	upstream.DeliverablesOutput = `{"st_1":{"summary":"摘要（已截断）...","team_id":"t-up","team_session_id":"sess-t-up","size_chars":8000,"truncated":true}}`
	teams.items["t-up"] = upstream

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if !strings.Contains(prefix, "摘要（已截断）...") {
		t.Fatalf("prefix should contain the envelope summary, got %q", prefix)
	}
	if !strings.Contains(prefix, `read_upstream_deliverable(team_id="t-up")`) {
		t.Fatalf("truncated ref should append full-text retrieval guidance, got %q", prefix)
	}
	if !strings.Contains(prefix, "8000") {
		t.Fatalf("guidance should mention the full size, got %q", prefix)
	}
}

// Untruncated envelopes and legacy strings carry no retrieval guidance —
// the summary IS the full content (or its truncation state is unknown).
func TestInjectUpstreamDeliverables_UntruncatedOrLegacy_NoGuidance(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "短内容团队", "不应使用")
	up.DeliverablesOutput = `{"st_1":{"summary":"完整短摘要","team_id":"t-up","team_session_id":"sess-t-up","size_chars":5,"truncated":false}}`
	teams.items["t-up"] = up
	legacy := seedCompletedTeam(teams, sessions, "t-legacy", "sp1", "st_0", "旧团队", "不应使用")
	legacy.DeliverablesOutput = `{"st_0":"旧格式摘要"}`
	teams.items["t-legacy"] = legacy

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1", "st_0"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if strings.Contains(prefix, "read_upstream_deliverable") {
		t.Fatalf("untruncated/legacy refs must not append guidance, got %q", prefix)
	}
	if !strings.Contains(prefix, "完整短摘要") || !strings.Contains(prefix, "旧格式摘要") {
		t.Fatalf("both summaries should be present, got %q", prefix)
	}
}

// ---------------------------------------------------------------------------
// P2 产物引用化：ReadUpstreamDeliverable（read_upstream_deliverable 工具的 biz 支撑）
// ---------------------------------------------------------------------------

// The tool's whole point: return the FULL deliverable text (well beyond the
// 500-rune summary budget) so downstream teams can consume it on demand.
func TestReadUpstreamDeliverable_ReturnsFullContent(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	longContent := strings.Repeat("全", 3000) + "\n- 发现X"
	seedCompletedTeamWithSteps(teams, sessions, steps, "t-up", "sp1", "st_1", "调研团队", longContent)

	out, err := u.ReadUpstreamDeliverable(context.Background(), "", "t-up", 0)
	if err != nil {
		t.Fatalf("ReadUpstreamDeliverable: %v", err)
	}
	if out.Content != longContent {
		t.Fatalf("content should be the FULL text (%d runes), got %d runes", len([]rune(longContent)), len([]rune(out.Content)))
	}
	if out.SizeChars != len([]rune(longContent)) {
		t.Fatalf("size_chars mismatch: %d", out.SizeChars)
	}
	if out.Truncated {
		t.Fatalf("content within default budget must not be marked truncated")
	}
	if out.TeamID != "t-up" || out.SessionID != "sess-t-up" {
		t.Fatalf("ids mismatch: %+v", out)
	}
}

func TestReadUpstreamDeliverable_TruncatesToMaxChars(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	longContent := strings.Repeat("详", 3000)
	seedCompletedTeamWithSteps(teams, sessions, steps, "t-up", "sp1", "st_1", "团队", longContent)

	out, err := u.ReadUpstreamDeliverable(context.Background(), "", "t-up", 100)
	if err != nil {
		t.Fatalf("ReadUpstreamDeliverable: %v", err)
	}
	if !out.Truncated {
		t.Fatalf("3000 runes with maxChars=100 must be truncated")
	}
	if len([]rune(out.Content)) > 200 {
		t.Fatalf("truncated content (with marker) should stay near maxChars, got %d runes", len([]rune(out.Content)))
	}
	if out.SizeChars != 3000 {
		t.Fatalf("size_chars must report the FULL size, got %d", out.SizeChars)
	}
}

// maxChars above the hard cap must be clamped to the default budget (defense
// against runaway context consumption).
func TestReadUpstreamDeliverable_MaxCharsClamped(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	longContent := strings.Repeat("量", MaxUpstreamDeliverableChars+5000)
	seedCompletedTeamWithSteps(teams, sessions, steps, "t-up", "sp1", "st_1", "团队", longContent)

	out, err := u.ReadUpstreamDeliverable(context.Background(), "", "t-up", MaxUpstreamDeliverableChars*2)
	if err != nil {
		t.Fatalf("ReadUpstreamDeliverable: %v", err)
	}
	if !out.Truncated {
		t.Fatalf("content beyond the default budget must be truncated after clamping")
	}
	if len([]rune(out.Content)) > DefaultUpstreamDeliverableMaxChars+100 {
		t.Fatalf("clamped budget should be the default %d, got %d runes", DefaultUpstreamDeliverableMaxChars, len([]rune(out.Content)))
	}
}

func TestReadUpstreamDeliverable_TeamNotCompleted_Error(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	teams.items["t-run"] = Team{ID: "t-run", SpiritSessionID: "sp1", DagNodeID: "st_1", Status: TeamStatusRunning}

	if _, err := u.ReadUpstreamDeliverable(context.Background(), "", "t-run", 0); err == nil {
		t.Fatalf("running team must reject full-text reads")
	}
}

func TestReadUpstreamDeliverable_NoContent_Error(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "团队", "")

	if _, err := u.ReadUpstreamDeliverable(context.Background(), "", "t-up", 0); err == nil {
		t.Fatalf("completed team without deliverable content must error")
	}
}

func TestReadUpstreamDeliverable_EmptyTeamID_Error(t *testing.T) {
	u := newDeliverableUsecase(newDeliverableTeamRepo(), newDeliverableSessionAccessor())
	if _, err := u.ReadUpstreamDeliverable(context.Background(), "", "  ", 0); err == nil {
		t.Fatalf("blank team_id must error")
	}
}

// ---------------------------------------------------------------------------
// P2 产物引用化（B.10.16）：DeliverableRef 信封 + 双模兼容读取
// ---------------------------------------------------------------------------

func TestParseDeliverableRefs_P2Envelope(t *testing.T) {
	raw := `{"st_1":{"summary":"摘要","key_findings":"- 发现A","team_id":"t1","team_session_id":"sess-t1","size_chars":1200,"truncated":true}}`
	refs := ParseDeliverableRefs(raw)
	ref, ok := refs["st_1"]
	if !ok {
		t.Fatalf("st_1 ref missing, got %+v", refs)
	}
	if ref.Summary != "摘要" || ref.KeyFindings != "- 发现A" {
		t.Fatalf("summary/key_findings mismatch: %+v", ref)
	}
	if ref.TeamID != "t1" || ref.TeamSessionID != "sess-t1" {
		t.Fatalf("team/session id mismatch: %+v", ref)
	}
	if ref.SizeChars != 1200 || !ref.Truncated {
		t.Fatalf("size/truncated mismatch: %+v", ref)
	}
}

// Legacy rows store a plain JSON string per dag node; they must still parse
// into a summary-only ref so downstream injection keeps working pre-migration.
func TestParseDeliverableRefs_LegacyString(t *testing.T) {
	refs := ParseDeliverableRefs(`{"st_1":"旧格式摘要"}`)
	ref, ok := refs["st_1"]
	if !ok {
		t.Fatalf("st_1 ref missing, got %+v", refs)
	}
	if ref.Summary != "旧格式摘要" {
		t.Fatalf("legacy string should become the summary, got %+v", ref)
	}
	if ref.Truncated || ref.TeamID != "" || ref.TeamSessionID != "" || ref.SizeChars != 0 {
		t.Fatalf("legacy ref must carry no P2 metadata, got %+v", ref)
	}
}

// A row may mix legacy string values (written before P2) with P2 envelopes
// (written after); both forms must coexist in one map.
func TestParseDeliverableRefs_MixedLegacyAndEnvelope(t *testing.T) {
	raw := `{"st_1":"旧摘要","st_2":{"summary":"新摘要","team_id":"t2","team_session_id":"sess-t2","size_chars":10,"truncated":false}}`
	refs := ParseDeliverableRefs(raw)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %+v", refs)
	}
	if refs["st_1"].Summary != "旧摘要" {
		t.Fatalf("legacy value mismatch: %+v", refs["st_1"])
	}
	if refs["st_2"].Summary != "新摘要" || refs["st_2"].TeamID != "t2" {
		t.Fatalf("envelope value mismatch: %+v", refs["st_2"])
	}
}

func TestParseDeliverableRefs_EmptyAndCorrupt(t *testing.T) {
	cases := map[string]string{
		"empty":        "",
		"empty object": "{}",
		"corrupt":      "{bad",
		"wrong value":  `{"st_1":123}`,
	}
	for name, raw := range cases {
		if refs := ParseDeliverableRefs(raw); len(refs) != 0 {
			t.Fatalf("%s: expected no refs, got %+v", name, refs)
		}
	}
}

// ---------------------------------------------------------------------------
// P1 形式契约（B.10.15.2）：注入前缀包含契约声明（name/type/format）
// ---------------------------------------------------------------------------

func TestInjectUpstreamDeliverables_IncludesContractDeclaration(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	upstream := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "调研团队", "不应使用")
	upstream.Deliverables = `[{"name":"research_report","type":"document","format":"markdown","description":"调研结论报告"}]`
	// 2026-07-25 Fix 1：注入源只认持久化 envelope，reply 不再是来源。
	upstream.DeliverablesOutput = `{"st_1":"调研摘要"}`
	teams.items["t-up"] = upstream

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if !strings.Contains(prefix, "契约: research_report (document/markdown) — 调研结论报告") {
		t.Fatalf("prefix should declare the upstream contract, got %q", prefix)
	}
	if !strings.Contains(prefix, "调研摘要") {
		t.Fatalf("prefix should still contain the summary, got %q", prefix)
	}
}

func TestInjectUpstreamDeliverables_ContractWithoutDescription(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	upstream := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "数据团队", "不应使用")
	upstream.Deliverables = `[{"name":"dataset","type":"data","format":"json"}]`
	// 2026-07-25 Fix 1：注入源只认持久化 envelope，reply 不再是来源。
	upstream.DeliverablesOutput = `{"st_1":"数据摘要"}`
	teams.items["t-up"] = upstream

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if !strings.Contains(prefix, "契约: dataset (data/json)") {
		t.Fatalf("contract without description should render name (type/format), got %q", prefix)
	}
	if strings.Contains(prefix, "dataset (data/json) —") {
		t.Fatalf("no description → no em-dash suffix, got %q", prefix)
	}
}

func TestInjectUpstreamDeliverables_NoContract_OmitsDeclarationLine(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	// Upstream has no contract declared → keep the legacy prefix shape.
	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "上游团队", "不应使用")
	up.DeliverablesOutput = `{"st_1":"成果"}`
	teams.items["t-up"] = up

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if strings.Contains(prefix, "契约:") {
		t.Fatalf("no contract declared → no declaration line, got %q", prefix)
	}
	if !strings.Contains(prefix, "上游团队") || !strings.Contains(prefix, "成果") {
		t.Fatalf("legacy prefix content must be preserved, got %q", prefix)
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

// ---------------------------------------------------------------------------
// Graph StateFields 桥接（B.10.15.4）：enable_state_deliverable 团队从
// graph final state 读 deliverable 充实落库信封
// ---------------------------------------------------------------------------

// graphDeliverableReaderStub stubs SpiritGraphDeliverableReader: returns a
// fixed deliverable map and records the session-key coordinates it was
// called with (appName/userID/sessionID).
type graphDeliverableReaderStub struct {
	data       map[string]any
	err        error
	calls      int
	gotAppName string
	gotUserID  string
	gotSession string
}

func (s *graphDeliverableReaderStub) ReadGraphDeliverable(_ context.Context, appName, userID, sessionID string) (map[string]any, error) {
	s.calls++
	s.gotAppName, s.gotUserID, s.gotSession = appName, userID, sessionID
	return s.data, s.err
}

func newDeliverableUsecaseWithGraphReader(teams *deliverableTeamRepo, sessions *deliverableSessionAccessor, steps *deliverableStepReader, reader *graphDeliverableReaderStub) *SpiritTeamUsecase {
	return NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop(),
		WithSpiritStepReader(steps), WithGraphDeliverableReader(reader))
}

// seedStateDeliverableTeam seeds a completed team carrying the given
// DefinitionJSON (the enable_state_deliverable switch lives there).
func seedStateDeliverableTeam(teams *deliverableTeamRepo, sessions *deliverableSessionAccessor, steps *deliverableStepReader, definitionJSON, assistantContent string) Team {
	t := seedCompletedTeamWithSteps(teams, sessions, steps, "t1", "sp1", "st_1", "分析团队", assistantContent)
	t.DefinitionJSON = definitionJSON
	teams.items["t1"] = t
	return t
}

// State deliverable present with a "summary" key: the envelope summary comes
// from the graph state (not the reply step), and the remaining keys are
// serialized into StructuredJSON.
func TestWriteDeliverablesToSession_GraphStateBridge_EnrichesEnvelope(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"summary":    "state 结构化摘要",
		"confidence": 0.9,
		"findings":   []any{"发现A", "发现B"},
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"intent_anchor_agent_id":"agent-anchor","members":[{"agent_id":"agent-anchor"},{"agent_id":"agent-m1"}]}`,
		"reply 摘要（不应使用）")

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	ref := ParseDeliverableRefs(teams.items["t1"].DeliverablesOutput)["st_1"]
	if ref.Summary != "state 结构化摘要" {
		t.Fatalf("summary should come from graph state, got %q", ref.Summary)
	}
	if ref.SizeChars != len([]rune("state 结构化摘要")) || ref.Truncated {
		t.Fatalf("size/truncated should describe the state summary, got %+v", ref)
	}
	if ref.StructuredJSON == "" {
		t.Fatalf("structured_json should carry the non-summary state keys, got %+v", ref)
	}
	var structured map[string]any
	if err := json.Unmarshal([]byte(ref.StructuredJSON), &structured); err != nil {
		t.Fatalf("structured_json should be valid JSON: %v", err)
	}
	if _, ok := structured["confidence"]; !ok {
		t.Fatalf("structured_json should contain confidence, got %q", ref.StructuredJSON)
	}
	if _, ok := structured["summary"]; ok {
		t.Fatalf("structured_json must exclude the summary key, got %q", ref.StructuredJSON)
	}
	// Session-key coordinates: appName = intent_anchor_agent_id, session = team main session.
	if reader.calls != 1 || reader.gotAppName != "agent-anchor" || reader.gotSession != "sess-t1" {
		t.Fatalf("reader coordinates mismatch: calls=%d appName=%q session=%q", reader.calls, reader.gotAppName, reader.gotSession)
	}
	if reader.gotUserID == "" {
		t.Fatalf("reader should receive the ctx user ID")
	}
}

// Without intent_anchor_agent_id, the runner uses the first member as the
// anchor (AppName) — the bridge must resolve the same anchor.
func TestWriteDeliverablesToSession_GraphStateBridge_AnchorFallsBackToFirstMember(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "s"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"},{"agent_id":"agent-m2"}]}`,
		"成果")

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	if reader.gotAppName != "agent-m1" {
		t.Fatalf("anchor should fall back to the first member, got %q", reader.gotAppName)
	}
}

// An intent anchor that names NO member is ignored by the runner (it warns
// and falls back to the first member) — the bridge must mirror that decision,
// otherwise it reads the state under an AppName the run never persisted to.
func TestWriteDeliverablesToSession_GraphStateBridge_IntentAnchorNotInMembers_FallsBackToFirstMember(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "s"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"intent_anchor_agent_id":"agent-ghost","members":[{"agent_id":"agent-m1"},{"agent_id":"agent-m2"}]}`,
		"成果")

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	if reader.gotAppName != "agent-m1" {
		t.Fatalf("intent anchor not in members → fall back to the first member (runner mirror), got %q", reader.gotAppName)
	}
}

// enable_state_deliverable absent/false: the state channel is never read and
// the write fails with ErrNoRealDeliverable — reply text is never a source.
func TestWriteDeliverablesToSession_GraphStateDisabled_ErrNoRealDeliverable(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "不应读取"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","members":[{"agent_id":"agent-m1"}]}`,
		"reply 摘要")

	err := u.WriteDeliverablesToSession(context.Background(), "t1")
	if !errors.Is(err, ErrNoRealDeliverable) {
		t.Fatalf("channel disabled → ErrNoRealDeliverable, got %v", err)
	}
	if reader.calls != 0 {
		t.Fatalf("state channel disabled → reader must not be called, got %d calls", reader.calls)
	}
	if teams.updateCalls != 0 {
		t.Fatalf("channel disabled → no envelope write, got %d updates", teams.updateCalls)
	}
}

// ---------------------------------------------------------------------------
// 2026-07-25 Fix 1 真实产出判定：只认 set_deliverable 写入的 graph state
// deliverable。reply 文本不是交付物 —— 只提问/只说话的团队没有产出，
// 必须被识别为「无真实交付物」而不是用 reply 编造一个。
// ---------------------------------------------------------------------------

// HasRealDeliverable: graph state deliverable 非空 → true。
func TestHasRealDeliverable_StatePresent_True(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "真实成果"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`, "")

	has, err := u.HasRealDeliverable(context.Background(), team)
	if err != nil || !has {
		t.Fatalf("state present → (true, nil), got (%v, %v)", has, err)
	}
	// 坐标必须与 runner 持久化 graph state 的 session key 一致。
	if reader.gotAppName != "agent-m1" || reader.gotSession != "sess-t1" {
		t.Fatalf("reader coordinates mismatch: appName=%q session=%q", reader.gotAppName, reader.gotSession)
	}
}

// HasRealDeliverable: state channel enabled but the map is absent → (false, nil)。
// 这是 19:29 场景的核心：团队只提问澄清、从未调用 set_deliverable。
func TestHasRealDeliverable_StateAbsent_False(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: nil}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`,
		"请问目标金额是多少？（仅澄清提问，无交付物）")

	has, err := u.HasRealDeliverable(context.Background(), team)
	if err != nil || has {
		t.Fatalf("state absent → (false, nil), got (%v, %v)", has, err)
	}
}

// HasRealDeliverable: enable_state_deliverable 未开启 → false 且根本不读 state
// （channel 不存在，读了也没有）。
func TestHasRealDeliverable_ChannelDisabled_False(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "x"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","members":[{"agent_id":"agent-m1"}]}`, "reply")

	has, err := u.HasRealDeliverable(context.Background(), team)
	if err != nil || has {
		t.Fatalf("channel disabled → (false, nil), got (%v, %v)", has, err)
	}
	if reader.calls != 0 {
		t.Fatalf("channel disabled → reader must not be called, got %d calls", reader.calls)
	}
}

// HasRealDeliverable: state 读取 infra 错误 → (false, err)，调用方据此区分
// 「未产出」与「校验失败」。
func TestHasRealDeliverable_ReaderError_ReturnsError(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{err: fmt.Errorf("session store down")}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`, "")

	has, err := u.HasRealDeliverable(context.Background(), team)
	if err == nil || has {
		t.Fatalf("reader error → (false, err), got (%v, %v)", has, err)
	}
}

// HasRealDeliverable: 团队连 session 都没有 → (false, nil)，不算 infra 错误。
func TestHasRealDeliverable_NoTeamSession_False(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "x"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := Team{ID: "ghost", SpiritSessionID: "sp1", DagNodeID: "st_1", Status: TeamStatusCompleted,
		DefinitionJSON: `{"enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`}
	teams.items["ghost"] = team

	has, err := u.HasRealDeliverable(context.Background(), team)
	if err != nil || has {
		t.Fatalf("no session → (false, nil), got (%v, %v)", has, err)
	}
}

// WriteDeliverablesToSession: 无 graph state deliverable → ErrNoRealDeliverable，
// 禁止用 reply 文本编造信封（19:29 谎报链的源头）。
func TestWriteDeliverablesToSession_NoStateDeliverable_ErrNoRealDeliverable(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: nil}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`,
		"请问目标金额是多少？（仅澄清提问）")

	err := u.WriteDeliverablesToSession(context.Background(), "t1")
	if !errors.Is(err, ErrNoRealDeliverable) {
		t.Fatalf("no state deliverable → ErrNoRealDeliverable, got %v", err)
	}
	if teams.updateCalls != 0 {
		t.Fatalf("no deliverable → DeliverablesOutput must stay untouched, got %d updates", teams.updateCalls)
	}
}

// WriteDeliverablesToSession: state 读取失败同样按「无真实交付物」处理（原因进日志），
// 绝不回退 reply 兜底。
func TestWriteDeliverablesToSession_StateUnreadable_ErrNoRealDeliverable(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{err: fmt.Errorf("session not found")}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`,
		"reply 摘要")

	err := u.WriteDeliverablesToSession(context.Background(), "t1")
	if !errors.Is(err, ErrNoRealDeliverable) {
		t.Fatalf("unreadable state → ErrNoRealDeliverable, got %v", err)
	}
}

// state 无 "summary" key：摘要从业务 key 的 StructuredJSON 派生 —— 仍 100%
// 源自真实交付物，绝不碰 reply。
func TestWriteDeliverablesToSession_NoSummaryKey_SummaryFromStructuredKeys(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"score": 0.8, "verdict": "pass"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`,
		"reply 摘要（不应使用）")

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	ref := ParseDeliverableRefs(teams.items["t1"].DeliverablesOutput)["st_1"]
	if ref.Summary == "" || strings.Contains(ref.Summary, "reply") {
		t.Fatalf("summary must derive from structured state keys, never reply, got %q", ref.Summary)
	}
	if !strings.Contains(ref.Summary, "score") {
		t.Fatalf("summary should carry the business keys JSON, got %q", ref.Summary)
	}
	if ref.StructuredJSON == "" || !strings.Contains(ref.StructuredJSON, "verdict") {
		t.Fatalf("structured_json should carry the state map, got %q", ref.StructuredJSON)
	}
}

// RecordTeamCompletion: 无真实交付物时静默跳过落库（service 侧闸门已把团队
// 标 failed；此处只是双保险，不产生 Warn 噪音、不写任何信封）。
func TestRecordTeamCompletion_NoStateDeliverable_SkipsWrite(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: nil}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`,
		"仅提问，无交付物")

	u.RecordTeamCompletion(context.Background(), team, 100)

	if stored := teams.items["t1"]; stored.DeliverablesOutput != "" {
		t.Fatalf("no real deliverable → no envelope write, got %q", stored.DeliverablesOutput)
	}
}

// InjectUpstreamDeliverables: 上游 completed 但无持久化信封 → 跳过该上游，
// 禁止 reply 兜底编造下游输入（Fix 1 后 completed ⇒ 信封必存在；
// 无信封 = 落库失败等异常，降级为「无注入」而非编造）。
func TestInjectUpstreamDeliverables_NoEnvelope_SkipsUpstream(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "上游团队", "即时提取的成果（不应使用）")

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if prefix != "" {
		t.Fatalf("no envelope → no injection (reply fallback removed), got %q", prefix)
	}
}

// ---------------------------------------------------------------------------
// Phase B: ReadUpstreamDeliverable runtime contract validation
// (read_upstream_deliverable 工具调用级契约校验：reader 团队的 InputContract
// 对上游团队声明的 Deliverables 做 name/type/format 校验，不匹配返回结构化
// *ContractMismatchError 供调用方自动纠正重试)
// ---------------------------------------------------------------------------

// seedReaderTeamWithContract seeds a downstream (reader) team + its main
// session carrying the given input contract JSON.
func seedReaderTeamWithContract(teams *deliverableTeamRepo, sessions *deliverableSessionAccessor, id, spiritSessionID, inputContractJSON string) Team {
	t := Team{
		ID:              id,
		SpiritSessionID: spiritSessionID,
		DagNodeID:       "st_2",
		DisplayName:     "下游团队",
		Status:          TeamStatusRunning,
		InputContract:   inputContractJSON,
	}
	teams.items[id] = t
	sessions.sessionsByTeam[id] = Session{ID: "sess-" + id, TeamID: id, ParentSessionID: spiritSessionID, SessionType: string(SessionTypeTeam)}
	return t
}

// Matching contracts: the read proceeds and returns the full content.
func TestReadUpstreamDeliverable_ContractMatch_ReturnsContent(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "设计团队", "设计规格全文")
	up.Deliverables = `[{"name":"design_spec","type":"document","format":"markdown"}]`
	teams.items["t-up"] = up
	seedReaderTeamWithContract(teams, sessions, "t-down", "sp1", `[{"name":"design_spec","type":"document","format":"markdown"}]`)

	out, err := u.ReadUpstreamDeliverable(context.Background(), "sess-t-down", "t-up", 0)
	if err != nil {
		t.Fatalf("matching contracts must not block the read: %v", err)
	}
	if out.Content != "设计规格全文" {
		t.Fatalf("content mismatch: %q", out.Content)
	}
}

// Mismatched contracts: type + format + missing entry must all be reported
// in one structured *ContractMismatchError (LLM-actionable, auto-retryable).
func TestReadUpstreamDeliverable_ContractMismatch_StructuredError(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "数据团队", "数据全文")
	up.Deliverables = `[{"name":"design_spec","type":"data","format":"json"}]`
	teams.items["t-up"] = up
	seedReaderTeamWithContract(teams, sessions, "t-down", "sp1",
		`[{"name":"design_spec","type":"document","format":"markdown"},{"name":"api_schema","type":"document","format":"json"}]`)

	_, err := u.ReadUpstreamDeliverable(context.Background(), "sess-t-down", "t-up", 0)
	if err == nil {
		t.Fatal("contract mismatch must block the read with a structured error")
	}
	var cmErr *ContractMismatchError
	if !errors.As(err, &cmErr) {
		t.Fatalf("error should be *ContractMismatchError, got %T: %v", err, err)
	}
	if cmErr.ReaderTeamID != "t-down" || cmErr.UpstreamTeamID != "t-up" {
		t.Fatalf("error team ids mismatch: %+v", cmErr)
	}
	if len(cmErr.Mismatches) != 3 {
		t.Fatalf("expected 3 mismatches (type, format, missing), got %+v", cmErr.Mismatches)
	}
	byKind := map[string]ContractMismatch{}
	for _, m := range cmErr.Mismatches {
		byKind[m.Kind] = m
	}
	if m := byKind[ContractMismatchType]; m.Name != "design_spec" || m.Expected != "document" || m.Actual != "data" {
		t.Fatalf("type mismatch detail wrong: %+v", m)
	}
	if m := byKind[ContractMismatchFormat]; m.Name != "design_spec" || m.Expected != "markdown" || m.Actual != "json" {
		t.Fatalf("format mismatch detail wrong: %+v", m)
	}
	if m := byKind[ContractMismatchMissing]; m.Name != "api_schema" {
		t.Fatalf("missing mismatch detail wrong: %+v", m)
	}
	// The message must be LLM-actionable: name the teams and the entries.
	msg := err.Error()
	for _, want := range []string{"t-down", "t-up", "design_spec", "api_schema"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message should mention %q, got %q", want, msg)
		}
	}
}

// No reader session (CLI / unresolvable caller): contract check is skipped —
// declarations stay advisory for callers without a resolvable reader team.
func TestReadUpstreamDeliverable_ContractCheckSkippedWithoutReaderSession(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "数据团队", "数据全文")
	up.Deliverables = `[{"name":"dataset","type":"data","format":"json"}]`
	teams.items["t-up"] = up

	out, err := u.ReadUpstreamDeliverable(context.Background(), "", "t-up", 0)
	if err != nil {
		t.Fatalf("empty reader session must skip the contract check: %v", err)
	}
	if out.Content != "数据全文" {
		t.Fatalf("content mismatch: %q", out.Content)
	}
}

// Undeclared contracts on either side: nothing to validate → read proceeds.
func TestReadUpstreamDeliverable_ContractCheckSkippedWhenUndeclared(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	// Case 1: reader has an input contract but upstream declares nothing.
	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "上游团队", "成果全文")
	teams.items["t-up"] = up // no Deliverables
	seedReaderTeamWithContract(teams, sessions, "t-down", "sp1", `[{"name":"design_spec","type":"document","format":"markdown"}]`)

	if _, err := u.ReadUpstreamDeliverable(context.Background(), "sess-t-down", "t-up", 0); err != nil {
		t.Fatalf("undeclared upstream contract must skip the check: %v", err)
	}

	// Case 2: upstream declares deliverables but reader has no input contract.
	up2 := seedCompletedTeam(teams, sessions, "t-up2", "sp1", "st_1", "上游团队2", "成果全文2")
	up2.Deliverables = `[{"name":"dataset","type":"data","format":"json"}]`
	teams.items["t-up2"] = up2
	seedReaderTeamWithContract(teams, sessions, "t-down2", "sp1", "")

	if _, err := u.ReadUpstreamDeliverable(context.Background(), "sess-t-down2", "t-up2", 0); err != nil {
		t.Fatalf("empty reader input contract must skip the check: %v", err)
	}
}

// ---------------------------------------------------------------------------
// C1 认知信封 + C4 血缘链（B.10.20.2 / B.10.20.5）
// ---------------------------------------------------------------------------

// State map with a "cognition" reserved key: the envelope carries the typed
// Cognition record, the key is excluded from StructuredJSON, and DerivedFrom
// is filled from Team.DependsOn.
func TestWriteDeliverablesToSession_GraphStateBridge_CognitionExtracted(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"summary": "state 摘要",
		"cognition": map[string]any{
			"decisions":      []any{map[string]any{"choice": "方案A", "rationale": "成本低", "confidence": 0.8}},
			"rejected":       []any{map[string]any{"option": "方案B", "reason": "太慢"}},
			"assumptions":    []any{"数据已封板"},
			"open_questions": []any{"样本偏差未校正"},
		},
		"extra": "kept",
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	team := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`,
		"reply 摘要")
	team.DependsOn = []string{"st_0"}
	teams.items["t1"] = team

	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	ref := ParseDeliverableRefs(teams.items["t1"].DeliverablesOutput)["st_1"]

	// C1: cognition bridged into the envelope.
	if ref.Cognition == nil {
		t.Fatalf("cognition should be extracted from the state map, got %+v", ref)
	}
	if len(ref.Cognition.Decisions) != 1 || ref.Cognition.Decisions[0].Choice != "方案A" || ref.Cognition.Decisions[0].Confidence != 0.8 {
		t.Fatalf("decisions mismatch: %+v", ref.Cognition.Decisions)
	}
	if len(ref.Cognition.Rejected) != 1 || ref.Cognition.Rejected[0].Option != "方案B" {
		t.Fatalf("rejected mismatch: %+v", ref.Cognition.Rejected)
	}
	if len(ref.Cognition.Assumptions) != 1 || ref.Cognition.Assumptions[0] != "数据已封板" {
		t.Fatalf("assumptions mismatch: %+v", ref.Cognition.Assumptions)
	}
	if len(ref.Cognition.OpenQuestions) != 1 || ref.Cognition.OpenQuestions[0] != "样本偏差未校正" {
		t.Fatalf("open_questions mismatch: %+v", ref.Cognition.OpenQuestions)
	}

	// Reserved keys must not land in StructuredJSON; other keys must.
	if strings.Contains(ref.StructuredJSON, "cognition") || strings.Contains(ref.StructuredJSON, "summary") {
		t.Fatalf("structured_json must exclude reserved keys, got %q", ref.StructuredJSON)
	}
	if !strings.Contains(ref.StructuredJSON, "extra") {
		t.Fatalf("structured_json should keep non-reserved keys, got %q", ref.StructuredJSON)
	}

	// C4: derived_from mirrors Team.DependsOn.
	if len(ref.DerivedFrom) != 1 || ref.DerivedFrom[0] != "st_0" {
		t.Fatalf("derived_from should mirror DependsOn, got %+v", ref.DerivedFrom)
	}
}

// A malformed cognition entry (wrong shape) is tolerated: nil Cognition, the
// write still succeeds.
func TestWriteDeliverablesToSession_GraphStateBridge_MalformedCognition_Tolerated(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"summary":   "state 摘要",
		"cognition": "not-an-object",
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`,
		"reply 摘要")
	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("malformed cognition must not block the write: %v", err)
	}
	ref := ParseDeliverableRefs(teams.items["t1"].DeliverablesOutput)["st_1"]
	if ref.Cognition != nil {
		t.Fatalf("malformed cognition → nil, got %+v", ref.Cognition)
	}
}

// No DependsOn → DerivedFrom stays empty (omitempty keeps the envelope
// byte-compatible with pre-C4 writes).
func TestWriteDeliverablesToSession_NoDependsOn_DerivedFromEmpty(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{"summary": "成果"}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)
	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"members":[{"agent_id":"agent-m1"}]}`, "")
	if err := u.WriteDeliverablesToSession(context.Background(), "t1"); err != nil {
		t.Fatalf("WriteDeliverablesToSession: %v", err)
	}
	if ref := ParseDeliverableRefs(teams.items["t1"].DeliverablesOutput)["st_1"]; len(ref.DerivedFrom) != 0 {
		t.Fatalf("no DependsOn → empty derived_from, got %+v", ref.DerivedFrom)
	}
	if strings.Contains(teams.items["t1"].DeliverablesOutput, "derived_from") {
		t.Fatalf("empty derived_from must be omitted, got %q", teams.items["t1"].DeliverablesOutput)
	}
}

// Envelope with cognition → the injection prefix renders the cognition lines
// after the summary.
func TestInjectUpstreamDeliverables_RendersCognition(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	upstream := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "调研团队", "不应使用")
	upstream.DeliverablesOutput = `{"st_1":{"summary":"结论摘要","team_id":"t-up","team_session_id":"sess-t-up","size_chars":4,"truncated":false,` +
		`"cognition":{"decisions":[{"choice":"方案A","rationale":"成本低","confidence":0.8}],` +
		`"rejected":[{"option":"方案B","reason":"太慢"}],` +
		`"assumptions":["数据已封板"],"open_questions":["样本偏差未校正"]}}}`
	teams.items["t-up"] = upstream

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	for _, want := range []string{
		"[上游决策] 选择 方案A（理由: 成本低，置信度 0.8）；否决 方案B（原因: 太慢）",
		"[上游假设] 数据已封板",
		"[上游遗留问题] 样本偏差未校正",
	} {
		if !strings.Contains(prefix, want) {
			t.Fatalf("prefix should contain %q, got %q", want, prefix)
		}
	}
}

// Envelope without cognition → the prefix keeps its legacy shape (no
// cognition lines).
func TestInjectUpstreamDeliverables_NoCognition_NoCognitionLines(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	upstream := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "调研团队", "成果")
	upstream.DeliverablesOutput = `{"st_1":{"summary":"摘要","team_id":"t-up","team_session_id":"sess-t-up","size_chars":2,"truncated":false}}`
	teams.items["t-up"] = upstream

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if strings.Contains(prefix, "[上游决策]") || strings.Contains(prefix, "[上游假设]") || strings.Contains(prefix, "[上游遗留问题]") {
		t.Fatalf("no cognition → no cognition lines, got %q", prefix)
	}
}

// Overlong cognition items are truncated to cognitionItemMaxRunes so a
// verbose upstream record cannot blow up the injection prefix.
func TestRenderCognitionLines_TruncatesLongItems(t *testing.T) {
	long := strings.Repeat("长", 500)
	out := renderCognitionLines(&DeliverableCognition{
		Decisions:   []DeliverableDecision{{Choice: "A", Rationale: long}},
		Assumptions: []string{long},
	})
	if out == "" {
		t.Fatal("expected rendered cognition")
	}
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > cognitionItemMaxRunes+len("[上游假设] ")+1 {
			t.Fatalf("line exceeds truncation budget (%d runes): %q", len([]rune(line)), line)
		}
	}
	if !strings.Contains(out, "[上游决策]") || !strings.Contains(out, "[上游假设]") {
		t.Fatalf("both aspects should render, got %q", out)
	}
}

// nil / empty cognition renders nothing.
func TestRenderCognitionLines_Empty(t *testing.T) {
	if got := renderCognitionLines(nil); got != "" {
		t.Fatalf("nil cognition → empty, got %q", got)
	}
	if got := renderCognitionLines(&DeliverableCognition{}); got != "" {
		t.Fatalf("empty cognition → empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// C2 契约内容级 Schema 校验（B.10.20.3）
// ---------------------------------------------------------------------------

// schema_json round-trips through contract parsing; absent schema stays empty.
func TestParseDeliverableContracts_SchemaJSON(t *testing.T) {
	contracts, err := ParseDeliverableContracts(`[{"name":"dataset","type":"data","format":"json","schema_json":"{\"type\":\"object\"}"},{"name":"doc","type":"document","format":"markdown"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 2 {
		t.Fatalf("expected 2 contracts, got %+v", contracts)
	}
	if contracts[0].SchemaJSON != `{"type":"object"}` {
		t.Fatalf("schema_json mismatch: %q", contracts[0].SchemaJSON)
	}
	if contracts[1].SchemaJSON != "" {
		t.Fatalf("absent schema_json must stay empty, got %q", contracts[1].SchemaJSON)
	}
}

// Content satisfying the reader's schema → the read proceeds.
func TestReadUpstreamDeliverable_SchemaMatch_ReturnsContent(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "数据团队", `{"budget":100,"currency":"CNY"}`)
	up.Deliverables = `[{"name":"dataset","type":"data","format":"json"}]`
	teams.items["t-up"] = up
	seedReaderTeamWithContract(teams, sessions, "t-down", "sp1",
		`[{"name":"dataset","type":"data","format":"json","schema_json":"{\"type\":\"object\",\"required\":[\"budget\"]}"}]`)

	out, err := u.ReadUpstreamDeliverable(context.Background(), "sess-t-down", "t-up", 0)
	if err != nil {
		t.Fatalf("schema-satisfying content must not be blocked: %v", err)
	}
	if !strings.Contains(out.Content, "budget") {
		t.Fatalf("content mismatch: %q", out.Content)
	}
}

// Content violating the reader's schema → structured *ContractMismatchError
// with a schema_mismatch entry carrying the violation detail.
func TestReadUpstreamDeliverable_SchemaMismatch_StructuredError(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "数据团队", `{"cost":5}`)
	up.Deliverables = `[{"name":"dataset","type":"data","format":"json"}]`
	teams.items["t-up"] = up
	seedReaderTeamWithContract(teams, sessions, "t-down", "sp1",
		`[{"name":"dataset","type":"data","format":"json","schema_json":"{\"type\":\"object\",\"required\":[\"budget\"]}"}]`)

	_, err := u.ReadUpstreamDeliverable(context.Background(), "sess-t-down", "t-up", 0)
	if err == nil {
		t.Fatal("schema violation must block the read with a structured error")
	}
	var cmErr *ContractMismatchError
	if !errors.As(err, &cmErr) {
		t.Fatalf("error should be *ContractMismatchError, got %T: %v", err, err)
	}
	if len(cmErr.Mismatches) != 1 {
		t.Fatalf("expected 1 schema mismatch, got %+v", cmErr.Mismatches)
	}
	m := cmErr.Mismatches[0]
	if m.Kind != ContractMismatchSchema || m.Name != "dataset" {
		t.Fatalf("mismatch detail wrong: %+v", m)
	}
	if !strings.Contains(m.Expected, "budget") {
		t.Fatalf("mismatch should carry the violation detail, got %q", m.Expected)
	}
	if msg := err.Error(); !strings.Contains(msg, "dataset") || !strings.Contains(msg, "schema") {
		t.Fatalf("error message should be LLM-actionable, got %q", msg)
	}
}

// Advisory skips: non-json format entries, non-JSON content, and invalid
// schema declarations must not block the read.
func TestReadUpstreamDeliverable_SchemaCheckSkips(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	// Case 1: entry format is markdown → schema ignored even though present.
	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "文档团队", "markdown 全文")
	up.Deliverables = `[{"name":"doc","type":"document","format":"markdown"}]`
	teams.items["t-up"] = up
	seedReaderTeamWithContract(teams, sessions, "t-down", "sp1",
		`[{"name":"doc","type":"document","format":"markdown","schema_json":"{\"type\":\"object\",\"required\":[\"x\"]}"}]`)
	if _, err := u.ReadUpstreamDeliverable(context.Background(), "sess-t-down", "t-up", 0); err != nil {
		t.Fatalf("non-json format must skip schema check: %v", err)
	}

	// Case 2: json format but content is not valid JSON → skip (advisory).
	up2 := seedCompletedTeam(teams, sessions, "t-up2", "sp1", "st_1", "数据团队", "不是 JSON 文本")
	up2.Deliverables = `[{"name":"dataset","type":"data","format":"json"}]`
	teams.items["t-up2"] = up2
	seedReaderTeamWithContract(teams, sessions, "t-down2", "sp1",
		`[{"name":"dataset","type":"data","format":"json","schema_json":"{\"type\":\"object\",\"required\":[\"budget\"]}"}]`)
	if _, err := u.ReadUpstreamDeliverable(context.Background(), "sess-t-down2", "t-up2", 0); err != nil {
		t.Fatalf("non-JSON content must skip schema check: %v", err)
	}

	// Case 3: invalid schema declaration on the reader side → execution error,
	// advisory skip (the reader's own contract is broken, not the upstream's).
	up3 := seedCompletedTeam(teams, sessions, "t-up3", "sp1", "st_1", "数据团队", `{"budget":1}`)
	up3.Deliverables = `[{"name":"dataset","type":"data","format":"json"}]`
	teams.items["t-up3"] = up3
	seedReaderTeamWithContract(teams, sessions, "t-down3", "sp1",
		`[{"name":"dataset","type":"data","format":"json","schema_json":"not-a-schema"}]`)
	if _, err := u.ReadUpstreamDeliverable(context.Background(), "sess-t-down3", "t-up3", 0); err != nil {
		t.Fatalf("invalid schema declaration must skip schema check: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 2026-07-25 Fix 7：ReadUpstreamDeliverable 内容源 = 交付物本身
// （graph state → 持久化信封），绝不读 reply —— 与注入前缀（信封摘要）同源，
// 否则「注入摘要是交付物、全文读取是 reply」自相矛盾。
// ---------------------------------------------------------------------------

// Graph state holds the full, untruncated deliverable; the reply step holds
// different text. The tool must return the state content (untruncated summary
// + structured keys), never the reply.
func TestReadUpstreamDeliverable_PrefersGraphStateOverReply(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	longSummary := strings.Repeat("摘", 600) // > MaxSummaryLen: the envelope would truncate it
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"summary": longSummary,
		"report":  map[string]any{"budget": 100},
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"intent_anchor_agent_id":"agent-anchor","members":[{"agent_id":"agent-anchor"}]}`,
		"reply 文本（不是交付物，不应返回）")

	out, err := u.ReadUpstreamDeliverable(context.Background(), "", "t1", 0)
	if err != nil {
		t.Fatalf("ReadUpstreamDeliverable: %v", err)
	}
	if strings.Contains(out.Content, "reply 文本") {
		t.Fatalf("content must not come from the reply, got %q", out.Content)
	}
	if !strings.HasPrefix(out.Content, longSummary) {
		t.Fatalf("content should start with the FULL untruncated state summary (600 runes), got %d runes", len([]rune(out.Content)))
	}
	if !strings.Contains(out.Content, `"budget":100`) {
		t.Fatalf("content should carry the structured state keys as JSON, got %q", out.Content)
	}
	if out.SessionID != "sess-t1" {
		t.Fatalf("session id should be the team main session, got %q", out.SessionID)
	}
}

// Graph session unreadable (empty state) but a persisted envelope exists →
// the tool degrades to the envelope content, still never the reply.
func TestReadUpstreamDeliverable_GraphStateEmpty_FallsBackToEnvelope(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{}} // state gone
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)

	tm := seedStateDeliverableTeam(teams, sessions, steps,
		`{"version":1,"mode":"sequential","enable_state_deliverable":true,"intent_anchor_agent_id":"agent-anchor","members":[{"agent_id":"agent-anchor"}]}`,
		"reply 文本（不应返回）")
	tm.DeliverablesOutput = `{"st_1":{"summary":"信封摘要","structured_json":"{\"a\":1}","team_id":"t1","team_session_id":"sess-t1","size_chars":100}}`
	teams.items["t1"] = tm

	out, err := u.ReadUpstreamDeliverable(context.Background(), "", "t1", 0)
	if err != nil {
		t.Fatalf("ReadUpstreamDeliverable: %v", err)
	}
	if !strings.Contains(out.Content, "信封摘要") || !strings.Contains(out.Content, `{"a":1}`) {
		t.Fatalf("content should come from the envelope, got %q", out.Content)
	}
	if strings.Contains(out.Content, "reply 文本") {
		t.Fatalf("content must not come from the reply, got %q", out.Content)
	}
	if out.SessionID != "sess-t1" {
		t.Fatalf("session id should come from the envelope, got %q", out.SessionID)
	}
}

// ---------------------------------------------------------------------------
// 2026-07-25 Fix 2b 交付协议强制化：BuildTeamTurnInput
// = 上游交付物前缀 + 任务描述 + 交付协议后缀。存储的 TaskDescription 保持纯净。
// ---------------------------------------------------------------------------

// DAG 团队（无依赖）→ 任务描述后追加交付协议块，告知必须调用 set_deliverable。
func TestBuildTeamTurnInput_DAGTeam_AppendsProtocol(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	team := Team{SpiritSessionID: "sp1", DagNodeID: "st_2", TaskDescription: "撰写调研报告"}
	out := u.BuildTeamTurnInput(context.Background(), team)

	if !strings.HasPrefix(out, "撰写调研报告") {
		t.Fatalf("input should start with the task description, got %q", out)
	}
	if !strings.Contains(out, "交付协议") || !strings.Contains(out, "set_deliverable") {
		t.Fatalf("DAG team input must carry the delivery protocol, got %q", out)
	}
	if !strings.Contains(out, "summary") {
		t.Fatalf("protocol should name the reserved summary key, got %q", out)
	}
}

// 声明了交付契约的 DAG 团队 → 协议块列出契约（name/type/format），
// 让团队知道必须按契约逐项提交。
func TestBuildTeamTurnInput_Contract_ProtocolDeclaresContract(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	team := Team{
		SpiritSessionID: "sp1", DagNodeID: "st_2", TaskDescription: "撰写调研报告",
		Deliverables: `[{"name":"research_report","type":"document","format":"markdown","description":"调研结论报告"}]`,
	}
	out := u.BuildTeamTurnInput(context.Background(), team)

	if !strings.Contains(out, "契约: research_report (document/markdown) — 调研结论报告") {
		t.Fatalf("protocol should declare the team's own contract, got %q", out)
	}
}

// 有上游依赖 → 顺序为：上游交付物前缀 → 任务描述 → 交付协议后缀。
func TestBuildTeamTurnInput_WithUpstream_PrefixDescProtocolOrder(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	up := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "调研团队", "不应使用")
	up.DeliverablesOutput = `{"st_1":"调研结论"}`
	teams.items["t-up"] = up

	downstream := Team{SpiritSessionID: "sp1", DagNodeID: "st_2", TaskDescription: "基于调研写报告", DependsOn: []string{"st_1"}}
	out := u.BuildTeamTurnInput(context.Background(), downstream)

	iPrefix := strings.Index(out, "--- 上游交付物 ---")
	iDesc := strings.Index(out, "基于调研写报告")
	iProtocol := strings.Index(out, "交付协议")
	if iPrefix < 0 || iDesc < 0 || iProtocol < 0 {
		t.Fatalf("input should contain upstream prefix + task + protocol, got %q", out)
	}
	if !(iPrefix < iDesc && iDesc < iProtocol) {
		t.Fatalf("order must be prefix → task → protocol, got %q", out)
	}
}

// 非 DAG 团队（无 DagNodeID）→ 不追加协议，任务描述原样返回。
func TestBuildTeamTurnInput_NonDAGTeam_NoProtocol(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	team := Team{SpiritSessionID: "sp1", TaskDescription: "临时任务"}
	out := u.BuildTeamTurnInput(context.Background(), team)

	if out != "临时任务" {
		t.Fatalf("non-DAG team input must stay untouched, got %q", out)
	}
}

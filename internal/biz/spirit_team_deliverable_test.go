package biz

import (
	"context"
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
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	// Content longer than MaxSummaryLen (500 runes) so the summary truncates.
	longContent := strings.Repeat("长", 600) + "\n- 关键发现一\n- 关键发现二"
	seedCompletedTeamWithSteps(teams, sessions, steps, "t1", "sp1", "st_1", "分析团队", longContent)

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
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	seedCompletedTeamWithSteps(teams, sessions, steps, "t1", "sp1", "st_1", "分析团队", "短成果")
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
	u := newDeliverableUsecase(teams, sessions)

	team := seedCompletedTeam(teams, sessions, "t1", "sp1", "st_1", "分析团队", "新成果")
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
	u := newDeliverableUsecase(teams, sessions)

	team := seedCompletedTeam(teams, sessions, "t1", "sp1", "st_1", "分析团队", "新成果")
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
	u := newDeliverableUsecase(teams, sessions)
	team := seedCompletedTeam(teams, sessions, "t1", "sp1", "st_1", "分析团队", "分析成果")

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

// Fallback path (cache missing): long extracted content is also truncated, so
// the guidance must be appended with the upstream team's ID.
func TestInjectUpstreamDeliverables_FallbackLongContent_AppendsGuidance(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	u := newDeliverableUsecaseWithSteps(teams, sessions, steps)

	longContent := strings.Repeat("详", 800)
	seedCompletedTeamWithSteps(teams, sessions, steps, "t-up", "sp1", "st_1", "上游团队", longContent)

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if !strings.Contains(prefix, `read_upstream_deliverable(team_id="t-up")`) {
		t.Fatalf("fallback extraction with truncated content should append guidance, got %q", prefix)
	}
}

// Fallback path with short content: no guidance (nothing was truncated).
func TestInjectUpstreamDeliverables_FallbackShortContent_NoGuidance(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "上游团队", "即时提取的成果")

	downstream := Team{SpiritSessionID: "sp1", DependsOn: []string{"st_1"}}
	prefix := u.InjectUpstreamDeliverables(context.Background(), downstream)
	if strings.Contains(prefix, "read_upstream_deliverable") {
		t.Fatalf("short fallback content must not append guidance, got %q", prefix)
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

	out, err := u.ReadUpstreamDeliverable(context.Background(), "t-up", 0)
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

	out, err := u.ReadUpstreamDeliverable(context.Background(), "t-up", 100)
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

	out, err := u.ReadUpstreamDeliverable(context.Background(), "t-up", MaxUpstreamDeliverableChars*2)
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

	if _, err := u.ReadUpstreamDeliverable(context.Background(), "t-run", 0); err == nil {
		t.Fatalf("running team must reject full-text reads")
	}
}

func TestReadUpstreamDeliverable_NoContent_Error(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)
	seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "团队", "")

	if _, err := u.ReadUpstreamDeliverable(context.Background(), "t-up", 0); err == nil {
		t.Fatalf("completed team without deliverable content must error")
	}
}

func TestReadUpstreamDeliverable_EmptyTeamID_Error(t *testing.T) {
	u := newDeliverableUsecase(newDeliverableTeamRepo(), newDeliverableSessionAccessor())
	if _, err := u.ReadUpstreamDeliverable(context.Background(), "  ", 0); err == nil {
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

	upstream := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "调研团队", "调研摘要")
	upstream.Deliverables = `[{"name":"research_report","type":"document","format":"markdown","description":"调研结论报告"}]`
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

	upstream := seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "数据团队", "数据摘要")
	upstream.Deliverables = `[{"name":"dataset","type":"data","format":"json"}]`
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
	seedCompletedTeam(teams, sessions, "t-up", "sp1", "st_1", "上游团队", "成果")

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

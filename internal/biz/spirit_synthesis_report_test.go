package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// Execution Report (B.10.17) tests — SynthesisOutput overview / deliverables /
// per-unit stats enrichment / degraded path, and CheckAllTeamsCompleted
// cancelled counting.
// ---------------------------------------------------------------------------

// stubSynthesisPublisher records PublishSynthesisCompleted calls.
type stubSynthesisPublisher struct {
	calls   int
	lastOut *SynthesisOutput
}

func (s *stubSynthesisPublisher) PublishSynthesisCompleted(_ context.Context, _ string, out *SynthesisOutput) {
	s.calls++
	s.lastOut = out
}

// stubTeamRunStats implements SpiritTeamRunStatsReader.
type stubTeamRunStats struct {
	stats map[string]SpiritTeamRunStats
	err   error
}

func (s *stubTeamRunStats) ListLatestRunStatsByTeams(_ context.Context, _ []string) (map[string]SpiritTeamRunStats, error) {
	return s.stats, s.err
}

func newReportUsecase(t *testing.T, teams *deliverableTeamRepo, sessions *deliverableSessionAccessor, engine *SynthesisEngine, pub SynthesisEventPublisher, opts ...SpiritTeamUsecaseOption) *SynthesisUsecase {
	t.Helper()
	t.Setenv("ARANEA_ENV", "dev")
	spiritUC := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop(), opts...)
	return NewSynthesisUsecase(spiritUC, engine, pub, loggateway.NewNoop())
}

func TestSynthesizeResults_AttachesOverviewAndDeliverables(t *testing.T) {
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{
		ID: "t1", DisplayName: "调研团队", TaskDescription: "任务A", Status: TeamStatusCompleted,
		SpiritSessionID: "spirit-1",
		CreatedAt:       "2026-07-22T10:00:00Z", UpdatedAt: "2026-07-22T10:00:08Z",
		DeliverablesOutput: `{"st_1": {"summary": "调研结论", "size_chars": 500, "team_id": "t1"}}`,
	}
	teams.items["t2"] = Team{
		ID: "t2", DisplayName: "分析团队", TaskDescription: "任务B", Status: TeamStatusCompleted,
		SpiritSessionID: "spirit-1",
		CreatedAt:       "2026-07-22T10:00:01Z", UpdatedAt: "2026-07-22T10:00:12Z",
		DeliverablesOutput: `{"st_2": "旧格式摘要"}`,
	}
	sessions := newDeliverableSessionAccessor()
	sessions.messages["spirit-1"] = []ChatMessage{{Role: "user", ContentMarkdown: "分析销售数据"}}
	sessions.children = []Session{
		{ID: "s1", TeamID: "t1", ParentSessionID: "spirit-1", InputTokens: 100, OutputTokens: 200},
		{ID: "s2", TeamID: "t2", ParentSessionID: "spirit-1", InputTokens: 300, OutputTokens: 400},
	}
	pub := &stubSynthesisPublisher{}
	uc := newReportUsecase(t, teams, sessions, NewSynthesisEngine(nil, loggateway.NewNoop()), pub)

	out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyTemplate))
	if err != nil {
		t.Fatalf("SynthesizeResults returned error: %v", err)
	}
	if out.Overview == nil {
		t.Fatal("expected Overview to be attached")
	}
	ov := out.Overview
	if ov.FinalStatus != "completed" {
		t.Fatalf("FinalStatus = %q, want completed", ov.FinalStatus)
	}
	if ov.TotalUnits != 2 || ov.CompletedUnits != 2 || ov.FailedUnits != 0 {
		t.Fatalf("unit counts = %d/%d/%d, want 2/2/0", ov.TotalUnits, ov.CompletedUnits, ov.FailedUnits)
	}
	if ov.DurationMs != 12000 {
		t.Fatalf("DurationMs = %d, want 12000 (max UpdatedAt - min CreatedAt)", ov.DurationMs)
	}
	if ov.TokenIn != 400 || ov.TokenOut != 600 {
		t.Fatalf("tokens = %d/%d, want 400/600", ov.TokenIn, ov.TokenOut)
	}
	if ov.Query != "分析销售数据" {
		t.Fatalf("Query = %q, want 分析销售数据", ov.Query)
	}
	if len(out.Deliverables) != 2 {
		t.Fatalf("deliverables = %d, want 2", len(out.Deliverables))
	}
	byNode := map[string]DeliverableItem{}
	for _, d := range out.Deliverables {
		byNode[d.NodeID] = d
	}
	d1, ok := byNode["st_1"]
	if !ok || d1.UnitName != "调研团队" || d1.Summary != "调研结论" || d1.SizeChars != 500 {
		t.Fatalf("envelope deliverable mismatch: %+v", d1)
	}
	d2, ok := byNode["st_2"]
	if !ok || d2.UnitName != "分析团队" || d2.Summary != "旧格式摘要" || d2.SizeChars != 5 {
		t.Fatalf("legacy deliverable mismatch (size should be rune count of summary): %+v", d2)
	}
	if pub.calls != 1 || pub.lastOut != out {
		t.Fatalf("publisher calls = %d, want 1 with the same output", pub.calls)
	}
}

func TestSynthesizeResults_OverviewStatusDerivation(t *testing.T) {
	cases := []struct {
		name     string
		statuses []string
		want     string
	}{
		{"all_completed", []string{TeamStatusCompleted, TeamStatusCompleted}, "completed"},
		{"all_failed", []string{TeamStatusFailed, TeamStatusFailed}, "failed"},
		{"mixed", []string{TeamStatusCompleted, TeamStatusFailed}, "partial_failure"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			teams := newDeliverableTeamRepo()
			for i, st := range tc.statuses {
				id := string(rune('a'+i)) + "-team"
				teams.items[id] = Team{ID: id, DisplayName: id, Status: st, SpiritSessionID: "spirit-1"}
			}
			sessions := newDeliverableSessionAccessor()
			uc := newReportUsecase(t, teams, sessions, NewSynthesisEngine(nil, loggateway.NewNoop()), nil)

			out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyTemplate))
			if err != nil {
				t.Fatalf("SynthesizeResults returned error: %v", err)
			}
			if out.Overview == nil || out.Overview.FinalStatus != tc.want {
				t.Fatalf("FinalStatus = %+v, want %q", out.Overview, tc.want)
			}
		})
	}
}

func TestSynthesizeResults_EnrichesTeamRunStats(t *testing.T) {
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", DisplayName: "团队A", Status: TeamStatusCompleted, SpiritSessionID: "spirit-1"}
	teams.items["t2"] = Team{ID: "t2", DisplayName: "团队B", Status: TeamStatusFailed, SpiritSessionID: "spirit-1"}
	sessions := newDeliverableSessionAccessor()
	stats := &stubTeamRunStats{stats: map[string]SpiritTeamRunStats{
		"t1": {TeamID: "t1", DurationMs: 8000},
		"t2": {TeamID: "t2", DurationMs: 5000, ErrorMessage: "timeout"},
	}}
	uc := newReportUsecase(t, teams, sessions, NewSynthesisEngine(nil, loggateway.NewNoop()), nil, WithSpiritTeamRunStatsReader(stats))

	out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyTemplate))
	if err != nil {
		t.Fatalf("SynthesizeResults returned error: %v", err)
	}
	byID := map[string]TeamSynthesisResult{}
	for _, r := range out.TeamResults {
		byID[r.TeamID] = r
	}
	if r := byID["t1"]; r.DurationMs != 8000 || r.ErrorMessage != "" {
		t.Fatalf("t1 stats = %d/%q, want 8000/\"\"", r.DurationMs, r.ErrorMessage)
	}
	if r := byID["t2"]; r.DurationMs != 5000 || r.ErrorMessage != "timeout" {
		t.Fatalf("t2 stats = %d/%q, want 5000/timeout", r.DurationMs, r.ErrorMessage)
	}
}

func TestSynthesizeResults_NilRunStatsReader_OmitsStats(t *testing.T) {
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", DisplayName: "团队A", Status: TeamStatusCompleted, SpiritSessionID: "spirit-1"}
	sessions := newDeliverableSessionAccessor()
	uc := newReportUsecase(t, teams, sessions, NewSynthesisEngine(nil, loggateway.NewNoop()), nil)

	out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyTemplate))
	if err != nil {
		t.Fatalf("SynthesizeResults returned error: %v", err)
	}
	if len(out.TeamResults) != 1 || out.TeamResults[0].DurationMs != 0 || out.TeamResults[0].ErrorMessage != "" {
		t.Fatalf("stats should be omitted when reader is nil: %+v", out.TeamResults)
	}
}

func TestSynthesizeResults_DegradedOnEngineError(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", DisplayName: "团队A", Status: TeamStatusCompleted, SpiritSessionID: "spirit-1"}
	sessions := newDeliverableSessionAccessor()
	pub := &stubSynthesisPublisher{}
	stub := &stubSynthesisModel{err: errors.New("LLM unavailable")}
	spiritUC := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop())
	uc := NewSynthesisUsecase(spiritUC, NewSynthesisEngine(stub, loggateway.NewNoop()), pub, loggateway.NewNoop())

	out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyPrompt))
	if err == nil {
		t.Fatal("expected error when model fails in production")
	}
	if !errors.Is(err, ErrSynthesisModelFailed) {
		t.Fatalf("want ErrSynthesisModelFailed, got %v", err)
	}
	if out == nil {
		t.Fatal("degraded output should be returned alongside the error")
	}
	if !out.Degraded {
		t.Fatal("degraded output should have Degraded=true")
	}
	if out.Content != "" {
		t.Fatalf("degraded output should have empty Content, got %q", out.Content)
	}
	if out.Overview == nil {
		t.Fatal("degraded output should still carry Overview")
	}
	if pub.calls != 1 || pub.lastOut == nil || !pub.lastOut.Degraded {
		t.Fatalf("publisher should publish the degraded report once, calls=%d", pub.calls)
	}
}

func TestCheckAllTeamsCompleted_CountsCancelled(t *testing.T) {
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", Status: TeamStatusCompleted, SpiritSessionID: "spirit-1"}
	teams.items["t2"] = Team{ID: "t2", Status: TeamStatusCancelled, SpiritSessionID: "spirit-1"}
	sessions := newDeliverableSessionAccessor()
	uc := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop())

	res := uc.CheckAllTeamsCompleted(context.Background(), "spirit-1")
	if !res.AllDone {
		t.Fatal("AllDone should be true when all teams are terminal")
	}
	if res.CancelledTeams != 1 {
		t.Fatalf("CancelledTeams = %d, want 1", res.CancelledTeams)
	}
	if res.FailedTeams != 1 {
		t.Fatalf("FailedTeams = %d, want 1 (cancelled still counted as failed)", res.FailedTeams)
	}
	if res.CompletedTeams != 1 {
		t.Fatalf("CompletedTeams = %d, want 1", res.CompletedTeams)
	}
}

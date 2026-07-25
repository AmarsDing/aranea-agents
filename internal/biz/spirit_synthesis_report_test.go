package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// Synthesis usecase tests — per-unit run stats enrichment, engine error
// propagation, and CheckAllTeamsCompleted cancelled counting.
// ---------------------------------------------------------------------------

// stubTeamRunStats implements SpiritTeamRunStatsReader.
type stubTeamRunStats struct {
	stats map[string]SpiritTeamRunStats
	err   error
}

func (s *stubTeamRunStats) ListLatestRunStatsByTeams(_ context.Context, _ []string) (map[string]SpiritTeamRunStats, error) {
	return s.stats, s.err
}

func newReportUsecase(t *testing.T, teams *deliverableTeamRepo, sessions *deliverableSessionAccessor, engine *SynthesisEngine, opts ...SpiritTeamUsecaseOption) *SynthesisUsecase {
	t.Helper()
	t.Setenv("ARANEA_ENV", "dev")
	spiritUC := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop(), opts...)
	return NewSynthesisUsecase(spiritUC, engine, loggateway.NewNoop())
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
	uc := newReportUsecase(t, teams, sessions, NewSynthesisEngine(nil, loggateway.NewNoop()), WithSpiritTeamRunStatsReader(stats))

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
	uc := newReportUsecase(t, teams, sessions, NewSynthesisEngine(nil, loggateway.NewNoop()))

	out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyTemplate))
	if err != nil {
		t.Fatalf("SynthesizeResults returned error: %v", err)
	}
	if len(out.TeamResults) != 1 || out.TeamResults[0].DurationMs != 0 || out.TeamResults[0].ErrorMessage != "" {
		t.Fatalf("stats should be omitted when reader is nil: %+v", out.TeamResults)
	}
}

// Engine failure propagates as (nil, err) — the usecase no longer builds a
// degraded report (the dedicated report UI was removed; the user-facing
// summary is produced by the Spirit summary turn).
func TestSynthesizeResults_EngineErrorReturnsNilOutput(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", DisplayName: "团队A", Status: TeamStatusCompleted, SpiritSessionID: "spirit-1"}
	sessions := newDeliverableSessionAccessor()
	stub := &stubSynthesisModel{err: errors.New("LLM unavailable")}
	spiritUC := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop())
	uc := NewSynthesisUsecase(spiritUC, NewSynthesisEngine(stub, loggateway.NewNoop()), loggateway.NewNoop())

	out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyPrompt))
	if err == nil {
		t.Fatal("expected error when model fails in production")
	}
	if !errors.Is(err, ErrSynthesisModelFailed) {
		t.Fatalf("want ErrSynthesisModelFailed, got %v", err)
	}
	if out != nil {
		t.Fatalf("output should be nil on engine error, got %+v", out)
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

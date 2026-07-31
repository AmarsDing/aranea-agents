package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// B10 运行时惰性兜底：Runner 加载 team 时经 TeamGraphAssetEnsurer 端口
// 确保图资产已物化；未装配端口时回退直接读 TeamReader。

type stubTeamReader struct {
	biz.TeamReader
	team  biz.Team
	calls int
}

func (s *stubTeamReader) GetTeamByID(_ context.Context, _ string) (biz.Team, error) {
	s.calls++
	return s.team, nil
}

type stubGraphEnsurer struct {
	team  biz.Team
	calls int
	gotID string
}

func (s *stubGraphEnsurer) EnsureTeamGraphAsset(_ context.Context, teamID string) (biz.Team, error) {
	s.calls++
	s.gotID = teamID
	return s.team, nil
}

func TestLoadTeamForRun_UsesEnsurerWhenConfigured(t *testing.T) {
	reader := &stubTeamReader{team: biz.Team{ID: "t1"}}
	ensurer := &stubGraphEnsurer{team: biz.Team{ID: "t1", LinkedGraphID: "g-1"}}
	r := &Runner{teamReader: reader, cfg: RunnerConfig{GraphEnsurer: ensurer}}

	got, err := r.loadTeamForRun(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if ensurer.calls != 1 || ensurer.gotID != "t1" {
		t.Fatalf("ensurer calls=%d gotID=%q, want 1/t1", ensurer.calls, ensurer.gotID)
	}
	if reader.calls != 0 {
		t.Fatalf("reader must not be called when ensurer configured, got %d", reader.calls)
	}
	if got.LinkedGraphID != "g-1" {
		t.Fatalf("team=%+v, want materialized linked graph", got)
	}
}

func TestLoadTeamForRun_FallsBackToReader(t *testing.T) {
	reader := &stubTeamReader{team: biz.Team{ID: "t1"}}
	r := &Runner{teamReader: reader}

	got, err := r.loadTeamForRun(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || got.ID != "t1" {
		t.Fatalf("reader calls=%d team=%+v, want 1/t1", reader.calls, got)
	}
}

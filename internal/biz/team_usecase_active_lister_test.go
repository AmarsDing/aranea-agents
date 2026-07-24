package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type stubActiveRunLister struct {
	seen []string
	ids  map[string]bool
	err  error
}

func (s *stubActiveRunLister) ListActiveRunTeamIDs(_ context.Context, teamIDs []string) (map[string]bool, error) {
	s.seen = append([]string(nil), teamIDs...)
	return s.ids, s.err
}

// activeRunReaderStub 覆盖 HasActiveTeamRun 以支持回退路径测试。
type activeRunReaderStub struct {
	stubTeamRunReader
	active map[string]bool
}

func (s *activeRunReaderStub) HasActiveTeamRun(_ context.Context, teamID string) (bool, error) {
	return s.active[teamID], nil
}

func TestListActiveRunTeamIDs_DelegatesToLister(t *testing.T) {
	lister := &stubActiveRunLister{ids: map[string]bool{"t1": true}}
	uc := NewTeamUsecase(TeamUsecaseOpts{ActiveLister: lister, Lg: loggateway.NewNoop()})
	got, err := uc.ListActiveRunTeamIDs(context.Background(), []string{"t1", "t2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got["t1"] || got["t2"] {
		t.Fatalf("unexpected result: %v", got)
	}
	if len(lister.seen) != 2 || lister.seen[0] != "t1" || lister.seen[1] != "t2" {
		t.Fatalf("lister saw %v", lister.seen)
	}
}

func TestListActiveRunTeamIDs_EmptyInputSkipsLister(t *testing.T) {
	lister := &stubActiveRunLister{ids: map[string]bool{"t1": true}}
	uc := NewTeamUsecase(TeamUsecaseOpts{ActiveLister: lister, Lg: loggateway.NewNoop()})
	got, err := uc.ListActiveRunTeamIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
	if lister.seen != nil {
		t.Fatalf("empty input must not reach the repo, saw %v", lister.seen)
	}
}

func TestListActiveRunTeamIDs_FallbackWhenListerNil(t *testing.T) {
	reader := &activeRunReaderStub{active: map[string]bool{"t2": true}}
	uc := NewTeamUsecase(TeamUsecaseOpts{RunReader: reader, Lg: loggateway.NewNoop()})
	got, err := uc.ListActiveRunTeamIDs(context.Background(), []string{"t1", "t2", "t3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got["t2"] || got["t1"] || got["t3"] {
		t.Fatalf("unexpected fallback result: %v", got)
	}
}

package jobs

import (
	"context"
	"errors"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type stubKnowledgeCurator struct {
	calls int
	opts  bizknowledge.CurateOptions
	reps  []bizknowledge.CurateReport
	err   error
}

func (s *stubKnowledgeCurator) CurateAllTeamKnowledge(_ context.Context, opts bizknowledge.CurateOptions) ([]bizknowledge.CurateReport, error) {
	s.calls++
	s.opts = opts
	return s.reps, s.err
}

func TestKnowledgeCurateWorker_RunOnceAppliesNotDryRun(t *testing.T) {
	cur := &stubKnowledgeCurator{reps: []bizknowledge.CurateReport{
		{DecayedEdges: 3, ProposalsPending: 1},
	}}
	w := NewKnowledgeCurateWorker(0, cur, loggateway.NewNoop())
	w.RunOnce(context.Background())
	if cur.calls != 1 {
		t.Fatalf("calls=%d", cur.calls)
	}
	if cur.opts.DryRun {
		t.Fatal("scheduled curate must apply (DryRun=false); high-risk stays pending inside CurateKnowledge")
	}
	if w.interval != KnowledgeCurateDefaultInterval {
		t.Fatalf("interval=%s", w.interval)
	}
}

func TestKnowledgeCurateWorker_NoTeamCollectionsIsQuiet(t *testing.T) {
	cur := &stubKnowledgeCurator{err: apierror.NotFound(apierror.DomainKnowledge, "no team knowledge collection to curate")}
	w := NewKnowledgeCurateWorker(0, cur, loggateway.NewNoop())
	w.RunOnce(context.Background())
	if cur.calls != 1 {
		t.Fatalf("calls=%d", cur.calls)
	}
}

func TestKnowledgeCurateWorker_GracefulFailure(t *testing.T) {
	cur := &stubKnowledgeCurator{err: errors.New("db down")}
	w := NewKnowledgeCurateWorker(0, cur, loggateway.NewNoop())
	w.RunOnce(context.Background())
	if cur.calls != 1 {
		t.Fatalf("calls=%d", cur.calls)
	}
}

func TestKnowledgeCurateWorker_NilDependency(t *testing.T) {
	if w := NewKnowledgeCurateWorker(0, nil, loggateway.NewNoop()); w != nil {
		t.Fatal("nil curator must disable worker")
	}
}

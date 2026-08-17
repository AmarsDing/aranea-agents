package jobs

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type fakeKnowledgeIndexRepairer struct {
	calls  int
	limit  int
	err    error
	fixed  int
	failed int
}

func (f *fakeKnowledgeIndexRepairer) RepairPendingKnowledgeIndexes(_ context.Context, limit int) (int, int, error) {
	f.calls++
	f.limit = limit
	return f.fixed, f.failed, f.err
}

func TestKnowledgeIndexRepairWorker_RunOnce(t *testing.T) {
	repairer := &fakeKnowledgeIndexRepairer{fixed: 2}
	w := NewKnowledgeIndexRepairWorker(0, repairer, loggateway.NewNoop())
	w.RunOnce(context.Background())
	if repairer.calls != 1 || repairer.limit != knowledgeIndexRepairMaxPerPass {
		t.Fatalf("calls=%d limit=%d", repairer.calls, repairer.limit)
	}
}

func TestKnowledgeIndexRepairWorker_GracefulFailure(t *testing.T) {
	repairer := &fakeKnowledgeIndexRepairer{err: errors.New("db down")}
	w := NewKnowledgeIndexRepairWorker(0, repairer, loggateway.NewNoop())
	w.RunOnce(context.Background())
	if repairer.calls != 1 {
		t.Fatalf("calls=%d, want 1", repairer.calls)
	}
}

func TestKnowledgeIndexRepairWorker_NilDependency(t *testing.T) {
	if w := NewKnowledgeIndexRepairWorker(0, nil, loggateway.NewNoop()); w != nil {
		t.Fatal("nil repairer must disable worker")
	}
}

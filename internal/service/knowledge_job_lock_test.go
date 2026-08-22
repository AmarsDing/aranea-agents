package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func TestMemKnowledgeJobLocker_Exclusive(t *testing.T) {
	l := NewMemKnowledgeJobLocker()
	rel, ok, err := l.TryAcquire(context.Background(), "knowledge-rebuild:c1")
	if err != nil || !ok {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}
	_, ok2, err2 := l.TryAcquire(context.Background(), "knowledge-rebuild:c1")
	if err2 != nil {
		t.Fatalf("second acquire err: %v", err2)
	}
	if ok2 {
		t.Fatal("expected second acquire to fail")
	}
	rel()
	rel3, ok3, err3 := l.TryAcquire(context.Background(), "knowledge-rebuild:c1")
	if err3 != nil || !ok3 {
		t.Fatalf("after release: ok=%v err=%v", ok3, err3)
	}
	rel3()
}

func TestAcquireJob_LocalAndDistributed(t *testing.T) {
	svc := NewKnowledgeService(nil, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	svc.SetJobLocker(NewMemKnowledgeJobLocker())
	rel, err := svc.acquireJob(context.Background(), &svc.rebuildRuns, "c1", knowledgeRebuildJobKey("c1"),
		"already running for %s", "c1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.acquireJob(context.Background(), &svc.rebuildRuns, "c1", knowledgeRebuildJobKey("c1"),
		"already running for %s", "c1")
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("want Conflict, got %v", err)
	}
	rel()
}

func TestNewKnowledgeService_BindsWriteBackReplay(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	if uc.HasWriteBackReplay() {
		t.Fatal("usecase should start unbound")
	}
	_ = NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	if !uc.HasWriteBackReplay() {
		t.Fatal("NewKnowledgeService must bind writeBackReplay")
	}
	svc := &KnowledgeService{uc: uc, lg: loggateway.NewNoop()}
	uc.SetWriteBackReplay(nil)
	svc.BindDerivedIndexHooks()
	if !uc.HasWriteBackReplay() {
		t.Fatal("BindDerivedIndexHooks must rebind writeBackReplay")
	}
}

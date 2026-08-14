package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/pkg/loggateway"
)

type fakeHealRecordRepo struct {
	deleted int
	err     error
}

func (f *fakeHealRecordRepo) InsertHealRecord(_ context.Context, _ heal.HealRecord) error {
	panic("unexpected call")
}

func (f *fakeHealRecordRepo) ListHealRecords(_ context.Context, _ heal.HealRecordQuery) (heal.HealRecordListResult, error) {
	panic("unexpected call")
}

func (f *fakeHealRecordRepo) DeleteHealRecordsOlderThan(_ context.Context, _ time.Time) (int, error) {
	return f.deleted, f.err
}

func TestAutoHealTTLCleanup_RunOnce_EmitsFlowLogOnError(t *testing.T) {
	flowLog := &canaryFakeFlowLog{}
	repo := &fakeHealRecordRepo{err: errors.New("delete boom")}
	c := NewAutoHealTTLCleanup(time.Hour, time.Hour, repo, loggateway.NewNoop(), flowLog)

	c.runOnce(context.Background())

	if len(flowLog.errors) != 1 {
		t.Fatalf("flow errors = %v, want 1 entry", flowLog.errors)
	}
	want := "system.auto_heal_ttl_cleanup.failed"
	if got := flowLog.errors[0]; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("flow error stepID = %q, want prefix %q", got, want)
	}
}

func TestAutoHealTTLCleanup_RunOnce_NoFlowLogOnSuccess(t *testing.T) {
	flowLog := &canaryFakeFlowLog{}
	repo := &fakeHealRecordRepo{deleted: 3}
	c := NewAutoHealTTLCleanup(time.Hour, time.Hour, repo, loggateway.NewNoop(), flowLog)

	c.runOnce(context.Background())

	if len(flowLog.errors) != 0 {
		t.Fatalf("flow errors = %v, want none", flowLog.errors)
	}
}

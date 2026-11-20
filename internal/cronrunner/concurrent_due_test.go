package cronrunner

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// blockingSyncAgent 第一次调用卡住直到 release 关闭，模拟一个在重试退避里的慢任务。
type blockingSyncAgent struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingSyncAgent) RunSync(context.Context) error {
	b.started <- struct{}{}
	<-b.release
	return nil
}

// 一个到期任务卡住时：同一 tick 里另一个任务照样算出下次时间，下一 tick 也不会把卡住的任务再触发一遍。
func TestRunDue_SlowTaskDoesNotBlockOthers(t *testing.T) {
	repo := newMissedFakeRepo()
	now := time.Now().UTC()
	repo.tasks["slow"] = biz.CronTask{
		ID: "slow", TaskKey: "slow", Name: "slow", Status: "active", Enabled: true,
		ConfigJSON:   `{"target_type":"model_registry_sync","message":"sync","schedule_type":"interval","interval_seconds":60}`,
		MetadataJSON: missedTaskMeta(t, cronTaskMetadata{NextRunAt: now.Add(-time.Second).Format(time.RFC3339)}),
	}
	repo.tasks["other"] = biz.CronTask{
		ID: "other", TaskKey: "other", Name: "other", Status: "active", Enabled: true,
		ConfigJSON: `{"target_type":"model_registry_sync","message":"sync","schedule_type":"interval","interval_seconds":3600}`,
		UpdatedAt:  now.Add(-time.Minute).Format(time.RFC3339),
	}
	agent := &blockingSyncAgent{started: make(chan struct{}, 4), release: make(chan struct{})}
	r := NewRunner(Deps{Cron: repo, RegistrySyncAgent: agent}, loggateway.NewNoop()).WithTickInterval(time.Minute)

	done := make(chan struct{})
	go func() { r.runDue(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runDue blocked on the slow task")
	}
	<-agent.started
	if meta := parseCronTaskMetadata(repo.tasks["other"].MetadataJSON, loggateway.NewNoop()); meta.NextRunAt == "" {
		t.Fatal("other task's next_run_at not computed while slow task runs")
	}
	r.runDue(context.Background()) // 慢任务还在跑：不能再触发一次
	select {
	case <-agent.started:
		t.Fatal("slow task dispatched twice")
	case <-time.After(200 * time.Millisecond):
	}
	close(agent.release)
	r.waitRunning()
}

// 某个任务正被手动执行（持有任务锁、在重试退避里）时，tick 不能等它的锁：其它任务照常算下次时间。
func TestRunDue_DoesNotWaitOnBusyTaskLock(t *testing.T) {
	repo := newMissedFakeRepo()
	now := time.Now().UTC()
	for _, id := range []string{"busy", "other"} {
		repo.tasks[id] = biz.CronTask{
			ID: id, TaskKey: id, Name: id, Status: "active", Enabled: true,
			ConfigJSON: `{"target_type":"model_registry_sync","message":"sync","schedule_type":"interval","interval_seconds":3600}`,
			UpdatedAt:  now.Add(-time.Minute).Format(time.RFC3339),
		}
	}
	r := NewRunner(Deps{Cron: repo}, loggateway.NewNoop()).WithTickInterval(time.Minute)
	unlock := r.lockTask("busy") // 模拟手动执行占着锁
	defer unlock()

	done := make(chan struct{})
	go func() { r.runDue(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runDue waited on a busy task lock")
	}
	if meta := parseCronTaskMetadata(repo.tasks["other"].MetadataJSON, loggateway.NewNoop()); meta.NextRunAt == "" {
		t.Fatal("other task's next_run_at not computed")
	}
}

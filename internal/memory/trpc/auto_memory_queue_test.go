package trpcmem

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"
)

func TestNewMemoryJobQueue_DefaultDebounce(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	if q.debounce != 30*time.Second {
		t.Fatalf("expected 30s default debounce, got %v", q.debounce)
	}
}

func TestNewMemoryJobQueue_CustomDebounce(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 5*time.Second, loggateway.NewNoop())
	defer q.Close()
	if q.debounce != 5*time.Second {
		t.Fatalf("expected 5s debounce, got %v", q.debounce)
	}
}

func TestMemoryJobQueue_EnqueueHighPriority(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	q.Enqueue(AutoMemoryJobRequest{
		AppName:    "app1",
		SessionID:  "sess1",
		Priority:   MemoryJobPriorityHigh,
		EnqueuedAt: time.Now(),
	})
	select {
	case r := <-q.Chan():
		if r.AppName != "app1" || r.Priority != MemoryJobPriorityHigh {
			t.Fatalf("unexpected: %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for high priority job")
	}
}

func TestMemoryJobQueue_EnqueueLowPriority(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	q.Enqueue(AutoMemoryJobRequest{
		AppName:    "app2",
		SessionID:  "sess2",
		Priority:   MemoryJobPriorityLow,
		EnqueuedAt: time.Now(),
	})
	select {
	case r := <-q.Chan():
		if r.Priority != MemoryJobPriorityLow {
			t.Fatalf("expected low priority, got %v", r.Priority)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for low priority job")
	}
}

func TestMemoryJobQueue_EnqueueNormalPriority(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	q.Enqueue(AutoMemoryJobRequest{
		AppName:    "app3",
		SessionID:  "sess3",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	})
	select {
	case r := <-q.Chan():
		if r.Priority != MemoryJobPriorityNormal {
			t.Fatalf("expected normal priority, got %v", r.Priority)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for normal priority job")
	}
}

func TestMemoryJobQueue_DebounceNormal(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 10*time.Second, loggateway.NewNoop())
	defer q.Close()
	q.Enqueue(AutoMemoryJobRequest{
		AppName:    "app",
		SessionID:  "sess-dedup",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	})
	<-q.Chan()
	q.Enqueue(AutoMemoryJobRequest{
		AppName:    "app",
		SessionID:  "sess-dedup",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	})
	_, debounced := q.Stats()
	if debounced == 0 {
		t.Fatal("expected debounced count > 0")
	}
}

func TestMemoryJobQueue_AckDone(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	r := AutoMemoryJobRequest{
		AppName:    "app",
		SessionID:  "sess-ack",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	}
	q.Enqueue(r)
	<-q.Chan()
	q.AckDone(r)
	q.mu.Lock()
	count := q.tenantInFlight[q.tenantID(r)]
	q.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 in-flight, got %d", count)
	}
}

func TestMemoryJobQueue_AckDone_NonNormal(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	r := AutoMemoryJobRequest{Priority: MemoryJobPriorityHigh}
	q.AckDone(r)
}

func TestMemoryJobQueue_StatsNil(t *testing.T) {
	var q *MemoryJobQueue
	d, db := q.Stats()
	if d != 0 || db != 0 {
		t.Fatal("expected zero stats for nil queue")
	}
}

func TestMemoryJobQueue_QueueStatsNil(t *testing.T) {
	var q *MemoryJobQueue
	s := q.QueueStats()
	if s != (MemoryQueueStats{}) {
		t.Fatalf("expected zero stats, got %+v", s)
	}
}

func TestMemoryJobQueue_ChanNil(t *testing.T) {
	var q *MemoryJobQueue
	if q.Chan() != nil {
		t.Fatal("expected nil channel")
	}
}

func TestMemoryJobQueue_CloseNil(t *testing.T) {
	var q *MemoryJobQueue
	q.Close()
}

func TestMemoryJobQueue_EnqueueNil(t *testing.T) {
	var q *MemoryJobQueue
	q.Enqueue(AutoMemoryJobRequest{})
}

func TestMemoryJobQueue_SetDeadLetterSinkNil(t *testing.T) {
	var q *MemoryJobQueue
	q.SetDeadLetterSink(nil)
}

func TestTenantID(t *testing.T) {
	q := &MemoryJobQueue{}
	if got := q.tenantID(AutoMemoryJobRequest{}); got != "default" {
		t.Fatalf("expected default, got %q", got)
	}
	if got := q.tenantID(AutoMemoryJobRequest{AppName: "app1"}); got != "app1" {
		t.Fatalf("expected app1, got %q", got)
	}
	if got := q.tenantID(AutoMemoryJobRequest{TenantID: "t1", AppName: "app1"}); got != "t1" {
		t.Fatalf("expected t1, got %q", got)
	}
	if got := q.tenantID(AutoMemoryJobRequest{TenantID: "  "}); got != "default" {
		t.Fatalf("expected default, got %q", got)
	}
}

func TestNewAutoMemoryEnqueuer(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	fn := NewAutoMemoryEnqueuer(q)
	fn("app1", "sess1", time.Now())
	select {
	case r := <-q.Chan():
		if r.Priority != MemoryJobPriorityNormal || r.AppName != "app1" {
			t.Fatalf("unexpected: %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestNewAutoMemoryEnqueuer_Nil(t *testing.T) {
	fn := NewAutoMemoryEnqueuer(nil)
	fn("app", "sess", time.Now())
}

func TestNewFeedbackMemoryEnqueuer(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	fn := NewFeedbackMemoryEnqueuer(q)
	fn("sess1", "msg1", "positive", "great", time.Now())
	select {
	case r := <-q.Chan():
		if r.Priority != MemoryJobPriorityHigh || r.FeedbackMessageID != "msg1" {
			t.Fatalf("unexpected: %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestNewFeedbackMemoryEnqueuer_Nil(t *testing.T) {
	fn := NewFeedbackMemoryEnqueuer(nil)
	fn("sess", "msg", "pos", "ok", time.Now())
}

func TestAssertL3WriteAllowed(t *testing.T) {
	t.Run("nil_loader_fail_closed", func(t *testing.T) {
		if err := assertL3WriteAllowed(context.Background(), nil, "a1"); err != ErrL3WriteDisabled {
			t.Fatalf("expected ErrL3WriteDisabled, got %v", err)
		}
	})
	t.Run("l3_disabled", func(t *testing.T) {
		loader := &mockSettingsLoader{settings: &biz.AgentRuntimeSettings{
			MemoryEnabled: true, L3Enabled: false,
		}}
		if err := assertL3WriteAllowed(context.Background(), loader, "a1"); err != ErrL3WriteDisabled {
			t.Fatalf("expected ErrL3WriteDisabled, got %v", err)
		}
	})
	t.Run("l3_enabled", func(t *testing.T) {
		loader := &mockSettingsLoader{settings: &biz.AgentRuntimeSettings{
			MemoryEnabled: true, L3Enabled: true,
		}}
		if err := assertL3WriteAllowed(context.Background(), loader, "a1"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestNewAgentRuntimeSettingsLoader_NilGetter(t *testing.T) {
	loader := NewAgentRuntimeSettingsLoader(nil)
	if loader != nil {
		t.Fatal("expected nil loader for nil getter")
	}
}

func TestNewAgentRuntimeSettingsLoader_NilLoader(t *testing.T) {
	var l *agentRuntimeSettingsLoader
	settings, err := l.GetAgentRuntimeSettings(context.Background(), "a1")
	if err != nil || settings != nil {
		t.Fatalf("expected nil,nil, got %v %v", settings, err)
	}
}

func TestResolveMemoryToolSearchLimits_DisabledWhenNoSettings(t *testing.T) {
	topK, minScore := resolveMemoryToolSearchLimits(context.Background(), nil, "", 0)
	if topK != 0 {
		t.Fatalf("expected topK=0 when no settings (memory disabled), got %d", topK)
	}
	if minScore != 1 {
		t.Fatalf("expected minScore=1 when no settings (memory disabled), got %f", minScore)
	}
}

func TestResolveMemoryToolSearchLimits_OptsMaxDisabledWhenNoSettings(t *testing.T) {
	topK, _ := resolveMemoryToolSearchLimits(context.Background(), nil, "", 5)
	if topK != 0 {
		t.Fatalf("expected topK=0 when no settings (optsMax cannot override disabled), got %d", topK)
	}
}

type mockSettingsLoader struct {
	settings *biz.AgentRuntimeSettings
}

func (m *mockSettingsLoader) GetAgentRuntimeSettings(_ context.Context, _ string) (*biz.AgentRuntimeSettings, error) {
	return m.settings, nil
}

func TestResolveMemoryToolSearchLimits_EnabledDefaults(t *testing.T) {
	loader := &mockSettingsLoader{settings: &biz.AgentRuntimeSettings{MemoryEnabled: true}}
	topK, minScore := resolveMemoryToolSearchLimits(context.Background(), loader, "a1", 0)
	if topK <= 0 {
		t.Fatalf("expected positive topK when enabled, got %d", topK)
	}
	if minScore < 0 || minScore > 1 {
		t.Fatalf("expected 0<=minScore<=1, got %f", minScore)
	}
}

func TestResolveMemoryToolSearchLimits_EnabledOptsMax(t *testing.T) {
	loader := &mockSettingsLoader{settings: &biz.AgentRuntimeSettings{MemoryEnabled: true}}
	topK, _ := resolveMemoryToolSearchLimits(context.Background(), loader, "a1", 5)
	if topK != 5 {
		t.Fatalf("expected topK=5, got %d", topK)
	}
}

func TestResolveMemoryToolSearchLimits_EnabledOptsMaxCannotExceedPolicy(t *testing.T) {
	loader := &mockSettingsLoader{settings: &biz.AgentRuntimeSettings{
		MemoryEnabled:    true,
		MemoryMaxResults: 3,
	}}
	topK, _ := resolveMemoryToolSearchLimits(context.Background(), loader, "a1", 10)
	if topK != 3 {
		t.Fatalf("expected topK=3 (policy cap), got %d", topK)
	}
}

func TestAutoMemoryJobRequest_EnqueuedAtDefault(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	r := AutoMemoryJobRequest{
		AppName:   "app",
		SessionID: "sess",
		Priority:  MemoryJobPriorityHigh,
	}
	q.Enqueue(r)
	select {
	case got := <-q.Chan():
		if got.EnqueuedAt.IsZero() {
			t.Fatal("expected EnqueuedAt to be set")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestMemoryJobQueue_QueueLaneStats(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 0, loggateway.NewNoop())
	defer q.Close()
	hl, nl, ll, hc, nc, lc, _, _ := q.QueueLaneStats()
	if hc != 64 || nc != 256 || lc != 128 {
		t.Fatalf("caps: high=%d normal=%d low=%d", hc, nc, lc)
	}
	if hl != 0 || nl != 0 || ll != 0 {
		t.Fatalf("lengths: high=%d normal=%d low=%d", hl, nl, ll)
	}
}

func TestMemoryJobPriorityConstants(t *testing.T) {
	if MemoryJobPriorityHigh != biz.MemoryJobPriorityHigh {
		t.Fatal("priority high mismatch")
	}
	if MemoryJobPriorityNormal != biz.MemoryJobPriorityNormal {
		t.Fatal("priority normal mismatch")
	}
	if MemoryJobPriorityLow != biz.MemoryJobPriorityLow {
		t.Fatal("priority low mismatch")
	}
}

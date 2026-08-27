package subagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
)

// usageEvent builds an event carrying one usage payload.
func usageEvent(prompt, completion, cached int, model string) *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Model: model,
			Usage: &trpcmodel.Usage{
				PromptTokens:        prompt,
				CompletionTokens:    completion,
				TotalTokens:         prompt + completion,
				PromptTokensDetails: trpcmodel.PromptTokensDetails{CachedTokens: cached},
			},
		},
	}
}

// P1-2: usageAccum must SUM across LLM rounds (each round is a separately
// billed API call) while tracking the cumulative max within one round.
func TestUsageAccum_PerRoundSum(t *testing.T) {
	a := &usageAccum{}
	// Round 1: cumulative stream chunks (max wins within the round).
	a.consume(usageEvent(100, 10, 40, "m1"))
	a.consume(usageEvent(100, 25, 40, "m1"))
	// Round 2 (tool loop): prompt grew → new billable round.
	a.consume(usageEvent(180, 5, 60, "m1"))
	a.consume(usageEvent(180, 30, 60, "m1"))
	p, c, cached := a.totals()
	if p != 280 || c != 55 || cached != 100 {
		t.Fatalf("totals = (%d,%d,%d), want (280,55,100)", p, c, cached)
	}
	if a.model != "m1" {
		t.Fatalf("model = %q, want m1", a.model)
	}
}

// Mid-run compaction shrinks the prompt; the smaller prompt is still a new
// billable round and must be summed, not dropped.
func TestUsageAccum_PromptShrinkCountsAsNewRound(t *testing.T) {
	a := &usageAccum{}
	a.consume(usageEvent(200, 20, 0, ""))
	a.consume(usageEvent(120, 8, 0, "")) // compaction → smaller prompt
	p, c, _ := a.totals()
	if p != 320 || c != 28 {
		t.Fatalf("totals = (%d,%d), want (320,28)", p, c)
	}
}

func TestUsageAccum_NilAndEmptyEvents(t *testing.T) {
	a := &usageAccum{}
	a.consume(nil)
	a.consume(&trpcevent.Event{}) // no response
	a.consume(&trpcevent.Event{Response: &trpcmodel.Response{}}) // no usage
	if p, c, cached := a.totals(); p != 0 || c != 0 || cached != 0 {
		t.Fatalf("totals = (%d,%d,%d), want zeros", p, c, cached)
	}
	var nilAccum *usageAccum
	nilAccum.consume(usageEvent(1, 1, 0, "")) // must not panic
	if p, c, cached := nilAccum.totals(); p != 0 || c != 0 || cached != 0 {
		t.Fatalf("nil totals = (%d,%d,%d), want zeros", p, c, cached)
	}
}

type stubUsageRecorder struct {
	calls []biz.AuxLLMUsageInput
	err   error
}

func (s *stubUsageRecorder) RecordAuxLLMUsage(_ context.Context, in biz.AuxLLMUsageInput) error {
	s.calls = append(s.calls, in)
	return s.err
}

func newUsageTestService(t *testing.T, rec UsageRecorder) *Service {
	t.Helper()
	svc, err := NewService(t.TempDir(), nil, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if rec != nil {
		svc.SetUsageRecorder(rec)
	}
	return svc
}

func TestRecordRunUsage_RecordsAuxSubagent(t *testing.T) {
	rec := &stubUsageRecorder{}
	svc := newUsageTestService(t, rec)

	record := &runRecord{
		Run: trpcsubagent.Run{
			ID:              "run-1",
			ParentSessionID: "sess-parent",
		},
		OwnerUserID: "user-1",
		Attribution: runAttribution{
			Provider: "deepseek",
			Model:    "deepseek-chat",
			AgentID:  "agent-1",
			AgentKey: "main",
		},
	}
	started := runningRun{startedAt: svc.clock().Add(-2 * time.Second)}
	usage := &usageAccum{}
	usage.consume(usageEvent(100, 20, 30, "deepseek-chat"))
	usage.consume(usageEvent(150, 10, 50, "deepseek-chat")) // round 2

	svc.recordRunUsage(record, started, usage, nil, "")

	if len(rec.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(rec.calls))
	}
	got := rec.calls[0]
	if got.Kind != biz.UsageKindAuxSubagent {
		t.Errorf("Kind = %q, want %q", got.Kind, biz.UsageKindAuxSubagent)
	}
	if got.SessionID != "sess-parent" || got.RunID != "run-1" || got.UserID != "user-1" {
		t.Errorf("attribution mismatch: %+v", got)
	}
	if got.Provider != "deepseek" || got.Model != "deepseek-chat" || got.AgentID != "agent-1" || got.AgentKey != "main" {
		t.Errorf("billing identity mismatch: %+v", got)
	}
	if got.PromptTok != 250 || got.CompletionTok != 30 || got.CachedTok != 80 {
		t.Errorf("tokens = (%d,%d,%d), want (250,30,80)", got.PromptTok, got.CompletionTok, got.CachedTok)
	}
	if got.Status != "success" {
		t.Errorf("Status = %q, want success", got.Status)
	}
	if got.UsageSource != "streaming" {
		t.Errorf("UsageSource = %q, want streaming", got.UsageSource)
	}
	if got.Latency < time.Second {
		t.Errorf("Latency = %v, want >= ~2s", got.Latency)
	}
}

// Provider-reported model id wins over the Spawn-time attribution snapshot.
func TestRecordRunUsage_StreamModelPreferred(t *testing.T) {
	rec := &stubUsageRecorder{}
	svc := newUsageTestService(t, rec)
	record := &runRecord{
		Run:         trpcsubagent.Run{ID: "run-2", ParentSessionID: "s"},
		Attribution: runAttribution{Provider: "p", Model: "snapshot-model"},
	}
	usage := &usageAccum{}
	usage.consume(usageEvent(10, 5, 0, "actual-model"))
	svc.recordRunUsage(record, runningRun{startedAt: svc.clock()}, usage, nil, "")
	if rec.calls[0].Model != "actual-model" {
		t.Fatalf("Model = %q, want actual-model", rec.calls[0].Model)
	}
}

func TestRecordRunUsage_Skips(t *testing.T) {
	t.Run("nil recorder", func(t *testing.T) {
		svc := newUsageTestService(t, nil)
		usage := &usageAccum{}
		usage.consume(usageEvent(10, 5, 0, "m"))
		// Must not panic.
		svc.recordRunUsage(&runRecord{Run: trpcsubagent.Run{ID: "r"}}, runningRun{startedAt: svc.clock()}, usage, nil, "")
	})

	t.Run("zero tokens", func(t *testing.T) {
		rec := &stubUsageRecorder{}
		svc := newUsageTestService(t, rec)
		svc.recordRunUsage(&runRecord{Run: trpcsubagent.Run{ID: "r"}}, runningRun{startedAt: svc.clock()}, &usageAccum{}, nil, "")
		if len(rec.calls) != 0 {
			t.Fatalf("calls = %d, want 0", len(rec.calls))
		}
	})

	t.Run("recorder error is swallowed", func(t *testing.T) {
		rec := &stubUsageRecorder{err: errors.New("db down")}
		svc := newUsageTestService(t, rec)
		usage := &usageAccum{}
		usage.consume(usageEvent(10, 5, 0, "m"))
		// Must not panic; error is Warn-logged only.
		svc.recordRunUsage(&runRecord{Run: trpcsubagent.Run{ID: "r"}}, runningRun{startedAt: svc.clock()}, usage, nil, "")
	})
}

func TestRecordRunUsage_StatusMapping(t *testing.T) {
	cases := []struct {
		name    string
		runErr  error
		want    string
		wantMsg bool
	}{
		{"success", nil, "success", false},
		{"cancelled", context.Canceled, "cancelled", false},
		{"timeout", context.DeadlineExceeded, "timeout", false},
		{"failed", errors.New("boom"), "failed", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &stubUsageRecorder{}
			svc := newUsageTestService(t, rec)
			usage := &usageAccum{}
			usage.consume(usageEvent(10, 5, 0, "m"))
			svc.recordRunUsage(&runRecord{Run: trpcsubagent.Run{ID: "r"}}, runningRun{startedAt: svc.clock()}, usage, tc.runErr, "")
			if len(rec.calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(rec.calls))
			}
			if rec.calls[0].Status != tc.want {
				t.Errorf("Status = %q, want %q", rec.calls[0].Status, tc.want)
			}
			if tc.wantMsg && rec.calls[0].ErrMsg == "" {
				t.Error("ErrMsg empty, want non-empty")
			}
		})
	}
}

// SetAttribution + Spawn snapshot: the record must capture the attribution
// current at spawn time, immune to later rotation.
func TestSpawnSnapshotsAttribution(t *testing.T) {
	svc := newUsageTestService(t, nil)
	svc.Start(context.Background())
	defer func() { _ = svc.Close() }()

	svc.SetAttribution("prov-a", "mod-a", "agent-a", "key-a")
	svc.mu.Lock()
	record := svc.newRunRecord(SpawnRequest{
		OwnerUserID:     "u1",
		ParentSessionID: "s1",
		Task:            "do something",
	})
	svc.mu.Unlock()
	if record.Attribution.Provider != "prov-a" || record.Attribution.Model != "mod-a" ||
		record.Attribution.AgentID != "agent-a" || record.Attribution.AgentKey != "key-a" {
		t.Fatalf("attribution not snapshotted: %+v", record.Attribution)
	}
}

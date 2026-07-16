package team

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

type captureHandler struct {
	mu   sync.Mutex
	got  []biz.ActivityEvent
	stop bool
}

func (h *captureHandler) HandleRunEvent(_ context.Context, aev biz.ActivityEvent) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.got = append(h.got, aev)
	return h.stop
}

func TestTeamRunPipeline_FansOutSystemNotice(t *testing.T) {
	bus := event.NewV2Bus()
	h := &captureHandler{}
	cancel := newTeamRunPipeline(h).Start(context.Background(), bus, "spirit-1", "sess-1")
	defer cancel()

	// Allow subscribe goroutine to start.
	time.Sleep(20 * time.Millisecond)

	bus.Publish(context.Background(), biz.NewSystemNoticeEvent("spirit-1", "member_status", "", map[string]any{
		"agent_id": "a1",
		"status":   "running",
	}))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		n := len(h.got)
		h.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("handler did not receive event")
}

func TestActivityEventFromBusEvent_SkipsOrchestrationStatus(t *testing.T) {
	notice := biz.NewSystemNoticeEvent("s1", "orchestration_status", "", map[string]any{})
	if _, ok := activityEventFromBusEvent(notice); ok {
		t.Fatal("orchestration_status should be skipped")
	}
}

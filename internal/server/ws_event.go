package server

import (
	"context"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

const defaultOutboxReplayLimit = 100

// sendConnected sends the initial "connected" downstream message to a new connection.
func (s *WSServer) sendConnected(wc *wsConn, sessionID, lastEventID string) {
	subscribed := wc.subscribedChannels()
	msg := wsDownstream{
		Direction: "server_to_client",
		Channel:   "system",
		Type:      "connected",
		Payload: map[string]any{
			"session_id":          sessionID,
			"server_time":         time.Now().UTC().Format(time.RFC3339Nano),
			"subscribed_channels": subscribed,
			"last_event_id":       lastEventID,
		},
	}
	wc.sendSystemDownstream(msg)
}

// replayOutbox pushes durable critical v2_event frames missed since lastEventID
// (B-06). Session-scoped only; best-effort — failures are logged and ignored.
func (s *WSServer) replayOutbox(wc *wsConn, sessionID, lastEventID string) {
	if s == nil || s.outbox == nil || wc == nil || lastEventID == "" || sessionID == "" || sessionID == "*" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.outbox.ListAfter(ctx, sessionID, lastEventID, 0, defaultOutboxReplayLimit)
	if err != nil {
		s.lg.Warn("WS outbox replay failed",
			loggateway.StepID("ws.outbox_replay"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return
	}
	cfg := s.wsConfig()
	for _, row := range rows {
		if len(row.Payload) == 0 {
			continue
		}
		if ok := wc.queues.enqueue(cfg, wsPriorityHigh, row.Payload); !ok {
			s.lg.Warn("WS outbox replay high-queue timeout",
				loggateway.StepID("ws.outbox_replay"),
				loggateway.SessionID(sessionID),
				loggateway.Str("event_id", row.EventID))
			break
		}
	}
	if len(rows) > 0 {
		wc.wakeWriter()
	}
}

// setupEventSubscription subscribes the connection to the monitor bus and
// sets wc.unsubscribe. The v1 ActivityEventBus subscription was removed in
// Phase 3b-D Tier 4 — chat events now flow exclusively via the v2 EventBus
// (WSV2Subscriber → BroadcastToSession → conn priority queue).
//
// B-06: critical-event reconnect replay uses the durable outbox via
// replayOutbox (last_event_id); REST snapshot hydrate remains the fallback.
func (s *WSServer) setupEventSubscription(wc *wsConn, globalMode bool) <-chan contract.MonitorEvent {
	var monitorCh <-chan contract.MonitorEvent

	unsubAll := func() {}

	if s.monitorBus != nil {
		monOpts := contract.MonitorSubscribeOptions{
			BufferSize: 128,
			GlobalMode: globalMode,
		}
		if !globalMode {
			monOpts.SessionID = wc.sessionID
		}
		mCh, mUnsub := s.monitorBus.Subscribe(monOpts)
		monitorCh = mCh
		prev := unsubAll
		unsubAll = func() {
			prev()
			mUnsub()
		}
	}

	wc.unsubscribe = unsubAll

	return monitorCh
}

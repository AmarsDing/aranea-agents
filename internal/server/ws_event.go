package server

import (
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
)

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

// setupEventSubscription subscribes the connection to the monitor bus and
// activity event bus. Returns the monitor and activity event channels,
// and sets wc.unsubscribe.
//
// Phase 5 Blocker A: WS replay path has been removed. Clients needing
// historical events should call the ListActivities RPC
// (GET /v1/sessions/{session_id}/activities) to fetch Activity records
// on reconnect. The server no longer replays buffered envelopes.
func (s *WSServer) setupEventSubscription(wc *wsConn, globalMode bool) (<-chan contract.MonitorEvent, <-chan biz.ActivityEvent) {
	var monitorCh <-chan contract.MonitorEvent
	var activityCh <-chan biz.ActivityEvent

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

	// Subscribe to ActivityEventBus for AF (Activity-First) chat rendering.
	if s.activityBus != nil {
		actOpts := biz.ActivityEventSubscribeOptions{
			BufferSize: 256,
			GlobalMode: globalMode,
		}
		if !globalMode {
			actOpts.SessionID = wc.sessionID
		}
		aCh, aUnsub := s.activityBus.Subscribe(actOpts)
		activityCh = aCh
		prev := unsubAll
		unsubAll = func() {
			prev()
			aUnsub()
		}
	}

	wc.unsubscribe = unsubAll

	return monitorCh, activityCh
}

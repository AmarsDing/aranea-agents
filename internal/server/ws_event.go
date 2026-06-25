package server

import (
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
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

// replayEvents replays historical events from the in-memory buffer to the connection.
//
// Phase 1c-2: cross-process Postgres event store fallback has been removed along
// with the event_store subsystem. WS reconnect replay now relies solely on the
// in-memory buffer; clients needing full history should use the ListActivities
// RPC (GET /v1/sessions/{session_id}/activities) to fetch Activity records.
func (s *WSServer) replayEvents(wc *wsConn, sessionID, lastEventID string) {
	events := s.eventBuffer.Replay(sessionID, lastEventID)

	startMsg := wsDownstream{
		Direction: "server_to_client",
		Channel:   wc.firstSubscribedChannel(),
		Type:      "replay_start",
		Payload: map[string]any{
			"session_id":    sessionID,
			"last_event_id": lastEventID,
			"count":         len(events),
		},
	}
	// H-02: all replay messages route through queues (normal priority).
	if data, err := json.Marshal(startMsg); err == nil {
		wc.queues.enqueueSystem(data)
		wc.wakeWriter()
	}

	for _, env := range events {
		if !wc.hasChannel(env.Channel) {
			continue
		}
		msg := wsDownstream{
			Direction: "server_to_client",
			Channel:   env.Channel,
			Type:      "replay",
			Envelope:  &env,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		wc.queues.enqueueSystem(data)
		wc.wakeWriter()
	}

	endMsg := wsDownstream{
		Direction: "server_to_client",
		Channel:   wc.firstSubscribedChannel(),
		Type:      "replay_end",
		Payload: map[string]any{
			"session_id": sessionID,
		},
	}
	if data, err := json.Marshal(endMsg); err == nil {
		wc.queues.enqueueSystem(data)
		wc.wakeWriter()
	}
}

// setupEventSubscription subscribes the connection to the event bus, monitor bus,
// and activity event bus. Returns the event, monitor, and activity event channels,
// and sets wc.unsubscribe.
func (s *WSServer) setupEventSubscription(wc *wsConn, globalMode bool) (<-chan event.Envelope, <-chan event.Envelope, <-chan biz.ActivityEvent) {
	var eventCh <-chan event.Envelope
	var monitorCh <-chan event.Envelope
	var activityCh <-chan biz.ActivityEvent

	subOpts := event.SubscribeOptions{
		BufferSize: 256,
		Reliable:   !globalMode,
	}
	if !globalMode {
		subOpts.SessionID = wc.sessionID
	}
	ch, unsub := s.eventBus.Subscribe(subOpts)
	eventCh = ch
	unsubSession := unsub
	unsubAll := func() { unsubSession() }

	if s.monitorBus != nil && s.monitorBus != s.eventBus {
		monOpts := event.SubscribeOptions{
			BufferSize: 128,
			DropPolicy: event.DropNewest,
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

	return eventCh, monitorCh, activityCh
}

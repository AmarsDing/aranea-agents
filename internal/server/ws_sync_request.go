package server

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// syncReplayLookbackWindow limits how far back the server queries the EventStore
// for revision-based replay. Envelopes older than this window are considered
// stale and skipped (the client should have already received them via
// event-ID-based replay or initial load).
const syncReplayLookbackWindow = 24 * time.Hour

// syncReplayMaxEnvelopes caps the number of envelopes replayed in a single
// sync_request response to prevent unbounded replay on large sessions.
const syncReplayMaxEnvelopes = 500

// handleSyncRequest processes a sync_request upstream event (T3.4).
//
// The client sends sync_request { session_id, after_revision } after WS
// reconnect to request replay of envelopes with session_revision > after_revision.
// The server queries the EventStore for the session's envelopes within the
// lookback window, filters by revision, and replays them to the connection.
//
// If the EventStore is not configured (eventStoreUsecase == nil), the request
// is silently ignored — event-ID-based replay (via lastEventId) still works.
//
// Validation runs synchronously (cheap); the EventStore query + replay runs in
// a goroutine because the query may block for up to 10s and must not stall the
// readPump goroutine that dispatches upstream messages.
func (s *WSServer) handleSyncRequest(wc *wsConn, up wsUpstream) {
	if s == nil || s.eventStoreUsecase == nil {
		// EventStore not configured — skip revision-based sync.
		// Event-ID-based replay (via lastEventId in URL) still applies.
		return
	}
	payload, ok := up.Payload.(map[string]any)
	if !ok {
		return
	}
	sessionID, _ := payload["session_id"].(string)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		// Fall back to the connection's session ID.
		sessionID = wc.sessionID
	}
	if sessionID == "" {
		return
	}
	afterRevision := int64(0)
	switch v := payload["after_revision"].(type) {
	case float64:
		afterRevision = int64(v)
	case int64:
		afterRevision = v
	case int:
		afterRevision = int64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			afterRevision = n
		}
	}
	if afterRevision <= 0 {
		// No revision to sync from — event-ID-based replay handles it.
		return
	}

	// Async: EventStore query may block up to 10s; don't stall readPump.
	safego.Go(wc.contextOrBackground(), "ws-sync-request", func() {
		s.runSyncReplay(wc, sessionID, afterRevision)
	})
}

// runSyncReplay queries the EventStore and replays matching envelopes to the
// connection. Runs in a goroutine spawned by handleSyncRequest.
func (s *WSServer) runSyncReplay(wc *wsConn, sessionID string, afterRevision int64) {
	lg := s.lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg = lg.With(loggateway.SessionID(sessionID), loggateway.StepID("ws.sync_request"))

	// Derive the timeout from the connection context so the replay aborts when
	// the connection closes (BC2: prefer wc.contextOrBackground over Background).
	ctx, cancel := context.WithTimeout(wc.contextOrBackground(), 10*time.Second)
	defer cancel()

	// Query EventStore for envelopes within the lookback window.
	since := time.Now().UTC().Add(-syncReplayLookbackWindow)
	result, err := s.eventStoreUsecase.List(ctx, biz.EventStoreQuery{
		SessionID: sessionID,
		Since:     since,
		Limit:     syncReplayMaxEnvelopes,
	})
	if err != nil {
		lg.Warn("sync_request: EventStore query failed", loggateway.Err(err))
		return
	}

	// Parse, filter, and collect envelopes.
	// The EventStore persists the full envelope JSON, so we parse it to
	// extract the session_revision field.
	type replayItem struct {
		env      *contract.Envelope
		revision int64
	}
	items := make([]replayItem, 0, len(result.Items))
	for _, rec := range result.Items {
		env, err := parseEnvelopeJSON(rec.EnvelopeJSON)
		if err != nil {
			continue
		}
		if env.SessionRevision <= afterRevision {
			continue
		}
		// Skip envelopes the client isn't subscribed to.
		if env.Channel != "" && !wc.hasChannel(env.Channel) {
			continue
		}
		items = append(items, replayItem{env: env, revision: env.SessionRevision})
	}

	// INV4: sort by SessionRevision ascending to preserve causal order.
	// The EventStore may return records in insertion order, which is usually
	// but not guaranteed to be revision order; sort to be safe.
	sort.Slice(items, func(i, j int) bool {
		return items[i].revision < items[j].revision
	})

	replayed := 0
	for _, item := range items {
		msg := wsDownstream{
			Direction: "server_to_client",
			Channel:   item.env.Channel,
			Type:      "replay",
			Envelope:  item.env,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		wc.queues.enqueueSystem(data)
		wc.wakeWriter()
		replayed++
	}

	lg.Info("sync_request: replay completed",
		loggateway.Any("after_revision", afterRevision),
		loggateway.Any("queried", len(result.Items)),
		loggateway.Any("replayed", replayed))
}

// parseEnvelopeJSON unmarshals an envelope JSON string into a contract.Envelope.
// Returns an error if the JSON is invalid or missing required fields.
func parseEnvelopeJSON(raw string) (*contract.Envelope, error) {
	var env contract.Envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return nil, err
	}
	if env.ID == "" {
		return nil, errInvalidEnvelope
	}
	return &env, nil
}

// errInvalidEnvelope is returned when an envelope JSON is missing required fields.
var errInvalidEnvelope = &invalidEnvelopeError{}

type invalidEnvelopeError struct{}

func (e *invalidEnvelopeError) Error() string { return "invalid envelope: missing id" }

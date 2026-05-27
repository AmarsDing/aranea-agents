package server

// ws_priority.go — MON-OPT-04: WebSocket 三优先级 send queue.
//
// Replaces the single wc.send channel with three priority lanes:
//   high   cap 64  — alert.notify, mcp health — NEVER silently dropped
//   normal cap 128 — runner.completion, team_run_*, intent_pass, system msgs
//   low    cap 256 — flow_log, log, usage.*
//
// writePump drains: all pending high → normal → low (round-robin low to avoid starvation).
// Drop policy:
//   high   block 5 s; if still full close connection so client reconnects
//   normal DropNewest + increment dropNormal counter
//   low    DropNewest + increment dropLow counter
//
// Every 10 s, if any drops occurred since last report, a "monitor.backpressure"
// JSON frame is injected into the normal queue (never into low, in case low is clogged).

import (
	"encoding/json"
	"sync/atomic"
	"time"

	"aranea-agents/internal/event"
)

const (
	wsHighCap   = 64
	wsNormalCap = 128
	wsLowCap    = 256

	wsHighBlockTimeout     = 5 * time.Second
	wsBackpressureInterval = 10 * time.Second
	// max low-priority messages drained per writePump loop to prevent high/normal starvation
	wsLowDrainPerLoop = 8
)

// wsPriority classifies an EnvelopeType into a send-queue priority.
type wsPriority int

const (
	wsPriorityHigh   wsPriority = 0
	wsPriorityNormal wsPriority = 1
	wsPriorityLow    wsPriority = 2
)

// wsEnvelopePriority returns the send-queue priority for an envelope type.
func wsEnvelopePriority(t event.EnvelopeType) wsPriority {
	switch t {
	case event.EnvelopeTypeAlertNotify,
		event.EnvelopeTypeMCPHealthAlert:
		return wsPriorityHigh
	case event.EnvelopeTypeFlowLog,
		event.EnvelopeTypeLog:
		return wsPriorityLow
	default:
		return wsPriorityNormal
	}
}

// connQueues holds the three priority channels and drop counters for one WS connection.
type connQueues struct {
	high   chan []byte
	normal chan []byte
	low    chan []byte

	dropNormal atomic.Uint64
	dropLow    atomic.Uint64
	dropHigh   atomic.Uint64 // connection-close events; usually 0
}

func newConnQueues() *connQueues {
	return &connQueues{
		high:   make(chan []byte, wsHighCap),
		normal: make(chan []byte, wsNormalCap),
		low:    make(chan []byte, wsLowCap),
	}
}

// enqueue routes data to the correct priority lane.
// For high priority: blocks up to wsHighBlockTimeout; returns false if the connection
// should be closed (caller must close the channel to signal writePump to exit).
func (q *connQueues) enqueue(priority wsPriority, data []byte) (ok bool) {
	switch priority {
	case wsPriorityHigh:
		select {
		case q.high <- data:
			return true
		default:
		}
		// high is full: block with timeout
		timer := time.NewTimer(wsHighBlockTimeout)
		defer timer.Stop()
		select {
		case q.high <- data:
			return true
		case <-timer.C:
			q.dropHigh.Add(1)
			return false // caller should close connection
		}
	case wsPriorityNormal:
		select {
		case q.normal <- data:
		default:
			q.dropNormal.Add(1)
		}
		return true
	default: // wsPriorityLow
		select {
		case q.low <- data:
		default:
			q.dropLow.Add(1)
		}
		return true
	}
}

// enqueueSystem routes a system-generated (non-Bus) message to the normal queue.
func (q *connQueues) enqueueSystem(data []byte) {
	select {
	case q.normal <- data:
	default:
		q.dropNormal.Add(1)
	}
}

// drainOnce returns the next message to send, prioritising high > normal > low.
// It drains up to wsLowDrainPerLoop low-priority messages per outer loop iteration.
// Returns nil if all queues are empty.
func (q *connQueues) drainSelect() (data []byte, priority wsPriority, ok bool) {
	// Always drain high first.
	select {
	case d, open := <-q.high:
		return d, wsPriorityHigh, open
	default:
	}
	// Then normal.
	select {
	case d, open := <-q.normal:
		return d, wsPriorityNormal, open
	default:
	}
	// Then low.
	select {
	case d, open := <-q.low:
		return d, wsPriorityLow, open
	default:
	}
	return nil, wsPriorityLow, false
}

// backpressurePayload builds the monitor.backpressure JSON frame.
func (q *connQueues) backpressurePayload(windowSecs int) []byte {
	droppedNormal := q.dropNormal.Swap(0)
	droppedLow := q.dropLow.Swap(0)
	droppedHigh := q.dropHigh.Load()
	if droppedNormal+droppedLow+droppedHigh == 0 {
		return nil
	}
	msg := map[string]any{
		"direction": "server_to_client",
		"channel":   "system",
		"type":      "monitor.backpressure",
		"payload": map[string]any{
			"dropped_high":    droppedHigh,
			"dropped_normal":  droppedNormal,
			"dropped_low":     droppedLow,
			"window_seconds":  windowSecs,
			"advice":          "reduce subscribed channels or pause non-critical streams",
		},
	}
	data, _ := json.Marshal(msg)
	return data
}

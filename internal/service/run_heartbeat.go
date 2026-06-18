package service

import (
	"context"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// defaultHeartbeatInterval is the default emit interval when interval <= 0.
const defaultHeartbeatInterval = 10 * time.Second

// RunProgress carries the progress snapshot published with each heartbeat.
type RunProgress struct {
	Percent     float64
	CurrentStep string
	TotalSteps  int
	ETA         string
}

// RunHeartbeatEmitter periodically publishes EnvelopeTypeRunHeartbeat events
// so the frontend can detect stale runs within 30s (P1-7).
//
// Classified as Informational (AS-EVT-01): loss only degrades progress
// visibility, does not corrupt state.
type RunHeartbeatEmitter struct {
	interval time.Duration
	bus      contract.Bus
	lg       loggateway.Logger
}

// NewRunHeartbeatEmitter constructs a RunHeartbeatEmitter.
// When interval <= 0 the default 10s interval is used.
// When lg == nil a no-op logger is used.
func NewRunHeartbeatEmitter(interval time.Duration, bus contract.Bus, lg loggateway.Logger) *RunHeartbeatEmitter {
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &RunHeartbeatEmitter{
		interval: interval,
		bus:      bus,
		lg:       lg.With(loggateway.Domain("run-heartbeat")),
	}
}

// Start launches a goroutine that publishes run_heartbeat events every
// interval. The returned stop function cancels the goroutine and must be
// called when the run completes to avoid leaks.
//
// When progress is nil, only an empty heartbeat (run_id only) is published
// to prove the run is still alive.
func (e *RunHeartbeatEmitter) Start(ctx context.Context, runID, sessionID string, progress func() RunProgress) func() {
	innerCtx, cancel := context.WithCancel(ctx)

	safego.Go(innerCtx, "run-heartbeat-emitter", func() {
		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		for {
			select {
			case <-innerCtx.Done():
				return
			case <-ticker.C:
				e.publishHeartbeat(innerCtx, runID, sessionID, progress)
			}
		}
	})

	return cancel
}

// publishHeartbeat publishes a single heartbeat envelope. When progress is
// non-nil its fields are included in the metadata; otherwise only run_id is
// sent to prove the run is alive.
func (e *RunHeartbeatEmitter) publishHeartbeat(ctx context.Context, runID, sessionID string, progress func() RunProgress) {
	if e.bus == nil {
		return
	}

	env := contract.NewEnvelope(contract.EnvelopeTypeRunHeartbeat, "run-heartbeat", sessionID)
	meta := map[string]any{
		"run_id": runID,
	}
	if progress != nil {
		p := progress()
		meta["progress_percent"] = p.Percent
		meta["current_step"] = p.CurrentStep
		meta["total_steps"] = p.TotalSteps
		meta["eta"] = p.ETA
	}
	env.Metadata = meta
	e.bus.Publish(ctx, env)
}

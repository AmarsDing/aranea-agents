package monitor

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz/types"
	"aranea-agents/pkg/loggateway"
)

const (
	// RepairCooldownSec is the cooldown period between same-checker repairs.
	RepairCooldownSec = 300 // 5 minutes
)

// SelfCheckRepairDispatcher routes check results to appropriate repairers
// with cooldown enforcement.
type SelfCheckRepairDispatcher struct {
	repairers []SelfCheckRepairer
	cooldowns map[string]time.Time // checkName → last repair time
	mu        sync.Mutex
	lg        loggateway.Logger
}

// NewSelfCheckRepairDispatcher creates a new repair dispatcher.
func NewSelfCheckRepairDispatcher(repairers []SelfCheckRepairer, lg loggateway.Logger) *SelfCheckRepairDispatcher {
	return &SelfCheckRepairDispatcher{
		repairers: repairers,
		cooldowns: make(map[string]time.Time),
		lg:        lg,
	}
}

// RegisterRepairer adds a repairer.
func (d *SelfCheckRepairDispatcher) RegisterRepairer(r SelfCheckRepairer) {
	if d == nil || r == nil {
		return
	}
	d.repairers = append(d.repairers, r)
}

// CanRepair checks if any registered repairer can handle the given check.
func (d *SelfCheckRepairDispatcher) CanRepair(checkName string, status types.SelfCheckStatus) bool {
	if d == nil {
		return false
	}
	for _, r := range d.repairers {
		if r.CanRepair(checkName, status) {
			return true
		}
	}
	return false
}

// Repair dispatches to the first matching repairer, respecting cooldown.
func (d *SelfCheckRepairDispatcher) Repair(ctx context.Context, result types.SelfCheckResult) RepairOutcome {
	if d == nil {
		return RepairOutcome{Success: false, Action: "none", Message: "dispatcher is nil"}
	}

	// Check cooldown
	if !d.checkCooldown(result.Checker) {
		return RepairOutcome{
			Success: false,
			Action:  "skipped_cooldown",
			Message: "same checker recently repaired, in cooldown period",
		}
	}

	for _, r := range d.repairers {
		if r.CanRepair(result.Checker, result.Status) {
			outcome := r.Repair(ctx, result)
			if outcome.Success {
				d.setCooldown(result.Checker, time.Now())
			}
			return outcome
		}
	}

	return RepairOutcome{Success: false, Action: "none", Message: "no repairer available"}
}

func (d *SelfCheckRepairDispatcher) checkCooldown(checkName string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if last, ok := d.cooldowns[checkName]; ok {
		return time.Since(last) > RepairCooldownSec*time.Second
	}
	return true
}

func (d *SelfCheckRepairDispatcher) setCooldown(checkName string, t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cooldowns[checkName] = t
}

// FlowFileRepairer attempts to free disk space by cleaning up old compressed files.
type FlowFileRepairer struct {
	appender *FlowFileAppender
}

// NewFlowFileRepairer creates a repairer for flow file issues.
func NewFlowFileRepairer(appender *FlowFileAppender) *FlowFileRepairer {
	return &FlowFileRepairer{appender: appender}
}

func (r *FlowFileRepairer) CanRepair(checkName string, status types.SelfCheckStatus) bool {
	return checkName == "flow_file" && status != types.SelfCheckStatusPassed
}

func (r *FlowFileRepairer) Repair(ctx context.Context, result types.SelfCheckResult) RepairOutcome {
	if r.appender == nil {
		return RepairOutcome{Success: false, Action: "flow_file_repair", Message: "appender is nil"}
	}
	// Purge expired files to free disk space
	r.appender.PurgeExpiredFiles()
	r.appender.CompressOldFiles()
	return RepairOutcome{
		Success: true,
		Action:  "purge_expired_and_compress",
		Message: "cleaned up expired flow files and compressed old files",
	}
}

// TraceProjectorBackfiller is the port for triggering trace backfill.
type TraceProjectorBackfiller interface {
	BackfillTraces(ctx context.Context) error
}

// TraceProjectorRepairer triggers a trace backfill when the projector has no active traces.
type TraceProjectorRepairer struct {
	backfiller TraceProjectorBackfiller
}

// NewTraceProjectorRepairer creates a repairer for trace projector issues.
func NewTraceProjectorRepairer(backfiller TraceProjectorBackfiller) *TraceProjectorRepairer {
	return &TraceProjectorRepairer{backfiller: backfiller}
}

func (r *TraceProjectorRepairer) CanRepair(checkName string, status types.SelfCheckStatus) bool {
	return checkName == "trace_projector" && status != types.SelfCheckStatusPassed
}

func (r *TraceProjectorRepairer) Repair(ctx context.Context, result types.SelfCheckResult) RepairOutcome {
	if r.backfiller == nil {
		return RepairOutcome{Success: false, Action: "trace_projector_repair", Message: "backfiller is nil"}
	}
	if err := r.backfiller.BackfillTraces(ctx); err != nil {
		return RepairOutcome{Success: false, Action: "trace_backfill", Message: "backfill failed: " + err.Error()}
	}
	return RepairOutcome{
		Success: true,
		Action:  "trace_backfill",
		Message: "triggered trace backfill successfully",
	}
}

// AlertEvalRestarter is the port for restarting the alert evaluation worker.
type AlertEvalRestarter interface {
	RestartEvalWorker(ctx context.Context)
}

// AlertEvalRepairer restarts the AlertEvalWorker goroutine when it is not ready.
type AlertEvalRepairer struct {
	restarter AlertEvalRestarter
}

// NewAlertEvalRepairer creates a repairer for alert eval worker issues.
func NewAlertEvalRepairer(restarter AlertEvalRestarter) *AlertEvalRepairer {
	return &AlertEvalRepairer{restarter: restarter}
}

func (r *AlertEvalRepairer) CanRepair(checkName string, status types.SelfCheckStatus) bool {
	return checkName == "alert_eval" && status != types.SelfCheckStatusPassed
}

func (r *AlertEvalRepairer) Repair(ctx context.Context, result types.SelfCheckResult) RepairOutcome {
	if r.restarter == nil {
		return RepairOutcome{Success: false, Action: "alert_eval_repair", Message: "restarter is nil"}
	}
	r.restarter.RestartEvalWorker(ctx)
	return RepairOutcome{
		Success: true,
		Action:  "restart_eval_worker",
		Message: "restarted alert evaluation worker",
	}
}

// EventBusResubscriber is the port for resubscribing to the event bus.
type EventBusResubscriber interface {
	Resubscribe(topic string) error
}

// EventBusRepairer resubscribes disconnected event bus handlers.
type EventBusRepairer struct {
	resubscriber EventBusResubscriber
}

// NewEventBusRepairer creates a repairer for event bus issues.
func NewEventBusRepairer(resubscriber EventBusResubscriber) *EventBusRepairer {
	return &EventBusRepairer{resubscriber: resubscriber}
}

func (r *EventBusRepairer) CanRepair(checkName string, status types.SelfCheckStatus) bool {
	return checkName == "eventbus" && status != types.SelfCheckStatusPassed
}

func (r *EventBusRepairer) Repair(ctx context.Context, result types.SelfCheckResult) RepairOutcome {
	if r.resubscriber == nil {
		return RepairOutcome{Success: false, Action: "eventbus_repair", Message: "resubscriber is nil"}
	}
	if err := r.resubscriber.Resubscribe("monitor"); err != nil {
		return RepairOutcome{Success: false, Action: "resubscribe", Message: "resubscribe failed: " + err.Error()}
	}
	return RepairOutcome{
		Success: true,
		Action:  "resubscribe",
		Message: "resubscribed to event bus topic 'monitor'",
	}
}

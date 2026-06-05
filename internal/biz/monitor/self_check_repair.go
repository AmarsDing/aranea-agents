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

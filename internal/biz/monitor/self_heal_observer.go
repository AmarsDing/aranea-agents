package monitor

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// SelfHealObserver is the Phase 2 self-healing implementation with circuit breaker.
// TODO(debt): DEV-06 — After migration, SelfHealObserver will be the sole heal orchestrator.
//
// It observes FlowLog events, tracks auto-heal outcomes,
// fires alerts for repeated failures, and persists HealRecords.
// It replaces SelfHealUsecase for the observation role (Phase 2 migration).
type SelfHealObserver struct {
	repo     HealRecordRepo
	engine   *RootCauseEngine
	notifier AlertNotifier
	lg       loggateway.Logger
	healConf conf.RuntimeSelfHealConfig

	mu         sync.Mutex
	cooldowns  map[string]time.Time   // ruleID → last alert time
	healEvents map[string][]time.Time // stepID → timestamps of recent heal events (sliding window)
}

// NewSelfHealObserver creates a new SelfHealObserver. // WIRE: needs *conf.Runtime
func NewSelfHealObserver(runtimeConf *conf.Runtime, repo HealRecordRepo, engine *RootCauseEngine, notifier AlertNotifier, lg loggateway.Logger) (*SelfHealObserver, error) {
	if repo == nil {
		return nil, kerrors.InternalServer("MONITOR", "HealRecordRepo is required")
	}
	if engine == nil {
		return nil, kerrors.InternalServer("MONITOR", "RootCauseEngine is required")
	}
	return &SelfHealObserver{
		repo:        repo,
		engine:      engine,
		notifier:    notifier,
		lg:          lg,
		healConf:    runtimeConf.SelfHealConfig(),
		cooldowns:   make(map[string]time.Time),
		healEvents:  make(map[string][]time.Time),
	}, nil
}

// ObserveFlowLogEvent processes a FlowLog event.
// REQ-SO-01: For each event with flow_phase=error, evaluate root causes and record.
// REQ-SO-02: auto_healed=true + heal_success=true → observed_healed.
// REQ-SO-03: auto_healed=true + heal_success=false → observed_failed, alert if 3+ consecutive.
// REQ-SO-04: auto_healed=false + phase=error → root cause analysis, alert if confidence >= 0.7.
func (o *SelfHealObserver) ObserveFlowLogEvent(ctx context.Context, meta map[string]any) {
	if o == nil || meta == nil {
		return
	}

	phase, _ := meta["flow_phase"].(string)
	if !stringsEqualFold(phase, "error") {
		return
	}

	stepID, _ := meta["step_id"].(string)
	traceID, _ := meta["trace_id"].(string)
	sessionID, _ := meta["session_id"].(string)
	if stepID == "" {
		return
	}

	autoHealed, _ := meta["auto_healed"].(bool)
	healSuccess, _ := meta["heal_success"].(bool)
	healStrategy, _ := meta["heal_strategy"].(string)
	healAttempts, _ := meta["heal_attempts"].(int)

	now := time.Now().UTC()

	if autoHealed {
		if healSuccess {
			// REQ-SO-02: Runtime auto-heal succeeded → observed_healed
			o.recordObservation(ctx, HealRecord{
				ID:                  generateHealID(),
				TriggerType:         "auto_error_event",
				TraceID:             traceID,
				SessionID:           sessionID,
				StepID:              stepID,
				Status:              string(HealStatusObservedHealed),
				RuntimeAutoHealed:   true,
				RuntimeHealAttempts: healAttempts,
				Reason:              "runtime auto-heal succeeded",
				CreatedAt:           now.Format(time.RFC3339),
				Metadata:            meta,
			})
			// Reset consecutive failure count for this step
			o.mu.Lock()
			delete(o.healEvents, stepID)
			o.mu.Unlock()
		} else {
			// REQ-SO-03: Runtime auto-heal failed → observed_failed
			o.recordObservation(ctx, HealRecord{
				ID:                  generateHealID(),
				TriggerType:         "auto_error_event",
				TraceID:             traceID,
				SessionID:           sessionID,
				StepID:              stepID,
				Status:              string(HealStatusObservedFailed),
				RuntimeAutoHealed:   true,
				RuntimeHealAttempts: healAttempts,
				Reason:              "runtime auto-heal failed",
				CreatedAt:           now.Format(time.RFC3339),
				Metadata:            meta,
			})

			// Track heal events in sliding window and fire alert if threshold exceeded
			o.mu.Lock()
			o.healEvents[stepID] = append(o.healEvents[stepID], now)
			// Prune events outside the window
			windowStart := now.Add(-o.healConf.CircuitBreakerWindow)
			pruned := make([]time.Time, 0, len(o.healEvents[stepID]))
			for _, t := range o.healEvents[stepID] {
				if !t.Before(windowStart) {
					pruned = append(pruned, t)
				}
			}
			o.healEvents[stepID] = pruned
			// Remove stepID key if empty to prevent map growth
			if len(pruned) == 0 {
				delete(o.healEvents, stepID)
			}
			countInWindow := len(pruned)
			o.mu.Unlock()

			if countInWindow >= int(o.healConf.CircuitBreakerThreshold) {
				ruleID := "rc-repeated-auto-heal-failure"
				if o.checkCooldown(ruleID, "critical") {
					o.fireCircuitOpenAlert(ctx, ruleID, stepID, sessionID, "Runtime auto-heal has failed repeatedly in sliding window ("+healStrategy+")", "critical", meta)
				}
			}
		}
		return
	}

	// REQ-SO-04: Runtime did not attempt heal → run root cause analysis
	causes := o.engine.Evaluate(ctx, stepID, "error", meta)
	if len(causes) == 0 {
		return
	}

	best := o.pickBestCause(causes)
	if best == nil {
		return
	}

	o.recordObservation(ctx, HealRecord{
		ID:                  generateHealID(),
		RuleID:              best.RuleID,
		TriggerType:         "auto_error_event",
		TraceID:             traceID,
		SessionID:           sessionID,
		StepID:              stepID,
		FixAction:           best.FixAction,
		Confidence:          best.Confidence,
		Status:              string(HealStatusObservedFailed),
		RuntimeAutoHealed:   false,
		RuntimeHealAttempts: 0,
		Reason:              best.RootCause,
		CreatedAt:           now.Format(time.RFC3339),
		Metadata:            meta,
	})

	if best.Confidence >= o.healConf.MinConfidence {
		if o.checkCooldown(best.RuleID, best.Severity) {
			o.fireAlert(ctx, best.RuleID, stepID, sessionID, best.RootCause, best.Severity, meta)
		}
	}
}

// StartEventDrivenObservation subscribes to FlowLog error events and observes them.
func (o *SelfHealObserver) StartEventDrivenObservation(ctx context.Context, ch <-chan Envelope) {
	safego.Go(ctx, "self-heal-observer-event-driven", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-ch:
				if !ok {
					return
				}
				o.ObserveFlowLogEvent(ctx, env.GetMetadata())
			}
		}
	})
}

// GetHealStats returns aggregated heal statistics.
func (o *SelfHealObserver) GetHealStats(ctx context.Context) (HealStats, error) {
	if o == nil || o.repo == nil {
		return HealStats{}, nil
	}

	// Query all recent records for stats
	result, err := o.repo.ListHealRecords(ctx, HealRecordQuery{Limit: 1000})
	if err != nil {
		return HealStats{}, err
	}

	stats := HealStats{TotalHeals: result.Total}
	if result.Total == 0 {
		return stats, nil
	}

	successCount := 0
	failByRule := make(map[string]int)
	for _, r := range result.Items {
		if r.Status == string(HealStatusObservedHealed) || r.Status == string(HealStatusApplied) {
			successCount++
		}
		if r.Status == string(HealStatusObservedFailed) || r.Status == string(HealStatusFailed) {
			if r.RuleID != "" {
				failByRule[r.RuleID]++
			}
		}
	}
	stats.SuccessRate = float64(successCount) / float64(result.Total)

	// Top 5 failing rules
	for ruleID, count := range failByRule {
		stats.TopFailRules = append(stats.TopFailRules, RuleFailCount{RuleID: ruleID, Count: count})
	}
	// Sort by count descending, keep top 5
	for i := 0; i < len(stats.TopFailRules); i++ {
		for j := i + 1; j < len(stats.TopFailRules); j++ {
			if stats.TopFailRules[j].Count > stats.TopFailRules[i].Count {
				stats.TopFailRules[i], stats.TopFailRules[j] = stats.TopFailRules[j], stats.TopFailRules[i]
			}
		}
	}
	if len(stats.TopFailRules) > 5 {
		stats.TopFailRules = stats.TopFailRules[:5]
	}

	return stats, nil
}

// ListHealRecords returns paginated heal records from persistent storage.
func (o *SelfHealObserver) ListHealRecords(ctx context.Context, query HealRecordQuery) (HealRecordListResult, error) {
	if o == nil || o.repo == nil {
		return HealRecordListResult{}, nil
	}
	return o.repo.ListHealRecords(ctx, query)
}

func (o *SelfHealObserver) recordObservation(ctx context.Context, record HealRecord) {
	if o == nil || o.repo == nil {
		return
	}
	if err := o.repo.InsertHealRecord(ctx, record); err != nil {
		o.lg.Error("SelfHealObserver: failed to persist heal record",
			loggateway.StepID("monitor.heal_record_persist_fail"),
			loggateway.Str("rule_id", record.RuleID),
			loggateway.Err(err))
	}
}

func (o *SelfHealObserver) fireAlert(ctx context.Context, ruleID, stepID, sessionID, rootCause, severity string, meta map[string]any) {
	o.lg.Warn("SelfHealObserver: firing alert for unhealed/repeated-failure error",
		loggateway.StepID("monitor.heal_alert_fired"),
		loggateway.Str("rule_id", ruleID),
		loggateway.Str("step_id", stepID),
		loggateway.Str("severity", severity))

	// Record alert_fired status
	now := time.Now().UTC()
	o.recordObservation(ctx, HealRecord{
		ID:          generateHealID(),
		RuleID:      ruleID,
		TriggerType: "auto_repeated_failure",
		TraceID:     metaStr(meta, "trace_id"),
		SessionID:   sessionID,
		StepID:      stepID,
		Status:      string(HealStatusAlertFired),
		Reason:      rootCause,
		CreatedAt:   now.Format(time.RFC3339),
		Metadata:    meta,
	})

	// Set cooldown for this rule
	o.mu.Lock()
	o.cooldowns[ruleID] = now
	o.mu.Unlock()

	// Notify via AlertNotifier
	if o.notifier != nil {
		o.notifier.Notify(ctx, AlertRule{
			ID:       ruleID,
			Name:     "Self-heal alert: " + ruleID,
			Severity: severity,
		}, map[string]any{
			"rule_id":    ruleID,
			"step_id":    stepID,
			"session_id": sessionID,
			"root_cause": rootCause,
			"severity":   severity,
		})
	}
}

// fireCircuitOpenAlert fires a circuit-open alert and emits a heal_circuit_open FlowLog event.
func (o *SelfHealObserver) fireCircuitOpenAlert(ctx context.Context, ruleID, stepID, sessionID, rootCause, severity string, meta map[string]any) {
	o.lg.Warn("SelfHealObserver: circuit breaker open - too many heal events in sliding window",
		loggateway.StepID("monitor.heal_circuit_open"),
		loggateway.Str("rule_id", ruleID),
		loggateway.Str("step_id", stepID),
		loggateway.Str("severity", severity),
		loggateway.Int("threshold", int(o.healConf.CircuitBreakerThreshold)),
		loggateway.Int64("window_ms", o.healConf.CircuitBreakerWindow.Milliseconds()))

	// Record circuit_open status
	now := time.Now().UTC()
	o.recordObservation(ctx, HealRecord{
		ID:          generateHealID(),
		RuleID:      ruleID,
		TriggerType: "circuit_breaker_open",
		TraceID:     metaStr(meta, "trace_id"),
		SessionID:   sessionID,
		StepID:      stepID,
		Status:      string(HealStatusAlertFired),
		Reason:      "circuit breaker open: " + rootCause,
		CreatedAt:   now.Format(time.RFC3339),
		Metadata:    meta,
	})

	// Set cooldown with 30-minute auto-reset
	o.mu.Lock()
	o.cooldowns[ruleID] = now
	o.mu.Unlock()

	// Notify via AlertNotifier
	if o.notifier != nil {
		o.notifier.Notify(ctx, AlertRule{
			ID:       ruleID,
			Name:     "Circuit breaker open: " + ruleID,
			Severity: severity,
		}, map[string]any{
			"rule_id":             ruleID,
			"step_id":             stepID,
			"session_id":          sessionID,
			"root_cause":          rootCause,
			"severity":            severity,
			"circuit_breaker":     true,
			"window":              o.healConf.CircuitBreakerWindow.String(),
			"threshold":           o.healConf.CircuitBreakerThreshold,
			"auto_reset_after":    o.healConf.CircuitBreakerResetAfter.String(),
		})
	}
}

// checkCooldown returns true if the rule can fire an alert (not in cooldown).
// REQ-SO-08: Cooldown is severity-dependent.
func (o *SelfHealObserver) checkCooldown(ruleID, severity string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	cooldown := o.severityCooldown(severity)

	if last, ok := o.cooldowns[ruleID]; ok {
		return time.Since(last) > cooldown
	}
	return true
}

// severityCooldown returns the cooldown duration for the given severity level
// using the observer's resolved config.
func (o *SelfHealObserver) severityCooldown(severity string) time.Duration {
	switch severity {
	case "critical":
		return o.healConf.SeverityCooldownCritical
	case "high":
		return o.healConf.SeverityCooldownHigh
	case "low":
		return o.healConf.SeverityCooldownLow
	default:
		return o.healConf.SeverityCooldownMedium
	}
}

// DiagnoseAndObserve runs diagnose → root-cause analysis → observe cycle.
// It replaces SelfHealUsecase.DiagnoseAndHeal for the migration period.
// Instead of applying fix actions (which the runtime now handles), it observes
// the outcome and fires alerts for unhealed errors.
func (o *SelfHealObserver) DiagnoseAndObserve(ctx context.Context, traceID, sessionID, stepID, triggerType string, contextMinutes int32) (*HealRecord, error) {
	if o == nil {
		return nil, kerrors.InternalServer("MONITOR", "SelfHealObserver is nil")
	}

	// Run root cause analysis using the engine
	causes := o.engine.Evaluate(ctx, stepID, "error", map[string]any{
		"trace_id":   traceID,
		"session_id": sessionID,
		"step_id":    stepID,
	})

	now := time.Now().UTC()
	record := &HealRecord{
		ID:          generateHealID(),
		TriggerType: triggerType,
		TraceID:     traceID,
		SessionID:   sessionID,
		StepID:      stepID,
		CreatedAt:   now.Format(time.RFC3339),
	}

	if len(causes) == 0 {
		record.Status = string(HealStatusSkippedNoAction)
		record.Reason = "no root causes identified"
		o.recordObservation(ctx, *record)
		return record, nil
	}

	best := o.pickBestCause(causes)
	if best == nil {
		record.Status = string(HealStatusSkippedNoAction)
		record.Reason = "no actionable root cause found"
		o.recordObservation(ctx, *record)
		return record, nil
	}

	record.RuleID = best.RuleID
	record.Confidence = best.Confidence
	record.FixAction = best.FixAction
	record.Metadata = best.Metadata
	record.RuntimeAutoHealed = best.RuntimeAutoHealed
	record.RuntimeHealAttempts = best.RuntimeHealAttempts

	if best.RuntimeAutoHealed {
		// Runtime already attempted auto-heal
		if best.Confidence >= o.healConf.MinConfidence {
			record.Status = string(HealStatusObservedHealed)
			record.Reason = "runtime auto-heal observed, root cause identified"
		} else {
			record.Status = string(HealStatusObservedFailed)
			record.Reason = "runtime auto-heal observed but low confidence"
		}
	} else {
		// No runtime auto-heal attempted → observe as failed
		record.Status = string(HealStatusObservedFailed)
		record.Reason = best.RootCause

		// Fire alert for unhealed error if confidence is high enough
		if best.Confidence >= o.healConf.MinConfidence {
			if o.checkCooldown(best.RuleID, best.Severity) {
				o.fireAlert(ctx, best.RuleID, stepID, sessionID, best.RootCause, best.Severity, best.Metadata)
			}
		}
	}

	o.recordObservation(ctx, *record)
	return record, nil
}

func (o *SelfHealObserver) pickBestCause(causes []RootCauseResult) *RootCauseResult {
	var best *RootCauseResult
	for i := range causes {
		c := &causes[i]
		if best == nil || c.Confidence > best.Confidence {
			best = c
		}
	}
	return best
}

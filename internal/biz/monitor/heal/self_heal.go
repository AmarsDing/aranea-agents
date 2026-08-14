package heal

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// HealStatus represents the outcome of a self-heal action.
type HealStatus string

const (
	HealStatusApplied              HealStatus = "applied"
	HealStatusSkippedLowConfidence HealStatus = "skipped_low_confidence"
	HealStatusSkippedCooldown      HealStatus = "skipped_cooldown"
	HealStatusSkippedNoAction      HealStatus = "skipped_no_action"
	HealStatusFailed               HealStatus = "failed"
	HealStatusObservedHealed       HealStatus = "observed_healed"
	HealStatusObservedFailed       HealStatus = "observed_failed"
	HealStatusAlertFired           HealStatus = "alert_fired"
)

// HealRecord documents a self-heal action taken.
type HealRecord struct {
	ID                  string         `json:"id"`
	RuleID              string         `json:"rule_id"`
	TriggerType         string         `json:"trigger_type"`
	TraceID             string         `json:"trace_id"`
	SessionID           string         `json:"session_id"`
	StepID              string         `json:"step_id"`
	FixAction           FixAction      `json:"fix_action"`
	Confidence          float64        `json:"confidence"`
	Status              string         `json:"status"` // HealStatus values: applied, skipped_low_confidence, skipped_cooldown, skipped_no_action, failed, observed_healed, observed_failed, alert_fired
	RuntimeAutoHealed   bool           `json:"runtime_auto_healed"`
	RuntimeHealAttempts int            `json:"runtime_heal_attempts"`
	Reason              string         `json:"reason,omitempty"`
	CreatedAt           string         `json:"created_at"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// HealActionHandler is the port for executing fix actions.
// Implementations are injected via Wire to decouple biz from runtime specifics.
type HealActionHandler interface {
	HandleFixAction(ctx context.Context, action FixAction, metadata map[string]any) error
}

// HealRecordRepo persists heal records to SQLite via Ent.
type HealRecordRepo interface {
	InsertHealRecord(ctx context.Context, record HealRecord) error
	ListHealRecords(ctx context.Context, query HealRecordQuery) (HealRecordListResult, error)
	DeleteHealRecordsOlderThan(ctx context.Context, olderThan time.Time) (int, error)
}

// HealRecordQuery filters heal records for listing.
type HealRecordQuery struct {
	RuleID    string
	Status    string
	SessionID string
	Limit     int
	Offset    int
}

// HealRecordListResult is a paginated heal record list.
type HealRecordListResult struct {
	Items []HealRecord
	Total int
}

// HealStats aggregates self-heal statistics.
type HealStats struct {
	TotalHeals   int             `json:"total_heals"`
	SuccessRate  float64         `json:"success_rate"`
	TopFailRules []RuleFailCount `json:"top_fail_rules,omitempty"`
}

// RuleFailCount tracks repeated failures for a single rule.
type RuleFailCount struct {
	RuleID string `json:"rule_id"`
	Count  int    `json:"count"`
}

// SelfHealUsecase is the Phase 1 self-healing implementation.
// TODO(debt): DEV-06 — Migrate execution logic to SelfHealObserver (Phase 2).
// SelfHealUsecase will become a thin wrapper delegating to SelfHealObserver.
//
// Deprecated: Use SelfHealObserver instead. SelfHealUsecase is kept for backward
// compatibility during the migration period. The observation role (tracking auto-heal
// outcomes, firing alerts) has moved to SelfHealObserver. The execution role (applying
// fix actions) is now handled by the trpc-agent-go runtime.
type SelfHealUsecase struct {
	diag      *DiagBundleGenerator
	handler   HealActionHandler
	history   []HealRecord
	mu        sync.Mutex
	lg        loggateway.Logger
	cooldowns map[string]time.Time // ruleID → last heal time
}

// NewSelfHealUsecase creates a new self-heal usecase.
// Both diag and handler must not be nil.
func NewSelfHealUsecase(diag *DiagBundleGenerator, handler HealActionHandler, lg loggateway.Logger) *SelfHealUsecase {
	if diag == nil || handler == nil {
		return nil
	}
	return &SelfHealUsecase{
		diag:      diag,
		handler:   handler,
		history:   make([]HealRecord, 0, SelfHealMaxHistory),
		cooldowns: make(map[string]time.Time),
		lg:        lg,
	}
}

// DiagnoseAndHeal runs the full diagnose → root-cause → auto-fix loop.
// It generates a diagnostic bundle, evaluates root causes, and applies
// the highest-confidence fix action if it exceeds the confidence threshold.
func (uc *SelfHealUsecase) DiagnoseAndHeal(ctx context.Context, traceID, sessionID, runID, stepID, triggerType string, contextMinutes int32) (*HealRecord, error) {
	if uc == nil {
		return nil, apierror.Internal("MONITOR", "SelfHealUsecase is nil")
	}

	// Step 1: Generate diagnostic bundle (includes root cause analysis)
	bundle, err := uc.diag.Generate(ctx, traceID, sessionID, runID, stepID, triggerType, contextMinutes)
	if err != nil {
		return nil, err
	}

	// Step 2: Find the best fixable root cause
	record := uc.evaluateAndFix(ctx, bundle.RootCauses, traceID, sessionID, stepID, triggerType)
	return record, nil
}

// evaluateAndFix picks the highest-confidence actionable root cause and applies the fix.
func (uc *SelfHealUsecase) evaluateAndFix(ctx context.Context, causes []RootCauseResult, traceID, sessionID, stepID, triggerType string) *HealRecord {
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
		uc.recordHeal(record)
		return record
	}

	// Pick the highest-confidence cause with a fixable action
	best := uc.pickBestFixableCause(causes)
	if best == nil {
		record.Status = string(HealStatusSkippedNoAction)
		record.Reason = "no fixable root cause with sufficient confidence"
		uc.recordHeal(record)
		return record
	}

	record.RuleID = best.RuleID
	record.Confidence = best.Confidence
	record.FixAction = best.FixAction
	record.Metadata = best.Metadata

	// Check confidence threshold
	if best.Confidence < SelfHealMinConfidence {
		record.Status = string(HealStatusSkippedLowConfidence)
		record.Reason = "confidence below threshold"
		uc.recordHeal(record)
		return record
	}

	// Check cooldown
	if !uc.checkCooldown(best.RuleID) {
		record.Status = string(HealStatusSkippedCooldown)
		record.Reason = "same rule recently applied, in cooldown period"
		uc.recordHeal(record)
		return record
	}

	// Apply fix action (if handler available)
	if uc.handler == nil {
		record.Status = string(HealStatusObservedFailed)
		record.Reason = "no heal action handler (runtime handles healing)"
		uc.recordHeal(record)
		return record
	}

	if err := uc.handler.HandleFixAction(ctx, best.FixAction, best.Metadata); err != nil {
		record.Status = string(HealStatusFailed)
		record.Reason = err.Error()
		uc.lg.Error("SelfHeal: fix action failed",
			loggateway.StepID("monitor.self_heal_fix_fail"),
			loggateway.Str("rule_id", best.RuleID),
			loggateway.Err(err))
		uc.recordHeal(record)
		return record
	}

	record.Status = string(HealStatusApplied)
	uc.setCooldown(best.RuleID, now)
	uc.lg.Info("SelfHeal: fix action applied",
		loggateway.StepID("monitor.self_heal_applied"),
		loggateway.Str("rule_id", best.RuleID),
		loggateway.Str("action_type", best.FixAction.Type),
		loggateway.Float64("confidence", best.Confidence))
	uc.recordHeal(record)
	return record
}

// pickBestFixableCause returns the highest-confidence cause with an actionable fix.
func (uc *SelfHealUsecase) pickBestFixableCause(causes []RootCauseResult) *RootCauseResult {
	var best *RootCauseResult
	for i := range causes {
		c := &causes[i]
		if c.FixAction.Type == "" || c.FixAction.Type == "log_only" {
			continue
		}
		if c.FixAction.MaxAttempts <= 0 {
			continue
		}
		if best == nil || c.Confidence > best.Confidence {
			best = c
		}
	}
	return best
}

// checkCooldown returns true if the rule can be applied (not in cooldown).
func (uc *SelfHealUsecase) checkCooldown(ruleID string) bool {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if last, ok := uc.cooldowns[ruleID]; ok {
		return time.Since(last) > SelfHealCooldownSec*time.Second
	}
	return true
}

// setCooldown marks a rule as recently applied.
func (uc *SelfHealUsecase) setCooldown(ruleID string, t time.Time) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.cooldowns[ruleID] = t
}

// recordHeal appends a heal record to history, trimming if needed.
func (uc *SelfHealUsecase) recordHeal(record *HealRecord) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.history = append(uc.history, *record)
	if len(uc.history) > SelfHealMaxHistory {
		uc.history = uc.history[len(uc.history)-SelfHealMaxHistory:]
	}
}

// ListHealRecords returns recent heal records.
func (uc *SelfHealUsecase) ListHealRecords(limit, offset int) ([]HealRecord, int) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	total := len(uc.history)
	if offset >= total {
		return nil, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	result := make([]HealRecord, end-offset)
	copy(result, uc.history[offset:end])
	return result, total
}

// StartEventDrivenHealing subscribes to FlowLog error events and auto-heals.
// This is the main entry point for the event-driven self-heal loop.
func (uc *SelfHealUsecase) StartEventDrivenHealing(ctx context.Context, ch <-chan Envelope) {
	safego.Go(ctx, "self-heal-event-driven", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-ch:
				if !ok {
					return
				}
				uc.processErrorEnvelope(ctx, env)
			}
		}
	})
}

// Envelope is a minimal interface for the event envelope consumed by self-heal.
type Envelope interface {
	GetMetadata() map[string]any
	GetType() string
}

// processErrorEnvelope checks if an envelope represents an error and triggers healing.
func (uc *SelfHealUsecase) processErrorEnvelope(ctx context.Context, env Envelope) {
	meta := env.GetMetadata()
	if meta == nil {
		return
	}

	// Only process error-phase flow logs
	phase, _ := meta["flow_phase"].(string)
	if !strings.EqualFold(phase, "error") {
		return
	}

	stepID, _ := meta["step_id"].(string)
	traceID, _ := meta["trace_id"].(string)
	sessionID, _ := meta["session_id"].(string)
	runID, _ := meta["run_id"].(string)

	if stepID == "" {
		return
	}

	// Run diagnose-and-heal asynchronously
	safego.Go(ctx, "self-heal-process-error", func() {
		_, err := uc.DiagnoseAndHeal(ctx, traceID, sessionID, runID, stepID, "auto_error_event", 5)
		if err != nil {
			uc.lg.Error("SelfHeal: auto-heal from event failed",
				loggateway.StepID("monitor.self_heal_event_fail"),
				loggateway.Err(err))
		}
	})
}

func generateHealID() string {
	return "heal-" + uuid.NewString()
}

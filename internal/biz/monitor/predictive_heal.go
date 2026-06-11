package monitor

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// PredictiveHealMinConfidence is the confidence threshold above which preventive
// actions are executed. Below this threshold, predictions are recorded but no
// action is taken.
const PredictiveHealMinConfidence = 0.8

// PredictiveHealCooldown is the cooldown period for the same action type.
const PredictiveHealCooldown = 30 * time.Minute

// SystemMetrics represents the current system state used for prediction.
type SystemMetrics struct {
	ProviderLatencyMs int64   `json:"provider_latency_ms"`
	MemoryUsagePct    float64 `json:"memory_usage_pct"`
	SessionBacklog    int     `json:"session_backlog"`
}

// SystemMetricsReader reads live system metrics for predictive analysis.
type SystemMetricsReader interface {
	ReadSystemMetrics(ctx context.Context) (SystemMetrics, error)
}

// PredictiveHealUsecase predicts upcoming errors based on historical patterns
// and takes preventive action when confidence exceeds the threshold.
type PredictiveHealUsecase struct {
	metricsReader  SystemMetricsReader
	patternReader  FailurePatternReader
	handler        HealActionHandler
	repo           HealRecordRepo
	lg             loggateway.Logger

	mu        sync.Mutex
	cooldowns map[string]time.Time // actionType → last heal time
}

// NewPredictiveHealUsecase creates a new predictive heal usecase.
// All dependencies must be non-nil.
func NewPredictiveHealUsecase(
	metricsReader SystemMetricsReader,
	patternReader FailurePatternReader,
	handler HealActionHandler,
	repo HealRecordRepo,
	lg loggateway.Logger,
) *PredictiveHealUsecase {
	if metricsReader == nil || patternReader == nil || handler == nil || repo == nil {
		return nil
	}
	return &PredictiveHealUsecase{
		metricsReader: metricsReader,
		patternReader: patternReader,
		handler:       handler,
		repo:          repo,
		lg:            lg,
		cooldowns:     make(map[string]time.Time),
	}
}

// PredictAndHeal reads system metrics, matches active failure patterns, and
// executes preventive actions for patterns with confidence > 0.8.
// All actions (applied, skipped, failed) are recorded as HealRecords for audit.
func (uc *PredictiveHealUsecase) PredictAndHeal(ctx context.Context) ([]HealRecord, error) {
	if uc == nil {
		return nil, apierror.Internal("MONITOR", "PredictiveHealUsecase is nil")
	}

	// Prune stale cooldowns before processing
	uc.pruneStaleCooldowns()

	metrics, err := uc.metricsReader.ReadSystemMetrics(ctx)
	if err != nil {
		return nil, err
	}

	patterns, err := uc.patternReader.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	var records []HealRecord
	for _, pattern := range patterns {
		if !pattern.IsActive {
			continue
		}

		confidence := uc.calculateConfidence(pattern, metrics)
		if confidence <= 0 {
			continue
		}

		now := time.Now().UTC()
		record := HealRecord{
			ID:          generateHealID(),
			RuleID:      pattern.ID,
			TriggerType: "predictive",
			FixAction:   pattern.FixAction,
			Confidence:  confidence,
			CreatedAt:   now.Format(time.RFC3339),
			Metadata: map[string]any{
				"pattern_type":        pattern.Type,
				"provider_latency_ms": metrics.ProviderLatencyMs,
				"memory_usage_pct":    metrics.MemoryUsagePct,
				"session_backlog":     metrics.SessionBacklog,
				"source":              string(pattern.Source),
			},
		}

		// Check confidence threshold
		if confidence < PredictiveHealMinConfidence {
			record.Status = string(HealStatusSkippedLowConfidence)
			record.Reason = "prediction confidence below threshold"
			uc.persistRecord(ctx, record)
			records = append(records, record)
			continue
		}

		// Check cooldown
		if !uc.checkCooldown(pattern.Type) {
			record.Status = string(HealStatusSkippedCooldown)
			record.Reason = "same action type recently applied, in cooldown period"
			uc.persistRecord(ctx, record)
			records = append(records, record)
			continue
		}

		// Execute preventive action
		if err := uc.handler.HandleFixAction(ctx, pattern.FixAction, record.Metadata); err != nil {
			record.Status = string(HealStatusFailed)
			record.Reason = err.Error()
			uc.lg.Error("PredictiveHeal: preventive action failed",
				loggateway.StepID("monitor.predictive_heal_fail"),
				loggateway.Str("pattern_id", pattern.ID),
				loggateway.Str("action_type", pattern.FixAction.Type),
				loggateway.Err(err))
			uc.persistRecord(ctx, record)
			records = append(records, record)
			continue
		}

		record.Status = string(HealStatusApplied)
		uc.setCooldown(pattern.Type, now)
		uc.lg.Info("PredictiveHeal: preventive action applied",
			loggateway.StepID("monitor.predictive_heal_applied"),
			loggateway.Str("pattern_id", pattern.ID),
			loggateway.Str("action_type", pattern.FixAction.Type),
			loggateway.Float64("confidence", confidence))
		uc.persistRecord(ctx, record)
		records = append(records, record)
	}

	return records, nil
}

// calculateConfidence computes the prediction confidence for a given pattern
// based on how well current system metrics match the pattern's preconditions.
// Returns 0 if the pattern does not match current metrics at all.
func (uc *PredictiveHealUsecase) calculateConfidence(pattern FailurePattern, metrics SystemMetrics) float64 {
	baseConfidence := pattern.Confidence
	if baseConfidence <= 0 {
		return 0
	}

	boost := 0.0

	// Pattern type → metric matching
	switch pattern.Type {
	case "provider_timeout":
		// High latency is a strong signal for upcoming provider timeouts
		if metrics.ProviderLatencyMs > 3000 {
			boost += 0.15
		} else if metrics.ProviderLatencyMs > 1500 {
			boost += 0.05
		}
		if metrics.SessionBacklog > 20 {
			boost += 0.05
		}
	case "memory_pressure":
		// High memory usage is a strong signal for memory pressure
		if metrics.MemoryUsagePct > 85 {
			boost += 0.15
		} else if metrics.MemoryUsagePct > 70 {
			boost += 0.05
		}
	case "session_overload":
		// High session backlog signals session overload
		if metrics.SessionBacklog > 30 {
			boost += 0.15
		} else if metrics.SessionBacklog > 15 {
			boost += 0.05
		}
	default:
		// Unknown pattern types get no metric boost but still use base confidence
	}

	confidence := baseConfidence + boost
	if confidence > 1.0 {
		confidence = 1.0
	}
	return confidence
}

// checkCooldown returns true if the action type can be applied (not in cooldown).
func (uc *PredictiveHealUsecase) checkCooldown(actionType string) bool {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if last, ok := uc.cooldowns[actionType]; ok {
		return time.Since(last) > PredictiveHealCooldown
	}
	return true
}

// setCooldown marks an action type as recently applied.
func (uc *PredictiveHealUsecase) setCooldown(actionType string, t time.Time) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.cooldowns[actionType] = t
}

// persistRecord saves a heal record to the persistent store.
func (uc *PredictiveHealUsecase) persistRecord(ctx context.Context, record HealRecord) {
	if uc.repo == nil {
		return
	}
	if err := uc.repo.InsertHealRecord(ctx, record); err != nil {
		uc.lg.Error("PredictiveHeal: failed to persist heal record",
			loggateway.StepID("monitor.predictive_heal_persist_fail"),
			loggateway.Str("rule_id", record.RuleID),
			loggateway.Err(err))
	}
}

// pruneStaleCooldowns removes cooldown entries that expired more than twice
// the cooldown duration ago, preventing unbounded map growth.
func (uc *PredictiveHealUsecase) pruneStaleCooldowns() {
	if uc == nil {
		return
	}
	now := time.Now().UTC()
	uc.mu.Lock()
	defer uc.mu.Unlock()
	for actionType, lastTime := range uc.cooldowns {
		if now.Sub(lastTime) > PredictiveHealCooldown*2 {
			delete(uc.cooldowns, actionType)
		}
	}
}

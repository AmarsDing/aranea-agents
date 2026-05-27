// Package conf provides runtime configuration helpers.
package conf

import (
	"os"
	"strings"
)

// BLO-PRE-02: M56 Business Logic Optimization feature flags.
// All flags default to false (off). Enable in dev/staging by setting the env var to "1" or "true".
// Never enable BLO_UNIFIED_JOB_ENABLED in production until Sprint A3 gate passes.

// BLOUnifiedJobEnabled controls the Unified BackgroundJob subsystem (BLO-5).
// When true, new jobs are written to the background_jobs table via BackgroundJobRepo.
func BLOUnifiedJobEnabled() bool {
	return parseBoolFlag("BLO_UNIFIED_JOB_ENABLED")
}

// BLOPendingTaskV2 controls Non-Blocking HITL via PendingTask async (BLO-4).
func BLOPendingTaskV2() bool {
	return parseBoolFlag("BLO_PENDING_TASK_V2")
}

// BLOEscalationV2 controls Multi-Signal Escalation (BLO-2).
func BLOEscalationV2() bool {
	return parseBoolFlag("BLO_ESCALATION_V2")
}

// BLOIntentClassifier controls Intent-Aware Admission (BLO-1).
func BLOIntentClassifier() bool {
	return parseBoolFlag("BLO_INTENT_CLASSIFIER")
}

// BLOTriggerRules controls Channel Trigger Rules (BLO-3).
func BLOTriggerRules() bool {
	return parseBoolFlag("BLO_TRIGGER_RULES")
}

func parseBoolFlag(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

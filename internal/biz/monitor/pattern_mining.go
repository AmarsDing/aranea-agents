package monitor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// PatternMiningMinSuccesses is the minimum number of successful fixes required
// in a cluster to generate a mined fix template.
const PatternMiningMinSuccesses = 3

// PatternMiningInitialConfidence is the starting confidence for newly mined patterns.
const PatternMiningInitialConfidence = 0.5

// PatternMiningPromotedConfidence is the confidence after 3+ successful verifications.
const PatternMiningPromotedConfidence = 0.8

// PatternMiningAutoDisableRatio is the fail/success ratio threshold for auto-disable.
// A pattern is deactivated when fail_count > success_count * PatternMiningAutoDisableRatio.
const PatternMiningAutoDisableRatio = 2

// PatternMiningResult summarizes the outcome of a mining run.
type PatternMiningResult struct {
	PatternsCreated    int
	PatternsUpdated    int
	PatternsDeactivated int
	ClustersAnalyzed   int
}

// PatternMiningUsecase automatically extracts fix templates from historical
// repair records and dynamically updates the knowledge base.
//
// It reads HealRecords (runtime repair records), clusters similar failure modes
// (same error_code + similar stack_trace), and generates fix templates when a
// cluster has >= 3 successful fixes. Mined patterns are written to the
// failure_pattern table with source="mined".
type PatternMiningUsecase struct {
	healRepo       HealRecordRepo
	patternReader  FailurePatternReader
	patternWriter  FailurePatternWriter
	lg             loggateway.Logger
}

// NewPatternMiningUsecase creates a new PatternMiningUsecase.
// All dependencies must be non-nil.
func NewPatternMiningUsecase(
	healRepo HealRecordRepo,
	patternReader FailurePatternReader,
	patternWriter FailurePatternWriter,
	lg loggateway.Logger,
) *PatternMiningUsecase {
	if healRepo == nil || patternReader == nil || patternWriter == nil {
		return nil
	}
	return &PatternMiningUsecase{
		healRepo:      healRepo,
		patternReader: patternReader,
		patternWriter: patternWriter,
		lg:            lg,
	}
}

// Mine executes one mining cycle: reads heal records, clusters them, and
// generates or updates mined failure patterns.
func (uc *PatternMiningUsecase) Mine(ctx context.Context) (PatternMiningResult, error) {
	if uc == nil {
		return PatternMiningResult{}, kerrors.InternalServer("MONITOR", "PatternMiningUsecase is nil")
	}

	// Step 1: Read applied heal records
	result, err := uc.healRepo.ListHealRecords(ctx, HealRecordQuery{
		Status: string(HealStatusApplied),
		Limit:  1000,
	})
	if err != nil {
		uc.lg.Error("PatternMining: failed to list heal records",
			loggateway.StepID("monitor.pattern_mining_list_fail"),
			loggateway.Err(err))
		return PatternMiningResult{}, err
	}

	if len(result.Items) == 0 {
		return PatternMiningResult{}, nil
	}

	// Step 2: Cluster by error_code + normalized stack_trace
	clusters := uc.clusterRecords(result.Items)

	// Step 3: Process each cluster
	var miningResult PatternMiningResult
	miningResult.ClustersAnalyzed = len(clusters)

	for i := range clusters {
		cluster := &clusters[i]
		if len(cluster.appliedRecords) < PatternMiningMinSuccesses {
			continue
		}

		// Compute pattern hash for this cluster
		hash := MinedPatternHash(cluster.errorCode, cluster.normalizedStack)

		// Check if pattern already exists
		existing, err := uc.patternReader.GetByPatternHash(ctx, hash)
		if err != nil {
			uc.lg.Warn("PatternMining: failed to check existing pattern",
				loggateway.StepID("monitor.pattern_mining_check_fail"),
				loggateway.Str("hash", hash),
				loggateway.Err(err))
			continue
		}

		if existing != nil {
			// Update existing pattern
			updated := uc.updateExistingPattern(existing, cluster)
			if err := uc.patternWriter.Update(ctx, updated); err != nil {
				uc.lg.Warn("PatternMining: failed to update pattern",
					loggateway.StepID("monitor.pattern_mining_update_fail"),
					loggateway.Str("id", existing.ID),
					loggateway.Err(err))
				continue
			}
			miningResult.PatternsUpdated++
			if !updated.IsActive && existing.IsActive {
				miningResult.PatternsDeactivated++
			}
		} else {
			// Create new mined pattern
			pattern := uc.createNewPattern(cluster, hash)
			if err := uc.patternWriter.Create(ctx, pattern); err != nil {
				uc.lg.Warn("PatternMining: failed to create pattern",
					loggateway.StepID("monitor.pattern_mining_create_fail"),
					loggateway.Str("hash", hash),
					loggateway.Err(err))
				continue
			}
			miningResult.PatternsCreated++
		}
	}

	uc.lg.Info("PatternMining: mining cycle complete",
		loggateway.StepID("monitor.pattern_mining_done"),
		loggateway.Int("records_analyzed", len(result.Items)),
		loggateway.Int("clusters", miningResult.ClustersAnalyzed),
		loggateway.Int("created", miningResult.PatternsCreated),
		loggateway.Int("updated", miningResult.PatternsUpdated),
		loggateway.Int("deactivated", miningResult.PatternsDeactivated))

	return miningResult, nil
}

// failureCluster groups HealRecords by error_code and normalized stack trace.
type failureCluster struct {
	errorCode       string
	normalizedStack string
	appliedRecords  []HealRecord
	failedRecords   []HealRecord
	fixAction       FixAction
}

// clusterRecords groups heal records by error_code + top N frames of stack trace.
func (uc *PatternMiningUsecase) clusterRecords(records []HealRecord) []failureCluster {
	clusterMap := make(map[string]*failureCluster)

	for _, rec := range records {
		errorCode := metaStrFromMap(rec.Metadata, "error_code")
		stackTrace := metaStrFromMap(rec.Metadata, "stack_trace")
		normalizedStack := normalizeStackTrace(stackTrace)

		key := errorCode + "\x00" + normalizedStack
		cluster, ok := clusterMap[key]
		if !ok {
			cluster = &failureCluster{
				errorCode:       errorCode,
				normalizedStack: normalizedStack,
			}
			clusterMap[key] = cluster
		}

		if rec.Status == string(HealStatusApplied) {
			cluster.appliedRecords = append(cluster.appliedRecords, rec)
			// Use the fix action from the most recent applied record
			if rec.FixAction.Type != "" {
				cluster.fixAction = rec.FixAction
			}
		} else if rec.Status == string(HealStatusFailed) {
			cluster.failedRecords = append(cluster.failedRecords, rec)
		}
	}

	result := make([]failureCluster, 0, len(clusterMap))
	for _, c := range clusterMap {
		result = append(result, *c)
	}
	return result
}

// createNewPattern creates a new FailurePattern from a cluster.
func (uc *PatternMiningUsecase) createNewPattern(cluster *failureCluster, hash string) FailurePattern {
	now := time.Now().UTC()
	return FailurePattern{
		ID:           fmt.Sprintf("fp-mined-%s", hash[:16]),
		Source:       FailurePatternSourceMined,
		Type:         cluster.errorCode,
		PatternHash:  hash,
		PatternRegex: cluster.errorCode,
		FixAction:    cluster.fixAction,
		Confidence:   PatternMiningInitialConfidence,
		SuccessCount: len(cluster.appliedRecords),
		FailCount:    len(cluster.failedRecords),
		Version:      1,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// updateExistingPattern updates an existing FailurePattern with new cluster data.
func (uc *PatternMiningUsecase) updateExistingPattern(existing *FailurePattern, cluster *failureCluster) FailurePattern {
	updated := *existing
	updated.SuccessCount = existing.SuccessCount + len(cluster.appliedRecords)
	updated.FailCount = existing.FailCount + len(cluster.failedRecords)
	updated.Version = existing.Version + 1
	updated.UpdatedAt = time.Now().UTC()

	// Confidence promotion: after 3+ total successes, promote to 0.8
	if updated.SuccessCount >= PatternMiningMinSuccesses {
		updated.Confidence = PatternMiningPromotedConfidence
	}

	// Auto-disable: fail_count > success_count * 2
	// This checks the total (existing + new) counts
	if updated.FailCount > updated.SuccessCount*PatternMiningAutoDisableRatio {
		updated.IsActive = false
	}

	return updated
}

// normalizeStackTrace extracts the top N frames from a stack trace for clustering.
// This allows similar but not identical stack traces to be grouped together.
func normalizeStackTrace(stackTrace string) string {
	if stackTrace == "" {
		return ""
	}
	lines := strings.Split(stackTrace, "\n")
	maxFrames := 3
	if len(lines) < maxFrames {
		maxFrames = len(lines)
	}
	// Take top N frames and trim line numbers for normalization
	normalized := make([]string, maxFrames)
	for i := 0; i < maxFrames; i++ {
		line := strings.TrimSpace(lines[i])
		// Remove line numbers (e.g., "runtime/llm.go:42" → "runtime/llm.go")
		if idx := strings.LastIndex(line, ":"); idx > 0 {
			line = line[:idx]
		}
		normalized[i] = line
	}
	return strings.Join(normalized, "\n")
}

// MinedPatternHash computes a deterministic hash for a mined pattern based on
// error_code and normalized stack trace.
func MinedPatternHash(errorCode, normalizedStack string) string {
	h := sha256.New()
	h.Write([]byte("mined"))
	h.Write([]byte{0})
	h.Write([]byte(errorCode))
	h.Write([]byte{0})
	h.Write([]byte(normalizedStack))
	return fmt.Sprintf("sha256:%x", h.Sum(nil))[:48]
}

// metaStrFromMap extracts a string value from a map[string]any, similar to metaStr
// but works with HealRecord.Metadata directly.
func metaStrFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

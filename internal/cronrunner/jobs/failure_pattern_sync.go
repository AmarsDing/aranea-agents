package jobs

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// FailurePatternSyncJob synchronizes RootCauseEngine rules and CI patterns.jsonl
// into the failure_pattern table on a daily basis.
//
// Configurable via environment variables:
//
//	FAILURE_PATTERN_SYNC_INTERVAL — sync tick interval (e.g. "6h", "24h"); default 24h
//	FAILURE_PATTERN_SYNC_FILE     — path to patterns.jsonl; default ".auto-fix/patterns.jsonl"
type FailurePatternSyncJob struct {
	interval     time.Duration
	engine       *heal.RootCauseEngine
	writer       heal.FailurePatternWriter
	reader       heal.FailurePatternReader
	patternsFile string
	lg           loggateway.Logger
}

// ciPatternEntry represents a single line in patterns.jsonl.
type ciPatternEntry struct {
	Type         string               `json:"type"`
	PatternRegex string               `json:"pattern_regex"`
	FixAction    heal.FixAction `json:"fix_action"`
}

func defaultFailurePatternSyncInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("FAILURE_PATTERN_SYNC_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return 24 * time.Hour
}

func defaultFailurePatternSyncFile() string {
	if raw := strings.TrimSpace(os.Getenv("FAILURE_PATTERN_SYNC_FILE")); raw != "" {
		return raw
	}
	return ".auto-fix/patterns.jsonl"
}

// NewFailurePatternSyncJob creates the sync worker.
// Pass interval ≤ 0 to use environment-variable defaults.
func NewFailurePatternSyncJob(
	interval time.Duration,
	engine *heal.RootCauseEngine,
	writer heal.FailurePatternWriter,
	reader heal.FailurePatternReader,
	lg loggateway.Logger,
) *FailurePatternSyncJob {
	if interval <= 0 {
		interval = defaultFailurePatternSyncInterval()
	}
	return &FailurePatternSyncJob{
		interval:     interval,
		engine:       engine,
		writer:       writer,
		reader:       reader,
		patternsFile: defaultFailurePatternSyncFile(),
		lg:           lg,
	}
}

// Start runs the sync loop until ctx is cancelled.
func (j *FailurePatternSyncJob) Start(ctx context.Context) {
	if j == nil || j.writer == nil {
		return
	}
	safego.Go(ctx, "failure_pattern_sync", func() {
		ticker := time.NewTicker(j.interval)
		defer ticker.Stop()
		// Run once immediately
		j.runOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				j.runOnce(ctx)
			}
		}
	})
}

func (j *FailurePatternSyncJob) runOnce(ctx context.Context) {
	runtimeSynced := j.syncRuntimeRules(ctx)
	ciSynced := j.syncCIPatterns(ctx)
	j.lg.Info("FailurePatternSyncJob: sync complete",
		loggateway.StepID("failure_pattern_sync"),
		loggateway.Int("runtime_synced", runtimeSynced),
		loggateway.Int("ci_synced", ciSynced))
}

// syncRuntimeRules syncs RootCauseEngine builtin rules to failure_pattern table.
// Each rule is converted to a FailurePattern with source="runtime" and confidence=0.9.
// If a pattern with the same pattern_hash already exists, it is skipped.
func (j *FailurePatternSyncJob) syncRuntimeRules(ctx context.Context) int {
	if j.engine == nil || j.reader == nil {
		return 0
	}

	rules := j.engine.Rules()
	synced := 0
	now := time.Now().UTC()

	for _, rule := range rules {
		patternRegex := rule.Condition.Pattern
		if patternRegex == "" {
			// Rules without patterns use error codes as the match criteria
			if len(rule.Condition.ErrorCodes) > 0 {
				patternRegex = strings.Join(rule.Condition.ErrorCodes, "|")
			} else {
				continue
			}
		}

		hash := patternHash(string(heal.FailurePatternSourceRuntime), rule.ID, patternRegex)

		existing, err := j.reader.GetByPatternHash(ctx, hash)
		if err != nil {
			j.lg.Warn("FailurePatternSyncJob: failed to check existing pattern",
				loggateway.StepID("failure_pattern_sync_check_fail"),
				loggateway.Str("rule_id", rule.ID),
				loggateway.Err(err))
			continue
		}
		if existing != nil {
			continue // already synced
		}

		pattern := heal.FailurePattern{
			ID:           fmt.Sprintf("fp-rt-%s", rule.ID),
			Source:       heal.FailurePatternSourceRuntime,
			Type:         rule.ID,
			PatternHash:  hash,
			PatternRegex: patternRegex,
			FixAction:    rule.FixAction,
			Confidence:   0.9,
			SuccessCount: 0,
			FailCount:    0,
			Version:      1,
			IsActive:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		if err := j.writer.Create(ctx, pattern); err != nil {
			j.lg.Warn("FailurePatternSyncJob: failed to create runtime pattern",
				loggateway.StepID("failure_pattern_sync_create_fail"),
				loggateway.Str("rule_id", rule.ID),
				loggateway.Err(err))
			continue
		}
		synced++
	}

	return synced
}

// syncCIPatterns reads patterns.jsonl and syncs entries to failure_pattern table.
// Each entry is converted to a FailurePattern with source="ci".
// If a pattern with the same pattern_hash already exists, it is skipped.
func (j *FailurePatternSyncJob) syncCIPatterns(ctx context.Context) int {
	if j.reader == nil {
		return 0
	}

	file, err := os.Open(j.patternsFile)
	if err != nil {
		if !os.IsNotExist(err) {
			j.lg.Warn("FailurePatternSyncJob: failed to open patterns.jsonl",
				loggateway.StepID("failure_pattern_sync_file_fail"),
				loggateway.Str("path", j.patternsFile),
				loggateway.Err(err))
		}
		return 0
	}
	defer file.Close()

	synced := 0
	now := time.Now().UTC()
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry ciPatternEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			j.lg.Warn("FailurePatternSyncJob: failed to parse patterns.jsonl line",
				loggateway.StepID("failure_pattern_sync_parse_fail"),
				loggateway.Int("line", lineNum),
				loggateway.Err(err))
			continue
		}

		if entry.PatternRegex == "" {
			continue
		}

		hash := patternHash(string(heal.FailurePatternSourceCI), entry.Type, entry.PatternRegex)

		existing, err := j.reader.GetByPatternHash(ctx, hash)
		if err != nil {
			j.lg.Warn("FailurePatternSyncJob: failed to check existing CI pattern",
				loggateway.StepID("failure_pattern_sync_ci_check_fail"),
				loggateway.Err(err))
			continue
		}
		if existing != nil {
			continue
		}

		fixAction := entry.FixAction
		if fixAction.Type == "" {
			fixAction = heal.FixAction{Type: "log_only"}
		}

		pattern := heal.FailurePattern{
			ID:           fmt.Sprintf("fp-ci-%d", lineNum),
			Source:       heal.FailurePatternSourceCI,
			Type:         entry.Type,
			PatternHash:  hash,
			PatternRegex: entry.PatternRegex,
			FixAction:    fixAction,
			Confidence:   0.7,
			SuccessCount: 0,
			FailCount:    0,
			Version:      1,
			IsActive:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		if err := j.writer.Create(ctx, pattern); err != nil {
			j.lg.Warn("FailurePatternSyncJob: failed to create CI pattern",
				loggateway.StepID("failure_pattern_sync_ci_create_fail"),
				loggateway.Int("line", lineNum),
				loggateway.Err(err))
			continue
		}
		synced++
	}

	if err := scanner.Err(); err != nil {
		j.lg.Warn("FailurePatternSyncJob: error reading patterns.jsonl",
			loggateway.StepID("failure_pattern_sync_read_fail"),
			loggateway.Err(err))
	}

	return synced
}

// patternHash computes a SHA256 hash for a failure pattern based on source, type, and regex.
func patternHash(source, fpType, patternRegex string) string {
	h := sha256.New()
	h.Write([]byte(source))
	h.Write([]byte{0})
	h.Write([]byte(fpType))
	h.Write([]byte{0})
	h.Write([]byte(patternRegex))
	return fmt.Sprintf("sha256:%x", h.Sum(nil))[:48]
}

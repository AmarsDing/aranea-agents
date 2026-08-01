package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Outcome attribution (73-self-iteration-v3, design D8 / §5 self_improve_outcome) ──
//
// SelfImprovementOutcomeUsecase attributes terminal runs into PatchOutcome
// records and feeds the learning loop:
//
//	verdict 判定：closed→effective；rolled_back→regressed；verify_failed/rejected/failed→neutral
//	regressed → 写 FailurePattern KB 负面样本（error_code + 文件特征哈希）
//	触发器自适应：同一 trigger_source 连续 3 次 neutral/regressed → 冷却期 ×2
//
// metrics_before/after 取自 Watchdog 写入 run.Metadata 的观察窗快照。
// 逐 run 错误吸收（下 tick 重扫），仅列表失败向外返回。

// defaultSIOutcomeBatchSize caps runs attributed per tick.
const defaultSIOutcomeBatchSize = 50

// siTriggerEscalationWindow is the D8 consecutive non-effective count that
// doubles a trigger's cooldown.
const siTriggerEscalationWindow = 3

// SINegativePatternRecord is one regressed-patch anti-pattern for the
// FailurePattern KB (D8).
type SINegativePatternRecord struct {
	RunID         string
	SuggestionID  string
	TriggerSource string
	PatternHash   string
	PatternRegex  string
	Reason        string
}

// SINegativePatternSink writes regressed-patch anti-patterns to the
// FailurePattern KB (source=self_improvement).
// Stability:evolving
type SINegativePatternSink interface {
	RecordNegativePattern(ctx context.Context, rec SINegativePatternRecord) error
}

// SITriggerFeedbackSink escalates a trigger source's cooldown after repeated
// non-effective outcomes (D8 触发器自适应降频).
// Stability:evolving
type SITriggerFeedbackSink interface {
	EscalateTriggerCooldown(ctx context.Context, triggerSource string, factor float64) error
}

// SelfImprovementOutcomeDeps carries the outcome usecase's injected deps.
type SelfImprovementOutcomeDeps struct {
	RunReader SelfImprovementRunReader
	Outcomes  PatchOutcomeWriter
	// Patterns nil → 负面样本反哺降级（仅 outcome 归因）。
	Patterns SINegativePatternSink
	// Feedback nil → 触发器降频降级。
	Feedback SITriggerFeedbackSink
	// BatchSize ≤0 → defaultSIOutcomeBatchSize。
	BatchSize int
	Lg        loggateway.Logger
}

// SelfImprovementOutcomeUsecase attributes terminal runs (Learn stage).
type SelfImprovementOutcomeUsecase struct {
	runReader SelfImprovementRunReader
	outcomes  PatchOutcomeWriter
	patterns  SINegativePatternSink
	feedback  SITriggerFeedbackSink
	batchSize int
	lg        loggateway.Logger
}

// NewSelfImprovementOutcomeUsecase wires the outcome attribution usecase.
func NewSelfImprovementOutcomeUsecase(deps SelfImprovementOutcomeDeps) (*SelfImprovementOutcomeUsecase, error) {
	if deps.RunReader == nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "run reader is required")
	}
	if deps.Outcomes == nil {
		return nil, apierror.BadRequest("SELF_IMPROVEMENT", "outcome writer is required")
	}
	batch := deps.BatchSize
	if batch <= 0 {
		batch = defaultSIOutcomeBatchSize
	}
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SelfImprovementOutcomeUsecase{
		runReader: deps.RunReader, outcomes: deps.Outcomes,
		patterns: deps.Patterns, feedback: deps.Feedback,
		batchSize: batch,
		lg:        lg.With(loggateway.Domain("self_improve_outcome")),
	}, nil
}

// ScanOnce attributes one batch of terminal runs. Per-run failures are
// absorbed (the run resurfaces on the next tick); only the list failure is
// returned.
func (uc *SelfImprovementOutcomeUsecase) ScanOnce(ctx context.Context) error {
	runs, err := uc.runReader.ListTerminalPendingOutcome(ctx, uc.batchSize)
	if err != nil {
		return err
	}
	for i := range runs {
		uc.attribute(ctx, &runs[i])
	}
	return nil
}

// attribute builds and persists the PatchOutcome of one terminal run, then
// runs the D8 feedback loops (KB anti-pattern + trigger cooldown).
func (uc *SelfImprovementOutcomeUsecase) attribute(ctx context.Context, run *SelfImprovementRun) {
	if !IsSelfImprovementRunTerminal(run.Status) {
		return // 双保险：repo 已过滤终态。
	}
	outcome := &PatchOutcome{
		ID:            uuid.NewString(),
		RunID:         run.ID,
		SuggestionID:  run.SuggestionID,
		Verdict:       SIVerdictForStatus(run.Status),
		MetricsBefore: siWatchMetricFromMeta(run.Metadata, siMetaObserveBaseline),
		MetricsAfter:  siWatchMetricFromMeta(run.Metadata, siMetaObserveAfter),
		CreatedAt:     time.Now().UTC(),
	}
	if outcome.Verdict == VerdictRegressed {
		outcome.RollbackReason = run.ClosedReason
		outcome.PatternHash = siOutcomePatternHash(run)
	}
	if err := uc.outcomes.CreateOutcome(ctx, outcome); err != nil {
		uc.lg.Warn("self-improve outcome: create failed, retry next tick",
			loggateway.StepID("si_outcome.create"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	uc.lg.Info("self-improve run attributed",
		loggateway.StepID("si_outcome.attributed"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("verdict", string(outcome.Verdict)))

	if outcome.Verdict == VerdictRegressed && uc.patterns != nil {
		rec := SINegativePatternRecord{
			RunID: run.ID, SuggestionID: run.SuggestionID,
			TriggerSource: run.TriggerSource,
			PatternHash:   outcome.PatternHash,
			PatternRegex:  siOutcomePatternRegex(run),
			Reason:        run.ClosedReason,
		}
		if err := uc.patterns.RecordNegativePattern(ctx, rec); err != nil {
			uc.lg.Warn("self-improve outcome: negative pattern degraded",
				loggateway.StepID("si_outcome.pattern"),
				loggateway.Str("run_id", run.ID), loggateway.Err(err))
		}
	}
	uc.adaptTrigger(ctx, run, outcome)
}

// adaptTrigger doubles the trigger's cooldown when the newest
// siTriggerEscalationWindow outcomes of its source are all non-effective (D8).
// The just-created outcome is prepended defensively (read-after-write may lag
// on split pools).
func (uc *SelfImprovementOutcomeUsecase) adaptTrigger(ctx context.Context, run *SelfImprovementRun, fresh *PatchOutcome) {
	if uc.feedback == nil || run.TriggerSource == "" {
		return
	}
	recent, err := uc.outcomes.ListRecentOutcomesByTrigger(ctx, run.TriggerSource, siTriggerEscalationWindow)
	if err != nil {
		uc.lg.Warn("self-improve outcome: trigger feedback query degraded",
			loggateway.StepID("si_outcome.feedback"),
			loggateway.Str("run_id", run.ID), loggateway.Err(err))
		return
	}
	window := make([]PatchOutcome, 0, siTriggerEscalationWindow)
	window = append(window, *fresh)
	for _, o := range recent {
		if o.ID == fresh.ID {
			continue
		}
		window = append(window, o)
		if len(window) >= siTriggerEscalationWindow {
			break
		}
	}
	if len(window) < siTriggerEscalationWindow {
		return
	}
	for _, o := range window {
		if o.Verdict == VerdictEffective {
			return
		}
	}
	if err := uc.feedback.EscalateTriggerCooldown(ctx, run.TriggerSource, 2.0); err != nil {
		uc.lg.Warn("self-improve outcome: cooldown escalation degraded",
			loggateway.StepID("si_outcome.feedback"),
			loggateway.Str("trigger_source", run.TriggerSource), loggateway.Err(err))
		return
	}
	uc.lg.Info("self-improve trigger cooldown escalated",
		loggateway.StepID("si_outcome.feedback"),
		loggateway.Str("trigger_source", run.TriggerSource),
		loggateway.Int("window", siTriggerEscalationWindow))
}

// SIVerdictForStatus maps a terminal run status to its D8 verdict.
func SIVerdictForStatus(status SelfImprovementRunStatus) SelfImprovementVerdict {
	switch status {
	case RunStatusClosed:
		return VerdictEffective
	case RunStatusRolledBack:
		return VerdictRegressed
	default:
		return VerdictNeutral
	}
}

// siOutcomePatternHash hashes (trigger_source | sorted patch files) as the
// regressed-patch pattern identity (D8: error_code + 文件特征哈希).
func siOutcomePatternHash(run *SelfImprovementRun) string {
	files := siOutcomePatchFiles(run)
	h := sha256.Sum256([]byte(run.TriggerSource + "|" + strings.Join(files, ",")))
	return hex.EncodeToString(h[:])[:32]
}

// siOutcomePatternRegex matches the first touched file (KB matcher requires a
// non-empty regex; the hash carries the real identity).
func siOutcomePatternRegex(run *SelfImprovementRun) string {
	files := siOutcomePatchFiles(run)
	if len(files) == 0 {
		return `self_improvement\regressed`
	}
	return regexp.QuoteMeta(files[0])
}

// siOutcomePatchFiles extracts the sorted touched-file list from run.Diff.
func siOutcomePatchFiles(run *SelfImprovementRun) []string {
	changes := ParseUnifiedDiffFiles(run.Diff)
	files := make([]string, 0, len(changes))
	for _, c := range changes {
		files = append(files, c.Path)
	}
	sort.Strings(files)
	return files
}

// siWatchMetricFromMeta reads one metrics snapshot key from run.Metadata.
func siWatchMetricFromMeta(raw json.RawMessage, key string) *MetricsSnapshot {
	if len(raw) == 0 {
		return nil
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil
	}
	sub, ok := meta[key]
	if !ok {
		return nil
	}
	var snap MetricsSnapshot
	if err := json.Unmarshal(sub, &snap); err != nil {
		return nil
	}
	return &snap
}

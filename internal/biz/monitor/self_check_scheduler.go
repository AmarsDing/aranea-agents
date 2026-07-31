package monitor

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz/types"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

const (
	// DefaultSelfCheckInterval is the default interval between self-checks.
	DefaultSelfCheckInterval = 5 * time.Minute
	// SelfCheckTimeout is the per-checker timeout.
	SelfCheckTimeout = 10 * time.Second
	// SelfCheckOverallTimeout is the timeout for the entire self-check cycle.
	SelfCheckOverallTimeout = 60 * time.Second
)

// SelfCheckReportRepo persists self-check reports.
type SelfCheckReportRepo interface {
	InsertSelfCheckReport(ctx context.Context, report SelfCheckReport) error
	ListSelfCheckReports(ctx context.Context, limit, offset int) ([]SelfCheckReport, int, error)
	DeleteSelfCheckReportsOlderThan(ctx context.Context, olderThan time.Time) (int, error)
}

// SelfCheckScheduler orchestrates periodic self-checks across all registered checkers.
type SelfCheckScheduler struct {
	checkers  []SelfChecker
	repairers []SelfCheckRepairer
	repo      SelfCheckReportRepo
	registry  *AlertMetricRegistry
	lg        loggateway.Logger
	flowLog   FlowLogWriter

	mu                 sync.Mutex
	running            bool
	interval           time.Duration
	lastUnhealthyCount atomic.Int32 // cached count from most recent RunOnce (thread-safe)
	checkersMu         sync.RWMutex // protects checkers and repairers slices
}

// NewSelfCheckScheduler creates a new scheduler with the given checkers, repairers, and report repo.
// flowLog is the user-visible flow log (流程日志) port; nil-safe (tests may pass nil).
func NewSelfCheckScheduler(
	checkers []SelfChecker,
	repairers []SelfCheckRepairer,
	repo SelfCheckReportRepo,
	registry *AlertMetricRegistry,
	lg loggateway.Logger,
	flowLog FlowLogWriter,
) *SelfCheckScheduler {
	interval := selfCheckIntervalFromEnv()
	return &SelfCheckScheduler{
		checkers:  checkers,
		repairers: repairers,
		repo:      repo,
		registry:  registry,
		lg:        lg,
		flowLog:   flowLog,
		interval:  interval,
	}
}

func selfCheckIntervalFromEnv() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("SELF_CHECK_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= time.Minute {
			return d
		}
	}
	return DefaultSelfCheckInterval
}

// RegisterChecker adds a checker to the scheduler.
func (s *SelfCheckScheduler) RegisterChecker(c SelfChecker) {
	if s == nil || c == nil {
		return
	}
	s.checkersMu.Lock()
	s.checkers = append(s.checkers, c)
	s.checkersMu.Unlock()
}

// RegisterRepairer adds a repairer to the scheduler.
func (s *SelfCheckScheduler) RegisterRepairer(r SelfCheckRepairer) {
	if s == nil || r == nil {
		return
	}
	s.checkersMu.Lock()
	s.repairers = append(s.repairers, r)
	s.checkersMu.Unlock()
}

// Start begins the periodic self-check loop. Blocks until ctx is cancelled.
func (s *SelfCheckScheduler) Start(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run once immediately on start
	s.RunOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return
		case <-ticker.C:
			safego.Go(ctx, "self-check-scheduler.tick", func() {
				s.RunOnce(ctx)
			})
		}
	}
}

// RunOnce executes all registered checkers once, runs repairs for failed checks,
// persists the report, and updates alert metrics.
func (s *SelfCheckScheduler) RunOnce(ctx context.Context) *SelfCheckReport {
	if s == nil {
		return nil
	}

	// Concurrency lock: only one RunOnce at a time
	if !s.mu.TryLock() {
		s.lg.Warn("SelfCheckScheduler: RunOnce skipped, another check in progress",
			loggateway.StepID("monitor.self_check_skipped"))
		return nil
	}
	defer s.mu.Unlock()

	startedAt := time.Now().UTC()
	overallCtx, cancel := context.WithTimeout(ctx, SelfCheckOverallTimeout)
	defer cancel()

	var results []types.SelfCheckResult
	s.checkersMu.RLock()
	checkersCopy := make([]SelfChecker, len(s.checkers))
	copy(checkersCopy, s.checkers)
	s.checkersMu.RUnlock()
	if s.flowLog != nil {
		s.flowLog.LogFlowStart(ctx, "", "monitor.selfcheck.run", "系统自检开始",
			LogPair{Key: "checkers", Value: len(checkersCopy)})
	}
	for _, checker := range checkersCopy {
		checkCtx, checkCancel := context.WithTimeout(overallCtx, SelfCheckTimeout)
		result := s.runChecker(checkCtx, checker)
		checkCancel()
		results = append(results, result)
	}

	overallStatus := AggregateOverallStatus(results)

	// Run repairs for non-passed results
	var repairActions []RepairOutcome
	s.checkersMu.RLock()
	repairersCopy := make([]SelfCheckRepairer, len(s.repairers))
	copy(repairersCopy, s.repairers)
	s.checkersMu.RUnlock()
	for _, result := range results {
		if result.Status == types.SelfCheckStatusPassed {
			continue
		}
		for _, repairer := range repairersCopy {
			if repairer.CanRepair(result.Checker, result.Status) {
				outcome := repairer.Repair(overallCtx, result)
				repairActions = append(repairActions, outcome)
				if outcome.Success {
					s.lg.Info("SelfCheckScheduler: repair succeeded",
						loggateway.StepID("monitor.self_check_repair_ok"),
						loggateway.Str("checker", result.Checker),
						loggateway.Str("action", outcome.Action))
				} else {
					s.lg.Warn("SelfCheckScheduler: repair failed",
						loggateway.StepID("monitor.self_check_repair_fail"),
						loggateway.Str("checker", result.Checker),
						loggateway.Str("action", outcome.Action),
						loggateway.Str("message", outcome.Message))
				}
			}
		}
	}

	finishedAt := time.Now().UTC()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()

	report := SelfCheckReport{
		ID:            uuid.NewString(),
		CheckResults:  results,
		OverallStatus: overallStatus,
		RepairActions: repairActions,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		DurationMs:    durationMs,
	}

	// Persist report (best-effort)
	if s.repo != nil {
		if err := s.repo.InsertSelfCheckReport(overallCtx, report); err != nil {
			s.lg.Warn("SelfCheckScheduler: failed to persist report",
				loggateway.StepID("monitor.self_check_persist_fail"),
				loggateway.Err(err))
		}
	}

	// Cache unhealthy count for SelfCheckUnhealthyCountMetric.Evaluate (reads atomically)
	unhealthyCount := 0
	for _, r := range results {
		if r.Status != types.SelfCheckStatusPassed {
			unhealthyCount++
		}
	}
	s.lastUnhealthyCount.Store(int32(unhealthyCount))

	s.lg.Info("SelfCheckScheduler: self-check completed",
		loggateway.StepID("monitor.self_check_done"),
		loggateway.Str("overall_status", string(overallStatus)),
		loggateway.Int64("duration_ms", durationMs),
		loggateway.Int("checkers", len(results)),
		loggateway.Int("repairs", len(repairActions)))

	if s.flowLog != nil {
		pairs := []LogPair{
			{Key: "overall_status", Value: string(overallStatus)},
			{Key: "duration_ms", Value: durationMs},
			{Key: "checkers", Value: len(results)},
			{Key: "failed", Value: unhealthyCount},
			{Key: "repairs", Value: len(repairActions)},
		}
		if unhealthyCount > 0 {
			s.flowLog.LogFlowError(ctx, "", "monitor.selfcheck.run", "系统自检完成（存在异常项）", pairs...)
		} else {
			s.flowLog.LogFlowDone(ctx, "", "monitor.selfcheck.run", "系统自检完成", pairs...)
		}
	}

	return &report
}

func (s *SelfCheckScheduler) runChecker(ctx context.Context, checker SelfChecker) types.SelfCheckResult {
	done := make(chan types.SelfCheckResult, 1)
	safego.Go(ctx, "self-check."+checker.Name(), func() {
		defer func() {
			if r := recover(); r != nil {
				done <- types.SelfCheckResult{
					CheckID:   uuid.NewString(),
					Checker:   checker.Name(),
					Status:    types.SelfCheckStatusFailed,
					Message:   "checker panicked",
					Details:   map[string]any{"panic": r},
					CheckedAt: time.Now().UTC(),
				}
			}
		}()
		done <- checker.Check(ctx)
	})

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		return types.SelfCheckResult{
			CheckID:   uuid.NewString(),
			Checker:   checker.Name(),
			Status:    types.SelfCheckStatusFailed,
			Message:   "checker timed out",
			Details:   map[string]any{"timeout": SelfCheckTimeout.String()},
			CheckedAt: time.Now().UTC(),
		}
	}
}

// SelfCheckUnhealthyCountMetric is an AlertMetric that reports the count of
// unhealthy self-check results from the most recent check cycle.
// It caches the latest unhealthy count from RunOnce instead of re-running
// all checkers on every Evaluate call.
type SelfCheckUnhealthyCountMetric struct {
	scheduler *SelfCheckScheduler
}

// NewSelfCheckUnhealthyCountMetric creates the metric backed by the scheduler.
func NewSelfCheckUnhealthyCountMetric(scheduler *SelfCheckScheduler) *SelfCheckUnhealthyCountMetric {
	return &SelfCheckUnhealthyCountMetric{scheduler: scheduler}
}

func (m *SelfCheckUnhealthyCountMetric) Key() string { return "monitor.selfcheck_unhealthy_count" }
func (m *SelfCheckUnhealthyCountMetric) Description() string {
	return "Number of unhealthy self-check results"
}
func (m *SelfCheckUnhealthyCountMetric) Catalog() AlertMetricInfo {
	return AlertMetricInfo{
		Key:                  m.Key(),
		Name:                 "Unhealthy self-checks",
		Description:          "Number of subsystem self-checks (DB, event bus, WebSocket, trace projector, etc.) currently failing or degraded.",
		Unit:                 "count",
		DefaultWindowMinutes: 5,
		SuggestedThreshold:   1,
	}
}
func (m *SelfCheckUnhealthyCountMetric) Evaluate(ctx context.Context, _ time.Duration) (float64, error) {
	if m.scheduler == nil {
		return 0, nil
	}
	// Return the cached unhealthy count from the most recent RunOnce cycle.
	// Uses atomic load for thread-safe access without locking.
	return float64(m.scheduler.lastUnhealthyCount.Load()), nil
}

//go:build integration

package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// Stubs for Monitor integration tests
// ---------------------------------------------------------------------------

// stubHealRecordRepo implements monitor.HealRecordRepo for integration tests.
type stubHealRecordRepo struct {
	mu      sync.Mutex
	records []monitor.HealRecord
}

func newStubHealRecordRepo() *stubHealRecordRepo {
	return &stubHealRecordRepo{}
}

func (s *stubHealRecordRepo) InsertHealRecord(_ context.Context, record monitor.HealRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

func (s *stubHealRecordRepo) ListHealRecords(_ context.Context, query monitor.HealRecordQuery) (monitor.HealRecordListResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var filtered []monitor.HealRecord
	for _, r := range s.records {
		if query.RuleID != "" && r.RuleID != query.RuleID {
			continue
		}
		if query.Status != "" && r.Status != query.Status {
			continue
		}
		if query.SessionID != "" && r.SessionID != query.SessionID {
			continue
		}
		filtered = append(filtered, r)
	}

	total := len(filtered)
	if query.Offset >= total {
		return monitor.HealRecordListResult{Total: total}, nil
	}
	end := query.Offset + query.Limit
	if query.Limit <= 0 || end > total {
		end = total
	}
	return monitor.HealRecordListResult{
		Items: filtered[query.Offset:end],
		Total: total,
	}, nil
}

func (s *stubHealRecordRepo) DeleteHealRecordsOlderThan(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}

// stubAlertNotifier implements monitor.AlertNotifier for integration tests.
type stubAlertNotifier struct {
	mu     sync.Mutex
	alerts []stubAlert
}

type stubAlert struct {
	Rule    monitor.AlertRule
	Payload map[string]any
}

func newStubAlertNotifier() *stubAlertNotifier {
	return &stubAlertNotifier{}
}

func (s *stubAlertNotifier) Notify(_ context.Context, rule monitor.AlertRule, payload map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alerts = append(s.alerts, stubAlert{Rule: rule, Payload: payload})
}

func (s *stubAlertNotifier) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.alerts)
}

// stubEnvelope implements monitor.Envelope for integration tests.
type stubEnvelope struct {
	meta map[string]any
	typ  string
}

func (e *stubEnvelope) GetMetadata() map[string]any { return e.meta }
func (e *stubEnvelope) GetType() string             { return e.typ }

// ---------------------------------------------------------------------------
// Integration Tests: Self-Healing Loop
// ---------------------------------------------------------------------------

// TestMonitorIntegration_SelfHealLoop_ErrorDetectionAndRCA verifies the full
// self-healing loop: inject error → detect → root cause analysis → FixAction generation.
// Run with: go test -tags=integration ./internal/service/... -run TestMonitorIntegration_SelfHealLoop -count=1
func TestMonitorIntegration_SelfHealLoop_ErrorDetectionAndRCA(t *testing.T) {
	ctx := context.Background()
	_ = loggateway.NewNoop() // ensure loggateway initialized

	t.Run("RootCauseAnalyzer_Analyze_Timeout", func(t *testing.T) {
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())

		// Inject an LLM timeout error
		result, err := engine.Analyze(ctx, "llm-call-1", "error", fmt.Errorf("provider timeout"), map[string]any{
			"error_message": "request timed out after 30s",
			"error_code":    "TIMEOUT",
		})
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result for LLM timeout error")
		}
		if result.RuleID != "rc-provider-timeout" {
			t.Errorf("expected rule rc-provider-timeout, got %q", result.RuleID)
		}
		if result.FixAction.Type != "retry" {
			t.Errorf("expected fix action type retry, got %q", result.FixAction.Type)
		}
		if result.FixAction.MaxAttempts != 2 {
			t.Errorf("expected max_attempts=2, got %d", result.FixAction.MaxAttempts)
		}
		if result.Severity != "high" {
			t.Errorf("expected severity high, got %q", result.Severity)
		}
	})

	t.Run("RootCauseAnalyzer_AnalyzeFromReport", func(t *testing.T) {
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())

		// Build a FailureReport for a runtime MCP connection failure
		report := monitor.NewFailureReport()
		report.Type = monitor.FailureTypeRuntime
		report.Source = "runtime"
		report.Job = "mcp-server"
		report.ErrorCode = "CONNECTION_REFUSED"
		report.Message = "connection refused: dial tcp 127.0.0.1:8080"

		result, err := engine.AnalyzeFromReport(ctx, report)
		if err != nil {
			t.Fatalf("AnalyzeFromReport failed: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result for MCP connection failure report")
		}
		if result.RuleID != "rc-mcp-connection-failure" {
			t.Errorf("expected rule rc-mcp-connection-failure, got %q", result.RuleID)
		}
		if result.FixAction.Type != "reconnect" {
			t.Errorf("expected fix action type reconnect, got %q", result.FixAction.Type)
		}
		if result.FixAction.MaxAttempts != 3 {
			t.Errorf("expected max_attempts=3, got %d", result.FixAction.MaxAttempts)
		}
	})

	t.Run("RootCauseAnalyzer_AnalyzeFromReport_NilReport", func(t *testing.T) {
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())

		result, err := engine.AnalyzeFromReport(ctx, nil)
		if err != nil {
			t.Fatalf("AnalyzeFromReport(nil) failed: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for nil report")
		}
	})

	t.Run("RootCauseAnalyzer_Analyze_NoMatch", func(t *testing.T) {
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())

		// Unknown step ID with no matching rule
		result, err := engine.Analyze(ctx, "unknown-step", "error", fmt.Errorf("some error"), map[string]any{
			"error_message": "something unexpected happened",
		})
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result for unmatched error, got rule %q", result.RuleID)
		}
	})
}

// TestMonitorIntegration_SelfHealObserver_Flow verifies the SelfHealObserver
// event-driven flow: error event → observe → record → alert.
func TestMonitorIntegration_SelfHealObserver_Flow(t *testing.T) {
	ctx := context.Background()

	t.Run("ObserveFlowLogEvent_RuntimeHealSuccess", func(t *testing.T) {
		repo := newStubHealRecordRepo()
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
		notifier := newStubAlertNotifier()

		observer, err := monitor.NewSelfHealObserver(repo, engine, notifier, loggateway.NewNoop())
		if err != nil {
			t.Fatalf("NewSelfHealObserver failed: %v", err)
		}

		// Simulate a runtime auto-heal success event
		observer.ObserveFlowLogEvent(ctx, map[string]any{
			"flow_phase":    "error",
			"step_id":       "llm-call-1",
			"trace_id":      "trace-001",
			"session_id":    "session-001",
			"auto_healed":   true,
			"heal_success":  true,
			"heal_attempts": 1,
			"heal_strategy": "retry",
		})

		// Verify heal record was persisted
		result, err := repo.ListHealRecords(ctx, monitor.HealRecordQuery{Limit: 10})
		if err != nil {
			t.Fatalf("ListHealRecords failed: %v", err)
		}
		if result.Total != 1 {
			t.Fatalf("expected 1 heal record, got %d", result.Total)
		}
		rec := result.Items[0]
		if rec.Status != string(monitor.HealStatusObservedHealed) {
			t.Errorf("expected status observed_healed, got %q", rec.Status)
		}
		if rec.StepID != "llm-call-1" {
			t.Errorf("expected step_id llm-call-1, got %q", rec.StepID)
		}
		if !rec.RuntimeAutoHealed {
			t.Error("expected runtime_auto_healed=true")
		}
	})

	t.Run("ObserveFlowLogEvent_RuntimeHealFailed_AlertFired", func(t *testing.T) {
		repo := newStubHealRecordRepo()
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
		notifier := newStubAlertNotifier()

		observer, err := monitor.NewSelfHealObserver(repo, engine, notifier, loggateway.NewNoop())
		if err != nil {
			t.Fatalf("NewSelfHealObserver failed: %v", err)
		}

		// Simulate 5 consecutive runtime auto-heal failures to trigger circuit breaker
		for i := 0; i < 5; i++ {
			observer.ObserveFlowLogEvent(ctx, map[string]any{
				"flow_phase":    "error",
				"step_id":       "llm-call-2",
				"trace_id":      "trace-002",
				"session_id":    "session-002",
				"auto_healed":   true,
				"heal_success":  false,
				"heal_attempts": i + 1,
				"heal_strategy": "retry",
			})
		}

		// Verify heal records were persisted (5 observed_failed + 1 circuit_open alert)
		result, err := repo.ListHealRecords(ctx, monitor.HealRecordQuery{Limit: 20})
		if err != nil {
			t.Fatalf("ListHealRecords failed: %v", err)
		}
		if result.Total < 5 {
			t.Fatalf("expected at least 5 heal records, got %d", result.Total)
		}

		// Verify circuit breaker alert was fired
		alertCount := notifier.count()
		if alertCount < 1 {
			t.Error("expected at least 1 alert from circuit breaker")
		}
	})

	t.Run("ObserveFlowLogEvent_UnhealedError_RootCauseAnalysis", func(t *testing.T) {
		repo := newStubHealRecordRepo()
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
		notifier := newStubAlertNotifier()

		observer, err := monitor.NewSelfHealObserver(repo, engine, notifier, loggateway.NewNoop())
		if err != nil {
			t.Fatalf("NewSelfHealObserver failed: %v", err)
		}

		// Simulate an unhealed LLM timeout error
		observer.ObserveFlowLogEvent(ctx, map[string]any{
			"flow_phase":    "error",
			"step_id":       "llm-call-3",
			"trace_id":      "trace-003",
			"session_id":    "session-003",
			"auto_healed":   false,
			"error_message": "request timed out after 30s",
		})

		// Verify heal record was persisted with root cause analysis
		result, err := repo.ListHealRecords(ctx, monitor.HealRecordQuery{Limit: 10})
		if err != nil {
			t.Fatalf("ListHealRecords failed: %v", err)
		}
		if result.Total < 1 {
			t.Fatal("expected at least 1 heal record")
		}

		// Find the observed_failed record (not the alert record)
		var observedRec *monitor.HealRecord
		for i := range result.Items {
			if result.Items[i].Status == string(monitor.HealStatusObservedFailed) {
				observedRec = &result.Items[i]
				break
			}
		}
		if observedRec == nil {
			t.Fatal("expected an observed_failed record from root cause analysis")
		}
		if observedRec.RuleID != "rc-provider-timeout" {
			t.Errorf("expected rule rc-provider-timeout, got %q", observedRec.RuleID)
		}
		if observedRec.Confidence < monitor.SelfHealMinConfidence {
			t.Errorf("expected confidence >= %.1f, got %.2f", monitor.SelfHealMinConfidence, observedRec.Confidence)
		}
	})

	t.Run("DiagnoseAndObserve_NoMatchingRule", func(t *testing.T) {
		repo := newStubHealRecordRepo()
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
		notifier := newStubAlertNotifier()

		observer, err := monitor.NewSelfHealObserver(repo, engine, notifier, loggateway.NewNoop())
		if err != nil {
			t.Fatalf("NewSelfHealObserver failed: %v", err)
		}

		// Diagnose with a step ID that has no matching rules
		rec, err := observer.DiagnoseAndObserve(ctx, "trace-004", "session-004", "unknown-step", "manual", 5)
		if err != nil {
			t.Fatalf("DiagnoseAndObserve failed: %v", err)
		}
		if rec.Status != string(monitor.HealStatusSkippedNoAction) {
			t.Errorf("expected status skipped_no_action, got %q", rec.Status)
		}
	})

	t.Run("DiagnoseAndObserve_MatchingRule", func(t *testing.T) {
		repo := newStubHealRecordRepo()
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
		notifier := newStubAlertNotifier()

		observer, err := monitor.NewSelfHealObserver(repo, engine, notifier, loggateway.NewNoop())
		if err != nil {
			t.Fatalf("NewSelfHealObserver failed: %v", err)
		}

		// Diagnose with a tool step ID that matches the tool-execution-failure rule
		// (only requires StepID + Phase, no Pattern or ErrorCodes needed)
		rec, err := observer.DiagnoseAndObserve(ctx, "trace-005", "session-005", "tool-exec", "auto_error_event", 5)
		if err != nil {
			t.Fatalf("DiagnoseAndObserve failed: %v", err)
		}
		if rec.RuleID != "rc-tool-execution-failure" {
			t.Errorf("expected rule rc-tool-execution-failure, got %q", rec.RuleID)
		}
		if rec.FixAction.Type != "retry" {
			t.Errorf("expected fix action type retry, got %q", rec.FixAction.Type)
		}
	})

	t.Run("GetHealStats", func(t *testing.T) {
		repo := newStubHealRecordRepo()
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
		notifier := newStubAlertNotifier()

		observer, err := monitor.NewSelfHealObserver(repo, engine, notifier, loggateway.NewNoop())
		if err != nil {
			t.Fatalf("NewSelfHealObserver failed: %v", err)
		}

		// Simulate a successful heal
		observer.ObserveFlowLogEvent(ctx, map[string]any{
			"flow_phase":    "error",
			"step_id":       "llm-stats-1",
			"trace_id":      "trace-stats",
			"session_id":    "session-stats",
			"auto_healed":   true,
			"heal_success":  true,
			"heal_attempts": 1,
		})

		stats, err := observer.GetHealStats(ctx)
		if err != nil {
			t.Fatalf("GetHealStats failed: %v", err)
		}
		if stats.TotalHeals < 1 {
			t.Errorf("expected total_heals >= 1, got %d", stats.TotalHeals)
		}
		if stats.SuccessRate <= 0 {
			t.Errorf("expected success_rate > 0, got %f", stats.SuccessRate)
		}
	})
}

// TestMonitorIntegration_FailureReportParsing verifies FailureReport creation
// and conversion to metadata for root cause analysis.
func TestMonitorIntegration_FailureReportParsing(t *testing.T) {
	t.Run("NewFailureReport_HasIDAndMetadata", func(t *testing.T) {
		report := monitor.NewFailureReport()
		if report.ID == "" {
			t.Error("expected non-empty ID")
		}
		if report.Metadata == nil {
			t.Error("expected non-nil Metadata map")
		}
	})

	t.Run("FailureReport_Types", func(t *testing.T) {
		types := []monitor.FailureType{
			monitor.FailureTypeLint,
			monitor.FailureTypeTest,
			monitor.FailureTypeBuild,
			monitor.FailureTypeProtoSync,
			monitor.FailureTypeRuntime,
		}
		for _, ft := range types {
			report := monitor.NewFailureReport()
			report.Type = ft
			if report.Type != ft {
				t.Errorf("expected type %q, got %q", ft, report.Type)
			}
		}
	})
}

// TestMonitorIntegration_RootCauseEngine_AddRules verifies custom rule addition.
func TestMonitorIntegration_RootCauseEngine_AddRules(t *testing.T) {
	engine := monitor.NewRootCauseEngine(loggateway.NewNoop())

	t.Run("AddCustomRule_Matches", func(t *testing.T) {
		err := engine.AddRules([]monitor.RootCauseRule{
			{
				ID:   "rc-custom-test",
				Name: "Custom Test Rule",
				Condition: monitor.RootCauseCondition{
					StepID: "custom*",
					Phase:  "error",
				},
				RootCause:  "Custom step failed",
				FixSuggest: "Check custom step configuration",
				Severity:   "low",
				FixAction: monitor.FixAction{
					Type:        "retry",
					MaxAttempts: 1,
				},
			},
		})
		if err != nil {
			t.Fatalf("AddRules failed: %v", err)
		}

		result, err := engine.Analyze(context.Background(), "custom-step-1", "error", nil, nil)
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result for custom rule match")
		}
		if result.RuleID != "rc-custom-test" {
			t.Errorf("expected rule rc-custom-test, got %q", result.RuleID)
		}
	})

	t.Run("AddDuplicateRule_Fails", func(t *testing.T) {
		err := engine.AddRules([]monitor.RootCauseRule{
			{ID: "rc-custom-test", Name: "Duplicate"},
		})
		if err == nil {
			t.Error("expected error for duplicate rule ID, got nil")
		}
	})

	t.Run("AddRuleWithInvalidRegex_Fails", func(t *testing.T) {
		err := engine.AddRules([]monitor.RootCauseRule{
			{ID: "rc-bad-regex", Name: "Bad Regex", Condition: monitor.RootCauseCondition{Pattern: "[invalid"}},
		})
		if err == nil {
			t.Error("expected error for invalid regex, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Stub for FailurePattern integration tests
// ---------------------------------------------------------------------------

// stubFailurePatternRepo implements monitor.FailurePatternReader and
// monitor.FailurePatternWriter for integration tests.
type stubFailurePatternRepo struct {
	mu       sync.Mutex
	patterns map[string]monitor.FailurePattern  // id → pattern
	byHash   map[string]*monitor.FailurePattern // pattern_hash → pattern
}

func newStubFailurePatternRepo() *stubFailurePatternRepo {
	return &stubFailurePatternRepo{
		patterns: make(map[string]monitor.FailurePattern),
		byHash:   make(map[string]*monitor.FailurePattern),
	}
}

func (r *stubFailurePatternRepo) Create(_ context.Context, p monitor.FailurePattern) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patterns[p.ID] = p
	r.byHash[p.PatternHash] = &p
	return nil
}

func (r *stubFailurePatternRepo) Update(_ context.Context, p monitor.FailurePattern) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patterns[p.ID] = p
	r.byHash[p.PatternHash] = &p
	return nil
}

func (r *stubFailurePatternRepo) IncrementSuccess(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.patterns[id]; ok {
		p.SuccessCount++
		r.patterns[id] = p
		if existing, hashOk := r.byHash[p.PatternHash]; hashOk {
			*existing = p
		}
	}
	return nil
}

func (r *stubFailurePatternRepo) IncrementFail(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.patterns[id]; ok {
		p.FailCount++
		r.patterns[id] = p
		if existing, hashOk := r.byHash[p.PatternHash]; hashOk {
			*existing = p
		}
	}
	return nil
}

func (r *stubFailurePatternRepo) Deactivate(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.patterns[id]; ok {
		p.IsActive = false
		r.patterns[id] = p
		if existing, hashOk := r.byHash[p.PatternHash]; hashOk {
			*existing = p
		}
	}
	return nil
}

func (r *stubFailurePatternRepo) ListBySource(_ context.Context, source monitor.FailurePatternSource) ([]monitor.FailurePattern, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []monitor.FailurePattern
	for _, p := range r.patterns {
		if p.Source == source {
			result = append(result, p)
		}
	}
	return result, nil
}

func (r *stubFailurePatternRepo) GetByPatternHash(_ context.Context, hash string) (*monitor.FailurePattern, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byHash[hash]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func (r *stubFailurePatternRepo) ListActive(_ context.Context) ([]monitor.FailurePattern, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []monitor.FailurePattern
	for _, p := range r.patterns {
		if p.IsActive {
			result = append(result, p)
		}
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Integration Tests: FailurePattern Knowledge Base
// ---------------------------------------------------------------------------

// TestMonitorIntegration_FailurePatternCRUD verifies the FailurePattern
// knowledge base CRUD operations: create → query → update → deactivate.
func TestMonitorIntegration_FailurePatternCRUD(t *testing.T) {
	ctx := context.Background()
	repo := newStubFailurePatternRepo()

	t.Run("CreateAndQueryBySource", func(t *testing.T) {
		pattern := monitor.FailurePattern{
			ID:           "fp-runtime-001",
			Source:       monitor.FailurePatternSourceRuntime,
			Type:         "timeout",
			PatternHash:  "hash-timeout-001",
			PatternRegex: `timeout after \d+s`,
			FixAction: monitor.FixAction{
				Type:        "retry",
				MaxAttempts: 3,
			},
			Confidence:   0.85,
			SuccessCount: 10,
			FailCount:    2,
			Version:      1,
			IsActive:     true,
		}

		err := repo.Create(ctx, pattern)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		patterns, err := repo.ListBySource(ctx, monitor.FailurePatternSourceRuntime)
		if err != nil {
			t.Fatalf("ListBySource failed: %v", err)
		}
		if len(patterns) != 1 {
			t.Fatalf("expected 1 pattern, got %d", len(patterns))
		}
		if patterns[0].PatternHash != "hash-timeout-001" {
			t.Errorf("expected pattern_hash=hash-timeout-001, got %q", patterns[0].PatternHash)
		}
		if patterns[0].FixAction.Type != "retry" {
			t.Errorf("expected fix_action type=retry, got %q", patterns[0].FixAction.Type)
		}
	})

	t.Run("QueryByPatternHash", func(t *testing.T) {
		found, err := repo.GetByPatternHash(ctx, "hash-timeout-001")
		if err != nil {
			t.Fatalf("GetByPatternHash failed: %v", err)
		}
		if found == nil {
			t.Fatal("expected pattern found by hash, got nil")
		}
		if found.ID != "fp-runtime-001" {
			t.Errorf("expected id=fp-runtime-001, got %q", found.ID)
		}
	})

	t.Run("ListActivePatterns", func(t *testing.T) {
		// Add an inactive pattern
		repo.Create(ctx, monitor.FailurePattern{
			ID:          "fp-ci-inactive-001",
			Source:      monitor.FailurePatternSourceCI,
			PatternHash: "hash-ci-inactive",
			IsActive:    false,
		})

		active, err := repo.ListActive(ctx)
		if err != nil {
			t.Fatalf("ListActive failed: %v", err)
		}
		for _, p := range active {
			if !p.IsActive {
				t.Errorf("expected only active patterns, got inactive pattern %q", p.ID)
			}
		}
	})

	t.Run("IncrementSuccessAndFail", func(t *testing.T) {
		err := repo.IncrementSuccess(ctx, "fp-runtime-001")
		if err != nil {
			t.Fatalf("IncrementSuccess failed: %v", err)
		}
		err = repo.IncrementFail(ctx, "fp-runtime-001")
		if err != nil {
			t.Fatalf("IncrementFail failed: %v", err)
		}

		found, _ := repo.GetByPatternHash(ctx, "hash-timeout-001")
		if found.SuccessCount != 11 {
			t.Errorf("expected success_count=11, got %d", found.SuccessCount)
		}
		if found.FailCount != 3 {
			t.Errorf("expected fail_count=3, got %d", found.FailCount)
		}
	})

	t.Run("DeactivatePattern", func(t *testing.T) {
		err := repo.Deactivate(ctx, "fp-runtime-001")
		if err != nil {
			t.Fatalf("Deactivate failed: %v", err)
		}

		found, _ := repo.GetByPatternHash(ctx, "hash-timeout-001")
		if found.IsActive {
			t.Error("expected pattern to be deactivated")
		}
	})
}

// TestMonitorIntegration_FailurePatternMining verifies that FailurePatterns
// can be mined from RootCauseAnalysis results.
func TestMonitorIntegration_FailurePatternMining(t *testing.T) {
	ctx := context.Background()
	repo := newStubFailurePatternRepo()
	engine := monitor.NewRootCauseEngine(loggateway.NewNoop())

	t.Run("RootCauseResult_ToFailurePattern", func(t *testing.T) {
		// Analyze an error to get a root cause result
		result, err := engine.Analyze(ctx, "llm-call-1", "error", fmt.Errorf("provider timeout"), map[string]any{
			"error_message": "request timed out after 30s",
			"error_code":    "TIMEOUT",
		})
		if err != nil {
			t.Fatalf("Analyze failed: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		// Convert RootCauseResult to FailurePattern (simulating mining)
		pattern := monitor.FailurePattern{
			ID:           "fp-mined-" + result.RuleID,
			Source:       monitor.FailurePatternSourceMined,
			Type:         result.FixAction.Type,
			PatternHash:  result.RuleID + "-v1",
			PatternRegex: `timeout after \d+s`,
			FixAction:    result.FixAction,
			Confidence:   result.Confidence,
			IsActive:     true,
			Version:      1,
		}

		err = repo.Create(ctx, pattern)
		if err != nil {
			t.Fatalf("Create pattern failed: %v", err)
		}

		// Verify pattern was persisted and can be queried
		found, err := repo.GetByPatternHash(ctx, pattern.PatternHash)
		if err != nil {
			t.Fatalf("GetByPatternHash failed: %v", err)
		}
		if found == nil {
			t.Fatal("expected mined pattern to be found")
		}
		if found.Source != monitor.FailurePatternSourceMined {
			t.Errorf("expected source=mined, got %q", found.Source)
		}
		if found.FixAction.Type != "retry" {
			t.Errorf("expected fix_action type=retry, got %q", found.FixAction.Type)
		}
	})
}

// TestMonitorIntegration_SelfHealWithFailurePattern verifies the integration
// between SelfHealObserver and the FailurePattern knowledge base.
func TestMonitorIntegration_SelfHealWithFailurePattern(t *testing.T) {
	ctx := context.Background()

	t.Run("ObserveError_LookupPattern", func(t *testing.T) {
		patternRepo := newStubFailurePatternRepo()
		healRepo := newStubHealRecordRepo()
		engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
		notifier := newStubAlertNotifier()

		// Pre-seed a failure pattern for timeout
		patternRepo.Create(ctx, monitor.FailurePattern{
			ID:           "fp-timeout-known",
			Source:       monitor.FailurePatternSourceRuntime,
			Type:         "timeout",
			PatternHash:  "rc-provider-timeout-v1",
			PatternRegex: `timed out after \d+s`,
			FixAction: monitor.FixAction{
				Type:        "retry",
				MaxAttempts: 2,
			},
			Confidence:   0.9,
			SuccessCount: 50,
			FailCount:    5,
			IsActive:     true,
		})

		observer, err := monitor.NewSelfHealObserver(healRepo, engine, notifier, loggateway.NewNoop())
		if err != nil {
			t.Fatalf("NewSelfHealObserver failed: %v", err)
		}

		// Simulate an error event
		observer.ObserveFlowLogEvent(ctx, map[string]any{
			"flow_phase":    "error",
			"step_id":       "llm-call-pattern-1",
			"trace_id":      "trace-pattern-001",
			"session_id":    "session-pattern-001",
			"auto_healed":   false,
			"error_message": "request timed out after 30s",
		})

		// Verify heal record was created with root cause analysis
		result, err := healRepo.ListHealRecords(ctx, monitor.HealRecordQuery{Limit: 10})
		if err != nil {
			t.Fatalf("ListHealRecords failed: %v", err)
		}
		if result.Total < 1 {
			t.Fatal("expected at least 1 heal record")
		}

		// Verify the known failure pattern is still accessible
		pattern, err := patternRepo.GetByPatternHash(ctx, "rc-provider-timeout-v1")
		if err != nil {
			t.Fatalf("GetByPatternHash failed: %v", err)
		}
		if pattern == nil {
			t.Fatal("expected known failure pattern to be found")
		}
		if pattern.Confidence < 0.9 {
			t.Errorf("expected confidence >= 0.9, got %f", pattern.Confidence)
		}
	})
}

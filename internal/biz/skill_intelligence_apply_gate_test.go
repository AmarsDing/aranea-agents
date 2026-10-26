package biz

import (
	"context"
	"strings"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// applyGateCaptureLogger 捕获 Warn 的测试 Logger（E1 可观测回执钉）。
type applyGateCaptureLogger struct {
	mu    sync.Mutex
	warns []string
}

func (c *applyGateCaptureLogger) Debug(string, ...loggateway.Field) {}
func (c *applyGateCaptureLogger) Info(string, ...loggateway.Field)  {}
func (c *applyGateCaptureLogger) Error(string, ...loggateway.Field) {}
func (c *applyGateCaptureLogger) Warn(msg string, _ ...loggateway.Field) {
	c.mu.Lock()
	c.warns = append(c.warns, msg)
	c.mu.Unlock()
}
func (c *applyGateCaptureLogger) With(...loggateway.Field) loggateway.Logger { return c }

func (c *applyGateCaptureLogger) warnCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.warns)
}

func newApplyGateUsecase(bridge *mockEvolutionStoreBridge, reloader SkillReloader, lg loggateway.Logger) *SkillIntelligenceUsecase {
	scorer := NewSkillScoringUsecase(nil, lg)
	reporter := NewSkillReportUsecase(nil, nil, nil, scorer, nil, lg)
	return NewSkillIntelligenceUsecase(scorer, reporter, bridge, nil, lg, SkillIntelligenceConfig{
		Reloader: reloader,
	})
}

// E1（V 桶）pin ①：存量 approved+draft 卡死行撞 apply 门禁时必须有可观测
// 回执（Warn）——此前静默 return nil，卡死不可见且无任何重试提示。
func TestApplyGateBlockedLogsReceiptForStuckApprovedRow(t *testing.T) {
	seed := seedApprovedReadySuggestion()
	seed.LifecycleStatus = EvoLifecycleDraft // E2E-E1 门禁前的存量形态
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seed}}
	rel := &captureReloader{}
	lg := &applyGateCaptureLogger{}
	uc := newApplyGateUsecase(bridge, rel, lg)

	if err := uc.ApplyApprovedSuggestion(context.Background(), seed.ID); err != nil {
		t.Fatalf("gate must stay a guarded no-op (nil error), got %v", err)
	}
	if rel.called {
		t.Fatal("reloader must not be called when the gate blocks")
	}
	if lg.warnCount() != 1 {
		t.Fatalf("gate-blocked Warn count = %d, want 1 (E1 observable receipt)", lg.warnCount())
	}
}

// E1 pin ②：已 applied 行的幂等重入不得告警（正常 no-op，非卡死）。
func TestApplyGateSilentForAppliedRow(t *testing.T) {
	seed := seedApprovedReadySuggestion()
	seed.Status = EvoSuggestionApplied
	seed.LifecycleStatus = EvoLifecycleApplied
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seed}}
	rel := &captureReloader{}
	lg := &applyGateCaptureLogger{}
	uc := newApplyGateUsecase(bridge, rel, lg)

	if err := uc.ApplyApprovedSuggestion(context.Background(), seed.ID); err != nil {
		t.Fatalf("applied row re-entry must stay a silent no-op, got %v", err)
	}
	if rel.called {
		t.Fatal("reloader must not be called for an applied row")
	}
	if lg.warnCount() != 0 {
		t.Fatalf("applied row must not emit gate receipts, got %d warns: %v",
			lg.warnCount(), lg.warns)
	}
}

// E1 pin ③：sandbox 未过的 approved 行同样要有回执（卡死的另一形态）。
func TestApplyGateBlockedLogsReceiptWhenSandboxNotPassed(t *testing.T) {
	seed := seedApprovedReadySuggestion()
	seed.SandboxPassed = false
	bridge := &mockEvolutionStoreBridge{suggestions: []SkillEvolutionSuggestion{seed}}
	rel := &captureReloader{}
	lg := &applyGateCaptureLogger{}
	uc := newApplyGateUsecase(bridge, rel, lg)

	if err := uc.ApplyApprovedSuggestion(context.Background(), seed.ID); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if lg.warnCount() != 1 {
		t.Fatalf("gate-blocked Warn count = %d, want 1", lg.warnCount())
	}
	lg.mu.Lock()
	defer lg.mu.Unlock()
	if !strings.Contains(lg.warns[0], "apply gate blocked") {
		t.Fatalf("Warn message %q must identify the apply gate", lg.warns[0])
	}
}

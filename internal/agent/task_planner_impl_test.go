package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TestTaskPlanner_QuickAssess verifies P1-2: QuickAssess performs pure-computation
// complexity assessment without LLM/DB, returning the correct ComplexityLevel.
func TestTaskPlanner_QuickAssess(t *testing.T) {
	// Construct planner with nil deps — QuickAssess is pure computation.
	impl := &taskPlannerImpl{lg: loggateway.NewNoop()}
	ctx := context.Background()

	tests := []struct {
		name      string
		input     biz.PlanInput
		wantLevel biz.ComplexityLevel
	}{
		{
			name: "simple short message",
			input: biz.PlanInput{
				UserMessage:    "hello",
				HistoryDQScore: 0,
			},
			wantLevel: biz.ComplexitySimple,
		},
		{
			name: "moderate medium message with some history",
			input: biz.PlanInput{
				UserMessage:    strings.Repeat("请帮我分析这个中等长度的任务，包含一些上下文信息。", 12),
				HistoryDQScore: 0.5,
			},
			wantLevel: biz.ComplexityModerate,
		},
		{
			name: "complex long research message with high history",
			input: biz.PlanInput{
				UserMessage: strings.Repeat("1. 研究课题A\n2. 研究课题B\n3. 研究课题C\n", 40) +
					strings.Repeat("这是一个复杂的研究任务，需要深入分析。", 30),
				IntentArtifact: &biz.IntentArtifact{
					IntentKind:  "research",
					RiskFlags:   []string{"security", "compliance"},
					SearchHints: []string{"paper1", "paper2", "paper3"},
					Ambiguities: []string{"amb1", "amb2", "amb3"},
				},
				HistoryDQScore: 0.8,
			},
			wantLevel: biz.ComplexityComplex,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, score, err := impl.QuickAssess(ctx, tt.input)
			if err != nil {
				t.Fatalf("QuickAssess returned error: %v", err)
			}
			if level != tt.wantLevel {
				t.Errorf("QuickAssess level = %s, want %s (score=%.4f)", level, tt.wantLevel, score)
			}
			// Score sanity checks
			switch tt.wantLevel {
			case biz.ComplexitySimple:
				if score >= 0.3 {
					t.Errorf("Simple score should be < 0.3, got %.4f", score)
				}
			case biz.ComplexityModerate:
				if score < 0.3 || score >= 0.6 {
					t.Errorf("Moderate score should be in [0.3, 0.6), got %.4f", score)
				}
			case biz.ComplexityComplex:
				if score < 0.6 {
					t.Errorf("Complex score should be >= 0.6, got %.4f", score)
				}
			}
		})
	}
}

// TestTaskPlanner_QuickAssess_PureComputation verifies QuickAssess works with nil
// repo/catalog/httpClient — proving it's pure computation with no I/O.
func TestTaskPlanner_QuickAssess_PureComputation(t *testing.T) {
	impl := &taskPlannerImpl{
		repo:       nil,
		catalog:    nil,
		httpClient: nil,
		bus:        nil,
		orchCache:  nil,
		lg:         loggateway.NewNoop(),
	}
	level, _, err := impl.QuickAssess(context.Background(), biz.PlanInput{
		UserMessage: "test message",
	})
	if err != nil {
		t.Fatalf("QuickAssess with nil deps should not error: %v", err)
	}
	if level == "" {
		t.Error("QuickAssess should return a non-empty level")
	}
}

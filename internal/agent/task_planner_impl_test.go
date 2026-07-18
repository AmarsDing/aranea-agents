package agent

import (
	"context"
	"strings"
	"sync"
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

// TestTaskPlanner_QuickAssess_TeamIntentForcesPlanning verifies that QuickAssess
// detects team-formation keywords in the user message and forces at least
// Moderate complexity, so the pre-planning gate triggers ForcePlanning=true.
//
// Root cause: detectTeamIntent was only called inside Plan(), but Plan() is
// only invoked when ForcePlanning=true (hard gate) or when the LLM voluntarily
// calls plan_and_execute. This created a chicken-and-egg problem where
// team-formation requests rated "simple" never triggered planning.
// Fix: QuickAssess now calls detectTeamIntent and upgrades complexity.
func TestTaskPlanner_QuickAssess_TeamIntentForcesPlanning(t *testing.T) {
	impl := &taskPlannerImpl{lg: loggateway.NewNoop()}
	ctx := context.Background()

	tests := []struct {
		name       string
		message    string
		wantForced bool // expect level >= Moderate
	}{
		// Parallel keywords
		{"parallel keyword (zh)", "请并行处理这两个任务", true},
		{"parallel keyword (en)", "Please run these two tasks in parallel", true},
		{"concurrently keyword", "Run these concurrently", true},
		{"同时工作 keyword", "让两个agent同时工作", true},
		// Team formation keywords
		{"组建团队 keyword", "请组建团队完成这个项目", true},
		{"组建team keyword", "Please build a team to handle this", true},
		{"团队a团队b keyword", "团队A写诗，团队B写代码", true},
		{"多个团队 keyword", "需要多个团队协作完成", true},
		// Real-world prompt from the bug report
		{"real team prompt", "请组建两个团队并行工作：团队A写一首关于春天的五言绝句，团队B写一首关于秋天的五言绝句。两首诗完成后请汇总对比。", true},
		// Non-team messages should NOT be forced
		{"simple greeting", "你好", false},
		{"simple question", "今天天气怎么样", false},
		{"simple code question", "如何用Python写一个hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, score, err := impl.QuickAssess(ctx, biz.PlanInput{
				UserMessage: tt.message,
			})
			if err != nil {
				t.Fatalf("QuickAssess returned error: %v", err)
			}
			isForced := level == biz.ComplexityModerate || level == biz.ComplexityComplex
			if isForced != tt.wantForced {
				t.Errorf("message=%q: got level=%s score=%.4f (forced=%v), want forced=%v",
					tt.message, level, score, isForced, tt.wantForced)
			}
		})
	}
}

// TestDetermineStrategy_ModeOverride verifies that an explicit Mode in PlanInput
// selects the correct strategy. In the three-mode system (direct/parallel/dag),
// the LLM is the sole decision authority — complexity no longer drives selection.
//
// Mode values (from PlanAndExecuteInput.Mode jsonschema):
//   - direct: Spirit answers directly, no delegation
//   - parallel: N independent single-agent subtasks, 1 agent per subtask
//   - dag: N teams, each team has ≥2 members collaborating
//   - empty/auto/unknown: defaults to direct (LLM did not comply with DECISION.md)
func TestDetermineStrategy_ModeOverride(t *testing.T) {
	impl := &taskPlannerImpl{lg: loggateway.NewNoop()}

	tests := []struct {
		name         string
		mode         string
		level        biz.ComplexityLevel
		wantStrategy biz.OrchestrationStrategy
	}{
		// Empty/auto/unknown mode: defaults to direct (LLM is the decision authority)
		{"empty mode defaults to direct", "", biz.ComplexitySimple, biz.StrategyDirect},
		{"auto mode defaults to direct", "auto", biz.ComplexitySimple, biz.StrategyDirect},
		{"auto mode complex defaults to direct", "auto", biz.ComplexityComplex, biz.StrategyDirect},
		{"unknown mode defaults to direct", "unknown_mode", biz.ComplexityComplex, biz.StrategyDirect},

		// Explicit mode selects strategy regardless of complexity
		{"direct mode overrides complex", "direct", biz.ComplexityComplex, biz.StrategyDirect},
		{"parallel mode overrides simple", "parallel", biz.ComplexitySimple, biz.StrategyParallel},
		{"dag mode overrides simple", "dag", biz.ComplexitySimple, biz.StrategyDAG},

		// Deprecated modes (single/coordinator) now default to direct
		{"single mode defaults to direct", "single", biz.ComplexityComplex, biz.StrategyDirect},
		{"single_agent mode defaults to direct", "single_agent", biz.ComplexitySimple, biz.StrategyDirect},
		{"coordinator mode defaults to direct", "coordinator", biz.ComplexitySimple, biz.StrategyDirect},

		// Case-insensitive + whitespace-tolerant
		{"mode case insensitive", "PARALLEL", biz.ComplexitySimple, biz.StrategyParallel},
		{"mode with whitespace", " parallel ", biz.ComplexitySimple, biz.StrategyParallel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := biz.PlanInput{Mode: tt.mode, HistoryDQScore: 0}
			strategy, _, _ := impl.determineStrategy(tt.level, 0.5, input)
			if strategy != tt.wantStrategy {
				t.Errorf("mode=%q level=%s: got strategy=%s, want %s",
					tt.mode, tt.level, strategy, tt.wantStrategy)
			}
		})
	}
}

// TestShouldForceComplex verifies that team-forming modes (parallel/dag)
// trigger forced complexity upgrade so decomposition runs and produces subtasks
// for the allocator. Non-team modes (empty/auto/direct) do not force upgrade.
func TestShouldForceComplex(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"", false},
		{"auto", false},
		{"direct", false},
		{"parallel", true},
		{"dag", true},
		{"PARALLEL", true},
		{" parallel ", true},
		{"single", false},      // deprecated, no longer forces complex
		{"coordinator", false}, // deprecated, no longer forces complex
		{"unknown_mode", false},
	}
	for _, tt := range tests {
		got := shouldForceComplex(tt.mode)
		if got != tt.want {
			t.Errorf("shouldForceComplex(%q) = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

// TestDetectTeamIntent verifies the backend-side fallback that detects explicit
// team-formation intent from the user message when the Spirit LLM passes empty/auto
// mode. This ensures teams are created even when the LLM doesn't follow the
// DECISION.md instruction to pass explicit mode=parallel/dag.
//
// Mode selection (aligned with DECISION.md three-mode system):
//   - team formation keywords ("分派N个团队", "组建团队", "团队协作") → dag:
//     user expects one or more multi-member teams (≥2 members each).
//   - parallel keywords ("并行", "同时执行") → parallel: independent concurrent
//     subtasks, 1 agent per subtask (no multi-member teams).
//   - Team keywords take precedence over parallel keywords.
func TestDetectTeamIntent(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		// DAG intent (team formation keywords — checked first, takes precedence)
		// "分派N个团队"/"组建团队" implies multi-member teams → dag
		{"two teams Chinese", "分派两个团队分别负责代码分析和数据分析", "dag"},
		{"two teams English mix", "分派两个team进行，一个负责代码分析，一个负责模拟数据分析", "dag"},
		{"multiple teams", "需要多个团队协作完成", "dag"},
		{"parallel with team formation", "组建两个团队并行工作：团队A写诗，团队B写诗", "dag"},
		{"team formation Chinese", "请组建团队完成这个项目", "dag"},
		{"team A and team B", "团队A负责前端，团队B负责后端", "dag"},
		{"build a team English", "form a team to handle this", "dag"},
		{"team collaboration", "团队协作完成这个任务", "dag"},

		// Parallel intent (parallel keywords without team formation keywords)
		{"parallel keyword Chinese", "请并行处理这三个任务", "parallel"},
		{"parallel English", "please run these concurrently", "parallel"},
		{"parallel processing keyword", "并行处理多个子任务", "parallel"},

		// No team intent
		{"simple greeting", "你好", ""},
		{"single task", "写一首关于春天的诗", ""},
		{"question", "什么是五言绝句？", ""},
		{"empty message", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTeamIntent(tt.message)
			if got != tt.want {
				t.Errorf("detectTeamIntent(%q) = %q, want %q", tt.message, got, tt.want)
			}
		})
	}
}

// TestDetectTeamCount verifies that detectTeamCount correctly extracts the
// user's explicit team count from the message.
//
// 2026-07-04 问题 2 修复：detectTeamCount 用于在 decomposeTask 中约束 LLM
// 生成恰好 N 个 subtask，避免 orchestrateDAG 多创建 team。
func TestDetectTeamCount(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    int
	}{
		// 阿拉伯数字 + 量词 + team/团队
		{"2个团队", "分派2个团队分别负责代码分析和数据分析", 2},
		{"3个团队", "组建3个团队协作完成", 3},
		{"2支团队", "分派2支团队", 2},
		{"5 teams English", "please dispatch 5 teams to handle this", 5},
		{"2 team English mix", "分派两个team进行", 2},
		{"digit without 量词", "我需要3团队", 3},

		// 中文数字
		{"两个团队", "分派两个团队", 2},
		{"三个团队", "组建三个团队", 3},
		{"四支团队", "分派四支团队", 4},
		{"十个团队", "需要十个团队", 10},

		// 英文单词数字
		{"two teams", "I need two teams to handle this", 2},
		{"three teams", "dispatch three teams", 3},
		{"five teams", "five teams working in parallel", 5},

		// 不识别的数量
		{"no count", "组建团队完成项目", 0},
		{"no count generic", "团队协作", 0},
		{"simple greeting", "你好", 0},
		{"empty message", "", 0},
		{"unrelated number", "我需要5分钟完成", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTeamCount(tt.message)
			if got != tt.want {
				t.Errorf("detectTeamCount(%q) = %d, want %d", tt.message, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// P-ORCH: orchestration progress events
// ---------------------------------------------------------------------------

// plannerCaptureBus captures published v2 Events for assertions.
type plannerCaptureBus struct {
	mu        sync.Mutex
	published []biz.Event
}

func (b *plannerCaptureBus) Publish(_ context.Context, ev biz.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, ev)
}

func (b *plannerCaptureBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return nil, func() {}
}

func (b *plannerCaptureBus) getPublished() []biz.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.Event, len(b.published))
	copy(out, b.published)
	return out
}

// TestTaskPlanner_PublishOrchestrationProgress verifies the progress event
// helper publishes a well-formed orchestration_progress SystemNoticeEvent and
// is nil-safe for both nil bus and empty session.
func TestTaskPlanner_PublishOrchestrationProgress(t *testing.T) {
	// nil bus → no panic.
	nilBusPlanner := &taskPlannerImpl{lg: loggateway.NewNoop()}
	nilBusPlanner.publishOrchestrationProgress(context.Background(), "sess-1", "decomposing", nil)

	// empty session → skipped.
	bus := &plannerCaptureBus{}
	p := &taskPlannerImpl{eventBus: bus, lg: loggateway.NewNoop()}
	p.publishOrchestrationProgress(context.Background(), "", "decomposing", nil)
	if len(bus.getPublished()) != 0 {
		t.Fatal("empty session must skip publish")
	}

	// normal publish.
	p.publishOrchestrationProgress(context.Background(), "sess-orch", "decomposed", map[string]any{
		"sub_task_count": 4,
	})
	published := bus.getPublished()
	if len(published) != 1 {
		t.Fatalf("published=%d want 1", len(published))
	}
	notice, ok := published[0].(*biz.SystemNoticeEvent)
	if !ok {
		t.Fatalf("expected *biz.SystemNoticeEvent, got %T", published[0])
	}
	if notice.NoticeType != "orchestration_progress" {
		t.Errorf("NoticeType=%q want %q", notice.NoticeType, "orchestration_progress")
	}
	if notice.Meta["phase"] != "decomposed" {
		t.Errorf("phase=%v want %q", notice.Meta["phase"], "decomposed")
	}
	if notice.Meta["sub_task_count"] != 4 {
		t.Errorf("sub_task_count=%v want 4", notice.Meta["sub_task_count"])
	}
	if notice.SpiritSessionID() != "sess-orch" {
		t.Errorf("sessionID=%q want %q", notice.SpiritSessionID(), "sess-orch")
	}
}

package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// stubTaskPlanRepo is a minimal TaskPlanRepository for Plan() tests.
type stubTaskPlanRepo struct {
	created *biz.TaskPlan
}

func (r *stubTaskPlanRepo) Create(_ context.Context, p *biz.TaskPlan) (*biz.TaskPlan, error) {
	r.created = p
	return p, nil
}
func (r *stubTaskPlanRepo) GetByID(_ context.Context, id string) (*biz.TaskPlan, error) {
	return nil, nil
}
func (r *stubTaskPlanRepo) Update(_ context.Context, p *biz.TaskPlan) (*biz.TaskPlan, error) {
	return p, nil
}
func (r *stubTaskPlanRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]*biz.TaskPlan, error) {
	return nil, nil
}

// captureNoticeBus captures published events for assertion.
type captureNoticeBus struct {
	mu     sync.Mutex
	events []biz.Event
}

func (b *captureNoticeBus) Publish(_ context.Context, e biz.Event) {
	b.mu.Lock()
	b.events = append(b.events, e)
	b.mu.Unlock()
}
func (b *captureNoticeBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return nil, func() {}
}
func (b *captureNoticeBus) noticePhases() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var phases []string
	for _, e := range b.events {
		if sn, ok := e.(*biz.SystemNoticeEvent); ok && sn.NoticeType == "orchestration_progress" {
			if ph, _ := sn.Meta["phase"].(string); ph != "" {
				phases = append(phases, ph)
			}
		}
	}
	return phases
}

// 00:52 会话根因（B3）：分解失败时 planner 只写进程日志、不发任何前端进度
// 事件，用户在 60s 超时期间看不到「正在分解」之后的任何反馈。修复：分解
// 失败必须发布 decompose_failed 进度事件并显式降级 direct。
func TestTaskPlanner_Plan_DecomposeFailurePublishesFallback(t *testing.T) {
	repo := &stubTaskPlanRepo{}
	bus := &captureNoticeBus{}
	// catalog=nil + httpClient=nil → decomposeTask 必失败，走错误降级分支。
	impl := NewTaskPlanner(repo, nil, nil, bus, nil, loggateway.NewNoop(), nil, nil)

	plan, err := impl.Plan(context.Background(), biz.PlanInput{
		SpiritSessionID: "sp-test",
		UserMessage:     "组建两个 team 分别调研 A 和 B",
		Mode:            "parallel", // 显式组队模式 → 强制 Complex → 触发分解
	})
	if err != nil {
		t.Fatalf("Plan should not fail on decompose error (must degrade), got %v", err)
	}
	if plan.Strategy != biz.StrategyDirect {
		t.Fatalf("expected strategy direct after decompose failure, got %s", plan.Strategy)
	}
	if plan.DecomposeReason != "decompose_failed" {
		t.Fatalf("expected decompose_reason decompose_failed, got %q", plan.DecomposeReason)
	}
	phases := bus.noticePhases()
	foundDecomposing, foundFailed := false, false
	for _, ph := range phases {
		if ph == "decomposing" {
			foundDecomposing = true
		}
		if ph == "decompose_failed" {
			foundFailed = true
		}
	}
	if !foundDecomposing {
		t.Fatalf("expected decomposing progress event, got phases %v", phases)
	}
	if !foundFailed {
		t.Fatalf("expected decompose_failed progress event (user-visible fallback notice), got phases %v", phases)
	}
}

// TestTaskPlanner_QuickAssess verifies P1-2: QuickAssess performs pure-computation
// complexity assessment without LLM/DB, returning the correct ComplexityLevel.
func TestTaskPlanner_QuickAssess(t *testing.T) {
	// Construct planner with nil deps —QuickAssess is pure computation.
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
// repo/catalog/httpClient —proving it's pure computation with no I/O.
func TestTaskPlanner_QuickAssess_PureComputation(t *testing.T) {
	impl := &taskPlannerImpl{
		repo:       nil,
		catalog:    nil,
		httpClient: nil,
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

// TestTaskPlanner_QuickAssess_ExplicitToolRequestForcesPlanning verifies that a
// user message naming a concrete tool identifier (snake_case with ≥2
// underscores, e.g. cli_admin_skill_install_from_url) or plan_and_execute is
// upgraded to at least Moderate, so the pre-planning gate forces the planning
// path deterministically instead of leaving the routing to LLM discretion.
//
// Root cause (2026-07-28, session 784a8707): an install-skill request naming
// cli_admin_skill_install_from_url scored 0.215 (simple); the gate notice
// claimed "直接回答" while the Spirit LLM self-routed to plan_and_execute and
// launched two teams — notice text contradicted actual behavior.
func TestTaskPlanner_QuickAssess_ExplicitToolRequestForcesPlanning(t *testing.T) {
	impl := &taskPlannerImpl{lg: loggateway.NewNoop()}
	ctx := context.Background()

	tests := []struct {
		name       string
		message    string
		wantForced bool // expect level >= Moderate
	}{
		// Real-world prompt from the bug report
		{"explicit admin tool", "请使用 cli_admin_skill_install_from_url 工具从 GitHub 安装 slack-gif-creator 技能", true},
		{"explicit plan_and_execute", "调用 plan_and_execute 完成这个任务", true},
		// Single-underscore everyday tools must NOT trigger the override:
		// mentioning them is a normal direct request, not an orchestration demand.
		{"single underscore exec_command", "用 exec_command 看下当前目录有什么文件", false},
		{"single underscore web_search", "web_search 一下今天的天气", false},
		// Hyphenated names and plain text must not match.
		{"hyphenated skill name only", "安装 slack-gif-creator 技能", false},
		{"simple greeting", "你好", false},
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
// the LLM is the sole decision authority —complexity no longer drives selection.
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
		// DAG intent (team formation keywords —checked first, takes precedence)
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

// fakeSeqPublisher captures v2 events for PublishV2Board tests.
type fakeSeqPublisher struct {
	events []biz.Event
}

func (f *fakeSeqPublisher) Publish(_ context.Context, e biz.Event) {
	f.events = append(f.events, e)
}

// TestPublishV2Board_ReturnsPlanBoardID (C-18) verifies PublishV2Board returns
// the created PlanBoard with a pb_ ID.
func TestPublishV2Board_ReturnsPlanBoardID(t *testing.T) {
	seq := &fakeSeqPublisher{}
	impl := &taskPlannerImpl{lg: loggateway.NewNoop(), seq: seq}
	plan := &biz.TaskPlan{
		ID:              "tp-pub",
		SpiritSessionID: "spirit-1",
		Strategy:        biz.StrategyDAG,
		SubTasks: []biz.SubTask{
			{ID: "st-1", Name: "A"},
			{ID: "st-2", Name: "B", DependsOn: []string{"st-1"}},
		},
	}
	board, err := impl.PublishV2Board(context.Background(), plan, nil, "")
	if err != nil {
		t.Fatalf("PublishV2Board: %v", err)
	}
	if board.ID != "pb_tp-pub" {
		t.Fatalf("board.ID = %q, want pb_tp-pub (stable from plan.ID)", board.ID)
	}
	if len(board.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(board.Steps))
	}
	if len(seq.events) == 0 {
		t.Fatal("expected PlanBoard/GraphStage events to be published")
	}
	// Re-publish must reuse the same PlanBoard ID (no duplicate panels).
	board2, err := impl.PublishV2Board(context.Background(), plan, nil, "")
	if err != nil {
		t.Fatalf("PublishV2Board 2nd: %v", err)
	}
	if board2.ID != board.ID {
		t.Fatalf("2nd board.ID = %q, want same as first %q", board2.ID, board.ID)
	}
}

// TestPublishV2Board_EmptySubTasksReturnsZero (C-18) verifies skipped publish
// returns a zero PlanBoard without error.
func TestPublishV2Board_EmptySubTasksReturnsZero(t *testing.T) {
	impl := &taskPlannerImpl{lg: loggateway.NewNoop(), seq: &fakeSeqPublisher{}}
	board, err := impl.PublishV2Board(context.Background(), &biz.TaskPlan{
		ID: "tp", SpiritSessionID: "s",
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if board.ID != "" {
		t.Fatalf("expected empty board ID when SubTasks empty, got %q", board.ID)
	}
}

// ---------------------------------------------------------------------------
// P1 形式契约（B.10.15.2）：契约解析 + 兜底派生 + PlanStep 透传
// ---------------------------------------------------------------------------

// TestParseDecompositionOutput_Contracts verifies LLM-output deliverables /
// input_contract arrays are parsed into SubTask contract fields with names
// preserved verbatim (advisory contract, no enum enforcement at parse time).
func TestParseDecompositionOutput_Contracts(t *testing.T) {
	text := `[
	  {"id":"st_1","name":"Research","description":"d1","depends_on":[],
	   "required_capabilities":["research"],"priority":1,"estimated_complexity":0.5,
	   "deliverables":[{"name":"research_report","type":"document","format":"markdown","description":"调研报告"}]},
	  {"id":"st_2","name":"Write","description":"d2","depends_on":["st_1"],
	   "required_capabilities":["documentation"],"priority":2,"estimated_complexity":0.4,
	   "input_contract":[{"name":"research_report","type":"document","format":"markdown","description":"调研报告"}]}
	]`
	subTasks, err := parseDecompositionOutput(text)
	if err != nil {
		t.Fatalf("parseDecompositionOutput: %v", err)
	}
	if len(subTasks) != 2 {
		t.Fatalf("subTasks = %d, want 2", len(subTasks))
	}
	// 上游：LLM 提供的 deliverables 原样保留（名称不被兜底覆盖）。
	if len(subTasks[0].Deliverables) != 1 {
		t.Fatalf("Deliverables = %d, want 1", len(subTasks[0].Deliverables))
	}
	d := subTasks[0].Deliverables[0]
	if d.Name != "research_report" || d.Type != "document" || d.Format != "markdown" || d.Description != "调研报告" {
		t.Fatalf("Deliverables[0] = %+v, want research_report/document/markdown/调研报告", d)
	}
	// 下游：LLM 提供的 input_contract 原样保留，且不再追加兜底派生项。
	if len(subTasks[1].InputContract) != 1 {
		t.Fatalf("InputContract = %d, want 1 (LLM 已提供时不追加兜底)", len(subTasks[1].InputContract))
	}
	if subTasks[1].InputContract[0].Name != "research_report" {
		t.Fatalf("InputContract[0].Name = %q, want research_report", subTasks[1].InputContract[0].Name)
	}
}

// TestParseDecompositionOutput_DomainPath verifies domain_path is parsed and
// lexicon-normalized: hit → canonical, unknown deeper path → merged to the
// nearest known domain, missing → empty (advisory, 不阻断).
func TestParseDecompositionOutput_DomainPath(t *testing.T) {
	text := `[
	  {"id":"st_1","name":"写诗","description":"d1","depends_on":[],"domain_path":"创作/文学"},
	  {"id":"st_2","name":"中台","description":"d2","depends_on":[],"domain_path":"软件/中台"},
	  {"id":"st_3","name":"无域","description":"d3","depends_on":[]}
	]`
	subTasks, err := parseDecompositionOutput(text)
	if err != nil {
		t.Fatalf("parseDecompositionOutput: %v", err)
	}
	if len(subTasks) != 3 {
		t.Fatalf("subTasks = %d, want 3", len(subTasks))
	}
	if subTasks[0].DomainPath != "创作/文学" {
		t.Errorf("st_1.DomainPath = %q, want 创作/文学（词表命中）", subTasks[0].DomainPath)
	}
	if subTasks[1].DomainPath != "软件" {
		t.Errorf("st_2.DomainPath = %q, want 软件（词表外二级域归并一级域）", subTasks[1].DomainPath)
	}
	if subTasks[2].DomainPath != "" {
		t.Errorf("st_3.DomainPath = %q, want 空（LLM 未输出）", subTasks[2].DomainPath)
	}
	// plan 级主导域 = 首个非空 subtask 域。
	if got := PrimaryDomainPath(subTasks); got != "创作/文学" {
		t.Errorf("PrimaryDomainPath = %q, want 创作/文学", got)
	}
}

// TestParseDecompositionOutput_ContractFallbackDerivation verifies the
// deterministic fallback: when the LLM omits contracts on a DAG edge, the
// producer (subtask with dependents) gets a derived "{step_id}_output"
// deliverable and the consumer (subtask with depends_on) gets a derived
// input contract referencing the same name —so derived names match by
// construction and the validator has something to check.
func TestParseDecompositionOutput_ContractFallbackDerivation(t *testing.T) {
	text := `[
	  {"id":"st_1","name":"Research","depends_on":[]},
	  {"id":"st_2","name":"Write","depends_on":["st_1"]},
	  {"id":"st_3","name":"Solo","depends_on":[]}
	]`
	subTasks, err := parseDecompositionOutput(text)
	if err != nil {
		t.Fatalf("parseDecompositionOutput: %v", err)
	}
	if len(subTasks) != 3 {
		t.Fatalf("subTasks = %d, want 3", len(subTasks))
	}
	research, write, solo := subTasks[0], subTasks[1], subTasks[2]

	// 生产者兜底：有下游依赖但 LLM 未输出 deliverables 时，派生 {step_id}_output。
	if len(research.Deliverables) != 1 {
		t.Fatalf("research.Deliverables = %d, want 1 (fallback derived)", len(research.Deliverables))
	}
	wantName := research.ID + "_output"
	got := research.Deliverables[0]
	if got.Name != wantName || got.Type != "document" || got.Format != "markdown" {
		t.Fatalf("research.Deliverables[0] = %+v, want name=%q type=document format=markdown", got, wantName)
	}

	// 消费者兜底：当 depends_on 非空且 LLM 未输出 input_contract 时，每个上游派生一项，
	// 名称与上游兜底 deliverable 一致（构造即匹配）。
	if len(write.InputContract) != 1 {
		t.Fatalf("write.InputContract = %d, want 1 (fallback derived)", len(write.InputContract))
	}
	if write.InputContract[0].Name != wantName {
		t.Fatalf("write.InputContract[0].Name = %q, want %q (match upstream derived deliverable)",
			write.InputContract[0].Name, wantName)
	}

	// 无依赖边：solo 既不是生产者也不是消费者，不派生任何契约。
	if len(solo.Deliverables) != 0 || len(solo.InputContract) != 0 {
		t.Fatalf("solo contracts = %+v/%+v, want both empty", solo.Deliverables, solo.InputContract)
	}
	// 消费者自身若无下游依赖，不派生 deliverables。
	if len(write.Deliverables) != 0 {
		t.Fatalf("write.Deliverables = %+v, want empty (no dependents)", write.Deliverables)
	}
}

// TestParseDecompositionOutput_ContractToleratesInvalid verifies contract
// entries with a blank name are skipped (not an error), consistent with the
// parser's existing tolerant semantics.
func TestParseDecompositionOutput_ContractToleratesInvalid(t *testing.T) {
	text := `[
	  {"id":"st_1","name":"A","depends_on":[],
	   "deliverables":[{"name":"","type":"document","format":"markdown"},{"name":"ok","type":"data","format":"json"}]}
	]`
	subTasks, err := parseDecompositionOutput(text)
	if err != nil {
		t.Fatalf("parseDecompositionOutput: %v", err)
	}
	if len(subTasks) != 1 {
		t.Fatalf("subTasks = %d, want 1", len(subTasks))
	}
	if len(subTasks[0].Deliverables) != 1 || subTasks[0].Deliverables[0].Name != "ok" {
		t.Fatalf("Deliverables = %+v, want single entry 'ok' (blank-name skipped)", subTasks[0].Deliverables)
	}
}

// TS9-BUG-4: contract entries named after the reserved deliverable state keys
// ("summary"/"cognition") are dropped at parse time — they would become
// unsatisfiable MDC topics because set_deliverable rejects reserved-key writes.
func TestParseDecompositionOutput_ContractReservedNamesDropped(t *testing.T) {
	text := `[
	  {"id":"st_1","name":"A","depends_on":[],
	   "deliverables":[{"name":"summary","type":"document","format":"markdown"},
	                   {"name":"cognition","type":"data","format":"json"},
	                   {"name":"恢复执行报告","type":"document","format":"markdown"}]}
	]`
	subTasks, err := parseDecompositionOutput(text)
	if err != nil {
		t.Fatalf("parseDecompositionOutput: %v", err)
	}
	if len(subTasks) != 1 {
		t.Fatalf("subTasks = %d, want 1", len(subTasks))
	}
	if len(subTasks[0].Deliverables) != 1 || subTasks[0].Deliverables[0].Name != "恢复执行报告" {
		t.Fatalf("Deliverables = %+v, want single entry '恢复执行报告' (reserved names dropped)", subTasks[0].Deliverables)
	}
}

// TestPublishV2Board_ContractPassthrough verifies SubTask contracts are
// carried onto the corresponding PlanStep so crash-recovery and dagRun
// validation can read them from plan_steps_v2.
func TestPublishV2Board_ContractPassthrough(t *testing.T) {
	impl := &taskPlannerImpl{lg: loggateway.NewNoop(), seq: &fakeSeqPublisher{}}
	plan := &biz.TaskPlan{
		ID:              "tp-contract",
		SpiritSessionID: "spirit-1",
		Strategy:        biz.StrategyDAG,
		SubTasks: []biz.SubTask{
			{ID: "st-1", Name: "A", Deliverables: []biz.DeliverableContract{
				{Name: "spec", Type: "document", Format: "markdown", Description: "设计稿"},
			}},
			{ID: "st-2", Name: "B", DependsOn: []string{"st-1"}, InputContract: []biz.DeliverableContract{
				{Name: "spec", Type: "document", Format: "markdown"},
			}},
		},
	}
	board, err := impl.PublishV2Board(context.Background(), plan, nil, "")
	if err != nil {
		t.Fatalf("PublishV2Board: %v", err)
	}
	if len(board.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(board.Steps))
	}
	if len(board.Steps[0].Deliverables) != 1 || board.Steps[0].Deliverables[0].Name != "spec" {
		t.Fatalf("Steps[0].Deliverables = %+v, want spec", board.Steps[0].Deliverables)
	}
	if len(board.Steps[1].InputContract) != 1 || board.Steps[1].InputContract[0].Name != "spec" {
		t.Fatalf("Steps[1].InputContract = %+v, want spec", board.Steps[1].InputContract)
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

// ---------------------------------------------------------------------------
// 闭环任务自动追加复盘节点（TS9-GAP-1）
// ---------------------------------------------------------------------------

// ts9LikeSubTasks 模拟 TS-9 实跑中 LLM 分解出的 4 团队串行链（无复盘节点）。
func ts9LikeSubTasks() []biz.SubTask {
	return []biz.SubTask{
		{ID: "st_1", Name: "告警分诊", Description: "对告警进行聚合定级", DependsOn: []string{}, Priority: 1},
		{ID: "st_2", Name: "根因定位", Description: "定位故障根因", DependsOn: []string{"st_1"}, Priority: 2},
		{ID: "st_3", Name: "修复方案", Description: "制定修复方案", DependsOn: []string{"st_2"}, Priority: 3},
		{ID: "st_4", Name: "恢复执行与验证", Description: "执行回滚与验证", DependsOn: []string{"st_3"}, Priority: 4},
	}
}

// TestAppendClosedLoopPostmortem_Append 验证闭环类（事故/告警/恢复）任务在
// LLM 分解未产出复盘节点时，由引擎确定性追加一个依赖全部叶子节点的复盘
// subtask，且追加后 DAG 仍合法（TS9-GAP-1：TS-9 实跑中复盘靠人工补位）。
func TestAppendClosedLoopPostmortem_Append(t *testing.T) {
	msg := "电商平台订单服务 P2 生产事故：502 错误率 23%，请完成告警分诊、根因定位、修复方案与恢复执行验证"
	subTasks := ts9LikeSubTasks()

	updated, pm, appended := appendClosedLoopPostmortem(msg, subTasks)
	if !appended {
		t.Fatal("closed-loop incident message must append a postmortem subtask")
	}
	if len(updated) != len(subTasks)+1 {
		t.Fatalf("updated len=%d want %d", len(updated), len(subTasks)+1)
	}
	if !strings.Contains(pm.Name, "复盘") {
		t.Errorf("postmortem subtask name=%q should contain 复盘", pm.Name)
	}
	// 复盘节点必须依赖当前全部叶子节点（st_4 是唯一叶子）。
	if len(pm.DependsOn) != 1 || pm.DependsOn[0] != "st_4" {
		t.Errorf("postmortem DependsOn=%v want [st_4]（全部叶子节点）", pm.DependsOn)
	}
	// 执行团队只能看到自己的 description：必须自包含，且禁止保留字契约。
	if pm.Description == "" {
		t.Error("postmortem description must be non-empty and self-contained")
	}
	if len(pm.Deliverables) == 0 {
		t.Error("postmortem must declare a deliverable contract for the report")
	}
	for _, c := range pm.Deliverables {
		if c.Name == "summary" || c.Name == "cognition" {
			t.Errorf("deliverable contract name %q uses reserved key", c.Name)
		}
	}
	// 追加后 DAG 必须仍合法（无环、引用存在）。
	if err := validateSubTaskDAG(updated); err != nil {
		t.Errorf("DAG invalid after postmortem append: %v", err)
	}
	// 原切片不被修改（函数应返回新切片）。
	if len(subTasks) != 4 {
		t.Error("appendClosedLoopPostmortem must not mutate the input slice")
	}
}

// TestAppendClosedLoopPostmortem_Skip 验证非闭环任务、节点过少、已含复盘
// 节点三种情况下不追加。
func TestAppendClosedLoopPostmortem_Skip(t *testing.T) {
	t.Run("non-closed-loop message", func(t *testing.T) {
		_, _, appended := appendClosedLoopPostmortem("帮我写一首关于春天的诗并配一张图", ts9LikeSubTasks())
		if appended {
			t.Error("non-incident message must not append postmortem")
		}
	})

	t.Run("fewer than 2 subtasks", func(t *testing.T) {
		single := []biz.SubTask{{ID: "st_1", Name: "告警分诊", Description: "分诊", Priority: 1}}
		_, _, appended := appendClosedLoopPostmortem("P2 生产事故需要恢复", single)
		if appended {
			t.Error("single subtask plan must not append postmortem")
		}
	})

	t.Run("postmortem already present", func(t *testing.T) {
		with := append(ts9LikeSubTasks(), biz.SubTask{
			ID: "st_5", Name: "事故复盘", Description: "复盘", DependsOn: []string{"st_4"}, Priority: 5,
		})
		_, _, appended := appendClosedLoopPostmortem("P2 生产事故需要恢复并复盘", with)
		if appended {
			t.Error("must not duplicate postmortem when LLM already produced one")
		}
	})
}

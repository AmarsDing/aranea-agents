package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// taskPlannerImpl implements biz.TaskPlannerPort.
type taskPlannerImpl struct {
	repo           biz.TaskPlanRepository
	catalog        *biz.LlmProviderModelUsecase
	httpClient     *http.Client
	eventBus       biz.EventBus // P-ORCH: v2 bus for orchestration_progress notices
	orchCache      *biz.OrchestrationCache
	lg             loggateway.Logger
	plannerSetting PlannerModelLookup
	// seq is the v2 Sequencer (nil-safe) used to publish PlanBoard/PlanStep/
	// GraphStage/GraphNode events. Nil = v2 publish skipped (backwards compat).
	seq v2.SequencerPublisher

	// P3 测试钩子——生产为 nil，注入默认实现；测试可替换。
	retryBackoffFn func(attempt int) time.Duration
	llmAttemptFn   decomposeAttemptFn
	// maxDecomposeAttempts 限制瞬时故障重试上限（F8/Y3）：<=0 时用默认值 5。
	// 无限重试会让 turn 永远卡在「规划中」并持续烧 LLM 调用。
	maxDecomposeAttempts int

	// P4-G1 计划校验门：能力清单构建器。nil（旧测试构造路径）时校验门
	// 整体跳过，行为与旧版一致（fail-open）。
	capBuilder *AgentCapabilityBuilder
	// P4-G1 测试钩子——校验门违例时的有界重分解函数；生产为 nil 时回退
	// decomposeTask（同步分解，非流式——修复路径不需要流式中间态）。
	repairDecomposeFn func(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int, level biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error)
}

// decomposeAttemptFn 是单次 LLM 分解尝试的签名——供 P3 重试循环调用。
// planID 用于生成跨尝试确定性的 subtask 全局 ID（见 planStepID）。
// level 是 P2-5 思考强度路由的任务复杂度（空 = 不覆盖静态配置）。
type decomposeAttemptFn func(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int, spiritSessionID, planID string, level biz.ComplexityLevel, onSubTask func(biz.SubTask, int)) ([]biz.SubTask, *biz.PlanTaskDAG, error)

var _ biz.TaskPlannerPort = (*taskPlannerImpl)(nil)

// NewTaskPlanner creates a new TaskPlanner implementation.
// agentReader 为 P4-G1 计划校验门提供能力清单来源；传 nil 时校验门跳过。
func NewTaskPlanner(repo biz.TaskPlanRepository, catalog *biz.LlmProviderModelUsecase, httpClient *http.Client, eventBus biz.EventBus, orchCache *biz.OrchestrationCache, lg loggateway.Logger, plannerSetting PlannerModelLookup, seq v2.SequencerPublisher, agentReader biz.AgentReader) biz.TaskPlannerPort {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	impl := &taskPlannerImpl{
		repo:           repo,
		catalog:        catalog,
		httpClient:     httpClient,
		eventBus:       eventBus,
		orchCache:      orchCache,
		lg:             lg,
		plannerSetting: plannerSetting,
		seq:            seq,
	}
	if agentReader != nil {
		impl.capBuilder = NewAgentCapabilityBuilder(agentReader, lg)
	}
	return impl
}

// Plan assesses task complexity, optionally decomposes, and outputs a strategy.
func (impl *taskPlannerImpl) Plan(ctx context.Context, input biz.PlanInput) (*biz.TaskPlan, error) {
	traceID := input.TraceID
	if traceID == "" {
		traceID, _ = biz.SpiritTraceIDFromContext(ctx)
	}

	impl.lg.Info("TaskPlanner.Plan 开始",
		loggateway.StepID(biz.SpiritStepPlannerAssess),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("spirit_session_id", input.SpiritSessionID),
	)

	// Step 0: Query OrchestrationCache for memory-driven routing
	memoryHit := impl.queryMemory(ctx, input, traceID)
	if memoryHit != nil {
		impl.lg.Info("编排缓存命中，跳过完整复杂度评估",
			loggateway.StepID(biz.SpiritStepPlannerMemory),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("cache_id", memoryHit.CacheID),
			loggateway.Float64("dq_score", memoryHit.DQScore),
			loggateway.Str("topology", memoryHit.TopologyUsed),
		)

		// Reuse historical topology as strategy hint. Legacy topologies
		// (coordinator/sequential) are mapped to the closest new-mode strategy:
		// coordinator → dag (multi-member team), sequential → dag (ordered
		// execution). This preserves backward compatibility with cache entries
		// written before the three-mode refactor (direct/parallel/dag).
		topologyHint := biz.TopologyType(memoryHit.TopologyUsed)
		strategy := biz.StrategyDirect
		switch topologyHint {
		case biz.TopologyDirect:
			strategy = biz.StrategyDirect
		case biz.TopologyParallel:
			strategy = biz.StrategyParallel
		case biz.TopologyCoordinator, biz.TopologySequential, biz.TopologyHybrid:
			strategy = biz.StrategyDAG
		}

		// Build a lightweight plan from memory hit
		plan := &biz.TaskPlan{
			ID:              "tp_" + uuid.NewString(),
			SpiritSessionID: input.SpiritSessionID,
			TraceID:         traceID,
			UserMessage:     input.UserMessage,
			IntentArtifactJSON: func() string {
				if input.IntentArtifact == nil {
					return "{}"
				}
				b, _ := json.Marshal(input.IntentArtifact)
				return string(b)
			}(),
			ComplexityLevel: biz.ComplexityComplex,
			ComplexityScore: memoryHit.DQScore,
			Strategy:        strategy,
			StrategyReason:  "基于历史编排缓存推荐策略",
			TopologyHint:    topologyHint,
			MemoryHit:       memoryHit,
			Status:          biz.TaskPlanStatusDraft,
		}

		saved, err := impl.repo.Create(ctx, plan)
		if err != nil {
			impl.lg.Warn("TaskPlan 持久化失败",
				loggateway.StepID(biz.SpiritStepPlannerPersist),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
			return nil, apierror.Internal(apierror.DomainSpirit, "persist plan").WithCause(err)
		}
		// P4-G5：缓存命中也是策略决策——同样落证据链。
		impl.emitPlannerDecision(ctx, plannerDecision{
			TraceID:         traceID,
			DecisionSource:  "memory_cache",
			Mode:            strings.ToLower(strings.TrimSpace(input.Mode)),
			Strategy:        strategy,
			ComplexityLevel: biz.ComplexityComplex,
			ComplexityScore: memoryHit.DQScore,
			StrategyReason:  "基于历史编排缓存推荐策略",
			SpiritSessionID: input.SpiritSessionID,
		})
		impl.publishPlanCreated(ctx, saved, input.ChatSessionID)
		return saved, nil
	}

	// Fallback: detect team-formation intent from the user message when mode is
	// empty/auto. The DECISION.md prompt instructs the Spirit LLM to pass an
	// explicit mode (direct/parallel/dag), but LLMs don't always comply. This
	// ensures teams are created when the user clearly requests them, regardless
	// of LLM cooperation.
	//
	// 2026-07-04 问题 2 修复：同时提取用户请求的 team 数量约束，传给
	// decomposeTask 让 LLM 生成恰好 N 个 subtask（避免多出 team）。
	teamCount := detectTeamCount(input.UserMessage)
	if teamCount > 0 {
		impl.lg.Info("检测到用户消息中的团队数量约束",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("trace_id", traceID),
			loggateway.Int("team_count", teamCount),
		)
	}
	// P4-G5 决策证据链：decisionSource 记录策略来源——llm_mode（LLM/用户
	// 显式指定）/ keyword_fallback（关键词回退升级）/ complexity_auto
	// （六维评分自动评估）/ memory_cache（编排缓存命中）。
	decisionSource := "complexity_auto"
	if m := strings.ToLower(strings.TrimSpace(input.Mode)); m == "" || m == "auto" {
		if detected := detectTeamIntent(input.UserMessage); detected != "" {
			impl.lg.Info("检测到用户消息中的团队组建意图，自动升级模式",
				loggateway.StepID(biz.SpiritStepPlannerAssess),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("detected_mode", detected),
				loggateway.Str("original_mode", input.Mode),
				loggateway.Int("team_count", teamCount),
			)
			input.Mode = detected
			decisionSource = "keyword_fallback"
		}
	} else {
		decisionSource = "llm_mode"
	}

	// Step 1: Assess complexity (six dimensions)
	dimensions := impl.assessComplexity(input)
	complexityScore := dimensions.Semantic*0.25 +
		dimensions.Structural*0.15 +
		dimensions.Domain*0.15 +
		dimensions.Tool*0.10 +
		dimensions.Context*0.10 +
		dimensions.Historical*0.25

	var complexityLevel biz.ComplexityLevel
	switch {
	case complexityScore >= 0.6:
		complexityLevel = biz.ComplexityComplex
	case complexityScore >= 0.3:
		complexityLevel = biz.ComplexityModerate
	default:
		complexityLevel = biz.ComplexitySimple
	}

	// Honor explicit user/LLM mode: team-forming modes (parallel/dag/coordinator)
	// must trigger decomposition even when the complexity score is low, otherwise
	// the team would never be assembled. This is the root-cause fix for the
	// "user asks for teams but StrategyDirect short-circuits" bug.
	explicitMode := strings.ToLower(strings.TrimSpace(input.Mode))
	modeIsExplicit := explicitMode != "" && explicitMode != "auto"
	effectiveLevel := complexityLevel
	if modeIsExplicit && shouldForceComplex(input.Mode) && complexityLevel != biz.ComplexityComplex {
		effectiveLevel = biz.ComplexityComplex
		impl.lg.Info("用户显式指定编排模式，强制升级复杂度以触发任务分解",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("mode", input.Mode),
			loggateway.Str("original_level", string(complexityLevel)),
		)
	}

	impl.lg.Info("复杂度评估完成",
		loggateway.StepID(biz.SpiritStepPlannerAssess),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("complexity_level", string(complexityLevel)),
		loggateway.Float64("complexity_score", complexityScore),
	)

	// Step 2: Determine strategy
	strategy, strategyReason, topologyHint := impl.determineStrategy(complexityLevel, complexityScore, input)

	impl.lg.Info("策略路由完成",
		loggateway.StepID(biz.SpiritStepPlannerRoute),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("strategy", string(strategy)),
		loggateway.Str("topology_hint", string(topologyHint)),
	)

	// P4-G5 决策证据链：策略路由决策落 FlowLog（spirit.planner.decision），
	// 供 Monitor 流程日志审计 + MAST 失败模式标注消费。决策时刻的策略
	// 原样记录——后续分解失败/校验门降级走各自已有事件，不回写本条。
	impl.emitPlannerDecision(ctx, plannerDecision{
		TraceID:         traceID,
		DecisionSource:  decisionSource,
		Mode:            strings.ToLower(strings.TrimSpace(input.Mode)),
		Strategy:        strategy,
		ComplexityLevel: complexityLevel,
		ComplexityScore: complexityScore,
		TeamCount:       teamCount,
		KeywordFallback: decisionSource == "keyword_fallback",
		StrategyReason:  strategyReason,
		SpiritSessionID: input.SpiritSessionID,
	})

	// Step 3: Build intent artifact JSON（提前——P2 draft 落库需要）。
	intentArtifactJSON := "{}"
	if input.IntentArtifact != nil {
		b, err := json.Marshal(input.IntentArtifact)
		if err == nil {
			intentArtifactJSON = string(b)
		}
	}

	// Step 4: Decompose task (only for complex, or when explicit team-forming mode forces it)
	var subTasks []biz.SubTask
	var dag *biz.PlanTaskDAG
	var decomposeReason string
	streamPublished := false
	planID := "tp_" + uuid.NewString()
	if effectiveLevel == biz.ComplexityComplex {
		// P2：分解前先落库 draft——持久化与展示同时进行。崩溃后 draft
		// 可恢复、可观测「正在规划」，不再「先分解 60s、最后一次性落库」。
		draft := &biz.TaskPlan{
			ID:                 planID,
			SpiritSessionID:    input.SpiritSessionID,
			TraceID:            traceID,
			UserMessage:        input.UserMessage,
			IntentArtifactJSON: intentArtifactJSON,
			ComplexityLevel:    complexityLevel,
			ComplexityScore:    complexityScore,
			Dimensions:         dimensions,
			Strategy:           strategy,
			StrategyReason:     strategyReason,
			TopologyHint:       topologyHint,
			Status:             biz.TaskPlanStatusDraft,
		}
		saved, err := impl.repo.Create(ctx, draft)
		if err != nil {
			impl.lg.Warn("TaskPlan draft 持久化失败",
				loggateway.StepID(biz.SpiritStepPlannerPersist),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
			return nil, apierror.Internal(apierror.DomainSpirit, "persist draft plan: "+err.Error())
		}

		// P2：分解期间周期性心跳进度（用户可见存活信号），分解结束即停。
		stopHeartbeat := impl.startDecomposeHeartbeat(ctx, input.SpiritSessionID, 5*time.Second)

		// P-ORCH: notify the user that decomposition (LLM call, up to 60s) started.
		impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decomposing", nil)

		// 流式分解：当 v2 Sequencer 可用时，边生成边发布 PlanStep/GraphNode，
		// 前端可看到级联动画效果。否则回退到同步分解。
		if impl.seq != nil {
			impl.publishV2BoardShell(ctx, planID, strategy, input)
			onSubTask := func(st biz.SubTask, index int) {
				impl.publishV2PlanStep(ctx, st, planID, index, input)
			}
			subTasks, dag, err = impl.decomposeTaskStream(ctx, input.UserMessage, input.IntentArtifact, teamCount, input.SpiritSessionID, planID, complexityLevel, onSubTask)
			if err == nil && len(subTasks) > 0 {
				streamPublished = true
			}
		} else {
			subTasks, dag, err = impl.decomposeTask(ctx, input.UserMessage, input.IntentArtifact, teamCount, complexityLevel)
		}
		stopHeartbeat()
		if err != nil {
			impl.lg.Warn("任务分解失败，降级为 direct 策略",
				loggateway.StepID(biz.SpiritStepPlannerDecompose),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
			strategy = biz.StrategyDirect
			strategyReason = "任务分解失败，降级为 direct"
			decomposeReason = "decompose_failed"
			// P-ORCH: 分解失败必须通知前端——用户已看着「正在分解」等了至多 60s，
			// 静默降级会让用户认为系统卡死（00:52 会话根因 B3）。
			impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decompose_failed", map[string]any{
				"reason": "error",
			})
			// F9/Y4：v2 壳已发布（Status=planning/running）时必须补终态，
			// 否则前端计划面板永远停在「规划中」。
			impl.publishV2BoardFailed(ctx, planID, input)
		} else if len(subTasks) > 0 {
			// P4-G1 计划校验门：分解成功后、进度上报前做可行性校验。
			// 违例时有界重分解 1 次；仍违例按分解失败路径降级 direct。
			// 注意（已知折衷）：流式路径下原始子任务的 PlanStep 已边生成边
			// 发布，校验门修复后的计划不会重发中间步骤——终态 Board 由
			// PublishV2Board/失败补发保证一致，中间动画可能保留旧步骤。
			gate := impl.applyPlanVerifyGate(ctx, subTasks, dag, input, teamCount, complexityLevel)
			if gate.degraded {
				impl.lg.Warn("计划校验门未通过，降级为 direct 策略",
					loggateway.StepID(biz.SpiritStepPlannerDecompose),
					loggateway.Str("trace_id", traceID),
					loggateway.Str("note", gate.note),
				)
				strategy = biz.StrategyDirect
				strategyReason = gate.note
				decomposeReason = "verify_failed"
				subTasks = nil
				dag = nil
				impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decompose_failed", map[string]any{
					"reason": "verify_failed",
					"note":   gate.note,
				})
				impl.publishV2BoardFailed(ctx, planID, input)
			} else {
				subTasks = gate.subTasks
				dag = gate.dag
				gateNote := gate.note
				// P-ORCH: decomposition finished — report the subtask count.
				impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decomposed", map[string]any{
					"sub_task_count": len(subTasks),
				})
				decomposeReason = fmt.Sprintf("分解为 %d 个子任务%s", len(subTasks), gateNote)
				// 2026-07-04 问题 2 修复：当用户明确请求 N 个 team 但 LLM 产生
				// 不符数量的 subtask 时，截取前 N 个或记录警告。这是兜底——
				// buildDecompositionPrompt 已通过 prompt 硬约束 LLM，但 LLM
				// 可能不严格遵守。
				if teamCount > 0 && len(subTasks) > teamCount {
					impl.lg.Warn("LLM 分解的 subtask 数量超出用户请求的 team 数量，截取前 N 个",
						loggateway.StepID(biz.SpiritStepPlannerDecompose),
						loggateway.Str("trace_id", traceID),
						loggateway.Int("requested_team_count", teamCount),
						loggateway.Int("decomposed_subtask_count", len(subTasks)),
					)
					subTasks = subTasks[:teamCount]
					// 清理截取后悬挂的 DependsOn 引用，避免 DAG 执行失败
					validIDs := make(map[string]bool, len(subTasks))
					for _, st := range subTasks {
						validIDs[st.ID] = true
					}
					for i := range subTasks {
						filtered := subTasks[i].DependsOn[:0]
						for _, depID := range subTasks[i].DependsOn {
							if validIDs[depID] {
								filtered = append(filtered, depID)
							}
						}
						subTasks[i].DependsOn = filtered
					}
					dag = buildDAGFromSubTasks(subTasks)
					decomposeReason = fmt.Sprintf("分解为 %d 个子任务（按用户请求截取）", len(subTasks))
				} else if teamCount > 0 && len(subTasks) < teamCount {
					impl.lg.Warn("LLM 分解的 subtask 数量少于用户请求的 team 数量",
						loggateway.StepID(biz.SpiritStepPlannerDecompose),
						loggateway.Str("trace_id", traceID),
						loggateway.Int("requested_team_count", teamCount),
						loggateway.Int("decomposed_subtask_count", len(subTasks)),
					)
				}
				// TS9-GAP-1：闭环类任务（事故/告警/故障）由引擎确定性追加复盘节点，
				// 不再依赖 LLM 分解时自觉产出——流程控制权在引擎。追加发生在
				// teamCount 截取之后，避免被截取丢弃；追加后重建 DAG，流式路径
				// 补发该节点的 PlanStep/GraphNode 事件。
				if updated, pm, appended := appendClosedLoopPostmortem(input.UserMessage, subTasks); appended {
					subTasks = updated
					dag = buildDAGFromSubTasks(subTasks)
					if streamPublished {
						impl.publishV2PlanStep(ctx, *pm, planID, len(subTasks)-1, input)
					}
					impl.lg.Info("闭环任务自动追加复盘节点",
						loggateway.StepID(biz.SpiritStepPlannerDecompose),
						loggateway.Str("trace_id", traceID),
						loggateway.Str("postmortem_subtask_id", pm.ID),
					)
				}
				// Strategy is determined solely by the explicit mode (or detected
				// team intent). We no longer auto-refine based on DAG shape — the
				// LLM is the decision authority. When mode is empty (no explicit
				// request and no detected intent), strategy stays "direct" even if
				// decomposition produced subtasks; the subtasks are logged for
				// analysis but not executed by the orchestrator.
			}
		} else {
			// 分解调用成功但产出 0 个子任务（典型成因：LLM 流式超时静默返回空，
			// 由 llmcompat ctx 校验修复兜底）：与失败等价，显式降级 direct 并
			// 通知前端，避免 plan 带着 parallel/dag 策略却无 subtasks 的悬空态。
			impl.lg.Warn("任务分解产出空结果，降级为 direct 策略",
				loggateway.StepID(biz.SpiritStepPlannerDecompose),
				loggateway.Str("trace_id", traceID),
			)
			strategy = biz.StrategyDirect
			strategyReason = "任务分解结果为空，降级为 direct"
			decomposeReason = "decompose_empty"
			impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decompose_failed", map[string]any{
				"reason": "empty",
			})
			// F9/Y4：同失败路径——v2 壳必须收到终态更新。
			impl.publishV2BoardFailed(ctx, planID, input)
		}

		// P2：分解完成后增量 Update——填充 subtasks/dag 与最终策略（含降级结果），
		// Status 保持 draft（ConfirmPlan 才推进到 confirmed，语义不变）。
		saved.SubTasks = subTasks
		saved.TaskDAG = dag
		saved.DecomposeReason = decomposeReason
		saved.Strategy = strategy
		saved.StrategyReason = strategyReason
		saved.DomainPath = PrimaryDomainPath(subTasks)
		saved, err = impl.repo.Update(ctx, saved)
		if err != nil {
			impl.lg.Warn("TaskPlan 更新失败",
				loggateway.StepID(biz.SpiritStepPlannerPersist),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(err),
			)
			return nil, apierror.Internal(apierror.DomainSpirit, "update plan: "+err.Error())
		}
		// repo.Update 经 GetByID 重读，内存字段 StreamPublished 不在 DB 列中会
		// 丢失——重设，保证 PublishV2Board 走流式分支（不重复发 Created 事件）。
		saved.StreamPublished = streamPublished
		impl.publishPlanCreated(ctx, saved, input.ChatSessionID)
		return saved, nil
	}

	// 非分解路径（direct 策略）：单次 Create，无 draft 中间态。
	plan := &biz.TaskPlan{
		ID:                 planID,
		SpiritSessionID:    input.SpiritSessionID,
		TraceID:            traceID,
		UserMessage:        input.UserMessage,
		IntentArtifactJSON: intentArtifactJSON,
		ComplexityLevel:    complexityLevel,
		ComplexityScore:    complexityScore,
		Dimensions:         dimensions,
		SubTasks:           subTasks,
		TaskDAG:            dag,
		DecomposeReason:    decomposeReason,
		Strategy:           strategy,
		StrategyReason:     strategyReason,
		TopologyHint:       topologyHint,
		DomainPath:         PrimaryDomainPath(subTasks),
		MemoryHit:          nil, // Memory hit is handled in Step 0; normal path has no cache hit
		Status:             biz.TaskPlanStatusDraft,
		StreamPublished:    streamPublished,
	}

	impl.lg.Info("持久化 TaskPlan",
		loggateway.StepID(biz.SpiritStepPlannerPersist),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("plan_id", plan.ID),
	)

	saved, err := impl.repo.Create(ctx, plan)
	if err != nil {
		impl.lg.Warn("TaskPlan 持久化失败",
			loggateway.StepID(biz.SpiritStepPlannerPersist),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return nil, apierror.Internal(apierror.DomainSpirit, "persist plan: "+err.Error())
	}

	// Publish spirit_plan_created event.
	impl.publishPlanCreated(ctx, saved, input.ChatSessionID)

	return saved, nil
}

// GetPlan retrieves a plan by ID.
func (impl *taskPlannerImpl) GetPlan(ctx context.Context, planID string) (*biz.TaskPlan, error) {
	plan, err := impl.repo.GetByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// ListPlans returns all plans for a spirit session, newest first (T3.2).
func (impl *taskPlannerImpl) ListPlans(ctx context.Context, spiritSessionID string) ([]*biz.TaskPlan, error) {
	return impl.repo.ListBySpiritSessionID(ctx, spiritSessionID)
}

// QuickAssess performs a pure-computation complexity assessment (P1-2).
// It reuses the six-dimension assessComplexity logic but skips memory cache,
// LLM decomposition, and DB persistence — making it safe to call before the
// Spirit LLM runs. Used by PrePlanningGate to force planning for Moderate/Complex.
//
// Team-intent override: when the user message contains explicit team-formation
// keywords (parallel/coordinator), the level is upgraded to at least Moderate.
// This breaks the chicken-and-egg problem where detectTeamIntent was only
// called inside Plan(), but Plan() is only invoked when ForcePlanning=true
// (which QuickAssess controls). Without this override, team-formation requests
// rated "simple" never trigger planning, and no teams are created.
//
// Explicit-tool-request override (2026-07-28): messages naming a concrete
// snake_case tool identifier (≥2 underscores, e.g. cli_admin_skill_install_from_url)
// or plan_and_execute are likewise upgraded to at least Moderate, so the gate
// forces planning deterministically instead of leaving routing to LLM discretion.
func (impl *taskPlannerImpl) QuickAssess(_ context.Context, input biz.PlanInput) (biz.ComplexityLevel, float64, error) {
	dimensions := impl.assessComplexity(input)
	score := dimensions.Semantic*0.25 +
		dimensions.Structural*0.15 +
		dimensions.Domain*0.15 +
		dimensions.Tool*0.10 +
		dimensions.Context*0.10 +
		dimensions.Historical*0.25

	var level biz.ComplexityLevel
	switch {
	case score >= 0.6:
		level = biz.ComplexityComplex
	case score >= 0.3:
		level = biz.ComplexityModerate
	default:
		level = biz.ComplexitySimple
	}

	// Team-intent override: force at least Moderate so the pre-planning gate
	// triggers ForcePlanning=true and the hard gate calls Plan().
	if detected := detectTeamIntent(input.UserMessage); detected != "" && level == biz.ComplexitySimple {
		level = biz.ComplexityModerate
		if score < 0.3 {
			score = 0.3
		}
		impl.lg.Info("QuickAssess 检测到团队组建意图，升级复杂度以触发规划",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("detected_mode", detected),
			loggateway.Float64("complexity_score", score),
		)
	}

	// Explicit tool/orchestration request override: a message naming a concrete
	// snake_case tool identifier with ≥2 underscores (e.g.
	// cli_admin_skill_install_from_url, plan_and_execute) explicitly requests
	// capabilities beyond a direct answer. Upgrade to at least Moderate so the
	// gate forces the planning path deterministically, instead of leaving the
	// routing decision to LLM discretion.
	// (2026-07-28, session 784a8707: install-skill request scored 0.215 simple;
	// gate notice claimed "直接回答" while the LLM self-routed to
	// plan_and_execute and launched two teams — notice contradicted behavior.)
	if level == biz.ComplexitySimple && explicitToolRequestPattern.MatchString(strings.ToLower(input.UserMessage)) {
		level = biz.ComplexityModerate
		if score < 0.3 {
			score = 0.3
		}
		impl.lg.Info("QuickAssess 检测到显式工具/编排调用请求，升级复杂度以触发规划",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Float64("complexity_score", score),
		)
	}

	impl.lg.Debug("QuickAssess 完成",
		loggateway.StepID(biz.SpiritStepPlannerAssess),
		loggateway.Str("complexity_level", string(level)),
		loggateway.Float64("complexity_score", score),
	)
	return level, score, nil
}

// ConfirmPlan applies adjustments and confirms the plan.
func (impl *taskPlannerImpl) ConfirmPlan(ctx context.Context, planID string, adjustments biz.PlanAdjustments) (*biz.TaskPlan, error) {
	plan, err := impl.repo.GetByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan.Status != biz.TaskPlanStatusDraft {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "plan is not in draft status")
	}

	// Apply adjustments
	if len(adjustments.MergeSubTasks) > 0 {
		plan.SubTasks = mergeSubTasks(plan.SubTasks, adjustments.MergeSubTasks)
	}
	if adjustments.SplitSubTask != "" {
		plan.SubTasks = splitSubTask(plan.SubTasks, adjustments.SplitSubTask)
	}
	if adjustments.AddSubTask != nil {
		newTask := biz.SubTask{
			ID:                   "st_" + uuid.NewString(),
			Name:                 adjustments.AddSubTask.Name,
			Description:          adjustments.AddSubTask.Description,
			DependsOn:            adjustments.AddSubTask.DependsOn,
			RequiredCapabilities: adjustments.AddSubTask.RequiredCapabilities,
			Priority:             adjustments.AddSubTask.Priority,
			EstimatedComplexity:  0.5,
		}
		plan.SubTasks = append(plan.SubTasks, newTask)
	}
	if adjustments.RemoveSubTask != "" {
		plan.SubTasks = removeSubTask(plan.SubTasks, adjustments.RemoveSubTask)
	}
	if adjustments.StrategyOverride != "" {
		plan.Strategy = biz.OrchestrationStrategy(adjustments.StrategyOverride)
		plan.StrategyReason = "Spirit LLM 覆盖策略: " + adjustments.Reason
	}

	// Rebuild DAG if subtasks changed
	if len(adjustments.MergeSubTasks) > 0 || adjustments.SplitSubTask != "" || adjustments.AddSubTask != nil || adjustments.RemoveSubTask != "" {
		plan.TaskDAG = buildDAGFromSubTasks(plan.SubTasks)
	}

	plan.Status = biz.TaskPlanStatusConfirmed

	impl.lg.Info("TaskPlan 确认",
		loggateway.StepID(biz.SpiritStepPlannerConfirm),
		loggateway.Str("plan_id", planID),
		loggateway.Str("strategy", string(plan.Strategy)),
	)

	saved, err := impl.repo.Update(ctx, plan)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainSpirit, "update plan").WithCause(err)
	}
	return saved, nil
}

// queryMemory queries the OrchestrationCache for similar past orchestrations.
// Returns a MemoryHit if a high-quality historical match is found, nil otherwise.
func (impl *taskPlannerImpl) queryMemory(ctx context.Context, input biz.PlanInput, traceID string) *biz.MemoryHit {
	if impl.orchCache == nil {
		return nil
	}

	// Try SuggestTopology first for a quick hit
	topology, hit := impl.orchCache.SuggestTopology(input.UserMessage)
	if !hit {
		return nil
	}

	// Query for detailed cache entries
	cacheEntries, err := impl.orchCache.QueryByTaskPattern(ctx, input.UserMessage)
	if err != nil || len(cacheEntries) == 0 {
		return nil
	}

	best := cacheEntries[0]
	if best.DQScore < 0.7 {
		impl.lg.Info("编排缓存命中但 DQ 分数不足",
			loggateway.StepID(biz.SpiritStepPlannerMemory),
			loggateway.Str("trace_id", traceID),
			loggateway.Float64("dq_score", best.DQScore),
		)
		return nil
	}

	return &biz.MemoryHit{
		CacheID:       best.TaskPattern,
		DQScore:       best.DQScore,
		TopologyUsed:  string(topology),
		AgentKeysUsed: best.AgentKeys,
	}
}

// assessComplexity computes the six-dimension complexity assessment.
func (impl *taskPlannerImpl) assessComplexity(input biz.PlanInput) biz.DimensionScores {
	dims := biz.DimensionScores{}

	// Semantic: estimate from message length and intent kind (weight 0.25)
	runeCount := len([]rune(input.UserMessage))
	if runeCount > 500 {
		dims.Semantic = 0.7
	} else if runeCount > 200 {
		dims.Semantic = 0.4
	} else {
		dims.Semantic = 0.2
	}
	if input.IntentArtifact != nil {
		switch input.IntentArtifact.IntentKind {
		case "research", "debug":
			dims.Semantic += 0.15
		case "code_change":
			dims.Semantic += 0.1
		}
		if dims.Semantic > 1.0 {
			dims.Semantic = 1.0
		}
	}

	// Structural: check if user message contains multiple questions/tasks (weight 0.15)
	dims.Structural = assessStructural(input.UserMessage)

	// Domain: evaluate from intent kind and risk flags (weight 0.15)
	dims.Domain = assessDomain(input.IntentArtifact)

	// Tool: evaluate from intent kind and search hints (weight 0.10)
	dims.Tool = assessTool(input.IntentArtifact)

	// Context: check message length and ambiguity count (weight 0.10)
	dims.Context = assessContext(input.UserMessage, input.IntentArtifact)

	// Historical: use HistoryDQScore from input (weight 0.25)
	dims.Historical = input.HistoryDQScore

	return dims
}

// 复杂度评估正则——包级预编译（F10：函数内 MustCompile 每次调用重复编译，
// assessStructural/detectTeamCount 在每次 Plan 调用都会命中）。
var (
	sentenceEndersPattern = regexp.MustCompile(`[。！？.!?\n]`)
	enumPattern           = regexp.MustCompile(`(?:\d+[.、)]\s|[-*]\s)`)
	teamCountDigitPattern = regexp.MustCompile(`(\d+)\s*(?:个|支)?\s*(?:teams?|团队)`)
	teamCountCNPattern    = regexp.MustCompile(`(一|两|二|三|四|五|六|七|八|九|十)\s*(?:个|支)?\s*(?:teams?|团队)`)
	teamCountENPattern    = regexp.MustCompile(`\b(one|two|three|four|five|six|seven|eight|nine|ten)\s+teams?\b`)
)

// assessStructural evaluates structural complexity (multiple tasks/questions).
func assessStructural(userMessage string) float64 {
	score := 0.0
	// Count sentence-ending punctuation as proxy for multiple tasks
	matches := sentenceEndersPattern.FindAllString(userMessage, -1)
	if len(matches) >= 5 {
		score += 0.5
	} else if len(matches) >= 3 {
		score += 0.3
	} else {
		score += 0.1
	}

	// Check for enumeration patterns (1. 2. 3. or - - -)
	enumMatches := enumPattern.FindAllString(userMessage, -1)
	if len(enumMatches) >= 3 {
		score += 0.4
	} else if len(enumMatches) >= 1 {
		score += 0.2
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// assessDomain evaluates domain complexity from intent kind and risk flags.
func assessDomain(artifact *biz.IntentArtifact) float64 {
	if artifact == nil {
		return 0.1
	}
	score := 0.0
	// Risk flags indicate cross-domain concerns
	if len(artifact.RiskFlags) >= 2 {
		score += 0.4
	} else if len(artifact.RiskFlags) >= 1 {
		score += 0.2
	}
	// Certain intent kinds imply cross-domain work
	switch artifact.IntentKind {
	case "research", "debug":
		score += 0.3
	case "code_change":
		score += 0.15
	}
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.1 {
		score = 0.1
	}
	return score
}

// assessTool evaluates tool complexity from intent kind and search hints.
func assessTool(artifact *biz.IntentArtifact) float64 {
	if artifact == nil {
		return 0.1
	}
	score := 0.0
	// Certain intent kinds imply tool usage
	switch artifact.IntentKind {
	case "code_change", "debug":
		score += 0.3
	case "research":
		score += 0.2
	}
	// More search hints suggest more tool lookups needed
	if len(artifact.SearchHints) >= 3 {
		score += 0.3
	} else if len(artifact.SearchHints) >= 1 {
		score += 0.15
	}
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.1 {
		score = 0.1
	}
	return score
}

// assessContext evaluates context complexity.
func assessContext(userMessage string, artifact *biz.IntentArtifact) float64 {
	score := 0.0
	// Message length factor
	runeCount := len([]rune(userMessage))
	if runeCount > 1000 {
		score += 0.3
	} else if runeCount > 500 {
		score += 0.2
	} else {
		score += 0.1
	}

	// Ambiguity count
	if artifact != nil {
		ambCount := len(artifact.Ambiguities)
		if ambCount >= 3 {
			score += 0.5
		} else if ambCount >= 1 {
			score += 0.3
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// determineStrategy selects the orchestration strategy based on the explicit
// mode passed by the Spirit LLM. The three valid modes are: direct, parallel,
// dag (see DECISION.md). Empty/auto mode defaults to direct.
//
// The complexity level/score are still computed for logging and metrics, but
// no longer drive strategy selection — the LLM is the sole decision authority.
func (impl *taskPlannerImpl) determineStrategy(_ biz.ComplexityLevel, _ float64, input biz.PlanInput) (biz.OrchestrationStrategy, string, biz.TopologyType) {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	switch mode {
	case "direct":
		return biz.StrategyDirect, "用户指定 direct 模式", biz.TopologyDirect
	case "parallel":
		return biz.StrategyParallel, "用户指定 parallel 模式", biz.TopologyParallel
	case "dag":
		return biz.StrategyDAG, "用户指定 dag 模式", biz.TopologyHybrid
	}
	// Empty / auto / unknown mode: default to direct. The LLM is instructed
	// (DECISION.md) to always pass an explicit mode; reaching this branch means
	// the LLM did not comply — direct is the safest fallback.
	return biz.StrategyDirect, "未指定 mode，默认 direct 模式", biz.TopologyDirect
}

// shouldForceComplex returns true when the explicit mode requires subtask
// decomposition so the allocator has subtasks to assign agents to.
// Team-forming modes (parallel/dag) need decomposition regardless of the
// complexity score; otherwise a "simple" score would skip decomposition and
// the team would never be assembled.
func shouldForceComplex(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "parallel", "dag":
		return true
	}
	return false
}

// plannerDecision 是 P4-G5 策略决策证据链的记录单元。
type plannerDecision struct {
	TraceID         string
	DecisionSource  string // llm_mode / keyword_fallback / complexity_auto / memory_cache
	Mode            string
	Strategy        biz.OrchestrationStrategy
	ComplexityLevel biz.ComplexityLevel
	ComplexityScore float64
	TeamCount       int
	KeywordFallback bool
	StrategyReason  string
	SpiritSessionID string
}

// emitPlannerDecision 把策略路由决策写入 FlowLog（spirit.planner.decision）。
// 无 emitter 的 ctx（后台/测试路径）静默跳过。决策时刻的快照原样落盘——
// 后续分解失败或校验门降级各有专属事件，不回写本条，保证证据链只读追加。
func (impl *taskPlannerImpl) emitPlannerDecision(ctx context.Context, d plannerDecision) {
	em := event.TraceEmitterFromContext(ctx)
	if em == nil {
		return
	}
	em.LogDone("spirit.planner.decision", "策略决策",
		event.P("decision_source", d.DecisionSource),
		event.P("mode", d.Mode),
		event.P("strategy", string(d.Strategy)),
		event.P("complexity_level", string(d.ComplexityLevel)),
		event.P("complexity_score", d.ComplexityScore),
		event.P("team_count", d.TeamCount),
		event.P("fallback_triggered", d.KeywordFallback),
		event.P("strategy_reason", d.StrategyReason),
		event.P("spirit_session_id", d.SpiritSessionID),
	)
}

// detectTeamCount extracts the user's explicit team count from the message.
// Returns 0 when no count is specified (caller should use default range).
// Recognized patterns:
//   - "2个团队", "3支团队", "两个team", "三个团队"
//   - "组建3个团队", "分派两个团队", "创建2个团队"
//   - "two teams", "3 teams", "five teams"
//
// 2026-07-04 问题 2 修复：原 detectTeamIntent 只返回 mode（"dag"），
// 丢弃数量约束。这导致 LLM decomposition 自由产生 2-6 个 subtask，
// orchestrateDAG 为每个 subtask 创建一个 team，最终 team 数量与用户
// 请求不符（用户要 2 个 team，可能多出 3-5 个）。
func detectTeamCount(message string) int {
	lower := strings.ToLower(message)
	// 1. 阿拉伯数字 + 量词 + team/团队：例如 "2个团队"、"3支team"、"5 teams"
	if m := teamCountDigitPattern.FindStringSubmatch(lower); len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n <= 20 {
			return n
		}
	}
	// 2. 中文数字 + 量词 + team/团队：例如 "两个团队"、"三支team"
	cnNumMap := map[string]int{
		"一": 1, "两": 2, "二": 2, "三": 3, "四": 4, "五": 5,
		"六": 6, "七": 7, "八": 8, "九": 9, "十": 10,
	}
	if m := teamCountCNPattern.FindStringSubmatch(lower); len(m) >= 2 {
		if n, ok := cnNumMap[m[1]]; ok && n > 0 {
			return n
		}
	}
	// 3. 英文单词数字 + teams：例如 "two teams"、"three teams"
	enNumMap := map[string]int{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
		"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	}
	if m := teamCountENPattern.FindStringSubmatch(lower); len(m) >= 2 {
		if n, ok := enNumMap[m[1]]; ok && n > 0 {
			return n
		}
	}
	return 0
}

// explicitToolRequestPattern matches snake_case tool identifiers with at least
// two underscores (e.g. cli_admin_skill_install_from_url, plan_and_execute).
// Single-underscore everyday tools (exec_command, web_search) deliberately do
// not match — mentioning them is a normal direct request, not an orchestration
// demand.
var explicitToolRequestPattern = regexp.MustCompile(`[a-z][a-z0-9]*(?:_[a-z0-9]+){2,}`)

// detectTeamIntent scans the user message for explicit team-formation keywords
// and returns the recommended mode ("parallel" or "dag"). Returns "" if no
// team intent is detected.
//
// This is a backend-side fallback for when the Spirit LLM passes mode="" or
// "auto" despite the user explicitly asking for teams. The DECISION.md prompt
// instructs the LLM to pass explicit mode, but LLMs don't always comply —
// this ensures teams are created when the user clearly requests them.
//
// Mode selection rationale (aligned with DECISION.md three-mode system):
//   - "分派N个团队"/"组建团队"/"团队协作" (team formation keywords) → dag:
//     user expects one or more multi-member teams (≥2 members each).
//   - "并行"/"同时执行" (parallel keywords) → parallel: each subtask gets 1
//     agent, no multi-member teams formed. Use when subtasks are independent.
//   - Team keywords take precedence over parallel keywords: if the user
//     mentions "团队" they want dag (multi-member collaboration), not parallel.
func detectTeamIntent(message string) string {
	lower := strings.ToLower(message)
	// Team formation keywords: user wants one or more multi-member teams.
	// Includes quantity-based patterns ("分派两个团队") and generic team
	// keywords ("组建团队", "团队协作"). All route to `dag` mode so each team
	// can have ≥2 members (parallel would create 1-member teams, violating
	// the "team ≥2 members" rule in DECISION.md).
	teamKeywords := []string{
		// Quantity-based team requests (highest precedence)
		"两个team", "2个team", "两支team", "2支team",
		"两个团队", "2个团队", "两支团队", "2支团队",
		"三个team", "3个team", "三支team", "3支team",
		"三个团队", "3个团队", "三支团队", "3支团队",
		"多个团队", "多个team", "多支团队", "多支team",
		"分派两个", "分派2个", "分派三个", "分派3个",
		"分派团队", "分派team",
		// Generic team formation keywords
		// 2026-07-04 修复：扩展关键词，识别"组建一个团队"、"组建三个团队"等
		// 数量+量词变体，避免 QuickAssess 误判为 simple 导致不触发规划。
		"组建团队", "组建队", "组建team", "form a team", "build a team",
		"团队协作", "团队a", "团队b", "团队c",
		"组建一个团队", "组建一支团队", "组建1个团队", "组建1支团队",
		"组建两个团队", "组建两支团队", "组建2个团队", "组建2支团队",
		"组建三个团队", "组建三支团队", "组建3个团队", "组建3支团队",
		"组建多个团队", "组建多支团队",
		"创建团队", "创建一个团队", "创建一支团队",
		"成立团队", "成立一个团队",
		"编排团队", "编排一个团队",
		"调度团队", "调度一个团队",
	}
	for _, kw := range teamKeywords {
		if strings.Contains(lower, kw) {
			return "dag"
		}
	}
	// Parallel keywords: user wants concurrent independent subtasks. parallel
	// mode creates 1 agent per subtask (no multi-member teams), which is the
	// correct semantic for "并行处理多个独立子任务".
	parallelKeywords := []string{"并行", "同时执行", "并行处理", "parallel", "同时工作", "concurrently"}
	for _, kw := range parallelKeywords {
		if strings.Contains(lower, kw) {
			return "parallel"
		}
	}
	return ""
}

// decomposeTask uses LLM to decompose a complex task into subtasks (T1.6).
// teamCount > 0 时将作为硬约束传给 LLM，要求生成恰好 N 个 subtask（每个
// subtask 在 orchestrateDAG 中对应一个 team）。teamCount = 0 时使用默认范围。
// level 是 P2-5 思考强度路由的任务复杂度（空 = 不覆盖静态配置）。
func (impl *taskPlannerImpl) decomposeTask(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int, level biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "LLM catalog or HTTP client not configured")
	}

	prompt := buildDecompositionPrompt(userMessage, artifact, teamCount)

	// Resolve planner model via system setting (specify/inherit) with session
	// model fallback. Replaces legacy env-var + catalog-first approach.
	setting := biz.PlannerModelSetting{Mode: biz.PlannerModelModeInherit}
	if impl.plannerSetting != nil {
		if s, err := impl.plannerSetting.GetPlannerModel(ctx); err == nil {
			setting = s
		}
	}
	sessionProvider, sessionModel := biz.PlannerSessionModelFromCtx(ctx)
	provider, model := ResolvePlannerModel(ctx, setting, sessionProvider, sessionModel, impl.catalog, impl.lg, biz.SpiritStepPlannerAssess, "TaskPlanner")
	if provider == "" || model == "" {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "no provider/model configured for task decomposition (set planner_model_mode in system settings or add enabled models in catalog)")
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "get provider config").WithCause(err)
	}

	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)

	// P2-5：按任务复杂度路由 thinking effort（显式复杂度覆盖静态配置）。
	// 任务分解本身是规划类工作，复杂度来自外层 Plan() 的六维评估。
	if eff := biz.ResolveThinkingEffort(cfg.ThinkingEffort, level); eff != "" {
		cfg.ThinkingEffort = eff
	}

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Decompose the following task:\n\n" + userMessage},
	}

	callCtx, cancel := context.WithTimeout(ctx, tools.DecomposeLLMTimeout)
	defer cancel()

	text, _, _, _, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)
	if err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "LLM call failed").WithCause(err)
	}

	text = stripDecompositionFences(text)
	subTasks, err := parseDecompositionOutput(text)
	if err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "parse decomposition").WithCause(err)
	}

	if len(subTasks) == 0 {
		return nil, nil, nil
	}

	// Validate: no cycles, all depends_on references exist
	if err := validateSubTaskDAG(subTasks); err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "invalid DAG").WithCause(err)
	}

	dag := buildDAGFromSubTasks(subTasks)
	return subTasks, dag, nil
}

// decomposeTaskStream 是 decomposeTask 的流式变体，带 P3 重试可靠性：
//   - 单次尝试逻辑抽取为 llmDecomposeAttempt（由 llmAttemptFn 注入，生产默认指向该方法）
//   - 瞬时故障（idle/网络抖动/EOF）按指数退避重试，每次重试对前端发 decompose_retry 进度；
//     F8/Y3：重试有上限（默认 5 次，含首次尝试），耗尽后返回错误走 decompose_failed 降级——
//     无限重试会让 turn 永远卡在「规划中」并持续烧 LLM 调用
//   - 永久性错误（配置缺失/鉴权/上下文溢出）立即熔断不重试
//   - 父 ctx 取消立即穿透，不等待下一次退避
//
// 每次尝试的思考流独立发布——重试时旧 thinkingPub 已以 Fail() 闭合，新的 Attempt
// 会创建新的 planningThinkingPublisher，前端能看到「思考块失败 → 新思考块」的连贯动画。
//
// 事件流幂等（S1 修复）：subtask 全局 ID 由 planID+原始 ID 确定性生成（planStepID），
// 重试时新尝试重发的 PlanStep/GraphNode 事件与旧尝试同 ID，前端按 ID 合并去重，
// 计划面板不出现重复步骤。残余边界：重试后子任务数少于旧尝试已发布数时，多余步骤
// 残留（同 prompt 下结构稳定，罕见）——视为可接受。
func (impl *taskPlannerImpl) decomposeTaskStream(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int, spiritSessionID, planID string, level biz.ComplexityLevel, onSubTask func(st biz.SubTask, index int)) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
	attemptFn := impl.llmAttemptFn
	if attemptFn == nil {
		attemptFn = impl.llmDecomposeAttempt
	}
	backoffFn := impl.retryBackoffFn
	if backoffFn == nil {
		backoffFn = defaultDecomposeBackoff
	}
	maxAttempts := impl.maxDecomposeAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxDecomposeAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		subTasks, dag, err := attemptFn(ctx, userMessage, artifact, teamCount, spiritSessionID, planID, level, onSubTask)
		if err == nil {
			return subTasks, dag, nil
		}
		if !isRetriableDecomposeError(err) {
			// 永久性错误（鉴权/配置/上下文溢出/父 ctx 取消）——熔断。
			return nil, nil, err
		}
		// 父 ctx 取消（被包装为 Retriable 前先判）——立即穿透。
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		lastErr = err
		// F8/Y3：达到重试上限即熔断，不再退避——事件即将按 decompose_failed
		// 降级，额外 backoff 只会延迟失败信号。
		if attempt >= maxAttempts {
			break
		}

		// 发布重试进度（attempt 从 2 起——第 1 次就是首次尝试，不算「重试」）。
		if impl.eventBus != nil {
			impl.publishOrchestrationProgress(ctx, spiritSessionID, "decompose_retry", map[string]any{
				"attempt": attempt + 1,
				"reason":  "llm_stream_timeout_or_transient_failure",
			})
		}
		impl.lg.Warn("任务分解瞬时故障，准备重试",
			loggateway.StepID(biz.SpiritStepPlannerDecompose),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Int("attempt", attempt),
			loggateway.Err(err),
		)

		// 指数退避——父 ctx 取消立即穿透。
		delay := backoffFn(attempt)
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	impl.lg.Warn("任务分解重试耗尽上限，熔断降级",
		loggateway.StepID(biz.SpiritStepPlannerDecompose),
		loggateway.Str("spirit_session_id", spiritSessionID),
		loggateway.Int("max_attempts", maxAttempts),
		loggateway.Err(lastErr),
	)
	return nil, nil, apierror.Internal(apierror.DomainSpirit, "task decompose failed after %d attempts", maxAttempts).WithCause(lastErr)
}

// llmDecomposeAttempt 是 decomposeTaskStream 的单次尝试实现——负责一次完整的
// LLM 流式调用 + 解析。任何错误都会被上层重试循环分类处理。
func (impl *taskPlannerImpl) llmDecomposeAttempt(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int, spiritSessionID, planID string, level biz.ComplexityLevel, onSubTask func(st biz.SubTask, index int)) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
	prompt := buildDecompositionPrompt(userMessage, artifact, teamCount)

	setting := biz.PlannerModelSetting{Mode: biz.PlannerModelModeInherit}
	if impl.plannerSetting != nil {
		if s, err := impl.plannerSetting.GetPlannerModel(ctx); err == nil {
			setting = s
		}
	}
	sessionProvider, sessionModel := biz.PlannerSessionModelFromCtx(ctx)
	provider, model := ResolvePlannerModel(ctx, setting, sessionProvider, sessionModel, impl.catalog, impl.lg, biz.SpiritStepPlannerAssess, "TaskPlanner")
	if provider == "" || model == "" {
		return nil, nil, &decomposeConfigError{err: errors.New("no provider/model configured for task decomposition")}
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "get provider config").WithCause(err)
	}

	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)

	// P2-5：按任务复杂度路由 thinking effort（显式复杂度覆盖静态配置）。
	// 任务分解本身是规划类工作，复杂度来自外层 Plan() 的六维评估。
	if eff := biz.ResolveThinkingEffort(cfg.ThinkingEffort, level); eff != "" {
		cfg.ThinkingEffort = eff
	}

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Decompose the following task:\n\n" + userMessage},
	}

	callCtx, cancel := context.WithTimeout(ctx, tools.DecomposeLLMTimeout)
	defer cancel()

	parser := newStreamSubTaskParser()
	var subTasks []biz.SubTask
	idRemap := make(map[string]string)

	// P1b：规划思考可见性——每次尝试独立发布一个 thinking step，保证前端可见
	// 重试过程（旧 step 以 Failed 闭合、新 step 以 Created 新建）。
	thinkingPub := newPlanningThinkingPublisher(impl.seq, ctx, spiritSessionID)

	onDelta := func(piece string) error {
		objects := parser.Feed(piece)
		for _, objJSON := range objects {
			st, parseErr := parseStreamSubTask(objJSON, idRemap, planID)
			if parseErr != nil {
				impl.lg.Warn("流式 subtask 解析失败，跳过",
					loggateway.StepID(biz.SpiritStepPlannerDecompose),
					loggateway.Err(parseErr),
				)
				continue
			}
			subTasks = append(subTasks, st)
			if onSubTask != nil {
				onSubTask(st, len(subTasks)-1)
			}
		}
		return nil
	}

	callbacks := StreamCallbacks{OnContent: onDelta}
	if thinkingPub != nil {
		callbacks.OnReasoning = thinkingPub.OnReasoning
	}
	text, reasoning, _, _, callErr := CallOpenAICompatChatStream(callCtx, impl.httpClient, cfg, model, msgs, callbacks)
	if thinkingPub != nil {
		if callErr != nil {
			thinkingPub.Fail()
		} else {
			thinkingPub.Complete(reasoning)
		}
	}
	if callErr != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "LLM stream call failed").WithCause(callErr)
	}

	// 流式解析未能提取任何 subtask 时，回退到批量解析（处理边界情况如
	// LLM 输出的 JSON 格式与解析器预期不完全匹配）。
	if len(subTasks) == 0 {
		text = stripDecompositionFences(text)
		var batchErr error
		subTasks, batchErr = parseDecompositionOutput(text)
		if batchErr != nil {
			return nil, nil, apierror.Internal(apierror.DomainSpirit, "parse decomposition").WithCause(batchErr)
		}
	}

	if len(subTasks) == 0 {
		return nil, nil, nil
	}

	resolveForwardRefs(subTasks)

	if err := validateSubTaskDAG(subTasks); err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "invalid DAG").WithCause(err)
	}

	dag := buildDAGFromSubTasks(subTasks)
	return subTasks, dag, nil
}

// parseStreamSubTask 解析单个 subtask JSON 对象，重映射 ID 并解析 depends_on。
// idRemap 累积 LLM 原始 ID → 全局唯一 ID 的映射，供后续 subtask 的 depends_on
// 解析使用。planID 参与全局 ID 的确定性生成（跨重试尝试稳定）。
func parseStreamSubTask(objJSON string, idRemap map[string]string, planID string) (biz.SubTask, error) {
	var raw struct {
		ID                   string                    `json:"id"`
		Name                 string                    `json:"name"`
		Description          string                    `json:"description"`
		DependsOn            []string                  `json:"depends_on"`
		RequiredCapabilities []string                  `json:"required_capabilities"`
		Priority             int                       `json:"priority"`
		EstimatedComplexity  float64                   `json:"estimated_complexity"`
		Deliverables         []biz.DeliverableContract `json:"deliverables"`
		InputContract        []biz.DeliverableContract `json:"input_contract"`
		DomainPath           string                    `json:"domain_path"`
	}
	if err := json.Unmarshal([]byte(objJSON), &raw); err != nil {
		return biz.SubTask{}, err
	}
	if strings.TrimSpace(raw.ID) == "" || strings.TrimSpace(raw.Name) == "" {
		return biz.SubTask{}, fmt.Errorf("empty id or name")
	}

	// 重映射 ID：st_1 → 确定性全局 ID（planStepID）。与 parseDecompositionOutput
	// 的随机 UUID 不同——流式路径有 P3 重试，跨尝试必须稳定，前端按 ID 合并去重。
	if _, ok := idRemap[raw.ID]; !ok {
		idRemap[raw.ID] = planStepID(planID, raw.ID)
	}

	// 解析 depends_on：仅保留已映射的引用（前向引用在 resolveForwardRefs 中清理）。
	resolvedDeps := make([]string, 0, len(raw.DependsOn))
	for _, dep := range raw.DependsOn {
		if mapped, ok := idRemap[dep]; ok {
			resolvedDeps = append(resolvedDeps, mapped)
		}
	}

	if raw.DependsOn == nil {
		raw.DependsOn = []string{}
	}
	if raw.RequiredCapabilities == nil {
		raw.RequiredCapabilities = []string{}
	}

	return biz.SubTask{
		ID:                   idRemap[raw.ID],
		Name:                 raw.Name,
		Description:          raw.Description,
		DependsOn:            resolvedDeps,
		RequiredCapabilities: raw.RequiredCapabilities,
		Priority:             raw.Priority,
		EstimatedComplexity:  raw.EstimatedComplexity,
		Deliverables:         raw.Deliverables,
		InputContract:        raw.InputContract,
		DomainPath:           raw.DomainPath,
	}, nil
}

// planStepID 生成跨重试尝试确定性的 subtask 全局 ID：同一 plan + 同一 LLM 原始
// ID 恒映射同一值——P3 重试时新尝试重发的 PlanStep/GraphNode 事件与旧尝试同 ID，
// 前端按 ID 合并去重（事件流幂等），计划面板不出现重复步骤。
func planStepID(planID, rawID string) string {
	return "st_" + uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.plan_step.v2:"+planID+":"+rawID)).String()
}

// resolveForwardRefs 清理 subtask 中无效的 depends_on 引用（指向不存在的
// subtask ID）。用于流式解析后的最终清理。
func resolveForwardRefs(subTasks []biz.SubTask) {
	validIDs := make(map[string]bool, len(subTasks))
	for _, st := range subTasks {
		validIDs[st.ID] = true
	}
	for i := range subTasks {
		filtered := subTasks[i].DependsOn[:0]
		for _, depID := range subTasks[i].DependsOn {
			if validIDs[depID] {
				filtered = append(filtered, depID)
			}
		}
		subTasks[i].DependsOn = filtered
	}
}

// publishV2BoardShell 在流式分解开始前发布空的 PlanBoard + GraphStage 壳，
// 使前端能立即显示"规划中"面板，后续 PlanStep/GraphNode 事件渐进填充。
func (impl *taskPlannerImpl) publishV2BoardShell(ctx context.Context, planID string, strategy biz.OrchestrationStrategy, input biz.PlanInput) {
	if impl.seq == nil {
		return
	}
	rootTaskID := string(RootTaskActivityIDFromCtx(ctx))
	pbID := "pb_" + planID
	gsID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v2:"+pbID)).String()
	now := time.Now()

	pb := biz.PlanBoard{
		ID:        pbID,
		TaskID:    rootTaskID,
		TurnID:    event.TurnIDFromContext(ctx),
		SessionID: input.SpiritSessionID,
		Strategy:  mapV1StrategyToV2(strategy),
		Status:    biz.PlanStatusPlanning,
		Steps:     nil,
		StartedAt: now,
		Version:   1,
	}
	gs := biz.GraphStage{
		ID:          gsID,
		TaskID:      rootTaskID,
		TurnID:      event.TurnIDFromContext(ctx),
		SessionID:   input.SpiritSessionID,
		PlanBoardID: pbID,
		Nodes:       nil,
		Status:      biz.GraphStageStatusRunning,
		StartedAt:   now,
		Version:     1,
	}
	impl.seq.Publish(ctx, biz.NewPlanBoardCreatedEvent(pb))
	impl.seq.Publish(ctx, biz.NewGraphStageCreatedEvent(gs))
}

// publishV2BoardFailed 在分解失败/为空时给已发布的 PlanBoard/GraphStage 壳
// 补发终态事件（F9/Y4 修复）——否则前端计划面板永远停在「规划中」、DAG 块
// 永远 running。仅在 seq 非 nil（壳已发布）时有效。
func (impl *taskPlannerImpl) publishV2BoardFailed(ctx context.Context, planID string, input biz.PlanInput) {
	if impl.seq == nil {
		return
	}
	rootTaskID := string(RootTaskActivityIDFromCtx(ctx))
	pbID := "pb_" + planID
	gsID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v2:"+pbID)).String()
	now := time.Now()
	impl.seq.Publish(ctx, biz.NewPlanBoardUpdatedEvent(biz.PlanBoard{
		ID:          pbID,
		TaskID:      rootTaskID,
		TurnID:      event.TurnIDFromContext(ctx),
		SessionID:   input.SpiritSessionID,
		Status:      biz.PlanStatusFailed,
		CompletedAt: &now,
		Version:     2,
	}))
	impl.seq.Publish(ctx, biz.NewGraphStageFailedEvent(biz.GraphStage{
		ID:          gsID,
		TaskID:      rootTaskID,
		TurnID:      event.TurnIDFromContext(ctx),
		SessionID:   input.SpiritSessionID,
		PlanBoardID: pbID,
		Status:      biz.GraphStageStatusFailed,
		CompletedAt: &now,
		Version:     2,
	}))
}

// publishV2PlanStep 在流式分解中每解析出一个 subtask 时发布对应的
// PlanStepStartedEvent + GraphNodeUpdatedEvent，实现前端级联动画效果。
func (impl *taskPlannerImpl) publishV2PlanStep(ctx context.Context, st biz.SubTask, planID string, index int, input biz.PlanInput) {
	if impl.seq == nil {
		return
	}
	rootTaskID := string(RootTaskActivityIDFromCtx(ctx))
	pbID := "pb_" + planID
	gsID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v2:"+pbID)).String()
	now := time.Now()

	ps := biz.PlanStep{
		ID:            st.ID,
		PlanID:        pbID,
		TaskID:        rootTaskID,
		Label:         st.Name,
		Description:   st.Description,
		DependsOn:     append([]string(nil), st.DependsOn...),
		Status:        biz.PlanStepStatusPending,
		StartedAt:     now,
		Seq:           int64(index + 1),
		Version:       1,
		Deliverables:  append([]biz.DeliverableContract(nil), st.Deliverables...),
		InputContract: append([]biz.DeliverableContract(nil), st.InputContract...),
	}
	gn := biz.GraphNode{
		ID:           st.ID,
		GraphStageID: gsID,
		Label:        st.Name,
		DagNodeID:    st.ID,
		Status:       biz.MapPlanStepToGraphNodeStatus(biz.PlanStepStatusPending),
		DependsOn:    append([]string(nil), st.DependsOn...),
	}
	impl.seq.Publish(ctx, biz.NewPlanStepStartedEvent(ps, input.SpiritSessionID))
	impl.seq.Publish(ctx, biz.NewGraphNodeUpdatedEvent(gn, rootTaskID, input.SpiritSessionID))
}

// buildDecompositionPrompt creates the system prompt for task decomposition.
// teamCount > 0 时强制要求 LLM 生成恰好 N 个 subtask（用户明确请求 N 个 team）；
// teamCount = 0 时使用默认 2-6 范围。
//
// 2026-07-04 问题 2 修复：原 prompt 固定 "2-6 subtasks"，未传递用户的数量
// 约束。当用户说"派出2个team"时，LLM 可能产生 5 个 subtask，orchestrateDAG
// 为每个 subtask 创建一个 team，导致最终多出 3 个 team。
func buildDecompositionPrompt(userMessage string, artifact *biz.IntentArtifact, teamCount int) string {
	intentContext := ""
	if artifact != nil {
		intentContext = fmt.Sprintf("\nIntent analysis:\n- Refined goal: %s\n- Intent kind: %s\n- Risk flags: %v",
			artifact.RefinedGoal,
			artifact.IntentKind,
			artifact.RiskFlags,
		)
		// N5 (2026-08-13 链路审查): SuccessCriteria/SearchHints 由意图识别产物
		// 携带，但此前未进入分解 prompt——子任务契约可能与成功标准脱节。
		// 非空时才追加，保持无意图产物路径的字节稳定。
		if len(artifact.SuccessCriteria) > 0 {
			intentContext += fmt.Sprintf("\n- Success criteria: %v", artifact.SuccessCriteria)
		}
		if len(artifact.SearchHints) > 0 {
			intentContext += fmt.Sprintf("\n- Search hints: %v", artifact.SearchHints)
		}
	}

	countRule := "Break down complex tasks into 2-6 subtasks."
	if teamCount > 0 {
		// 用户明确请求 N 个 team：硬约束生成恰好 N 个 subtask。
		// 每个 subtask 将在 orchestrateDAG 中对应一个独立 team。
		countRule = fmt.Sprintf(`The user has explicitly requested EXACTLY %d teams.
You MUST produce EXACTLY %d subtasks — no more, no less.
Each subtask will be assigned to one dedicated team, so the subtask count MUST equal the requested team count.`, teamCount, teamCount)
	}

	return fmt.Sprintf(`You are a task decomposition specialist. %s

Rules:
- Each subtask must have: id (st_1, st_2, etc.), name, description, depends_on (array of other subtask IDs), required_capabilities (from the predefined list), priority (1-5, 1=highest), estimated_complexity (0.0-1.0), domain_path (domain classification from the lexicon below)
- The "name" field MUST be a short noun-phrase suitable for displaying as a team name (e.g. "Code Analysis Team", "Data Pipeline Builder"), NOT a sentence-length task description. The "name" will be shown to the user as the team's display name; "id" is internal-only and never shown.
- Output ONLY a JSON array, no markdown fences, no commentary
- required_capabilities must use these predefined tags: go-backend, go-kratos, vue3-frontend, quasar-ui, devops, database, architecture, testing, security, research, documentation, api-design, system-admin
- System administration subtasks (Skill/MCP/package installation, system resource management) MUST be tagged "system-admin", and their "description" MUST be intent-based: state the outcome to achieve (what), the source URL, and the exact cli_admin_* tool name to use (e.g. "使用 cli_admin_skill_install_from_url 从 https://github.com/example/xlsx-skill 安装 xlsx skill，完成后用 cli_admin_skill_get 确认 enabled=true"). NEVER put shell command text (pip install, git clone, etc.) into the description — the system butler has no shell/exec tools and shell text induces hallucinated calls to non-existent tools.
- domain_path must classify the subtask into this domain lexicon (use the most specific entry that fits; if none fits, use a top-level domain or "其他"): %s
- depends_on must only reference IDs of other subtasks in the array
- No circular dependencies allowed
- Subtasks should be independently executable where possible
- CRITICAL: Each subtask "description" must be fully self-contained. The executing team sees ONLY its own description — it cannot see the user message or other subtasks. Every concrete parameter the executor needs (URLs, file paths, branch/tag names, subpaths, skill/agent names, numeric values, flags) MUST be copied verbatim into the description. NEVER use context references such as "the given URL", "the above parameters", "使用给定的/上述的/前文提到的" — always inline the actual values.
- Each subtask MAY include "deliverables" (output contract array) and "input_contract" (input contract array). Contract element: {"name": string, "type": "document"|"code"|"data", "format": "markdown"|"json"|"zip", "description": string}. The contract "name" becomes the deliverable topic namespace that team members write via set_deliverable — keep it short (letters/digits in any language plus '_'/'-', no spaces or punctuation; a concise slug like "root-cause-report" or "根因报告" works) and NEVER use the reserved names "summary" or "cognition" (writes under them are rejected/overwritten).
- If subtask B depends_on subtask A, B's input_contract SHOULD declare references to A's deliverables using the SAME "name" values`, countRule, DomainLexiconPromptList()) + intentContext
}

// resolvePlannerProviderModel and resolveFallbackProviderModelFromCatalog
// were removed in favor of ResolvePlannerModel (planner_model_resolver.go),
// which reads the planner_model_mode system setting and falls back to the
// session's effective model (inherit mode) or the first enabled catalog model.

// parseDecompositionOutput parses the LLM output into SubTask slice.
func parseDecompositionOutput(text string) ([]biz.SubTask, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	// Extract JSON array
	if i := strings.Index(text, "["); i >= 0 {
		if j := strings.LastIndex(text, "]"); j > i {
			text = text[i : j+1]
		}
	}

	var rawTasks []struct {
		ID                   string                    `json:"id"`
		Name                 string                    `json:"name"`
		Description          string                    `json:"description"`
		DependsOn            []string                  `json:"depends_on"`
		RequiredCapabilities []string                  `json:"required_capabilities"`
		Priority             int                       `json:"priority"`
		EstimatedComplexity  float64                   `json:"estimated_complexity"`
		Deliverables         []biz.DeliverableContract `json:"deliverables"`
		InputContract        []biz.DeliverableContract `json:"input_contract"`
		DomainPath           string                    `json:"domain_path"`
	}

	if err := json.Unmarshal([]byte(text), &rawTasks); err != nil {
		return nil, apierror.Internal(apierror.DomainSpirit, "json unmarshal").WithCause(err)
	}

	// 2026-07-05 修复：LLM prompt 指定生成 st_1/st_2/... 格式的 ID，
	// 跨 session 会冲突（plan_steps_v2 表 id 字段全局 UNIQUE）。这里将
	// LLM 返回的 ID 重写为 st_<uuid>，同步重写 DependsOn 中的引用。
	// 所有后端/前端代码都不解析 ID 内容，仅作不透明字符串使用，因此
	// 重写不会破坏引用链。
	idRemap := make(map[string]string, len(rawTasks))
	for _, rt := range rawTasks {
		if strings.TrimSpace(rt.ID) == "" || strings.TrimSpace(rt.Name) == "" {
			continue
		}
		if _, exists := idRemap[rt.ID]; !exists {
			idRemap[rt.ID] = "st_" + uuid.NewString()
		}
	}

	subTasks := make([]biz.SubTask, 0, len(rawTasks))
	for _, rt := range rawTasks {
		if strings.TrimSpace(rt.ID) == "" || strings.TrimSpace(rt.Name) == "" {
			continue
		}
		if rt.DependsOn == nil {
			rt.DependsOn = []string{}
		}
		if rt.RequiredCapabilities == nil {
			rt.RequiredCapabilities = []string{}
		}
		if rt.Priority == 0 {
			rt.Priority = 3
		}
		if rt.EstimatedComplexity == 0 {
			rt.EstimatedComplexity = 0.5
		}
		// 重写 DependsOn 中的旧 ID 引用为新 UUID 格式
		rewiredDeps := make([]string, 0, len(rt.DependsOn))
		for _, depID := range rt.DependsOn {
			if newID, ok := idRemap[depID]; ok {
				rewiredDeps = append(rewiredDeps, newID)
			} else {
				// 未知引用保留原值，validateSubTaskDAG 会报错
				rewiredDeps = append(rewiredDeps, depID)
			}
		}
		subTasks = append(subTasks, biz.SubTask{
			ID:                   idRemap[rt.ID],
			Name:                 rt.Name,
			Description:          rt.Description,
			DependsOn:            rewiredDeps,
			RequiredCapabilities: rt.RequiredCapabilities,
			Priority:             rt.Priority,
			EstimatedComplexity:  rt.EstimatedComplexity,
			Deliverables:         sanitizeContracts(rt.Deliverables),
			InputContract:        sanitizeContracts(rt.InputContract),
			DomainPath:           NormalizeDomainPath(rt.DomainPath),
		})
	}

	// P1 形式契约（B.10.15.2）兜底派生：LLM 未输出契约但存在 DAG 依赖时，
	// 从 subtask 确定性派生（{step_id}_output, document/markdown），保证注入
	// 提示可引用、验证器有事可验。派生名在 DAG 内构造即匹配。
	deriveFallbackContracts(subTasks)

	return subTasks, nil
}

// sanitizeContracts drops contract entries with a blank name (advisory
// contract — malformed entries must not break planning) and entries named
// after the reserved deliverable state keys ("summary"/"cognition"): those
// names become MDC topics 1:1, but set_deliverable rejects writes under
// reserved keys, so keeping them would create unsatisfiable member contracts
// (TS9-BUG-4: planner authored a contract literally named "summary").
func sanitizeContracts(in []biz.DeliverableContract) []biz.DeliverableContract {
	if len(in) == 0 {
		return nil
	}
	out := make([]biz.DeliverableContract, 0, len(in))
	for _, c := range in {
		name := strings.TrimSpace(c.Name)
		if name == "" || name == "summary" || name == "cognition" {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// deriveFallbackContracts deterministically derives contracts for DAG edges
// the LLM left undeclared:
//   - producer (subtask with dependents, no LLM deliverables) gets
//     {name: "<step_id>_output", type: document, format: markdown};
//   - consumer (subtask with depends_on, no LLM input_contract) gets one
//     derived entry per upstream referencing the same name.
//
// Derived names match by construction, so the advisory validator only flags
// genuine LLM inconsistencies. Subtasks without any dependency edge (neither
// producer nor consumer) get nothing.
func deriveFallbackContracts(subTasks []biz.SubTask) {
	hasDependent := make(map[string]bool, len(subTasks))
	for _, st := range subTasks {
		for _, depID := range st.DependsOn {
			hasDependent[depID] = true
		}
	}
	for i := range subTasks {
		st := &subTasks[i]
		if len(st.Deliverables) == 0 && hasDependent[st.ID] {
			st.Deliverables = []biz.DeliverableContract{{
				Name:        st.ID + "_output",
				Type:        "document",
				Format:      "markdown",
				Description: "derived fallback deliverable for " + st.Name,
			}}
		}
	}
	for i := range subTasks {
		st := &subTasks[i]
		if len(st.InputContract) > 0 || len(st.DependsOn) == 0 {
			continue
		}
		for _, depID := range st.DependsOn {
			st.InputContract = append(st.InputContract, biz.DeliverableContract{
				Name:        depID + "_output",
				Type:        "document",
				Format:      "markdown",
				Description: "derived fallback input from upstream " + depID,
			})
		}
	}
}

// closedLoopSignalPattern 匹配事故/运维闭环类信号词。刻意不含「修复/恢复」
// 等泛化词——代码修复类任务不应触发复盘节点追加。
var closedLoopSignalPattern = regexp.MustCompile(`告警|事故|故障|宕机|停机|复盘|incident|outage|postmortem`)

// appendClosedLoopPostmortem 实现 TS9-GAP-1：闭环类任务（事故/告警/故障处置）
// 在 LLM 分解未产出复盘节点时，由引擎确定性追加一个「事故复盘」subtask，
// 依赖当前 DAG 的全部叶子节点（所有处置完成后才复盘）。与
// deriveFallbackContracts 同模式：引擎补齐 LLM 之不足，流程控制权在引擎。
//
// 跳过条件：subtask < 2（非编排任务）、消息无闭环信号、LLM 已产出复盘节点。
// 返回新切片，不修改入参。
func appendClosedLoopPostmortem(userMessage string, subTasks []biz.SubTask) ([]biz.SubTask, *biz.SubTask, bool) {
	if len(subTasks) < 2 {
		return nil, nil, false
	}
	if !closedLoopSignalPattern.MatchString(strings.ToLower(userMessage)) {
		return nil, nil, false
	}
	for _, st := range subTasks {
		name := strings.ToLower(st.Name)
		if strings.Contains(name, "复盘") || strings.Contains(name, "postmortem") {
			return nil, nil, false
		}
	}
	// 叶子节点：没有任何其他节点依赖它（与 buildDAGFromSubTasks LeafIDs 同义）。
	depended := make(map[string]bool, len(subTasks))
	for _, st := range subTasks {
		for _, depID := range st.DependsOn {
			depended[depID] = true
		}
	}
	leaves := make([]string, 0, 1)
	for _, st := range subTasks {
		if !depended[st.ID] {
			leaves = append(leaves, st.ID)
		}
	}
	if len(leaves) == 0 {
		return nil, nil, false
	}
	pm := biz.SubTask{
		ID:   "st_" + uuid.NewString(),
		Name: "事故复盘",
		// description 自包含（执行团队只能看到自己的 description）：说明输入
		// 来源（上游交付物）与产出要求（复盘报告结构与交付 topic）。
		Description:          "对本次事故处置全流程进行复盘：基于上游团队的交付物（告警分诊、根因定位、修复方案、恢复执行与验证结果），产出标准事故复盘报告，包含事故时间线、根因分析（直接/间接/系统层面）、处置过程评估、可执行的改进项清单。通过 set_deliverable 将复盘报告写入 postmortem-report。",
		DependsOn:            leaves,
		RequiredCapabilities: []string{"documentation", "research"},
		Priority:             5,
		EstimatedComplexity:  0.3,
		Deliverables: []biz.DeliverableContract{{
			Name:        "postmortem-report",
			Type:        "document",
			Format:      "markdown",
			Description: "事故复盘报告（时间线/根因/处置评估/改进项）",
		}},
		DomainPath: "办公/文档",
	}
	updated := make([]biz.SubTask, 0, len(subTasks)+1)
	updated = append(updated, subTasks...)
	updated = append(updated, pm)
	return updated, &pm, true
}

// validateSubTaskDAG checks for duplicate IDs, cycles and invalid references.
func validateSubTaskDAG(tasks []biz.SubTask) error {
	idSet := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		// F7a/B2：重复 ID 必须拒绝——同名节点互相遮蔽，依赖悬空/错接，
		// 执行器会静默跑错图。
		if idSet[t.ID] {
			return apierror.BadRequest(apierror.DomainSpirit, "duplicate subtask ID %s", t.ID)
		}
		idSet[t.ID] = true
	}

	// Check all depends_on references exist
	for _, t := range tasks {
		for _, depID := range t.DependsOn {
			if !idSet[depID] {
				return apierror.BadRequest(apierror.DomainSpirit, "subtask %s depends on non-existent subtask %s", t.ID, depID)
			}
		}
	}

	// Check for cycles using DFS
	const (
		white = 0
		gray  = 1
		black = 2
	)
	colors := make(map[string]int, len(tasks))
	for _, t := range tasks {
		colors[t.ID] = white
	}

	taskMap := make(map[string]*biz.SubTask, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
	}

	var dfs func(id string) error
	dfs = func(id string) error {
		colors[id] = gray
		t := taskMap[id]
		if t != nil {
			for _, depID := range t.DependsOn {
				switch colors[depID] {
				case gray:
					return apierror.BadRequest(apierror.DomainSpirit, "cycle detected: %s → %s", id, depID)
				case white:
					if err := dfs(depID); err != nil {
						return err
					}
				}
			}
		}
		colors[id] = black
		return nil
	}

	for _, t := range tasks {
		if colors[t.ID] == white {
			if err := dfs(t.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildDAGFromSubTasks constructs a PlanTaskDAG from subtasks.
func buildDAGFromSubTasks(tasks []biz.SubTask) *biz.PlanTaskDAG {
	if len(tasks) == 0 {
		return nil
	}

	dag := &biz.PlanTaskDAG{
		Nodes: tasks,
	}

	// Compute root IDs (no dependencies)
	hasIncoming := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		hasIncoming[t.ID] = false
	}
	for _, t := range tasks {
		for _, depID := range t.DependsOn {
			hasIncoming[depID] = true
		}
	}
	for id, has := range hasIncoming {
		if !has {
			dag.RootIDs = append(dag.RootIDs, id)
		}
	}

	// Compute leaf IDs (nothing depends on them)
	hasOutgoing := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		for _, depID := range t.DependsOn {
			hasOutgoing[depID] = true
		}
	}
	for _, t := range tasks {
		if !hasOutgoing[t.ID] {
			dag.LeafIDs = append(dag.LeafIDs, t.ID)
		}
	}

	return dag
}

// mergeSubTasks merges specified subtasks into one.
func mergeSubTasks(tasks []biz.SubTask, ids []string) []biz.SubTask {
	if len(ids) < 2 {
		return tasks
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	var merged biz.SubTask
	var remaining []biz.SubTask
	first := true
	for _, t := range tasks {
		if idSet[t.ID] {
			if first {
				merged = biz.SubTask{
					ID:                   t.ID,
					Name:                 t.Name,
					Description:          t.Description,
					DependsOn:            t.DependsOn,
					RequiredCapabilities: t.RequiredCapabilities,
					Priority:             t.Priority,
					EstimatedComplexity:  t.EstimatedComplexity,
				}
				first = false
			} else {
				merged.Description += "; " + t.Description
				for _, cap := range t.RequiredCapabilities {
					merged.RequiredCapabilities = append(merged.RequiredCapabilities, cap)
				}
				for _, dep := range t.DependsOn {
					if !idSet[dep] {
						merged.DependsOn = append(merged.DependsOn, dep)
					}
				}
				if t.Priority < merged.Priority {
					merged.Priority = t.Priority
				}
				merged.EstimatedComplexity = max(merged.EstimatedComplexity, t.EstimatedComplexity)
			}
		} else {
			remaining = append(remaining, t)
		}
	}
	if !first {
		remaining = append(remaining, merged)
	}
	return remaining
}

// splitSubTask splits a subtask into two.
func splitSubTask(tasks []biz.SubTask, id string) []biz.SubTask {
	var result []biz.SubTask
	for _, t := range tasks {
		if t.ID == id {
			// Split into two: original + new
			split := biz.SubTask{
				ID:                   id + "_b",
				Name:                 t.Name + " (Part 2)",
				Description:          t.Description,
				DependsOn:            []string{id},
				RequiredCapabilities: t.RequiredCapabilities,
				Priority:             t.Priority + 1,
				EstimatedComplexity:  t.EstimatedComplexity / 2,
			}
			t.Name = t.Name + " (Part 1)"
			t.EstimatedComplexity = t.EstimatedComplexity / 2
			result = append(result, t, split)
		} else {
			// Update references to the split task
			for i, dep := range t.DependsOn {
				if dep == id {
					t.DependsOn[i] = id + "_b"
				}
			}
			result = append(result, t)
		}
	}
	return result
}

// removeSubTask removes a subtask by ID.
func removeSubTask(tasks []biz.SubTask, id string) []biz.SubTask {
	var result []biz.SubTask
	for _, t := range tasks {
		if t.ID == id {
			continue
		}
		// Remove the deleted ID from depends_on
		var filteredDeps []string
		for _, dep := range t.DependsOn {
			if dep != id {
				filteredDeps = append(filteredDeps, dep)
			}
		}
		t.DependsOn = filteredDeps
		result = append(result, t)
	}
	return result
}

var fenceRE = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")

// stripDecompositionFences removes optional markdown code fences from model output.
func stripDecompositionFences(s string) string {
	s = strings.TrimSpace(s)
	if m := fenceRE.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return s
}

// publishOrchestrationProgress emits an orchestration_progress
// SystemNoticeEvent (WS-only, not persisted) so the frontend can render
// fine-grained loading feedback during planning. Nil bus or empty session
// → skipped (P-ORCH).
func (impl *taskPlannerImpl) publishOrchestrationProgress(ctx context.Context, spiritSessionID, phase string, extra map[string]any) {
	if impl.eventBus == nil || spiritSessionID == "" {
		return
	}
	meta := map[string]any{"phase": phase}
	for k, v := range extra {
		meta[k] = v
	}
	impl.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_progress", "orchestration progress: "+phase, meta))
}

// startDecomposeHeartbeat 在任务分解期间周期性发布 decomposing 进度事件
// （P2：用户可见的存活信号，避免长分解期间界面静止）。返回的 stop 函数
// 幂等且同步——返回后保证不再有心跳事件发出；ctx 取消时 goroutine 亦退出。
func (impl *taskPlannerImpl) startDecomposeHeartbeat(ctx context.Context, spiritSessionID string, interval time.Duration) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	safego.Go(ctx, "planner-decompose-heartbeat", func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				impl.publishOrchestrationProgress(ctx, spiritSessionID, "decomposing", map[string]any{
					"elapsed_seconds": int(time.Since(start).Seconds()),
				})
			}
		}
	})
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			<-done
		})
	}
}

// publishPlanCreated is retained as a no-op hook after TaskPlan persist.
// UI plan/graph boards are published exclusively via PublishV2Board (v2
// PlanBoard/GraphStage events). The legacy ActivityEventBus plan activity
// path has been removed.
func (impl *taskPlannerImpl) publishPlanCreated(ctx context.Context, plan *biz.TaskPlan, chatSessionID string) {
	_ = ctx
	_ = plan
	_ = chatSessionID
}

// PublishV2Board 通过 v2 Sequencer 发布 PlanBoard + PlanStep + GraphStage + GraphNode
// 创建事件。这些事件会被 Sequencer 持久化到 v2 表 + 推送到 WS，前端 v2 store 收到后
// 渲染 PlanBoardCard 和 GraphStageBlock。
//
// 设计参考：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md
// §3.2.2 GraphStage / §3.5.2 执行流程
//
// 关键关系：
//   - PlanBoard 与 GraphStage 一对一（GraphStage.PlanBoardID = PlanBoard.ID）
//   - PlanStep 与 GraphNode 一对一（GraphNode.ID = PlanStep.ID，确定性派生）
//   - 所有实体共用同一个 TaskID（rootTaskID）和 SpiritSessionID
//
// 2026-07-05 Step 3 修复：从 publishPlanCreated 内部移到 spirit_tools.go Phase 2
// 之后调用，使 PlanStep.AgentKeys 能从 allocPlan.Allocations 填充。原先在 Phase 1
// 发布时 allocPlan 不存在，导致 RealTeamOrchestrator 无法获取 LLM 分配结果，
// 退回查 DB 取错 agent（所有 team 用同一 agent）。
//
// 注意：此处仅发布 created 事件（status=pending/running）。后续节点状态与
// GraphStage terminal（completed/failed/interrupted）由 PlanExecutor 发布。
// spirit_team 的 v1 graph_stage 快照路径已废弃（2026-07-16）。
func (impl *taskPlannerImpl) PublishV2Board(ctx context.Context, plan *biz.TaskPlan, allocPlan *biz.AllocationPlan, chatSessionID string) (biz.PlanBoard, error) {
	if impl.seq == nil || plan == nil {
		return biz.PlanBoard{}, nil
	}
	spiritSessionID := plan.SpiritSessionID
	if spiritSessionID == "" {
		return biz.PlanBoard{}, nil
	}
	// 当 plan.SubTasks 为空时跳过发布，避免创建空 PlanBoard 与空 GraphStage。
	// 触发场景：PrePlanningGate force planning（complexity >= Moderate）
	// 时调用 Plan()，但任务分解失败或编排缓存命中导致 SubTasks 为空。
	if len(plan.SubTasks) == 0 {
		impl.lg.Info("PublishV2Board: SubTasks 为空，跳过发布 PlanBoard/GraphStage",
			loggateway.StepID(biz.SpiritStepPlannerPersist),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Str("plan_id", plan.ID),
		)
		return biz.PlanBoard{}, nil
	}
	// rootTaskID 从 ctx 读取（与 publishPlanCreated 一致）。
	rootTaskID := string(RootTaskActivityIDFromCtx(ctx))
	// SessionID 优先使用 chatSessionID（与 v1 Activity 一致），便于前端按 chat session
	// 过滤；fallback 到 spiritSessionID。
	sessionID := chatSessionID
	if sessionID == "" {
		sessionID = spiritSessionID
	}
	now := time.Now()
	// 同 turn/task 固定 PlanBoard ID：重复 PublishV2Board 更新原面板，不新建。
	// 优先 plan.ID（TaskPlan 稳定主键），否则用 rootTaskID；二者皆空时退回随机（不应发生）。
	pbSeed := plan.ID
	if pbSeed == "" {
		pbSeed = rootTaskID
	}
	pbID := "pb_" + pbSeed
	if pbSeed == "" {
		pbID = "pb_" + uuid.NewString()
	}
	// 构建 v2 PlanStep 列表（每个 SubTask 对应一个 PlanStep）。
	// 2026-07-05 Step 3 修复：从 allocPlan.Allocations 填充 AgentKeys，
	// 匹配规则：alloc.SubTaskID == SubTask.ID（== PlanStep.ID）。
	// 2026-07-05 FIX-A：DAG 模式下 alloc.TeamMemberKeys 也必须并入 AgentKeys，
	// 否则下游 RealTeamOrchestrator 只能组装 lead 单 agent team，
	// 导致前端左侧 agent 列表与实际 team 成员数不一致。
	planSteps := make([]biz.PlanStep, 0, len(plan.SubTasks))
	for i, st := range plan.SubTasks {
		ps := biz.PlanStep{
			ID:            st.ID,
			PlanID:        pbID,
			TaskID:        rootTaskID,
			Label:         st.Name,
			Description:   st.Description,
			DependsOn:     append([]string(nil), st.DependsOn...),
			Status:        biz.PlanStepStatusPending,
			StartedAt:     now,
			Seq:           int64(i + 1),
			Version:       1,
			Deliverables:  append([]biz.DeliverableContract(nil), st.Deliverables...),
			InputContract: append([]biz.DeliverableContract(nil), st.InputContract...),
		}
		if allocPlan != nil {
			for _, alloc := range allocPlan.Allocations {
				if alloc.SubTaskID != st.ID {
					continue
				}
				if alloc.AssignedKey != "" {
					ps.AgentKeys = append(ps.AgentKeys, alloc.AssignedKey)
				}
				for _, mk := range alloc.TeamMemberKeys {
					if mk != "" {
						ps.AgentKeys = append(ps.AgentKeys, mk)
					}
				}
			}
		}
		planSteps = append(planSteps, ps)
	}
	// 派生 GraphStage ID（确定性，基于 PlanBoard ID）。
	gsID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v2:"+pbID)).String()
	// 构建 GraphNode 列表（每个 PlanStep 对应一个 GraphNode）。
	graphNodes := make([]biz.GraphNode, 0, len(planSteps))
	for _, ps := range planSteps {
		gn := biz.GraphNode{
			ID:           ps.ID,
			GraphStageID: gsID,
			Label:        ps.Label,
			DagNodeID:    ps.ID,
			Status:       biz.MapPlanStepToGraphNodeStatus(ps.Status),
			DependsOn:    append([]string(nil), ps.DependsOn...),
		}
		graphNodes = append(graphNodes, gn)
	}
	// 构建 PlanBoard 实体。
	pb := biz.PlanBoard{
		ID:        pbID,
		TaskID:    rootTaskID,
		TurnID:    event.TurnIDFromContext(ctx), // from event.WithTurnID on the turn ctx
		SessionID: spiritSessionID,
		Strategy:  mapV1StrategyToV2(plan.Strategy),
		Status:    biz.PlanStatusPlanning,
		Steps:     planSteps,
		StartedAt: now,
		Version:   1,
	}
	// 构建 GraphStage 实体（1:1 关联 PlanBoard）。
	gs := biz.GraphStage{
		ID:          gsID,
		TaskID:      rootTaskID,
		TurnID:      event.TurnIDFromContext(ctx),
		SessionID:   spiritSessionID,
		PlanBoardID: pbID,
		Nodes:       graphNodes,
		Status:      biz.GraphStageStatusRunning,
		StartedAt:   now,
		Version:     1,
	}

	if plan.StreamPublished {
		// 流式路径：PlanBoard/GraphStage 壳 + PlanStep/GraphNode 已在 Plan()
		// 中渐进发布。此处更新 PlanSteps（填充 AgentKeys）、补发 GraphNode
		// 更新（携带截取/清理后的最终 DependsOn——F7b/B2 修复：否则 DAG
		// 视图永久残留悬挂边）并发送 PlanBoardUpdatedEvent（携带完整 Steps）。
		pb.Version = 2
		for _, ps := range planSteps {
			psUp := ps
			psUp.Version = 2
			impl.seq.Publish(ctx, biz.NewPlanStepUpdatedEvent(psUp, spiritSessionID))
		}
		for _, gn := range graphNodes {
			impl.seq.Publish(ctx, biz.NewGraphNodeUpdatedEvent(gn, rootTaskID, spiritSessionID))
		}
		impl.seq.Publish(ctx, biz.NewPlanBoardUpdatedEvent(pb))
	} else {
		// 非流式路径：批量发布 Created 事件。
		// 发布 PlanBoardCreatedEvent（先于 PlanStep 事件，保证前端先创建 PlanBoard）。
		impl.seq.Publish(ctx, biz.NewPlanBoardCreatedEvent(pb))
		// 发布 GraphStageCreatedEvent（先于 GraphNode 事件，保证前端先创建 GraphStage）。
		impl.seq.Publish(ctx, biz.NewGraphStageCreatedEvent(gs))
		// 发布 PlanStepStartedEvent（status=pending，使用 Started 状态表示已创建待执行）。
		for _, ps := range planSteps {
			impl.seq.Publish(ctx, biz.NewPlanStepStartedEvent(ps, spiritSessionID))
		}
		// 发布 GraphNodeUpdatedEvent（每个节点初始状态=pending）。
		for _, gn := range graphNodes {
			impl.seq.Publish(ctx, biz.NewGraphNodeUpdatedEvent(gn, rootTaskID, spiritSessionID))
		}
	}
	_ = sessionID // 暂未使用 chatSessionID 派生其他字段；保留供未来扩展
	return pb, nil
}

// mapV1StrategyToV2 将 v1 biz.OrchestrationStrategy 映射为 v2 biz.PlanStrategy。
func mapV1StrategyToV2(s biz.OrchestrationStrategy) biz.PlanStrategy {
	switch s {
	case biz.StrategyDirect:
		return biz.PlanStrategySequential
	case biz.StrategyParallel:
		return biz.PlanStrategyParallel
	case biz.StrategyDAG:
		return biz.PlanStrategyDAG
	default:
		return biz.PlanStrategySequential
	}
}

// ---------------------------------------------------------------------------
// P3：分解调用可靠性辅助——错误分类 + 重试退避。
// ---------------------------------------------------------------------------

// decomposeConfigError 表示分解前置条件不满足（catalog/http client 缺失、
// provider/model 未配置等）——重试相同请求无意义，属永久性错误。
type decomposeConfigError struct {
	err error
}

func (e *decomposeConfigError) Error() string {
	if e == nil || e.err == nil {
		return "decompose config error"
	}
	return e.err.Error()
}

func (e *decomposeConfigError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// decomposeFatalMarkers 标记 LLM 返回的永久性错误——重试相同请求不会成功。
var decomposeFatalMarkers = []string{
	"invalid_api_key",
	"unauthorized",
	"forbidden",
	"context length exceeded",
	"context_length_exceeded",
	"content_filter",
	"content filter",
}

// decomposeRetriableMarkers 标记瞬时故障——重试有意义。
// *StreamIdleError 的 Error() 输出必含 "llm stream idle"。
var decomposeRetriableMarkers = []string{
	"llm stream idle",
}

// isRetriableDecomposeError 对分解调用错误做保险丝分类：
//   - nil / 父 ctx 取消 / 永久性错误 → 不重试
//   - 其他（含被包装的 *StreamIdleError、io.EOF、网络抖动、超时）→ 无限重试
//
// 调用方必须在返回 Retriable 前检查 ctx 是否已取消——退避期间的穿透由调用方负责。
func isRetriableDecomposeError(err error) bool {
	if err == nil {
		return false
	}
	// 父 ctx 取消：用户停止 / 外层预算耗尽——永不重试。
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// 配置级错误（catalog/http 缺失、provider/model 未配置）——重试无意义。
	var cfgErr *decomposeConfigError
	if errors.As(err, &cfgErr) {
		return false
	}
	msg := strings.ToLower(err.Error())
	// 鉴权/上下文溢出/内容过滤——永久性错误。
	for _, m := range decomposeFatalMarkers {
		if strings.Contains(msg, m) {
			return false
		}
	}
	// 显式标记的瞬时故障（如 idle 停滞）——即使被上层 apierror 包装也能识别。
	for _, m := range decomposeRetriableMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	// 其他默认瞬时故障：重试有意义（网络抖动、EOF 等）。
	return true
}

// defaultMaxDecomposeAttempts 是分解瞬时故障的默认重试上限（含首次尝试）。
// F8/Y3：上限存在是为了让持续性故障最终走 decompose_failed 降级，而非无限重试。
const defaultMaxDecomposeAttempts = 5

// defaultDecomposeBackoff 是生产默认的指数退避——attempt 从 1 起。
// 100ms → 200ms → 400ms → 800ms → 1.6s → 3.2s → 6.4s → 10s（封顶）。
func defaultDecomposeBackoff(attempt int) time.Duration {
	const cap = 10 * time.Second
	d := 100 * time.Millisecond * (1 << (attempt - 1))
	if d > cap {
		d = cap
	}
	return d
}

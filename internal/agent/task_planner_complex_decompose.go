package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func (impl *taskPlannerImpl) finishDeferredComplexDecompose(ctx context.Context, input biz.PlanInput, planID string) (*biz.TaskPlan, error) {
	saved, err := impl.repo.GetByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if saved.DecomposeReason != biz.DecomposeReasonDeferred && len(saved.SubTasks) > 0 {
		return saved, nil
	}
	if strings.TrimSpace(input.UserMessage) == "" {
		input.UserMessage = saved.UserMessage
	}
	if strings.TrimSpace(input.SpiritSessionID) == "" {
		input.SpiritSessionID = saved.SpiritSessionID
	}
	if strings.TrimSpace(input.TraceID) == "" {
		input.TraceID = saved.TraceID
	}
	teamCount := detectTeamCount(input.UserMessage)
	gear := ClassifyTaskGear(GearInput{
		UserWantsOrgChain: HasOrgChainIntent(input.UserMessage),
		LongTask:          saved.ComplexityLevel == biz.ComplexityComplex || saved.ComplexityScore >= 0.6,
		CompanyNodeCount:  impl.companyNodeCount(ctx),
		FactQuery:         biz.LooksLikeFactQuery(input.UserMessage),
	})
	return impl.runComplexLLMDecompose(ctx, saved, input, teamCount, saved.ComplexityLevel, saved.Strategy, saved.StrategyReason, saved.TopologyHint, gear, saved.MemoryHit)
}

func (impl *taskPlannerImpl) runComplexLLMDecompose(
	ctx context.Context,
	saved *biz.TaskPlan,
	input biz.PlanInput,
	teamCount int,
	complexityLevel biz.ComplexityLevel,
	strategy biz.OrchestrationStrategy,
	strategyReason string,
	topologyHint biz.TopologyType,
	gear TaskGear,
	playbookHit *biz.MemoryHit,
) (*biz.TaskPlan, error) {
	var (
		subTasks        []biz.SubTask
		dag             *biz.PlanTaskDAG
		decomposeReason string
		streamPublished bool
		err             error
	)
	planID := saved.ID
	traceID := saved.TraceID
	if traceID == "" {
		traceID = input.TraceID
	}

	stopHeartbeat := impl.startDecomposeHeartbeat(ctx, input.SpiritSessionID, 5*time.Second)
	impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decomposing", nil)
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
	var clarifyErr *decomposeClarificationError
	if errors.As(err, &clarifyErr) && len(clarifyErr.questions) > 0 {
		impl.lg.Info("任务分解请求用户澄清（阻塞性信息缺失，拒绝虚构参数）",
			loggateway.StepID(biz.SpiritStepPlannerDecompose),
			loggateway.Str("trace_id", traceID),
			loggateway.Int("question_count", len(clarifyErr.questions)),
		)
		impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "needs_clarification", map[string]any{
			"questions": clarifyErr.questions,
		})
		impl.publishV2BoardFailed(ctx, planID, input)
		saved.Strategy = biz.StrategyDirect
		saved.StrategyReason = "任务存在阻塞性信息缺失，等待用户澄清"
		saved.DecomposeReason = "needs_clarification"
		if _, uerr := impl.repo.Update(ctx, saved); uerr != nil {
			impl.lg.Warn("澄清计划留痕更新失败（不阻断澄清透传）",
				loggateway.StepID(biz.SpiritStepPlannerPersist),
				loggateway.Str("trace_id", traceID),
				loggateway.Err(uerr),
			)
		}
		saved.ClarificationQuestions = append([]string(nil), clarifyErr.questions...)
		return saved, nil
	}
	if err != nil {
		impl.lg.Warn("任务分解失败",
			loggateway.StepID(biz.SpiritStepPlannerDecompose),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		strategy, strategyReason, topologyHint, decomposeReason = applyDecomposeDowngrade(
			strategy, topologyHint, complexityLevel, input.Mode,
			biz.DecomposeReasonFailed, "任务分解失败，降级为 direct")
		impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decompose_failed", map[string]any{
			"reason": "error",
		})
		impl.publishV2BoardFailed(ctx, planID, input)
	} else if len(subTasks) > 0 {
		gate := impl.applyPlanVerifyGate(ctx, subTasks, dag, input, teamCount, complexityLevel)
		if gate.degraded {
			impl.lg.Warn("计划校验门未通过",
				loggateway.StepID(biz.SpiritStepPlannerDecompose),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("note", gate.note),
			)
			strategy, strategyReason, topologyHint, decomposeReason = applyDecomposeDowngrade(
				strategy, topologyHint, complexityLevel, input.Mode,
				biz.DecomposeReasonVerifyFailed, gate.note)
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
			impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decomposed", map[string]any{
				"sub_task_count": len(subTasks),
			})
			decomposeReason = fmt.Sprintf("分解为 %d 个子任务%s", len(subTasks), gateNote)
			if teamCount > 0 && len(subTasks) > teamCount {
				originalCount := len(subTasks)
				droppedNames := make([]string, 0, originalCount-teamCount)
				for _, st := range subTasks[teamCount:] {
					droppedNames = append(droppedNames, st.Name)
				}
				impl.lg.Warn("LLM 分解的 subtask 数量超出用户请求的 team 数量，截取前 N 个",
					loggateway.StepID(biz.SpiritStepPlannerDecompose),
					loggateway.Str("trace_id", traceID),
					loggateway.Int("requested_team_count", teamCount),
					loggateway.Int("decomposed_subtask_count", originalCount),
				)
				subTasks = subTasks[:teamCount]
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
				impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "team_count_mismatch", map[string]any{
					"action":                   "truncate",
					"requested_team_count":     teamCount,
					"decomposed_subtask_count": originalCount,
					"dropped_subtask_names":    droppedNames,
				})
				impl.emitTeamCountMismatchGate(ctx, traceID, input.SpiritSessionID, "truncate", teamCount, originalCount, droppedNames)
			} else if teamCount > 0 && len(subTasks) < teamCount {
				impl.lg.Warn("LLM 分解的 subtask 数量少于用户请求的 team 数量",
					loggateway.StepID(biz.SpiritStepPlannerDecompose),
					loggateway.Str("trace_id", traceID),
					loggateway.Int("requested_team_count", teamCount),
					loggateway.Int("decomposed_subtask_count", len(subTasks)),
				)
				impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "team_count_mismatch", map[string]any{
					"action":                   "proceed",
					"requested_team_count":     teamCount,
					"decomposed_subtask_count": len(subTasks),
				})
				impl.emitTeamCountMismatchGate(ctx, traceID, input.SpiritSessionID, "proceed", teamCount, len(subTasks), nil)
			}
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
			strategy, strategyReason, topologyHint = promoteStrategyForSubTasks(
				strategy, strategyReason, topologyHint, complexityLevel, input.Mode, false)
		}
	} else {
		impl.lg.Warn("任务分解产出空结果",
			loggateway.StepID(biz.SpiritStepPlannerDecompose),
			loggateway.Str("trace_id", traceID),
		)
		strategy, strategyReason, topologyHint, decomposeReason = applyDecomposeDowngrade(
			strategy, topologyHint, complexityLevel, input.Mode,
			biz.DecomposeReasonEmpty, "任务分解结果为空，降级为 direct")
		impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, "decompose_failed", map[string]any{
			"reason": "empty",
		})
		impl.publishV2BoardFailed(ctx, planID, input)
	}

	if len(subTasks) > 0 {
		gear, playbookHit, subTasks, dag, strategy, strategyReason, decomposeReason = impl.applyCrossDeptGearUpgrade(
			ctx, input, gear, playbookHit, subTasks, dag, strategy, strategyReason, decomposeReason, streamPublished)
		if playbookHit != nil {
			saved.MemoryHit = playbookHit
		}
	}

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
	saved.StreamPublished = streamPublished
	impl.publishPlanCreated(ctx, saved, input.ChatSessionID)
	return saved, nil
}

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	"trpc.group/trpc-go/trpc-agent-go/graph"
)

var _ biz.TaskOrchestratorPort = (*TaskOrchestratorImpl)(nil)

// TaskOrchestratorImpl implements biz.TaskOrchestratorPort.
type TaskOrchestratorImpl struct {
	controller      tools.SpiritTeamControllerPort
	repo            biz.OrchestrationRepository
	plans           *planStore
	synthesis       tools.SpiritSynthesisPort
	checkpointSaver graph.CheckpointSaver
	orchCache       *biz.OrchestrationCache
	perfRepo        biz.AgentPerformanceRepository
	evolutionSugg   biz.EvolutionSuggestionCreator
	eventBus        biz.EventBus
	lg              loggateway.Logger
}

// NewTaskOrchestratorImpl creates a new TaskOrchestratorImpl.
//
// ADR-2（2026-08-20）：Orchestrate team 组装死路径已删除，构造函数随之移除
// spiritUC/assembler/matcher/deps/nl2graph 依赖；生产 team 创建路径为
// PlanExecutor + RealTeamOrchestrator。本实现仅保留 handle 记账（CheckProgress/
// Cancel/Synthesize/Recover/RecoverAllInterrupted）与在线学习回路。
func NewTaskOrchestratorImpl(
	controller tools.SpiritTeamControllerPort,
	repo biz.OrchestrationRepository,
	taskPlanRepo biz.TaskPlanRepository,
	allocPlanRepo biz.AllocationPlanRepository,
	synthesis tools.SpiritSynthesisPort,
	checkpointSaver graph.CheckpointSaver,
	orchCache *biz.OrchestrationCache,
	perfRepo biz.AgentPerformanceRepository,
	evolutionSugg biz.EvolutionSuggestionCreator,
	eventBus biz.EventBus,
	lg loggateway.Logger,
) *TaskOrchestratorImpl {
	return &TaskOrchestratorImpl{
		controller:      controller,
		repo:            repo,
		plans:           newPlanStore(taskPlanRepo, allocPlanRepo),
		synthesis:       synthesis,
		checkpointSaver: checkpointSaver,
		orchCache:       orchCache,
		perfRepo:        perfRepo,
		evolutionSugg:   evolutionSugg,
		eventBus:        eventBus,
		lg:              lg,
	}
}

// CheckProgress returns the progress of each subtask in the orchestration.
func (o *TaskOrchestratorImpl) CheckProgress(ctx context.Context, orchestrationID string) ([]biz.TaskProgress, error) {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return nil, apierror.NotFound(apierror.DomainSpirit, "orchestration not found")
	}

	// For team-based strategies, delegate to team progress checking.
	if len(handle.TeamIDs) > 0 && o.controller != nil {
		teamProgresses, err := o.controller.CheckTeamProgress(ctx, handle.SpiritSessionID)
		if err != nil {
			return nil, err
		}
		// Convert TeamProgress to TaskProgress.
		out := make([]biz.TaskProgress, 0, len(teamProgresses))
		for _, tp := range teamProgresses {
			out = append(out, biz.TaskProgress{
				SubTaskName: tp.TeamName,
				Status:      tp.Status,
				Progress:    tp.ProgressPct / 100.0,
			})
		}
		return out, nil
	}

	// For direct/single-agent strategies, return based on handle status.
	progress := 0.0
	status := string(handle.Status)
	switch handle.Status {
	case biz.OrchestrationStatusCompleted:
		progress = 1.0
	case biz.OrchestrationStatusRunning:
		progress = 0.5
	}
	return []biz.TaskProgress{{
		Status:   status,
		Progress: progress,
	}}, nil
}

// Cancel cancels the orchestration and all associated teams.
// reason 是 P2-6 取消原因（空 = user_cancel，保持向后兼容）。
// 父级联子：编排取消时，所有子 team 以 parent_cancel 级联取消。
func (o *TaskOrchestratorImpl) Cancel(ctx context.Context, orchestrationID string, reason biz.CancelReason) error {
	if reason == "" {
		reason = biz.CancelReasonUser
	}
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return apierror.NotFound(apierror.DomainSpirit, "orchestration not found")
	}

	if handle.Status != biz.OrchestrationStatusPending && handle.Status != biz.OrchestrationStatusRunning {
		return apierror.BadRequest(apierror.DomainSpirit, "only pending or running orchestrations can be cancelled")
	}

	// P2-6 父级联子：编排取消时，所有子 team 以 parent_cancel 级联取消。
	// 子 team 的 CancelReason 覆盖为 Parent（无论调用方传入什么），确保
	// 事件 meta 能区分「用户直接取消 team」vs「编排取消级联 team」。
	childReason := biz.CancelReasonParent
	if reason == biz.CancelReasonUser {
		// 用户显式取消编排时，子 team 也标记为 user_cancel 更符合直觉。
		childReason = biz.CancelReasonUser
	}

	// Cancel all teams.
	for _, teamID := range handle.TeamIDs {
		if o.controller != nil {
			if cancelErr := o.controller.CancelTeam(ctx, teamID, childReason); cancelErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to cancel team",
					loggateway.StepID(biz.SpiritStepOrchestratorExecute),
					loggateway.Str("team_id", teamID),
					loggateway.Str("cancel_reason", string(childReason)),
					loggateway.Err(cancelErr),
				)
			}
		}
	}

	handle.CancelReason = reason
	if tErr := o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusCancelled); tErr != nil {
		return tErr
	}
	_, err = o.repo.Update(ctx, handle)
	if err != nil {
		o.lg.Warn("TaskOrchestrator: failed to update cancelled orchestration",
			loggateway.StepID(biz.SpiritStepOrchestratorExecute),
			loggateway.Str("orchestration_id", orchestrationID),
			loggateway.Err(err),
		)
	}

	// Publish spirit_orchestration_interrupted event.
	o.publishOrchestrationInterrupted(ctx, handle)

	return nil
}

// Synthesize synthesizes the results of the orchestration and persists the result.
func (o *TaskOrchestratorImpl) Synthesize(ctx context.Context, orchestrationID string) (*biz.SynthesisOutput, error) {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return nil, apierror.NotFound(apierror.DomainSpirit, "orchestration not found")
	}

	if handle.SpiritSessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainSpirit, "orchestration has no spirit_session_id")
	}

	// Delegate to the SpiritSynthesisService.
	if o.synthesis != nil {
		output, err := o.synthesis.SynthesizeResults(ctx, handle.SpiritSessionID, "")
		if err != nil {
			return nil, err
		}
		// Persist synthesis result to handle so it survives process restarts.
		if output != nil {
			synthesisJSON, marshalErr := marshalSynthesisOutput(output)
			if marshalErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to marshal synthesis result",
					loggateway.StepID(biz.SpiritStepOrchestratorSynthesize),
					loggateway.Str("orchestration_id", orchestrationID),
					loggateway.Err(marshalErr),
				)
			} else {
				handle.SynthesisResultJSON = synthesisJSON
				if tErr := o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusCompleted); tErr == nil {
					if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
						o.lg.Warn("TaskOrchestrator: failed to update orchestration with synthesis result",
							loggateway.StepID(biz.SpiritStepOrchestratorSynthesize),
							loggateway.Str("orchestration_id", orchestrationID),
							loggateway.Err(updateErr),
						)
					}
				}
			}

			// Online learning loop: update cache and performance after synthesis
			o.learnFromOrchestration(ctx, handle, output)
		}
		return output, nil
	}

	return nil, apierror.Internal(apierror.DomainSpirit, "synthesis service not available")
}

// Recover recovers an interrupted orchestration from its last checkpoint.
func (o *TaskOrchestratorImpl) Recover(ctx context.Context, orchestrationID string) error {
	handle, err := o.repo.GetByID(ctx, orchestrationID)
	if err != nil {
		return apierror.NotFound(apierror.DomainSpirit, "orchestration not found")
	}

	if handle.Status != biz.OrchestrationStatusInterrupted {
		return apierror.BadRequest(apierror.DomainSpirit, "only interrupted orchestrations can be recovered (status: %s)", handle.Status)
	}

	o.lg.Info("TaskOrchestrator: recovering orchestration",
		loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		loggateway.Str("orchestration_id", orchestrationID),
		loggateway.Str("checkpoint_id", handle.CheckpointID),
	)

	// If no checkpoint is available, we cannot recover the graph state.
	// Mark as failed since there is no way to resume.
	if handle.CheckpointID == "" {
		o.lg.Warn("TaskOrchestrator: no checkpoint available, marking orchestration as failed",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", orchestrationID),
		)
		if tErr := o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed); tErr == nil {
			if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
				o.lg.Warn("TaskOrchestrator: failed to update orchestration to failed",
					loggateway.StepID(biz.SpiritStepOrchestratorRecover),
					loggateway.Str("orchestration_id", orchestrationID),
					loggateway.Err(updateErr),
				)
			}
		}
		return apierror.NotFound(apierror.DomainSpirit, "no checkpoint available for orchestration %s", orchestrationID)
	}

	// Attempt to load the latest checkpoint from the CheckpointSaver.
	if o.checkpointSaver != nil {
		lineageID := handle.ID
		config := graph.CreateCheckpointConfig(lineageID, handle.CheckpointID, "")
		tuple, loadErr := o.checkpointSaver.GetTuple(ctx, config)
		if loadErr != nil {
			o.lg.Warn("TaskOrchestrator: failed to load checkpoint",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", orchestrationID),
				loggateway.Str("checkpoint_id", handle.CheckpointID),
				loggateway.Err(loadErr),
			)
			// Cannot load checkpoint; mark as failed.
			if tErr := o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed); tErr == nil {
				if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
					o.lg.Warn("TaskOrchestrator: failed to update orchestration to failed",
						loggateway.StepID(biz.SpiritStepOrchestratorRecover),
						loggateway.Str("orchestration_id", orchestrationID),
						loggateway.Err(updateErr),
					)
				}
			}
			return apierror.Internal(apierror.DomainSpirit, "failed to load checkpoint for orchestration %s", orchestrationID).WithCause(loadErr)
		}

		if tuple == nil || tuple.Checkpoint == nil {
			o.lg.Warn("TaskOrchestrator: checkpoint not found, marking orchestration as failed",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", orchestrationID),
				loggateway.Str("checkpoint_id", handle.CheckpointID),
			)
			if tErr := o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusFailed); tErr == nil {
				if _, updateErr := o.repo.Update(ctx, handle); updateErr != nil {
					o.lg.Warn("TaskOrchestrator: failed to update orchestration to failed",
						loggateway.StepID(biz.SpiritStepOrchestratorRecover),
						loggateway.Str("orchestration_id", orchestrationID),
						loggateway.Err(updateErr),
					)
				}
			}
			return apierror.NotFound(apierror.DomainSpirit, "checkpoint %s not found for orchestration %s", handle.CheckpointID, orchestrationID)
		}

		o.lg.Info("TaskOrchestrator: checkpoint loaded successfully",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", orchestrationID),
			loggateway.Str("checkpoint_id", tuple.Checkpoint.ID),
		)

		// Rebuild GraphAgent state from checkpoint: validate critical fields
		// and prepare the handle for graph runtime resumption.
		if err := o.restoreGraphFromCheckpoint(ctx, handle, tuple); err != nil {
			o.lg.Warn("TaskOrchestrator: GraphAgent rebuild from checkpoint failed",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", orchestrationID),
				loggateway.Err(err),
			)
			// Non-fatal: the checkpoint was loaded but state validation failed.
			// Continue recovery so the orchestration can at least be tracked.
		}
	}

	// Mark as running so the orchestration can be tracked.
	if tErr := o.transitionOrchestrationStatus(ctx, handle, biz.OrchestrationStatusRunning); tErr != nil {
		return tErr
	}
	_, err = o.repo.Update(ctx, handle)
	if err != nil {
		o.lg.Warn("TaskOrchestrator: failed to update recovered orchestration",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("orchestration_id", orchestrationID),
			loggateway.Err(err),
		)
		return err
	}

	o.lg.Info("TaskOrchestrator: orchestration recovered",
		loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		loggateway.Str("orchestration_id", orchestrationID),
	)

	// P1-10: reload persisted Phase 1/2 plans so resume continues the original
	// plan instead of re-running LLM decomposition.
	o.restorePlansForHandle(ctx, handle)
	return nil
}

// restoreGraphFromCheckpoint rebuilds GraphAgent state from a loaded checkpoint.
// It validates that critical state fields in the checkpoint match the orchestration
// handle and updates the handle's checkpoint ID for graph runtime resumption.
func (o *TaskOrchestratorImpl) restoreGraphFromCheckpoint(ctx context.Context, handle *biz.OrchestrationHandle, tuple *graph.CheckpointTuple) error {
	if tuple == nil || tuple.Checkpoint == nil {
		return apierror.Internal(apierror.DomainSpirit, "checkpoint tuple is nil")
	}

	ckpt := tuple.Checkpoint
	values := ckpt.ChannelValues

	// Validate critical state fields match the handle.
	if orchID, ok := values["orchestration_id"].(string); ok && orchID != "" && orchID != handle.ID {
		o.lg.Warn("Checkpoint orchestration_id 与 handle 不匹配",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("handle_id", handle.ID),
			loggateway.Str("checkpoint_orchestration_id", orchID),
		)
	}

	if sessionID, ok := values["spirit_session_id"].(string); ok && sessionID != "" && sessionID != handle.SpiritSessionID {
		o.lg.Warn("Checkpoint spirit_session_id 与 handle 不匹配",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Str("handle_session_id", handle.SpiritSessionID),
			loggateway.Str("checkpoint_session_id", sessionID),
		)
	}

	// Restore strategy from checkpoint if handle is missing it.
	if handle.Strategy == "" {
		if strategy, ok := values["strategy"].(string); ok && strategy != "" {
			handle.Strategy = biz.OrchestrationStrategy(strategy)
		}
	}

	// Update checkpoint ID to the latest loaded checkpoint so the graph runtime
	// can resume from this exact point when the team runner restarts.
	handle.CheckpointID = ckpt.ID

	o.lg.Info("GraphAgent 状态已从 checkpoint 重建",
		loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("checkpoint_id", ckpt.ID),
		loggateway.Str("strategy", string(handle.Strategy)),
		loggateway.Int("channel_count", len(values)),
	)

	return nil
}

// RecoverAllInterrupted finds all interrupted orchestrations and attempts recovery.
// P1-10: also reloads persisted Phase 1 (TaskPlan) / Phase 2 (AllocationPlan)
// rows into the orchestrator so a subsequent plan_and_execute continues the
// original plan instead of generating a new one.
func (o *TaskOrchestratorImpl) RecoverAllInterrupted(ctx context.Context) error {
	sysCtx := workspace.WithSystemWorkspace(ctx)

	handles, err := o.repo.ListByStatus(sysCtx, biz.OrchestrationStatusInterrupted)
	if err != nil {
		return apierror.Internal(apierror.DomainSpirit, "list interrupted orchestrations").WithCause(err)
	}

	if len(handles) == 0 {
		o.lg.Info("TaskOrchestrator: no interrupted orchestrations; restoring orphaned plans",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
		)
	} else {
		o.lg.Info("TaskOrchestrator: recovering interrupted orchestrations",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Int("count", len(handles)),
		)
	}

	var failedCount int
	for _, h := range handles {
		if err := o.Recover(sysCtx, h.ID); err != nil {
			failedCount++
			o.lg.Warn("TaskOrchestrator: failed to recover orchestration",
				loggateway.StepID(biz.SpiritStepOrchestratorRecover),
				loggateway.Str("orchestration_id", h.ID),
				loggateway.Err(err),
			)
			continue
		}
	}

	if failedCount > 0 {
		o.lg.Warn("TaskOrchestrator: some orchestrations failed to recover",
			loggateway.StepID(biz.SpiritStepOrchestratorRecover),
			loggateway.Int("total", len(handles)),
			loggateway.Int("failed", failedCount),
		)
	}

	o.restoreOrphanedPlans(sysCtx)
	return nil
}

// learnFromOrchestration updates OrchestrationCache and AgentPerformance after
// an orchestration completes, implementing the online learning loop (T3.5).
//
// NOTE: 非原子操作，允许部分成功（在线学习侧效）。三个独立更新（OrchestrationCache、
// AgentPerformance、EvolutionSuggestion）各自独立持久化，失败仅 Warn 不回滚。
// 这是有意设计：每个更新独立有意义，部分失败不影响其他维度的学习信号。
func (o *TaskOrchestratorImpl) learnFromOrchestration(ctx context.Context, handle *biz.OrchestrationHandle, synthesis *biz.SynthesisOutput) {
	if handle == nil || synthesis == nil {
		return
	}

	dqScore := computeDQScoreFromSynthesis(synthesis)
	primaryDomainPath := o.resolvePrimaryDomainPath(ctx, handle)

	o.lg.Info("在线学习: 更新编排缓存和 Agent 性能",
		loggateway.StepID(biz.SpiritStepOrchestratorLearn),
		loggateway.Str("orchestration_id", handle.ID),
		loggateway.Str("strategy", string(handle.Strategy)),
		loggateway.Float64("dq_score", dqScore),
	)

	// 1. Update OrchestrationCache with DQ score
	topology := biz.TopologyCoordinator
	switch handle.Strategy {
	case biz.StrategyDirect:
		topology = biz.TopologyDirect
	case biz.StrategyParallel:
		topology = biz.TopologyParallel
	case biz.StrategyDAG:
		topology = biz.TopologyHybrid
	}
	if o.orchCache != nil {
		agentKeys := extractAgentKeysFromHandle(handle)
		if primaryDomainPath != "" {
			// B.10.21.7: 配方以 domain key 记录，使同类任务可复用。
			o.orchCache.RecordDomainRecipe(ctx, primaryDomainPath, topology, dqScore, len(handle.TeamIDs), agentKeys)
			o.lg.Info("在线学习: 领域配方已更新",
				loggateway.StepID(biz.SpiritStepOrchestratorLearn),
				loggateway.Str("domain_path", primaryDomainPath),
				loggateway.Float64("dq_score", dqScore),
			)
		} else {
			// 无域回退：旧 key 行为（orchestration ID 派生，write-only 兼容）。
			taskPattern := biz.ExtractTaskPattern(handle.ID)
			o.orchCache.RecordCompletionWithAgents(ctx, taskPattern, topology, dqScore, len(handle.TeamIDs), 0, agentKeys)
			o.lg.Info("在线学习: 编排缓存已更新",
				loggateway.StepID(biz.SpiritStepOrchestratorLearn),
				loggateway.Str("task_pattern", taskPattern),
				loggateway.Float64("dq_score", dqScore),
			)
		}
	}

	// 2. Generate evolution suggestion when DQ Score is low
	if dqScore < biz.DQEvolutionThreshold && o.evolutionSugg != nil {
		o.maybeCreateEvolutionSuggestion(ctx, handle, dqScore, topology)
	}

	// 3. Update AgentPerformance for each agent in the orchestration
	if o.perfRepo != nil {
		taskType := string(handle.Strategy)
		if primaryDomainPath != "" {
			taskType = "domain:" + primaryDomainPath // B.10.21.2: TaskType 语义扩展
		}
		successCount := 0
		if dqScore >= biz.DQSuccessThreshold {
			successCount = 1
		}
		agentKeys := extractAgentKeysFromHandle(handle)
		for _, agentKey := range agentKeys {
			existing, err := o.perfRepo.Get(ctx, agentKey, taskType)
			if err != nil || existing == nil {
				// New performance record
				perf := &biz.AgentPerformance{
					AgentKey:       agentKey,
					TaskType:       taskType,
					TotalRuns:      1,
					SuccessRuns:    successCount,
					SuccessRate:    float64(successCount),
					AvgDQScore:     dqScore,
					LastExecutedAt: time.Now().UTC().Format(time.RFC3339),
				}
				if upsertErr := o.perfRepo.Upsert(ctx, perf); upsertErr != nil {
					o.lg.Warn("在线学习: AgentPerformance 更新失败",
						loggateway.StepID(biz.SpiritStepOrchestratorLearn),
						loggateway.Str("agent_key", agentKey),
						loggateway.Err(upsertErr),
					)
				}
			} else {
				// Update existing performance record with running average
				existing.TotalRuns++
				existing.SuccessRuns += successCount
				existing.SuccessRate = float64(existing.SuccessRuns) / float64(existing.TotalRuns)
				existing.AvgDQScore = (existing.AvgDQScore*float64(existing.TotalRuns-1) + dqScore) / float64(existing.TotalRuns)
				existing.LastExecutedAt = time.Now().UTC().Format(time.RFC3339)
				if upsertErr := o.perfRepo.Upsert(ctx, existing); upsertErr != nil {
					o.lg.Warn("在线学习: AgentPerformance 更新失败",
						loggateway.StepID(biz.SpiritStepOrchestratorLearn),
						loggateway.Str("agent_key", agentKey),
						loggateway.Err(upsertErr),
					)
				}
			}
		}
		o.lg.Info("在线学习: Agent 性能已更新",
			loggateway.StepID(biz.SpiritStepOrchestratorLearn),
			loggateway.Int("agent_count", len(agentKeys)),
		)
	}
}

// resolvePrimaryDomainPath 取 handle 对应 plan 的主导域（首个非空 subtask
// DomainPath）。查询失败或无域返回空——调用方回退旧 key 行为（B.10.21.7）。
func (o *TaskOrchestratorImpl) resolvePrimaryDomainPath(ctx context.Context, handle *biz.OrchestrationHandle) string {
	if o.plans == nil || handle.TaskPlanID == "" {
		return ""
	}
	plan, err := o.plans.getTaskPlan(ctx, handle.TaskPlanID)
	if err != nil || plan == nil {
		if err != nil {
			o.lg.Warn("在线学习: 查询 TaskPlan 失败，跳过领域配方记录",
				loggateway.StepID(biz.SpiritStepOrchestratorLearn),
				loggateway.Str("task_plan_id", handle.TaskPlanID),
				loggateway.Err(err),
			)
		}
		return ""
	}
	return PrimaryDomainPath(plan.SubTasks)
}

// maybeCreateEvolutionSuggestion generates an orchestration_optimization evolution suggestion
// when DQ Score is below the evolution threshold. It performs dedup by checking pending
// suggestions for the same agentID + type + title combination.
func (o *TaskOrchestratorImpl) maybeCreateEvolutionSuggestion(ctx context.Context, handle *biz.OrchestrationHandle, dqScore float64, topology biz.TopologyType) {
	// Use SpiritSessionID as the evolution target — it represents the spirit session
	// that owns this orchestration, enabling cross-orchestration dedup within the same session.
	targetID := handle.SpiritSessionID
	if targetID == "" {
		targetID = handle.ID // fallback for legacy handles without SpiritSessionID
	}
	suggType := "orchestration_optimization"
	title := fmt.Sprintf("编排优化建议: %s", biz.TruncateRunes(handle.ID, biz.MaxSuggestionTitleLen))

	// Dedup: skip if a pending suggestion with same type+title already exists
	pending, listErr := o.evolutionSugg.GetEvolutionSuggestions(ctx, targetID, "pending")
	if listErr != nil {
		o.lg.Warn("进化建议: 查询已有建议失败，跳过去重检查",
			loggateway.StepID(biz.SpiritStepOrchestratorLearn),
			loggateway.Str("target_id", targetID),
			loggateway.Err(listErr),
		)
		// Continue to create — better to risk a duplicate than to miss a suggestion
	} else {
		for _, s := range pending {
			if strings.EqualFold(strings.TrimSpace(s.Type), suggType) && strings.TrimSpace(s.Title) == title {
				o.lg.Info("进化建议: 已存在相同待处理建议，跳过创建",
					loggateway.StepID(biz.SpiritStepOrchestratorLearn),
					loggateway.Str("target_id", targetID),
					loggateway.Str("existing_id", s.ID),
				)
				return
			}
		}
	}

	content := fmt.Sprintf("编排 %q 的 DQ Score 为 %.2f（低于阈值 %.1f），当前拓扑 %s 执行效果不佳。", handle.ID, dqScore, biz.DQEvolutionThreshold, topology)
	if o.orchCache != nil {
		altTopology, altFound := o.orchCache.SuggestBestAlternativeTopology(handle.ID, topology)
		if altFound {
			content += fmt.Sprintf("建议尝试 %s 拓扑。", altTopology)
		} else {
			content += "暂无历史数据推荐替代拓扑，建议调整任务描述或减少团队数量。"
		}
	} else {
		content += "暂无历史数据推荐替代拓扑，建议调整任务描述或减少团队数量。"
	}

	sugg, suggErr := o.evolutionSugg.CreateSuggestion(ctx, biz.EvolutionSuggestion{
		ID:        fmt.Sprintf("evo-orch-%s", uuid.NewString()[:12]),
		AgentID:   targetID,
		Type:      suggType,
		Title:     title,
		Content:   content,
		Status:    "pending",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if suggErr != nil {
		o.lg.Warn("创建编排优化建议失败",
			loggateway.StepID(biz.SpiritStepOrchestratorLearn),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Err(suggErr),
		)
		return
	}
	// Emit orchestration evolution suggested event as a v2 SystemNoticeEvent
	// (replaces the legacy system-domain ActivityEvent; NOT persisted, WS-only broadcast).
	if o.eventBus != nil {
		meta := map[string]any{
			"event_type":        "orchestration.evolution_suggested",
			"orchestration_id":  handle.ID,
			"spirit_session_id": handle.SpiritSessionID,
			"dq_score":          dqScore,
			"topology":          string(topology),
			"suggestion_id":     sugg.ID,
		}
		o.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(handle.SpiritSessionID, "orchestration.evolution_suggested", content, meta))
	}
}

// computeDQScoreFromSynthesis computes a DQ score from the synthesis output.
// - Successful synthesis with key findings: DQ = 0.8 + (findings_count * 0.05), capped at 1.0
// - Partial results: DQ = 0.5
// - Failed: DQ = 0.2
func computeDQScoreFromSynthesis(synthesis *biz.SynthesisOutput) float64 {
	if synthesis == nil {
		return 0.2
	}

	// Check if synthesis has meaningful content
	if synthesis.Content == "" && len(synthesis.TeamResults) == 0 {
		return 0.2
	}

	// Count completed team results
	completedCount := 0
	for _, tr := range synthesis.TeamResults {
		if tr.Status == "completed" {
			completedCount++
		}
	}

	// All teams failed
	if completedCount == 0 && len(synthesis.TeamResults) > 0 {
		return 0.2
	}

	// Partial success (some teams completed, some didn't)
	if completedCount > 0 && completedCount < len(synthesis.TeamResults) {
		return 0.5
	}

	// Full success — compute DQ from key findings
	dq := 0.8
	findingsCount := 0
	for _, tr := range synthesis.TeamResults {
		if tr.KeyFindings != "" {
			findingsCount++
		}
	}
	dq += float64(findingsCount) * 0.05
	if dq > 1.0 {
		dq = 1.0
	}
	return dq
}

// extractAgentKeysFromHandle extracts agent keys from the orchestration handle.
// It prefers handle.AgentKeys (real agent identifiers from AllocationPlan) over
// handle.TeamIDs (team identifiers) because TeamIDs and AgentKeys are different
// semantic entities — using TeamIDs as AgentKeys corrupts performance data.
func extractAgentKeysFromHandle(handle *biz.OrchestrationHandle) []string {
	// Prefer real agent keys from the allocation plan.
	if len(handle.AgentKeys) > 0 {
		return handle.AgentKeys
	}
	// Fallback: use TeamIDs as proxy for backward compatibility.
	var keys []string
	for _, teamID := range handle.TeamIDs {
		keys = append(keys, teamID)
	}
	if len(keys) == 0 {
		keys = append(keys, "spirit")
	}
	return keys
}

// transitionOrchestrationStatus validates and applies a state transition on
// the orchestration handle using the AS-FSM-01 state machine.
//
// C-19: fail-closed — illegal transitions return an error and do NOT apply
// the target status. Callers must log and skip persist / return on error.
func (o *TaskOrchestratorImpl) transitionOrchestrationStatus(ctx context.Context, handle *biz.OrchestrationHandle, target biz.OrchestrationStatus) error {
	from := handle.Status
	if from == target {
		return nil
	}
	if !biz.CanTransitionOrchestrationStatus(from, target) {
		o.lg.Warn("TaskOrchestrator: illegal orchestration state transition (rejected)",
			loggateway.StepID(biz.SpiritStepOrchestratorStrategy),
			loggateway.Str("orchestration_id", handle.ID),
			loggateway.Str("from", string(from)),
			loggateway.Str("to", string(target)),
		)
		return fmt.Errorf("illegal orchestration transition %s → %s", from, target)
	}
	handle.Status = target
	return nil
}

// marshalSynthesisOutput serializes a SynthesisOutput to JSON.
func marshalSynthesisOutput(output *biz.SynthesisOutput) (string, error) {
	type jsonOutput struct {
		Content       string                    `json:"content"`
		Strategy      string                    `json:"strategy"`
		TeamResults   []biz.TeamSynthesisResult `json:"team_results"`
		SynthesizedAt string                    `json:"synthesized_at"`
	}
	jo := jsonOutput{
		Content:       output.Content,
		Strategy:      string(output.Strategy),
		TeamResults:   output.TeamResults,
		SynthesizedAt: output.SynthesizedAt,
	}
	b, err := json.Marshal(jo)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// publishOrchestrationInterrupted publishes the spirit_orchestration_interrupted event as a v2 SystemNoticeEvent.
func (o *TaskOrchestratorImpl) publishOrchestrationInterrupted(ctx context.Context, handle *biz.OrchestrationHandle) {
	if o.eventBus == nil || handle == nil {
		return
	}
	spiritSessionID := handle.SpiritSessionID

	meta := map[string]any{
		"orchestration_id":  handle.ID,
		"spirit_session_id": spiritSessionID,
		"status":            string(handle.Status),
		"notice_type":       "warning",
	}
	if handle.CancelReason != "" {
		meta["cancel_reason"] = string(handle.CancelReason)
	}
	o.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_interrupted", "任务编排已中断", meta))
}

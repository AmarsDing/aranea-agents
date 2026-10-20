package agent

import (
	"aranea-agents/internal/biz"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"

	"fmt"

	"strings"

	"golang.org/x/sync/errgroup"
)

// phaseAResult carries one subtask's Phase A match outcome (P-ORCH.5).
type phaseAResult struct {
	alloc    biz.TaskAllocation
	matchErr error
}

// runPhaseAMatch executes Phase A of the two-phase allocation (P-ORCH.5):
// parallel matchSubTask (Layer 0-3, no factory) across all subtasks.
// Each goroutine writes its result to a pre-allocated slice slot so the
// input subtask order is preserved regardless of completion timing.
// Match errors are captured per-slot, never returned — Phase B handles them.
func (impl *agentAllocatorImpl) runPhaseAMatch(ctx context.Context, subTasks []biz.SubTask, capabilities []biz.AgentCapability, traceID string) []phaseAResult {
	results := make([]phaseAResult, len(subTasks))
	if len(subTasks) == 0 {
		return results
	}

	shared := newLayer2Shared()
	if impl.embedder != nil {
		_, _, _ = shared.loadAgentVectors(ctx, impl.embedder, capabilities)
	}
	g, gctx := errgroup.WithContext(ctx)
	for i, subTask := range subTasks {
		i, subTask := i, subTask
		g.Go(func() error {
			pool, _ := impl.matchingPool(subTask.DomainPath, capabilities, traceID)
			alloc, err := impl.matchSubTask(gctx, subTask, pool, traceID, shared)
			results[i] = phaseAResult{alloc: alloc, matchErr: err}
			return nil
		})
	}
	_ = g.Wait()
	return results
}

// matchSubTask matches a single subtask to the best agent/team.
// shared 为可选的 per-Allocate Layer2 共享态（P3b）；不传时退化为一次性实例。
func (impl *agentAllocatorImpl) matchSubTask(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string, sharedOpt ...*layer2Shared) (biz.TaskAllocation, error) {
	var shared *layer2Shared
	if len(sharedOpt) > 0 {
		shared = ensureLayer2Shared(sharedOpt[0])
	} else {
		shared = newLayer2Shared()
	}

	assignedType := "agent"
	if subTask.EstimatedComplexity >= 0.5 {
		assignedType = "team"
	}

	if subTask.DomainPath != "" {
		if cap, members, dq, ok := impl.tryDomainRecipe(subTask.DomainPath, capabilities, traceID); ok {
			alloc := biz.TaskAllocation{
				SubTaskID:    subTask.ID,
				SubTaskName:  subTask.Name,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   dq,
				MatchLayer:   "domain_recipe",
				MatchReason:  fmt.Sprintf("领域配方复用 (domain: %s, DQ %.2f)", subTask.DomainPath, dq),
			}
			if len(members) > 0 {
				alloc.TeamMemberKeys = members
			}
			return alloc, nil
		}
		if cap, backup, ok := bindRosterSpecialist(subTask.DomainPath, subTask.Name+" "+subTask.Description, capabilities); ok {
			alloc := biz.TaskAllocation{
				SubTaskID:    subTask.ID,
				SubTaskName:  subTask.Name,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   1,
				MatchLayer:   "roster",
				MatchReason:  "专项花名册绑定 " + NormalizeDomainPath(subTask.DomainPath),
			}
			if backup != "" {
				alloc.FallbackKey = backup
			}
			return alloc, nil
		}

		if spec := NormalizeDomainPath(subTask.DomainPath); spec == "研究/调研" {
			if cap, backup, ok := bindRosterSpecialist("办公/文档", subTask.Name+" "+subTask.Description, capabilities); ok {
				alloc := biz.TaskAllocation{
					SubTaskID:    subTask.ID,
					SubTaskName:  subTask.Name,
					AssignedType: assignedType,
					AssignedKey:  cap.AgentKey,
					AssignedName: cap.DisplayName,
					MatchScore:   0.8,
					MatchLayer:   "roster",
					MatchReason:  "研究/调研 名册缺失，回退 办公/文档",
				}
				if backup != "" {
					alloc.FallbackKey = backup
				}
				return alloc, nil
			}
		}
		taskText := subTask.Name + " " + subTask.Description
		if cap, score, candCount, ok := impl.tryMissionMatch(ctx, taskText, subTask.DomainPath, capabilities, traceID); ok {
			return biz.TaskAllocation{
				SubTaskID:    subTask.ID,
				SubTaskName:  subTask.Name,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   score,
				MatchLayer:   "mission",
				MatchReason:  missionMatchReason(subTask.DomainPath, candCount, score),
			}, nil
		}
	}

	if impl.perfRepo != nil {
		taskTypes := make([]string, 0, 2)
		if subTask.DomainPath != "" {
			taskTypes = append(taskTypes, "domain:"+subTask.DomainPath)
		}
		if len(subTask.RequiredCapabilities) > 0 {
			taskTypes = append(taskTypes, subTask.RequiredCapabilities[0])
		}
		for _, taskType := range taskTypes {
			bestPerfs, err := impl.perfRepo.GetBestForTaskType(ctx, taskType, 1)
			if err != nil || len(bestPerfs) == 0 {
				continue
			}
			bestAgentKey := bestPerfs[0].AgentKey
			for _, cap := range capabilities {
				if cap.AgentKey == bestAgentKey {
					impl.lg.Info("AgentPerformance.GetBestForTaskType 命中",
						loggateway.StepID(biz.SpiritStepAllocatorMatch),
						loggateway.Str("trace_id", traceID),
						loggateway.Str("sub_task_id", subTask.ID),
						loggateway.Str("agent_key", bestAgentKey),
						loggateway.Str("task_type", taskType),
						loggateway.Float64("success_rate", bestPerfs[0].SuccessRate),
					)
					return biz.TaskAllocation{
						SubTaskID:    subTask.ID,
						SubTaskName:  subTask.Name,
						AssignedType: assignedType,
						AssignedKey:  bestAgentKey,
						AssignedName: cap.DisplayName,
						MatchScore:   bestPerfs[0].SuccessRate,
						MatchLayer:   "performance",
						MatchReason:  fmt.Sprintf("历史性能最优 (成功率 %.2f, DQ %.2f)", bestPerfs[0].SuccessRate, bestPerfs[0].AvgDQScore),
					}, nil
				}
			}
		}
	}

	bestMatch, bestScore, matchReason := impl.exactMatch(subTask.RequiredCapabilities, capabilities)

	if bestScore > 0.5 {
		return biz.TaskAllocation{
			SubTaskID:    subTask.ID,
			SubTaskName:  subTask.Name,
			AssignedType: assignedType,
			AssignedKey:  bestMatch.AgentKey,
			AssignedName: bestMatch.DisplayName,
			MatchScore:   bestScore,
			MatchLayer:   "exact",
			MatchReason:  matchReason,
		}, nil
	}

	if strings.TrimSpace(subTask.DomainPath) != "" {
		return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
	}

	taskBlob := strings.TrimSpace(subTask.Name + " " + subTask.Description)
	if biz.HasTaskActionSignal(taskBlob) {
		return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
	}

	semCap, semScore, semReason := impl.matchLayer2(ctx, subTask, capabilities, traceID, shared)
	if semScore > 0.3 && semCap.AgentKey != "" {
		impl.lg.Info("Layer 2 语义匹配命中",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("sub_task_id", subTask.ID),
			loggateway.Str("agent_key", semCap.AgentKey),
			loggateway.Float64("score", semScore),
		)
		return biz.TaskAllocation{
			SubTaskID:    subTask.ID,
			SubTaskName:  subTask.Name,
			AssignedType: assignedType,
			AssignedKey:  semCap.AgentKey,
			AssignedName: semCap.DisplayName,
			MatchScore:   semScore,
			MatchLayer:   "semantic",
			MatchReason:  semReason,
		}, nil
	}

	llmMatch, err := impl.llmColdStart(ctx, subTask, capabilities, traceID)
	if err == nil && llmMatch != "" {

		displayName := llmMatch
		for _, cap := range capabilities {
			if cap.AgentKey == llmMatch {
				displayName = cap.DisplayName
				break
			}
		}
		return biz.TaskAllocation{
			SubTaskID:     subTask.ID,
			SubTaskName:   subTask.Name,
			AssignedType:  assignedType,
			AssignedKey:   llmMatch,
			AssignedName:  displayName,
			MatchScore:    bestScore,
			MatchLayer:    "llm_cold_start",
			MatchReason:   "LLM 冷启动匹配",
			FallbackKey:   bestMatch.AgentKey,
			FallbackScore: bestScore,
		}, nil
	}

	return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for subtask %s", subTask.ID)
}

// matchWholePlan handles allocation for plans without subtasks (simple/moderate).
func (impl *agentAllocatorImpl) matchWholePlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (biz.TaskAllocation, error) {

	assignedType := "agent"
	if taskPlan.Strategy == biz.StrategyCoordinator || taskPlan.Strategy == biz.StrategyParallel || taskPlan.Strategy == biz.StrategyDAG {
		assignedType = "team"
	}

	if taskPlan.DomainPath != "" {
		if cap, members, dq, ok := impl.tryDomainRecipe(taskPlan.DomainPath, capabilities, traceID); ok {
			alloc := biz.TaskAllocation{
				SubTaskID:    "whole",
				SubTaskName:  taskPlan.UserMessage,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   dq,
				MatchLayer:   "domain_recipe",
				MatchReason:  fmt.Sprintf("领域配方复用 (domain: %s, DQ %.2f)", taskPlan.DomainPath, dq),
			}
			if len(members) > 0 {
				alloc.TeamMemberKeys = members
			}
			return alloc, nil
		}
		if cap, backup, ok := bindRosterSpecialist(taskPlan.DomainPath, taskPlan.UserMessage, capabilities); ok {
			alloc := biz.TaskAllocation{
				SubTaskID:    "whole",
				SubTaskName:  taskPlan.UserMessage,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   1,
				MatchLayer:   "roster",
				MatchReason:  "专项花名册绑定 " + NormalizeDomainPath(taskPlan.DomainPath),
			}
			if backup != "" {
				alloc.FallbackKey = backup
			}
			return alloc, nil
		}
		if cap, score, candCount, ok := impl.tryMissionMatch(ctx, taskPlan.UserMessage, taskPlan.DomainPath, capabilities, traceID); ok {
			return biz.TaskAllocation{
				SubTaskID:    "whole",
				SubTaskName:  taskPlan.UserMessage,
				AssignedType: assignedType,
				AssignedKey:  cap.AgentKey,
				AssignedName: cap.DisplayName,
				MatchScore:   score,
				MatchLayer:   "mission",
				MatchReason:  missionMatchReason(taskPlan.DomainPath, candCount, score),
			}, nil
		}
	}

	if impl.perfRepo != nil {
		capHints := extractCapabilityHints(taskPlan.UserMessage)
		taskTypes := make([]string, 0, 2)
		if taskPlan.DomainPath != "" {
			taskTypes = append(taskTypes, "domain:"+taskPlan.DomainPath)
		}
		taskTypes = append(taskTypes, capHints[0])
		for _, taskType := range taskTypes {
			bestPerfs, err := impl.perfRepo.GetBestForTaskType(ctx, taskType, 1)
			if err != nil || len(bestPerfs) == 0 {
				continue
			}
			bestAgentKey := bestPerfs[0].AgentKey
			for _, cap := range capabilities {
				if cap.AgentKey == bestAgentKey {
					impl.lg.Info("AgentPerformance.GetBestForTaskType 命中 (whole plan)",
						loggateway.StepID(biz.SpiritStepAllocatorMatch),
						loggateway.Str("trace_id", traceID),
						loggateway.Str("agent_key", bestAgentKey),
						loggateway.Str("task_type", taskType),
						loggateway.Float64("success_rate", bestPerfs[0].SuccessRate),
					)
					return biz.TaskAllocation{
						SubTaskID:    "whole",
						SubTaskName:  taskPlan.UserMessage,
						AssignedType: assignedType,
						AssignedKey:  bestAgentKey,
						AssignedName: cap.DisplayName,
						MatchScore:   bestPerfs[0].SuccessRate,
						MatchLayer:   "performance",
						MatchReason:  fmt.Sprintf("历史性能最优 (成功率 %.2f, DQ %.2f)", bestPerfs[0].SuccessRate, bestPerfs[0].AvgDQScore),
					}, nil
				}
			}
		}
	}

	capHints := extractCapabilityHints(taskPlan.UserMessage)
	bestMatch, bestScore, matchReason := impl.exactMatch(capHints, capabilities)

	if bestScore > 0.3 && bestMatch.AgentKey != "" {
		return biz.TaskAllocation{
			SubTaskID:    "whole",
			SubTaskName:  taskPlan.UserMessage,
			AssignedType: assignedType,
			AssignedKey:  bestMatch.AgentKey,
			AssignedName: bestMatch.DisplayName,
			MatchScore:   bestScore,
			MatchLayer:   "exact",
			MatchReason:  matchReason,
		}, nil
	}

	if strings.TrimSpace(taskPlan.DomainPath) != "" {
		return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for plan")
	}

	semCap, semScore, semReason := impl.matchLayer2ForPlan(ctx, taskPlan, capabilities, traceID)
	if semScore > 0.3 && semCap.AgentKey != "" {
		impl.lg.Info("Layer 2 语义匹配命中 (whole plan)",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("agent_key", semCap.AgentKey),
			loggateway.Float64("score", semScore),
		)
		return biz.TaskAllocation{
			SubTaskID:    "whole",
			SubTaskName:  taskPlan.UserMessage,
			AssignedType: assignedType,
			AssignedKey:  semCap.AgentKey,
			AssignedName: semCap.DisplayName,
			MatchScore:   semScore,
			MatchLayer:   "semantic",
			MatchReason:  semReason,
		}, nil
	}

	llmMatch, err := impl.llmColdStartForPlan(ctx, taskPlan, capabilities, traceID)
	if err == nil && llmMatch != "" {
		displayName := llmMatch
		for _, cap := range capabilities {
			if cap.AgentKey == llmMatch {
				displayName = cap.DisplayName
				break
			}
		}
		return biz.TaskAllocation{
			SubTaskID:     "whole",
			SubTaskName:   taskPlan.UserMessage,
			AssignedType:  assignedType,
			AssignedKey:   llmMatch,
			AssignedName:  displayName,
			MatchScore:    bestScore,
			MatchLayer:    "llm_cold_start",
			MatchReason:   "LLM 冷启动匹配",
			FallbackKey:   bestMatch.AgentKey,
			FallbackScore: bestScore,
		}, nil
	}

	return biz.TaskAllocation{}, apierror.NotFound(apierror.DomainSpirit, "no agent found for plan")
}

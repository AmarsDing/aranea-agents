package agent

import (
	"aranea-agents/internal/biz"

	"aranea-agents/pkg/loggateway"
	"context"

	"strings"
)

// tryAgentFactoryForSubTask calls AgentFactory to dynamically create an Agent
// when 4-layer matching fails for a subtask (P1-4). Returns the allocation
// and true on success; returns zero value and false when AgentFactory is
// unavailable or creation fails (caller should fall back).
func (impl *agentAllocatorImpl) tryAgentFactoryForSubTask(ctx context.Context, subTask biz.SubTask, spiritSessionID, traceID string, pool []biz.AgentCapability) (biz.TaskAllocation, bool) {
	if impl.agentFactory == nil {
		return biz.TaskAllocation{}, false
	}
	desc := strings.TrimSpace(subTask.Description)
	if desc == "" {
		desc = subTask.Name
	}
	domain := "engineering"
	if subTask.DomainPath != "" {
		domain = TopLevelDomain(subTask.DomainPath)
	}
	profile := biz.TaskProfile{
		RequiredCapabilities: subTask.RequiredCapabilities,
		Domain:               domain,
		DomainPath:           subTask.DomainPath,
		TaskDescription:      desc,
		SpiritSessionID:      spiritSessionID,
		DepartmentID:         departmentIDFromPool(subTask.DomainPath, pool),
	}
	agentKey, err := impl.agentFactory.EnsureAgent(ctx, profile)
	if err != nil {
		impl.lg.Warn("AgentFactory 创建失败，降级为 fallback",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("sub_task_id", subTask.ID),
			loggateway.Err(err),
		)
		return biz.TaskAllocation{}, false
	}
	impl.lg.Info("AgentFactory 动态创建 Agent 命中",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("sub_task_id", subTask.ID),
		loggateway.AgentKey(agentKey),
	)
	return biz.TaskAllocation{
		SubTaskID:    subTask.ID,
		SubTaskName:  subTask.Name,
		AssignedType: "agent",
		AssignedKey:  agentKey,
		AssignedName: subTask.Name,
		MatchScore:   0,
		MatchLayer:   "factory",
		MatchReason:  "AgentFactory 动态创建",
	}, true
}

// tryAgentFactoryForPlan calls AgentFactory for a whole-plan allocation when
// 4-layer matching fails (P1-4). Same contract as tryAgentFactoryForSubTask.
func (impl *agentAllocatorImpl) tryAgentFactoryForPlan(ctx context.Context, taskPlan *biz.TaskPlan, traceID string, pool []biz.AgentCapability) (biz.TaskAllocation, bool) {
	if impl.agentFactory == nil {
		return biz.TaskAllocation{}, false
	}
	domain := "engineering"
	if taskPlan.DomainPath != "" {
		domain = TopLevelDomain(taskPlan.DomainPath)
	}
	profile := biz.TaskProfile{
		RequiredCapabilities: extractCapabilityHints(taskPlan.UserMessage),
		Domain:               domain,
		DomainPath:           taskPlan.DomainPath,
		TaskDescription:      taskPlan.UserMessage,
		SpiritSessionID:      taskPlan.SpiritSessionID,
		DepartmentID:         departmentIDFromPool(taskPlan.DomainPath, pool),
	}
	agentKey, err := impl.agentFactory.EnsureAgent(ctx, profile)
	if err != nil {
		impl.lg.Warn("AgentFactory 创建失败 (whole plan)，降级为 fallback",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return biz.TaskAllocation{}, false
	}
	impl.lg.Info("AgentFactory 动态创建 Agent 命中 (whole plan)",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.AgentKey(agentKey),
	)
	return biz.TaskAllocation{
		SubTaskID:    "whole",
		SubTaskName:  taskPlan.UserMessage,
		AssignedType: "agent",
		AssignedKey:  agentKey,
		AssignedName: taskPlan.UserMessage,
		MatchScore:   0,
		MatchLayer:   "factory",
		MatchReason:  "AgentFactory 动态创建",
	}, true
}

// fallbackAllocation creates a fallback allocation when matching fails.
func (impl *agentAllocatorImpl) fallbackAllocation(subTask biz.SubTask, capabilities []biz.AgentCapability) biz.TaskAllocation {
	if len(capabilities) > 0 {
		return biz.TaskAllocation{
			SubTaskID:    subTask.ID,
			SubTaskName:  subTask.Name,
			AssignedType: "agent",
			AssignedKey:  capabilities[0].AgentKey,
			AssignedName: capabilities[0].DisplayName,
			MatchScore:   0,
			MatchLayer:   "fallback",
			MatchReason:  "匹配失败，使用第一个可用 Agent",
		}
	}
	return biz.TaskAllocation{
		SubTaskID:    subTask.ID,
		SubTaskName:  subTask.Name,
		AssignedType: "agent",
		AssignedKey:  biz.SpiritAgentKey,
		AssignedName: "Spirit",
		MatchScore:   0,
		MatchLayer:   "fallback",
		MatchReason:  "无可用 Agent，降级为 Spirit 直接回答",
	}
}

// fallbackWholePlanAllocation creates a fallback allocation for a whole plan.
func (impl *agentAllocatorImpl) fallbackWholePlanAllocation(taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability) biz.TaskAllocation {
	if len(capabilities) > 0 {
		return biz.TaskAllocation{
			SubTaskID:    "whole",
			SubTaskName:  taskPlan.UserMessage,
			AssignedType: "agent",
			AssignedKey:  capabilities[0].AgentKey,
			AssignedName: capabilities[0].DisplayName,
			MatchScore:   0,
			MatchLayer:   "fallback",
			MatchReason:  "匹配失败，使用第一个可用 Agent",
		}
	}
	return biz.TaskAllocation{
		SubTaskID:    "whole",
		SubTaskName:  taskPlan.UserMessage,
		AssignedType: "agent",
		AssignedKey:  biz.SpiritAgentKey,
		AssignedName: "Spirit",
		MatchScore:   0,
		MatchLayer:   "fallback",
		MatchReason:  "无可用 Agent，降级为 Spirit 直接回答",
	}
}

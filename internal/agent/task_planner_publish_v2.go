package agent

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

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

	if len(plan.SubTasks) == 0 {
		impl.lg.Info("PublishV2Board: SubTasks 为空，跳过发布 PlanBoard/GraphStage",
			loggateway.StepID(biz.SpiritStepPlannerPersist),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Str("plan_id", plan.ID),
		)
		return biz.PlanBoard{}, nil
	}

	rootTaskID := string(RootTaskActivityIDFromCtx(ctx))

	sessionID := chatSessionID
	if sessionID == "" {
		sessionID = spiritSessionID
	}
	now := time.Now()

	pbSeed := plan.ID
	if pbSeed == "" {
		pbSeed = rootTaskID
	}
	pbID := "pb_" + pbSeed
	if pbSeed == "" {
		pbID = "pb_" + uuid.NewString()
	}

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
				if alloc.DepartmentID != "" && ps.DepartmentID == "" {
					ps.DepartmentID = alloc.DepartmentID
				}
				if len(alloc.CrossDeptMemberKeys) > 0 {
					ps.CrossDeptMemberKeys = append(ps.CrossDeptMemberKeys, alloc.CrossDeptMemberKeys...)
				}
				if alloc.MatchLayer != "" {
					ps.MatchLayer = alloc.MatchLayer
				}
				if alloc.MatchReason != "" {
					ps.MatchReason = alloc.MatchReason
				}
				if alloc.AssignedName != "" {
					ps.AssignedName = alloc.AssignedName
				}
			}
		}
		if st.DomainPath != "" && ps.DomainPath == "" {
			ps.DomainPath = st.DomainPath
		}
		if st.GraphTemplateID != "" {
			ps.GraphTemplateID = st.GraphTemplateID
		}
		if st.ConfirmBefore {
			ps.ConfirmBefore = true
		}
		if ids := biz.NormalizeCollectionIDs(st.CollectionIDs); len(ids) > 0 {
			ps.CollectionIDs = ids
		}
		ps.Mode = biz.SpiritTeamModeForStep(string(plan.Strategy), len(ps.AgentKeys))
		planSteps = append(planSteps, ps)
	}

	gsID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v2:"+pbID)).String()

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

	pb := biz.PlanBoard{
		ID:        pbID,
		TaskID:    rootTaskID,
		TurnID:    event.TurnIDFromContext(ctx),
		SessionID: spiritSessionID,
		Strategy:  mapV1StrategyToV2(plan.Strategy),
		Status:    biz.PlanStatusPlanning,
		Steps:     planSteps,
		StartedAt: now,
		Version:   1,
	}

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

		impl.seq.Publish(ctx, biz.NewPlanBoardCreatedEvent(pb))

		impl.seq.Publish(ctx, biz.NewGraphStageCreatedEvent(gs))

		for _, ps := range planSteps {
			impl.seq.Publish(ctx, biz.NewPlanStepStartedEvent(ps, spiritSessionID))
		}

		for _, gn := range graphNodes {
			impl.seq.Publish(ctx, biz.NewGraphNodeUpdatedEvent(gn, rootTaskID, spiritSessionID))
		}
	}
	_ = sessionID
	return pb, nil
}

// mapV1StrategyToV2 将 v1 biz.OrchestrationStrategy 映射为 v2 biz.PlanStrategy。
func mapV1StrategyToV2(s biz.OrchestrationStrategy) biz.PlanStrategy {
	switch s {
	case biz.StrategyDirect, biz.StrategySingleAgent:
		return biz.PlanStrategySequential
	case biz.StrategyParallel:
		return biz.PlanStrategyParallel
	case biz.StrategyDAG:
		return biz.PlanStrategyDAG
	case biz.StrategyCoordinator:
		return biz.PlanStrategyCoordinator
	default:
		return biz.PlanStrategySequential
	}
}

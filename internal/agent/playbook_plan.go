package agent

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

func playbookMemoryHit(pb biz.Playbook) *biz.MemoryHit {
	return &biz.MemoryHit{
		PlaybookID:            pb.ID,
		ConstraintFingerprint: biz.ConstraintFingerprint(pb.ID, nil),
	}
}

func (impl *taskPlannerImpl) companyNodeCount(ctx context.Context) int {
	if impl == nil || impl.org == nil {
		return 0
	}
	companies, err := impl.org.ListOrgNodesByLevel(ctx, "company")
	if err != nil {
		// org 读取失败会让剧本旁路静默失效（回落 planner LLM），必须留痕。
		impl.lg.Warn("公司节点读取失败，剧本探测降级为空",
			loggateway.StepID(biz.SpiritStepPlannerRoute),
			loggateway.Err(err),
		)
		return 0
	}
	return len(companies)
}

func (impl *taskPlannerImpl) eachCompanyMetadata(ctx context.Context) []string {
	if impl == nil || impl.org == nil {
		return nil
	}
	companies, err := impl.org.ListOrgNodesByLevel(ctx, "company")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(companies))
	for _, c := range companies {
		out = append(out, c.MetadataJSON)
	}
	return out
}

func (impl *taskPlannerImpl) currentConstraintFingerprint(ctx context.Context, taskText string) string {
	for _, meta := range impl.eachCompanyMetadata(ctx) {
		if pb, _, ok := biz.TryPlaybookForTask(meta, taskText); ok {
			return biz.ConstraintFingerprint(pb.ID, nil)
		}
	}
	return ""
}

func (impl *taskPlannerImpl) lookupNamedPlaybook(ctx context.Context, taskText string) (biz.Playbook, []biz.SubTask, bool) {
	for _, meta := range impl.eachCompanyMetadata(ctx) {
		if pb, steps, ok := biz.TryPlaybookForTask(meta, taskText); ok {
			return pb, steps, true
		}
	}
	return biz.Playbook{}, nil, false
}

// lookupSolePlaybookIfHeavy expands the workspace's sole authorized playbook.
// org-invariants §1: a workspace defaults to ONE company tree, so "the company's
// only authorized playbook" is unambiguous there. When multiple companies each
// hold a sole authorized playbook (multi-company fixtures), first-match would
// hijack unrelated heavy tasks into the wrong company's process — fail closed
// and let the planner LLM decompose instead (2026-09-02: planner LLM path was
// silently dead on multi-company workspaces since playbooks were authorized).
func (impl *taskPlannerImpl) lookupSolePlaybookIfHeavy(ctx context.Context, gear TaskGear) (biz.Playbook, []biz.SubTask, bool) {
	if gear != GearHeavy {
		return biz.Playbook{}, nil, false
	}
	var (
		solePB    biz.Playbook
		soleSteps []biz.SubTask
		found     int
	)
	for _, meta := range impl.eachCompanyMetadata(ctx) {
		if pb, steps, ok := biz.TrySoleAuthorizedPlaybook(meta); ok {
			solePB, soleSteps = pb, steps
			found++
		}
	}
	if found != 1 {
		if found > 1 {
			// 配置异常（多公司多独家剧本）导致重型任务静默走 LLM 分解，
			// 偏离预期剧本路径——Warn 而非 Info。
			impl.lg.Warn("多家公司各有独家授权剧本，重型档旁路歧义 fail-closed，回落 planner LLM 分解",
				loggateway.StepID(biz.SpiritStepPlannerRoute),
				loggateway.Int("candidate_count", found),
			)
		}
		return biz.Playbook{}, nil, false
	}
	return solePB, soleSteps, true
}

func (impl *taskPlannerImpl) planFromNamedPlaybook(ctx context.Context, input biz.PlanInput, traceID string) (*biz.TaskPlan, bool) {
	pb, steps, ok := impl.lookupNamedPlaybook(ctx, input.UserMessage)
	if !ok || len(steps) == 0 {
		return nil, false
	}
	return impl.persistPlaybookPlan(ctx, input, traceID, pb, steps)
}

func (impl *taskPlannerImpl) persistPlaybookPlan(ctx context.Context, input biz.PlanInput, traceID string, pb biz.Playbook, steps []biz.SubTask) (*biz.TaskPlan, bool) {
	strategy := biz.StrategyParallel
	if len(steps) >= 2 {
		strategy = biz.StrategyDAG
	}
	reason := "authorized playbook expand; skip planner LLM"
	plan := &biz.TaskPlan{
		ID:                 "tp_" + uuid.NewString(),
		SpiritSessionID:    input.SpiritSessionID,
		TraceID:            traceID,
		UserMessage:        input.UserMessage,
		IntentArtifactJSON: "{}",
		ComplexityLevel:    biz.ComplexityComplex,
		Strategy:           strategy,
		StrategyReason:     reason,
		TopologyHint:       biz.TopologyHybrid,
		SubTasks:           steps,
		TaskDAG:            buildDAGFromSubTasks(steps),
		DecomposeReason:    reason,
		DomainPath:         PrimaryDomainPath(steps),
		MemoryHit:          playbookMemoryHit(pb),
		Status:             biz.TaskPlanStatusDraft,
	}
	if input.IntentArtifact != nil {
		if b, err := json.Marshal(input.IntentArtifact); err == nil {
			plan.IntentArtifactJSON = string(b)
		}
	}
	saved, err := impl.repo.Create(ctx, plan)
	if err != nil {
		impl.lg.Warn("TaskPlan 剧本展开持久化失败",
			loggateway.StepID(biz.SpiritStepPlannerPersist),
			loggateway.Str("trace_id", traceID),
			loggateway.Err(err),
		)
		return nil, false
	}
	impl.lg.Info("已授权剧本展开，跳过分解 LLM",
		loggateway.StepID(biz.SpiritStepPlannerRoute),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("playbook_id", pb.ID),
		loggateway.Int("stage_count", len(steps)),
	)
	impl.emitPlannerDecision(ctx, plannerDecision{
		TraceID:         traceID,
		DecisionSource:  "playbook",
		Mode:            strings.ToLower(strings.TrimSpace(input.Mode)),
		Strategy:        strategy,
		ComplexityLevel: biz.ComplexityComplex,
		StrategyReason:  reason,
		SpiritSessionID: input.SpiritSessionID,
	})
	impl.publishPlanCreated(ctx, saved, input.ChatSessionID)
	return saved, true
}

func (impl *taskPlannerImpl) planPlaybookFillRequired(ctx context.Context, input biz.PlanInput, traceID string) (*biz.TaskPlan, error) {
	reason := biz.PlaybookFillRequiredReason
	plan := &biz.TaskPlan{
		ID:                 "tp_" + uuid.NewString(),
		SpiritSessionID:    input.SpiritSessionID,
		TraceID:            traceID,
		UserMessage:        input.UserMessage,
		IntentArtifactJSON: "{}",
		ComplexityLevel:    biz.ComplexityComplex,
		Strategy:           biz.StrategyDirect,
		StrategyReason:     reason,
		DecomposeReason:    biz.PlaybookFillUserHint,
		Status:             biz.TaskPlanStatusDraft,
	}
	if input.IntentArtifact != nil {
		if b, err := json.Marshal(input.IntentArtifact); err == nil {
			plan.IntentArtifactJSON = string(b)
		}
	}
	impl.publishOrchestrationProgress(ctx, input.SpiritSessionID, reason, map[string]any{
		"pipe":             biz.PipeDownwardGrant,
		"summary":          biz.PlaybookFillUserHint,
		"dispatch_barrier": false,
	})
	saved, err := impl.repo.Create(ctx, plan)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainSpirit, "persist playbook-fill plan").WithCause(err)
	}
	impl.emitPlannerDecision(ctx, plannerDecision{
		TraceID:         traceID,
		DecisionSource:  "playbook_fill_required",
		Mode:            strings.ToLower(strings.TrimSpace(input.Mode)),
		Strategy:        biz.StrategyDirect,
		ComplexityLevel: biz.ComplexityComplex,
		StrategyReason:  reason,
		SpiritSessionID: input.SpiritSessionID,
	})
	impl.publishPlanCreated(ctx, saved, input.ChatSessionID)
	return saved, nil
}

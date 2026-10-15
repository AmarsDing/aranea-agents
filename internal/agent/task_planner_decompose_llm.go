package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// decomposeTask uses LLM to decompose a complex task into subtasks (T1.6).
// teamCount > 0 时将作为硬约束传给 LLM，要求生成恰好 N 个 subtask（每个
// subtask 在 orchestrateDAG 中对应一个 team）。teamCount = 0 时使用默认范围。
// level 是 P2-5 思考强度路由的任务复杂度（空 = 不覆盖静态配置）。
func (impl *taskPlannerImpl) decomposeTask(ctx context.Context, userMessage string, artifact *biz.IntentArtifact, teamCount int, level biz.ComplexityLevel) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "LLM catalog or HTTP client not configured")
	}

	capTags, rosterLines := impl.decomposeRosterContext(ctx)
	prompt := buildDecompositionPrompt(userMessage, artifact, teamCount, capTags, rosterLines...)

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
	ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)

	if eff := biz.ResolveThinkingEffort(cfg.ThinkingEffort, level); eff != "" {
		cfg.ThinkingEffort = eff
	}

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Decompose the following task:\n\n" + userMessage},
	}

	callCtx, cancel := context.WithTimeout(ctx, tools.DecomposeLLMTimeout)
	defer cancel()

	callStart := time.Now()
	text, _, promptTok, completionTok, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)

	traceID, _ := biz.SpiritTraceIDFromContext(ctx)
	recordSpiritAuxUsage(ctx, impl.auxUsage, impl.lg, biz.SpiritStepPlannerDecompose, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxPlannerDecompose,
		RunID:         traceID,
		Provider:      provider,
		Model:         model,
		Status:        spiritAuxCallStatus(err),
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		UsageSource:   biz.UsageSourceResponse,
		Effort:        cfg.ThinkingEffort,
		Latency:       time.Since(callStart),
		ErrMsg:        spiritAuxErrMsg(err),
		MetadataJSON:  spiritAuxMeta(ctx, "spirit_planner_decompose"),
	})
	if err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "LLM call failed").WithCause(err)
	}

	text = stripDecompositionFences(text)

	if qs, ok := parseClarificationRequest(text); ok {
		return nil, nil, &decomposeClarificationError{questions: qs}
	}
	subTasks, err := parseDecompositionOutput(text)
	if err != nil {
		return nil, nil, apierror.Internal(apierror.DomainSpirit, "parse decomposition").WithCause(err)
	}

	if len(subTasks) == 0 {
		return nil, nil, nil
	}

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

			return nil, nil, err
		}

		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		lastErr = err

		if attempt >= maxAttempts {
			break
		}

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
	capTags, rosterLines := impl.decomposeRosterContext(ctx)
	prompt := buildDecompositionPrompt(userMessage, artifact, teamCount, capTags, rosterLines...)

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
	ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)

	if eff := biz.ResolveThinkingEffort(cfg.ThinkingEffort, level); eff != "" {
		cfg.ThinkingEffort = eff
	}

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Decompose the following task:\n\n" + userMessage},
	}

	callCtx := ctx

	parser := newStreamSubTaskParser()
	var subTasks []biz.SubTask
	idRemap := make(map[string]string)

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
	callStart := time.Now()
	text, reasoning, promptTok, completionTok, callErr := CallOpenAICompatChatStream(callCtx, impl.httpClient, cfg, model, msgs, callbacks)

	recordSpiritAuxUsage(ctx, impl.auxUsage, impl.lg, biz.SpiritStepPlannerDecompose, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxPlannerDecompose,
		SessionID:     spiritSessionID,
		RunID:         planID,
		Provider:      provider,
		Model:         model,
		Status:        spiritAuxCallStatus(callErr),
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		UsageSource:   UsageSourceStreaming,
		Effort:        cfg.ThinkingEffort,
		Latency:       time.Since(callStart),
		ErrMsg:        spiritAuxErrMsg(callErr),
		MetadataJSON:  spiritAuxMeta(ctx, "spirit_planner_decompose"),
	})
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

	if len(subTasks) == 0 {
		text = stripDecompositionFences(text)

		if qs, ok := parseClarificationRequest(text); ok {
			return nil, nil, &decomposeClarificationError{questions: qs}
		}
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

	if _, ok := idRemap[raw.ID]; !ok {
		idRemap[raw.ID] = planStepID(planID, raw.ID)
	}

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

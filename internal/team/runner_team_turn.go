package team

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

type anchorResolution struct {
	member    MemberDef
	agent     biz.Agent
	prov      string
	mod       string
	attRefs   []artifactbiz.Ref
	attN      int
	teamID    string // 新增：标识消息属于哪个团队
	sessionID string // P3 aux 缓存：注入 intent pass 缓存键
}

// resolveTeamProviderModel resolves the effective provider/model for a team
// turn. Priority: explicit turn option → session default → anchor agent
// config, then catalog validation with fallback (RefineLLM → first enabled).
// The catalog validation keeps the observation path (context window) aligned
// with the execution path, which falls back to the system default model at
// agent build time when the configured model is not in the catalog.
func resolveTeamProviderModel(
	ctx context.Context,
	catalog biz.TeamModelCatalog,
	refine biz.RefineLLMLookup,
	lg loggateway.Logger,
	provOpt, modOpt string,
	sess biz.Session,
	firstAg biz.Agent,
) (string, string) {
	prov := strutil.FirstNonEmpty(provOpt, sess.DefaultProvider, firstAg.Provider)
	mod := strutil.FirstNonEmpty(modOpt, sess.DefaultModel, firstAg.Model)
	return biz.ResolveProviderModelWithFallback(ctx, catalog, refine, lg, prov, mod)
}

func (r *Runner) resolveAnchorAndAttachments(
	ctx context.Context,
	members []MemberDef,
	intentAnchorAgentID string,
	sess biz.Session,
	input biz.TurnInput,
	provOpt, modOpt string,
	run *biz.TeamRunRecord,
	t0 time.Time,
) (ar anchorResolution, turnStatus string, err error) {
	turnStatus = biz.TeamMemberStepStatusOK
	anchorMem := members[0]
	if want := strings.TrimSpace(intentAnchorAgentID); want != "" {
		found := false
		for _, m := range members {
			if strings.TrimSpace(m.AgentID) == want {
				anchorMem = m
				found = true
				break
			}
		}
		if !found {
			r.lg.Warn("团队意图锚点不在成员列表，使用首个成员",
				loggateway.StepID("team.intent_anchor_fallback"),
				loggateway.Str("intent_anchor_agent_id", want))
		}
	}
	firstAg, err := r.lookupAgent(ctx, anchorMem.AgentID)
	if err != nil {
		if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
			err = apierror.NotFound("AGENT", "team member agent not found")
		}
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, run, t0, err.Error())
		return
	}

	prov0, mod0 := resolveTeamProviderModel(ctx, r.td.ReadDeps.LLM, r.td.ReadDeps.Settings, r.lg, provOpt, modOpt, sess, firstAg)
	var attachmentRefs []artifactbiz.Ref
	attN := 0
	if r.td.Persist.ArtifactUC != nil && len(artifactbiz.NormalizeAttachmentIDs(input.Options.AttachmentIDs)) > 0 {
		attachmentRefs, err = artifactbiz.ResolveAttachmentRefs(ctx, r.td.Persist.ArtifactUC, sess.ID, input.Options.AttachmentIDs)
		if err != nil {
			turnStatus = biz.TeamMemberStepStatusError
			r.finishRunErr(ctx, run, t0, err.Error())
			return
		}
		attN = len(attachmentRefs)
		if refsContainImageAttachment(attachmentRefs) && !provider.ModelSupportsImageAttachments(ctx, r.td.ReadDeps.LLM, prov0, mod0, r.lg) {
			err = apierror.BadRequest("CHAT_AGENT", fmt.Sprintf("当前模型不支持该附件类型 (%s/%s does not support image attachments)", strings.TrimSpace(prov0), strings.TrimSpace(mod0)))
			turnStatus = biz.TeamMemberStepStatusError
			r.finishRunErr(ctx, run, t0, err.Error())
			return
		}
		if refsContainFileAttachment(attachmentRefs) && !provider.ModelSupportsFileAttachments(ctx, r.td.ReadDeps.LLM, prov0, mod0, r.lg) {
			err = apierror.BadRequest("CHAT_AGENT", fmt.Sprintf("当前模型不支持该附件类型 (%s/%s does not support file attachments)", strings.TrimSpace(prov0), strings.TrimSpace(mod0)))
			turnStatus = biz.TeamMemberStepStatusError
			r.finishRunErr(ctx, run, t0, err.Error())
			return
		}
	}
	ar = anchorResolution{
		member:    anchorMem,
		agent:     firstAg,
		prov:      prov0,
		mod:       mod0,
		attRefs:   attachmentRefs,
		attN:      attN,
		teamID:    run.TeamID, // 新增：透传 team_id 供 assistant 消息锚点使用
		sessionID: sess.ID,    // P3 aux 缓存键
	}
	return
}

type userTurnOptions struct {
	userOpts      string
	intentRunOpts []trpcagent.RunOption
	attN          int
}

// teamIntentTimeout matches the chat C2 fuse (2.5s). Team intent used to
// wait up to 45s serially after compileTeamRuntime.
const teamIntentTimeout = 2*time.Second + 500*time.Millisecond

// pendingTeamIntent is an in-flight team intent pass started before graph
// compile so the LLM hop overlaps compileTeamRuntime.
type pendingTeamIntent struct {
	ch     chan intent.RunResult
	cancel context.CancelFunc
	skip   bool
}

func (p pendingTeamIntent) stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (r *Runner) startTeamIntentPass(ctx context.Context, ar anchorResolution, content string) pendingTeamIntent {
	if intent.SkipForDirectReply(content) {
		return pendingTeamIntent{skip: true}
	}
	if !intent.ShouldRun(ar.agent, content) {
		return pendingTeamIntent{}
	}
	ch := make(chan intent.RunResult, 1)
	ictx, cancel := context.WithTimeout(ctx, teamIntentTimeout)
	// P3 aux 缓存：注入 sessionID 供缓存键隔离（team 场景 session 即 team session）。
	ictx = intent.WithSessionID(ictx, ar.sessionID)
	safego.Go(ictx, "team.intent.pass", func() {
		ch <- intent.RunForAgent(ictx, ar.agent, r.td.ReadDeps.LLM, r.td.LLMHTTP, ar.prov, ar.mod, content, nil, r.lg)
	})
	return pendingTeamIntent{ch: ch, cancel: cancel}
}

func (r *Runner) prepareUserTurnOptions(
	ctx context.Context,
	ar anchorResolution,
	content string,
	sess biz.Session,
	run *biz.TeamRunRecord,
	teamRow biz.Team,
	dialogMode string,
	t0 time.Time,
	pending pendingTeamIntent,
) (opts userTurnOptions, turnStatus string, err error) {
	turnStatus = biz.TeamMemberStepStatusOK
	anchor := &agent.TeamMemberAnchor{
		AgentID: ar.agent.ID,
		Name:    strutil.FirstNonEmpty(ar.agent.DisplayName, ar.agent.AgentKey),
		Role:    ar.member.Role,
		TeamID:  teamRow.ID, // 新增：传递 team_id
	}
	userOpts, err := agent.UserOptionsJSON(ar.agent, dialogMode, ar.prov, ar.mod, sess.ContextUsedRatio, anchor)
	if err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, run, t0, err.Error())
		return
	}
	var intentRunOpts []trpcagent.RunOption
	var intRes intent.RunResult

	// 输入级安全三档（Q6，与 chat 同表）：Deny 停 intent pass 并零 LLM
	// 拒绝；HITL/Inform 不阻断。content 来自 validateTeamTurnInput。
	inputRisk := intent.ClassifyInputSafety(content)
	inputRiskFlags := inputRisk.Flags
	if inputRisk.Action == intent.SafetyDeny {
		pending.stop()
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, run, t0, intent.SafetyDenyUserMessage)
		err = apierror.Forbidden(apierror.DomainChatTeamNative, intent.SafetyDenyUserMessage)
		event.EmitGate(ctx, r.cfg.DecisionCollector, decision.GateDecision{
			TriggerRule: decision.TriggerInputRiskFlagged,
			Outcome:     "blocked",
			Scenario:    "团队会话用户输入命中 Deny 级破坏性操作，零 LLM 拒绝",
			Reasoning:   fmt.Sprintf("action=deny hits=%v", inputRisk.Hits),
			GuardName:   "input_safety_scan",
			RunID:       run.ID,
			SessionID:   run.SessionID,
			Extra:       map[string]any{"action": string(inputRisk.Action)},
		})
		return
	}
	if inputRisk.Action == intent.SafetyHITL {
		event.EmitGate(ctx, r.cfg.DecisionCollector, decision.GateDecision{
			TriggerRule: decision.TriggerInputRiskFlagged,
			Outcome:     "tripped",
			Scenario:    "团队会话用户输入命中 HITL 级风险扫描",
			Reasoning:   fmt.Sprintf("action=hitl hits=%v", inputRisk.Hits),
			GuardName:   "input_safety_scan",
			RunID:       run.ID,
			SessionID:   run.SessionID,
			Extra:       map[string]any{"action": string(inputRisk.Action), "flags": strings.Join(inputRiskFlags, ",")},
		})
	} else if inputRisk.Action == intent.SafetyInform {
		r.lg.Info("input risk shadow hit (not flagged)",
			loggateway.StepID("team.input_risk.shadow"),
			loggateway.Str("session_id", run.SessionID),
			loggateway.Str("shadow_hits", strings.Join(inputRisk.Hits, ",")),
		)
	}

	shouldRunIntent := !pending.skip && intent.ShouldRun(ar.agent, content)
	artifactInjected := false
	if pending.ch != nil {
		intRes = <-pending.ch
		pending.stop()
		shouldRunIntent = true
	} else if shouldRunIntent {
		intentCtx, cancel := context.WithTimeout(ctx, teamIntentTimeout)
		intRes = intent.RunForAgent(intentCtx, ar.agent, r.td.ReadDeps.LLM, r.td.LLMHTTP, ar.prov, ar.mod, content, nil, r.lg)
		cancel()
	}
	if shouldRunIntent {
		// P1-2 (2026-08-19): 记录团队 intent pass 旁路用量（此前完全漏记）。
		r.recordAuxUsage(ctx, biz.AuxLLMUsageInput{
			Kind:          biz.UsageKindAuxIntent,
			SessionID:     run.SessionID,
			TeamID:        teamRow.ID,
			AgentID:       ar.agent.ID,
			AgentKey:      ar.agent.AgentKey,
			Provider:      ar.prov,
			Model:         ar.mod,
			Status:        "success",
			PromptTok:     intRes.PromptTok,
			CompletionTok: intRes.CompletionTok,
			UsageSource:   biz.UsageSourceResponse,
			Latency:       intRes.Duration,
		})
		if intRes.Artifact != nil {
			if strings.TrimSpace(intRes.RawJSON) != "" {
				merged, merr := intent.MergeIntoUserOptionsJSON(userOpts, intRes.RawJSON)
				if merr != nil {
					r.lg.Warn("团队意图合并失败，将继续执行", loggateway.StepID("team.intent.merge_fail"), loggateway.Err(merr))
				} else {
					userOpts = merged
				}
			}
			intentRunOpts = append(intentRunOpts, intent.RunOptionInject(intRes.Artifact))
			artifactInjected = true
		}
	}
	if !artifactInjected && len(inputRiskFlags) > 0 {
		// 方案② S3-2 降级注入（对齐 chat 路径）：intent pass 无产物（未运行/失败/
		// 超时/parse 失败）但确定性扫描命中 → 注入风险提示，提醒主 LLM 对潜在
		// 破坏性操作先确认再执行；不改变流程、不挂起。
		intentRunOpts = append(intentRunOpts, intent.RunOptionInjectInputRisk(inputRiskFlags))
	}
	if len(ar.attRefs) > 0 {
		var merr error
		userOpts, merr = artifactbiz.MergeRefsIntoOptionsJSON(userOpts, ar.attRefs)
		if merr != nil {
			turnStatus = biz.TeamMemberStepStatusError
			r.finishRunErr(ctx, run, t0, merr.Error())
			return
		}
	}
	if shouldRunIntent && r.hasPublisher() {
		meta := intent.RunMeta{
			AgentID:   ar.agent.ID,
			SessionID: sess.ID,
			RunID:     run.ID,
			TeamID:    teamRow.ID,
		}
		payload := intent.BuildIntentPassPayload(intRes, meta)
		if payload == nil {
			payload = map[string]any{}
		}
		payload["team_id"] = teamRow.ID
		payload["agent_key"] = ar.agent.ID
		r.publishEvent(ctx, biz.NewSystemNoticeEvent(deriveSpiritSessionID(sess), "intent_pass", "", payload))
	}
	opts = userTurnOptions{
		userOpts:      userOpts,
		intentRunOpts: intentRunOpts,
		attN:          ar.attN,
	}
	return
}

// finalizeTeamRun persists the terminal state of a team run.
// Returns the updated TeamRun value (no pointer side-effects on the caller).
func (r *Runner) finalizeTeamRun(
	ctx context.Context,
	sess biz.Session,
	run biz.TeamRunRecord,
	teamRow biz.Team,
	ar anchorResolution,
	assistantMsg biz.ChatMessage,
	promptTok, completionTok, cachedTok int,
	dialogMode string,
	usageSource string,
	graphExecID string,
	t0 time.Time,
	teamEmitter *event.TraceEmitter,
) biz.TeamRunRecord {
	// R1-a（2026-09-01）：team_turn usage 行必须在所有终态出口前落库一次。
	// 交付物质量门 revise/fail 拦截的 run 同样消耗了真实 token；此前
	// recordTeamRunUsage 位于 success 路径末尾，被拦截即跳过，导致
	// 该 run 消耗从会话级统计中凭空消失。defer + 单次守卫确保任何
	// 终态（success / binary-fail / revise-fail）都入账。
	usageRecorded := false
	recordUsage := func() {
		if usageRecorded {
			return
		}
		usageRecorded = true
		r.recordTeamRunUsage(ctx, run, teamRow.ID, ar.agent, promptTok, completionTok, cachedTok, ar.prov, ar.mod, dialogMode, usageSource)
	}
	defer recordUsage()

	// 2026-07-28 修复3 真实产出闸门（runner 侧）：DAG 团队从未调用
	// set_deliverable（无 graph state 交付物）时禁止把 run 标为 success。
	// run 的 FSM 终态不可逆（success→failed 非法），必须在 success 转换前
	// 拦截；闸门否决走 finishRunErr（running→failed 合法）。与 service 层
	// HandleTeamTurnResult 闸门互为双保险，语义保持一致：infra 错误按无
	// 交付物处理。
	if teamRow.DagNodeID != "" && r.deliverableGate != nil {
		has, gateErr := r.deliverableGate(ctx, teamRow)
		if gateErr != nil {
			r.lg.Warn("真实交付物校验失败（infra），按无交付物处理",
				loggateway.StepID("team.run.deliverable_gate"),
				loggateway.Str("team_id", teamRow.ID),
				loggateway.Str("run_id", run.ID),
				loggateway.Err(gateErr))
		}
		if gateErr != nil || !has {
			r.finishRunErr(ctx, &run, t0, "团队未通过 set_deliverable 提交真实交付物（无真实产出，运行标记失败）")
			return run
		}
		// G3（ADR-G）：二元门通过后评估交付物内容质量。revise/fail 且修订
		// 预算内 → followup 打回 + run 标 failed；预算耗尽/判分异常/未装配
		// 修订通道 → fail-open 放行（防回归：二元门会放行的交付物不被卡死）。
		if r.qualityGateBlocks(ctx, sess, teamRow, &run, t0, teamEmitter) {
			return run
		}
	}
	// Transition status through the state machine for consistent validation & timestamps.
	updatedRun, transitionErr := r.runTransitioner.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusSuccess)
	if transitionErr != nil {
		r.lg.Error("TransitionRunStatus failed in finalizeTeamRun",
			loggateway.StepID("team.run.transition_fail"),
			loggateway.Str("team_run_id", run.ID), loggateway.Err(transitionErr))
	} else {
		// Preserve token/duration data from the original run before the transition.
		updatedRun.TokenIn = promptTok
		updatedRun.TokenOut = completionTok
		updatedRun.DurationMS = int(time.Since(t0).Milliseconds())
		updatedRun.OutputPreview = preview(assistantMsg.ContentMarkdown, 512)
		// Preserve transient SpiritSessionID (team_runs has no such column;
		// publishTeamRunSummary → TeamSummaryActivityEvent relies on it).
		updatedRun.SpiritSessionID = run.SpiritSessionID
		if err := r.runWriter.UpdateTeamRun(ctx, updatedRun); err != nil {
			r.lg.Warn("UpdateTeamRun failed in finalizeTeamRun",
				loggateway.StepID("team.run.finish_update_fail"),
				loggateway.Str("team_run_id", updatedRun.ID), loggateway.Str("update_error", err.Error()))
		}
		run = updatedRun
		r.recordRunCompletion(ctx, run, "", t0)
	}

	// F-B：graph 运行已完成（未走 HITL 延迟），显式收敛 graph_executions ——
	// team 路径没有 consumeRuntimeEvents 消费者替我们做这件事。幂等，重复
	// 调用（如 watch 已先收敛）无副作用。
	if graphExecID != "" && r.mediator != nil {
		_ = r.mediator.FinalizeTeamGraphExecution(ctx, graphExecID, false, "")
	}

	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.finish", "团队任务结束", event.P("status", run.Status))
	}
	if r.hasPublisher() {
		cp := run
		spiritSID := deriveSpiritSessionID(sess)
		now := time.Now().UTC()
		ts := biz.TeamStage{
			ID:        string(agent.NewTeamStageActivityID(teamRow.ID, string(agent.RootTaskActivityIDFromCtx(ctx)))),
			TeamID:    teamRow.ID,
			TeamName:  teamRow.DisplayName,
			SessionID: spiritSID,
			Status:    biz.TeamStageStatusCompleted,
			Stage:     biz.TeamStageStageCompleted,
			StartedAt: now,
			Version:   1,
		}
		r.publishEvent(ctx, biz.NewTeamStageCompletedEvent(ts))
		r.publishEvent(ctx, biz.NewSystemNoticeEvent(spiritSID, "team_stage_completed", "", map[string]any{
			"run_id":  run.ID,
			"run":     cp,
			"team_id": teamRow.ID,
		}))
		r.publishTeamRunSummary(ctx, run)
	}
	return run
}

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
	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

type anchorResolution struct {
	member  MemberDef
	agent   biz.Agent
	prov    string
	mod     string
	attRefs []artifactbiz.Ref
	attN    int
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
		member:  anchorMem,
		agent:   firstAg,
		prov:    prov0,
		mod:     mod0,
		attRefs: attachmentRefs,
		attN:    attN,
	}
	return
}

type userTurnOptions struct {
	userOpts      string
	intentRunOpts []trpcagent.RunOption
	attN          int
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
) (opts userTurnOptions, turnStatus string, err error) {
	turnStatus = biz.TeamMemberStepStatusOK
	anchor := &agent.TeamMemberAnchor{
		AgentID: ar.agent.ID,
		Name:    strutil.FirstNonEmpty(ar.agent.DisplayName, ar.agent.AgentKey),
		Role:    ar.member.Role,
	}
	userOpts, err := agent.UserOptionsJSON(ar.agent, dialogMode, ar.prov, ar.mod, sess.ContextUsedRatio, anchor)
	if err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, run, t0, err.Error())
		return
	}
	var intentRunOpts []trpcagent.RunOption
	var intRes intent.RunResult
	shouldRunIntent := intent.ShouldRun(ar.agent, content)
	if shouldRunIntent {
		// history 传 nil：成员 turn 的 content 是 leader 规划合成的指令（非用户原始
		// 追问），无指代/省略需解析；注入会话历史反而可能干扰成员对指令的判定。
		intRes = intent.RunForAgent(ctx, ar.agent, r.td.ReadDeps.LLM, r.td.LLMHTTP, ar.prov, ar.mod, content, nil, r.lg)
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
		}
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
	graphExecID string,
	t0 time.Time,
	teamEmitter *event.TraceEmitter,
) biz.TeamRunRecord {
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

	r.recordTeamRunUsage(ctx, run, teamRow.ID, ar.agent, promptTok, completionTok, cachedTok, ar.prov, ar.mod, dialogMode)

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

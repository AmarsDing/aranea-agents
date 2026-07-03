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

	"github.com/google/uuid"

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

	prov0 := strutil.FirstNonEmpty(provOpt, sess.DefaultProvider, firstAg.Provider)
	mod0 := strutil.FirstNonEmpty(modOpt, sess.DefaultModel, firstAg.Model)
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
		intRes = intent.RunForAgent(ctx, ar.agent, r.td.ReadDeps.LLM, r.td.LLMHTTP, ar.prov, ar.mod, content, r.lg)
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
		ev := biz.ActivityEvent{
			Event: biz.ActivityEventCreated,
			Activity: biz.Activity{
				ID:              uuid.NewString(),
				Kind:            biz.ActivityKindNotice,
				Status:          biz.ActivityStatusCompleted,
				SessionID:       sess.ID,
				SpiritSessionID: deriveSpiritSessionID(sess),
				TeamID:          teamRow.ID,
				AgentKey:        ar.agent.ID,
				Timestamp:       time.Now().UTC(),
				Stage:           "intent_pass",
				Meta:            intent.BuildIntentPassPayload(intRes, meta),
			},
			Domain: biz.ActivityDomainChat,
		}
		// Phase 3b-D: bridge to v2 EventBus. ActivityBridgeEvent preserves the
		// v1 intent_pass Notice activity (Meta carries intent pass payload).
		r.publishEvent(ctx, biz.NewActivityBridgeEvent(ev))
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
	promptTok, completionTok int,
	dialogMode string,
	graphExecID string,
	t0 time.Time,
	teamEmitter *event.TraceEmitter,
) biz.TeamRunRecord {
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
	}

	r.recordTeamRunUsage(ctx, run, teamRow.ID, ar.agent, promptTok, completionTok, ar.prov, ar.mod, dialogMode)

	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.finish", "团队任务结束", event.P("status", run.Status))
	}
	if r.hasPublisher() {
		cp := run
		// SessionID = spirit session ID (not run.SessionID which is the team
		// session ID) so the frontend WS filter and listActivities API return
		// this team_stage "completed" event. Matches publishSpiritTeamAssembled.
		spiritSID := deriveSpiritSessionID(sess)
		ev := biz.ActivityEvent{
			Event: biz.ActivityEventCompleted,
			Activity: biz.Activity{
				ID:              string(agent.NewTeamStageActivityID(teamRow.ID)),
				Kind:            biz.ActivityKindTeamStage,
				Status:          biz.ActivityStatusCompleted,
				SessionID:       spiritSID,
				SpiritSessionID: spiritSID,
				TeamID:          teamRow.ID,
				Timestamp:       time.Now().UTC(),
				Stage:           "completed",
				Meta:            map[string]any{"run_id": run.ID, "run": cp},
			},
			Domain: biz.ActivityDomainChat,
		}
		// Phase 3b-D: bridge to v2 EventBus. ActivityBridgeEvent preserves the
		// v1 TeamStage Completed activity (Meta.run_id, Meta.run snapshot).
		r.publishEvent(ctx, biz.NewActivityBridgeEvent(ev))
		r.publishTeamRunSummary(ctx, run)
	}
	return run
}

package team

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/event"
	sessctx "aranea-agents/internal/session"
	"aranea-agents/internal/telemetry/turntrace"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

// teamTurnInput holds validated inputs extracted from biz.TurnInput.
type teamTurnInput struct {
	content    string
	dialogMode string
	provOpt    string
	modOpt     string
	members    []MemberDef
}

// validateTeamTurnInput validates the turn input and extracts common options.
func (r *Runner) validateTeamTurnInput(input biz.TurnInput, sess biz.Session, def Definition) (teamTurnInput, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return teamTurnInput{}, apierror.BadRequest("CHAT_NATIVE", "content is required")
	}
	dialogMode, provOpt, modOpt, _ := extractOptsFromInput(input)
	dialogMode = strutil.FirstNonEmpty(dialogMode, sess.DialogMode, "default")

	members := EnabledMembers(def)
	if len(members) == 0 {
		return teamTurnInput{}, apierror.BadRequest(apierror.DomainTeam, "team has no enabled members")
	}
	return teamTurnInput{
		content:    content,
		dialogMode: dialogMode,
		provOpt:    provOpt,
		modOpt:     modOpt,
		members:    members,
	}, nil
}

// createInitialTeamRun builds and persists the initial TeamRun record.
func (r *Runner) createInitialTeamRun(ctx context.Context, sess biz.Session, teamRow biz.Team, def Definition, mode, content string) (biz.TeamRunRecord, error) {
	run := biz.TeamRunRecord{
		ID:                     uuid.NewString(),
		TeamID:                 teamRow.ID,
		SessionID:              sess.ID,
		Mode:                   mode,
		Status:                 biz.TeamRunStatusRunning,
		InputPreview:           preview(content, 512),
		TopologyJSON:           topologyJSON(def),
		DefinitionSnapshotJSON: strings.TrimSpace(teamRow.DefinitionJSON),
		StartedAt:              agent.RFC3339Now(),
		CreatedAt:              agent.RFC3339Now(),
		UpdatedAt:              agent.RFC3339Now(),
		SpiritSessionID:        deriveSpiritSessionID(sess),
	}
	saved, err := r.runWriter.CreateTeamRun(ctx, run)
	if err != nil {
		return biz.TeamRunRecord{}, err
	}
	// Restore transient SpiritSessionID: team_runs table has no spirit_session_id
	// column, so entTeamRunToBiz returns it empty. Callers (publishTeamStepActivity,
	// publishTeamRunFailedActivity) rely on this field to attribute session/team_stage
	// activities to the spirit session for correct WS delivery and frontend rendering.
	saved.SpiritSessionID = run.SpiritSessionID
	return saved, nil
}

// teamTracingSetup holds tracing/emitter artifacts returned by setupTeamTracing.
type teamTracingSetup struct {
	ctx        context.Context
	bridge     *turntrace.Bridge
	emitter    *event.TraceEmitter
	turnStatus string
}

// setupTeamTracing initializes turntrace span and event trace emitter.
// The caller is responsible for deferring bridge.Finish(err) and emitter.FinishRoot(turnStatus).
func (r *Runner) setupTeamTracing(ctx context.Context, sess biz.Session, teamRow biz.Team, run biz.TeamRunRecord, mode string, memberCount int) teamTracingSetup {
	ts := teamTracingSetup{turnStatus: biz.TeamMemberStepStatusOK}
	ctx, bridge, _ := turntrace.Start(ctx, turntrace.Config{
		Domain:    turntrace.DomainTeam,
		SpanName:  "team.run",
		SessionID: sess.ID,
		RunID:     run.ID,
		AgentKey:  teamRow.ID,
	})
	ctx = turntrace.WithBridge(ctx, bridge)
	ts.ctx = ctx
	ts.bridge = bridge

	emitter := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sess.ID, RunID: run.ID, AgentKey: teamRow.ID,
		Domain: event.TraceDomainTeam, LG: r.lg,
		Infra: event.NewInfraFromBus(r.td.Pipeline.MonitorEventBus),
	})
	emitter.SetOtelRefs(bridge.TraceID(), bridge.RootSpanID())
	ctx = event.WithTraceEmitter(ctx, emitter)
	ts.ctx = ctx
	ts.emitter = emitter

	if tid := strings.TrimSpace(bridge.TraceID()); tid != "" {
		if uerr := r.runWriter.UpdateTeamRunTraceID(ctx, run.ID, tid); uerr != nil {
			r.lg.Warn("trace_id 持久化失败", loggateway.StepID("team.run.trace_id"), loggateway.Err(uerr))
		}
	}
	emitter.LogStart("team.run.start", "开始团队协作",
		event.P("team_id", teamRow.ID), event.P("mode", mode), event.P("members", memberCount))
	return ts
}

// deriveSpiritSessionID returns the spirit session ID for cross-session aggregation.
// For a team session created by SpiritTeamAssembler, RootSessionID points to the
// spirit session that initiated the tree. Fallback to ParentSessionID for legacy
// data without RootSessionID set. 2026-07-29 F-3：standalone（Mode A）团队
// session 无 Root/Parent，回退 sess.ID——团队 session 即自己的聚合根，与
// service/team_pause.go resolveSpiritSessionIDForTeam 的回退语义一致；
// 否则 runner 侧所有 v2 事件（TeamStage/MemberSession/notice）的
// SpiritSessionID 为空，WS 路由丢失且 DB 落空值。
func deriveSpiritSessionID(sess biz.Session) string {
	if sess.RootSessionID != "" {
		return sess.RootSessionID
	}
	if sess.ParentSessionID != "" {
		return sess.ParentSessionID
	}
	return sess.ID
}

// publishTeamRunStartedEvent publishes the TeamRunStarted ActivityEvent if a bus is configured.
// NOTE: This event does NOT include member info in meta. The member list with correct
// individual session IDs is already published by publishSpiritTeamAssembled (which sync-persists
// before this event is published). Including team_summary.members here would overwrite the
// correct session IDs with the team session ID (sess.ID), causing the frontend to fail to
// lazy-load member execution processes.
func (r *Runner) publishTeamRunStartedEvent(ctx context.Context, sess biz.Session, teamRow biz.Team, run biz.TeamRunRecord, def Definition) {
	if !r.hasPublisher() {
		return
	}
	cp := run
	spiritSID := deriveSpiritSessionID(sess)

	meta := map[string]any{
		"run_id":    run.ID,
		"run":       cp,
		"team_name": teamRow.DisplayName,
	}

	// Prefer typed TeamStageUpdated (assembled). Spirit flow already publishes
	// TeamStageCreated via publishSpiritTeamAssembled; this covers standalone
	// team runs and progress updates under the graph parent.
	ts := biz.TeamStage{
		ID:        string(agent.NewTeamStageActivityID(teamRow.ID, string(agent.RootTaskActivityIDFromCtx(ctx)))),
		TeamID:    teamRow.ID,
		TeamName:  teamRow.DisplayName,
		SessionID: spiritSID,
		Status:    biz.TeamStageStatusRunning,
		Stage:     biz.TeamStageStageAssembled,
		StartedAt: time.Now().UTC(),
		Version:   1,
	}
	r.publishEvent(ctx, biz.NewTeamStageUpdatedEvent(ts))
	r.publishEvent(ctx, biz.NewSystemNoticeEvent(spiritSID, "team_stage_assembled", "", meta))
}

// buildTeamBuilderDeps assembles the TRPCBuilderDeps from runner configuration and anchor resolution.
func (r *Runner) buildTeamBuilderDeps(ctx context.Context, sess biz.Session, run biz.TeamRunRecord, ar anchorResolution, dialogMode string) agent.TRPCBuilderDeps {
	deps := agent.TRPCBuilderDeps{
		TRPCModelCatalogDeps: agent.TRPCModelCatalogDeps{
			ModelCatalog: r.td.ReadDeps.LLM,
			AgentUC:      r.td.ReadDeps.AgentsUC,
			Agents:       r.td.ReadDeps.Agents,
			Sys:          r.td.ReadDeps.Settings,
			Sessions:     r.td.Sessions,
		},
		TRPCModelRouteDeps: agent.TRPCModelRouteDeps{
			RT:         r.td.RoundTripForSession(sess.ID),
			Provider:   ar.prov,
			Model:      ar.mod,
			DialogMode: dialogMode,
		},
		TRPCToolAssemblyDeps: agent.TRPCToolAssemblyDeps{
			ToolUC:         r.td.ReadDeps.ToolUC,
			MCPTooling:     r.td.Persist.AgentMCP,
			KanbanBridge:   r.cfg.KanbanBridge,
			ComputerUseUC:  r.cfg.ComputerUseUC,
			MediaProviders: r.td.ReadDeps.MediaProviders,
			ArtifactWriter: r.td.Persist.ArtifactUC,
		},
		TRPCMemoryKnowledgeDeps: agent.TRPCMemoryKnowledgeDeps{
			HasMemory:              r.td.Persist.Memory.Available(),
			MemoryService:          r.td.Persist.Memory.TRPC,
			MemoryLayerPorts:       r.td.Persist.Memory.MemoryLayerPorts,
			MemoryActionLogWriter:  r.td.Persist.Memory.ActionLogWriter,
			ManualCompressor:       biz.ManualCompressorFromNative(r.td.Compress),
			MemoryL2Recall:         r.td.Persist.Memory.L2Recall,
			MemoryL3Recall:         r.td.Persist.Memory.L3Recall,
			MemoryCompositeRecall:  r.td.Persist.Memory.CompositeRecall,
			MemoryPreferenceLister: r.td.Persist.Memory.PreferenceLister,
			MemoryReconsolidator:   r.td.Persist.Memory.Reconsolidator,
			AgentCaseRecaller:      r.td.Persist.Memory.AgentCaseRecaller,
			KnowledgeRetriever:     r.cfg.Knowledge.Retriever,
			KnowledgeUsecase:       r.cfg.KnowledgeUsecase,
		},
		TRPCPluginDeps: agent.TRPCPluginDeps{
			PluginManager: r.cfg.PluginManager,
		},
		TRPCSkillDeps: agent.TRPCSkillDeps{
			SkillUC:         r.td.ReadDeps.SkillUC,
			SkillDBRepo:     r.skillDBRepo,
			CodeExecFactory: r.codeExecFactory,
		},
		TRPCExtensionDeps: agent.TRPCExtensionDeps{
			Organization:     r.cfg.OrganizationUC,
			ToolResultGate:   r.cfg.ToolResultGate,
			OutboundRouter:   r.cfg.OutboundRouter,
			SubAgentService:  r.cfg.SubAgentService,
			A2AEnabled:       r.cfg.A2AEnabled,
			L0SnapshotForcer: r.td.SessionRT,
			LearningLoop:     r.td.LearningLoop,
			LG:               r.lg,
		},
	}
	if r.cfg.AwaitHookProvider != nil {
		deps.AwaitHook = r.cfg.AwaitHookProvider(ctx, sess.ID, run.ID)
	}
	return deps
}

// resolveTeamPlugins resolves the plugin list for the anchor agent.
func (r *Runner) resolveTeamPlugins(ctx context.Context, agentID string) []trpcplugin.Plugin {
	wsID := workspace.IDFromContext(ctx)
	if r.cfg.PluginManager != nil {
		return r.cfg.PluginManager.RunnerPluginsForAgent(agentID, wsID)
	}
	if r.cfg.PluginRT != nil {
		return r.cfg.PluginRT.PluginsForAgent(agentID, wsID)
	}
	return nil
}

// buildTeamRunContext constructs the execution context with deadline, await-hook,
// knowledge tools, and artifact collector. Returns a cancel func that the caller
// must defer if non-nil.
func (r *Runner) buildTeamRunContext(ctx context.Context, def Definition, builderDeps agent.TRPCBuilderDeps, input biz.TurnInput) (context.Context, context.CancelFunc, *artifactbiz.TurnCollector) {
	runCtx := ctx
	var cancel context.CancelFunc
	if dur := TurnDeadlineDuration(def); dur > 0 {
		runCtx, cancel = context.WithTimeout(ctx, dur)
	}
	if builderDeps.AwaitHook != nil {
		runCtx = serviceawaitreply.WithReplyFunc(runCtx, builderDeps.AwaitHook)
	}
	if r.cfg.Knowledge != nil {
		if r.cfg.Knowledge.Retriever != nil {
			runCtx = knowledgetool.WithRetriever(runCtx, r.cfg.Knowledge.Retriever)
		}
		if r.cfg.Knowledge.Router != nil {
			runCtx = knowledgetool.WithAdaptiveRouter(runCtx, r.cfg.Knowledge.Router)
		}
		if r.cfg.Knowledge.FederatedRetriever != nil {
			runCtx = knowledgetool.WithFederatedRetriever(runCtx, r.cfg.Knowledge.FederatedRetriever)
		}
		if r.cfg.Knowledge.Evaluator != nil {
			runCtx = knowledgetool.WithRetrievalEvaluator(runCtx, r.cfg.Knowledge.Evaluator)
		}
	}
	if len(input.Options.KnowledgeBases) > 0 {
		runCtx = knowledgetool.WithKnowledgeCollections(runCtx, input.Options.KnowledgeBases)
	}
	runCtx, turnArtCollector := artifactbiz.WithTurnCollector(runCtx)
	return runCtx, cancel, turnArtCollector
}

// buildTeamProjectMeta assembles the ProjectMeta for stream consumption.
func (r *Runner) buildTeamProjectMeta(ctx context.Context, sess biz.Session, run biz.TeamRunRecord, teamRow biz.Team, def Definition, ar anchorResolution, memberKeys []string, content, traceID string) agent.ProjectMeta {
	memberKeySet := make(map[string]struct{}, len(memberKeys))
	for _, k := range memberKeys {
		memberKeySet[k] = struct{}{}
	}
	contextWin := sessctx.ResolveContextWindowTokens(ctx, r.teamLLMCatalog(), sess, ar.agent, ar.prov, ar.mod)
	nodeIDToAgentKey := r.buildNodeIDToAgentKey(ctx, def)
	// Phase 1a fix: propagate session tree hierarchy so team activities are
	// correctly attributed to the originating spirit session. sess is the team
	// session created by SpiritTeamAssembler with ParentSessionID/RootSessionID
	// pointing to the spirit session. SpiritSessionID for a team session equals
	// its RootSessionID (the spirit session that initiated the tree).
	spiritSessionID := deriveSpiritSessionID(sess)
	rootTaskID := string(agent.RootTaskActivityIDFromCtx(ctx))
	requestID := rootTaskID
	if requestID == "" {
		requestID = run.ID
	}
	teamStageID := string(agent.NewTeamStageActivityID(teamRow.ID, rootTaskID))
	return agent.ProjectMeta{
		SessionID:    sess.ID,
		RequestID:    requestID,
		InvocationID: run.ID,
		RunID:        run.ID,
		TraceID:      traceID,
		TeamID:       teamRow.ID,
		// 2026-07-05 P1 #7 修复：填充 TeamStageID（派生自 teamRow.ID），
		// 让 V2ProjectMetaFromV1 能正确透传到 Turn.TeamStageID。
		// TeamStageID = NewTeamStageActivityID(teamRow.ID)，
		// 与 service/spirit_team.go 的 publishV2TeamRunAndMemberSessions 中
		// 创建 TeamStage 时使用的 ID 一致。
		TeamStageID:      teamStageID,
		AgentID:          ar.agent.AgentKey,
		AgentDisplayName: ar.agent.DisplayName,
		MemberAgentKeys:  memberKeySet,
		ContextWindow:    contextWin,
		Source:           event.EnvelopeSourceFromContext(ctx),
		TaskContent:      content,
		// Phase 1a: session tree hierarchy for cross-session aggregation.
		SpiritSessionID: spiritSessionID,
		ParentSessionID: sess.ParentSessionID,
		RootSessionID:   sess.RootSessionID,
		// ParentActivityID: set to the deterministic session activity ID so that
		// the orchestrator's thinking/action/reply activities are nested under
		// the session activity (kind=session) in the spirit session's tree.
		// Uses the same deterministic ID as publishTeamStepActivity, ensuring
		// the frontend can link child activities to the correct parent.
		ParentActivityID: string(agent.NewMemberSessionActivityID(agent.NewTeamRunV2ID(teamStageID), ar.agent.AgentKey)),
		NodeIDToAgentKey: nodeIDToAgentKey,
	}
}

// buildNodeIDToAgentKey builds the mapping from graph node IDs (e.g. "member-1")
// to member agent keys (e.g. "spirit-worker-a"). Used by V2Projector to
// attribute Steps to the correct member agent in Graph mode.
//
// Node ID generation mirrors graph_compile.go:memberNodeID, using the same
// sortOrder/index logic. Agent key resolution uses the same lookupAgent path
// as runner_team_compiler.go:43-49 (CompileAgentKey callback).
func (r *Runner) buildNodeIDToAgentKey(ctx context.Context, def Definition) map[string]string {
	members := EnabledMembers(def)
	if len(members) == 0 {
		return nil
	}
	out := make(map[string]string, len(members))
	for i, m := range members {
		agentID := strings.TrimSpace(m.AgentID)
		if agentID == "" {
			continue
		}
		ag, gerr := r.lookupAgent(ctx, agentID)
		if gerr != nil {
			r.lg.Warn("buildNodeIDToAgentKey: lookup agent failed",
				loggateway.StepID("team.project_meta.node_id_map"),
				loggateway.Str("agent_id", agentID),
				loggateway.Err(gerr))
			continue
		}
		agentKey := strings.TrimSpace(ag.AgentKey)
		if agentKey == "" {
			continue
		}
		out[memberNodeID(m, i)] = agentKey
	}
	return out
}

// assistantBuildResult holds the constructed assistant message and token counts.
type assistantBuildResult struct {
	msg           biz.ChatMessage
	promptTok     int
	completionTok int
}

// buildAssistantMessageFromResult constructs the assistant ChatMessage from stream result,
// merging reasoning, display flags, and artifact refs into options JSON.
func (r *Runner) buildAssistantMessageFromResult(
	result agent.EventStreamResult,
	ar anchorResolution,
	sess biz.Session,
	modOpt string,
	turnArtCollector *artifactbiz.TurnCollector,
	content string,
) (assistantBuildResult, error) {
	reasoningText := strings.TrimSpace(result.Reasoning.String())
	displayMarkdown, reasoningAsDisplay := agent.DisplayMarkdownFromStream(result)
	promptTok, completionTok := agent.EstimateTokensIfMissing(result.PromptTok, result.CompletionTok, content, displayMarkdown)

	assistantOptsStr, err := agent.AssistantOptionsJSON(ar.agent, &agent.TeamMemberAnchor{
		AgentID: ar.agent.ID,
		Name:    strutil.FirstNonEmpty(ar.agent.DisplayName, ar.agent.AgentKey),
		Role:    ar.member.Role,
	})
	if err != nil {
		return assistantBuildResult{}, err
	}
	if reasoningText != "" {
		if assistantOptsStr, err = agent.MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, reasoningText); err != nil {
			return assistantBuildResult{}, err
		}
	}
	if reasoningAsDisplay {
		if assistantOptsStr, err = agent.MergeReasoningAsDisplayFlag(assistantOptsStr, true); err != nil {
			return assistantBuildResult{}, err
		}
	}
	assistantAttN := 0
	if turnArtCollector != nil {
		if merged, merr := artifactbiz.MergeRefsIntoOptionsJSON(assistantOptsStr, turnArtCollector.Refs()); merr != nil {
			return assistantBuildResult{}, merr
		} else {
			assistantOptsStr = merged
			assistantAttN = len(turnArtCollector.Refs())
		}
	}

	msg := biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sess.ID,
		Role:             "assistant",
		ContentMarkdown:  displayMarkdown,
		ModelName:        strutil.FirstNonEmpty(modOpt, sess.DefaultModel, ar.agent.Model),
		Status:           biz.TeamMemberStepStatusOK,
		OptionsJSON:      assistantOptsStr,
		CreatedAt:        agent.RFC3339Now(),
		TokenIn:          promptTok,
		TokenOut:         completionTok,
		AttachmentsCount: assistantAttN,
	}
	return assistantBuildResult{msg: msg, promptTok: promptTok, completionTok: completionTok}, nil
}

// validateAssistantOutput checks that the stream produced usable output,
// returning a descriptive error if not.
func validateAssistantOutput(result agent.EventStreamResult) error {
	displayMarkdown, _ := agent.DisplayMarkdownFromStream(result)
	if displayMarkdown != "" {
		return nil
	}
	fallback := "The team workflow produced no usable assistant reply. This may indicate a configuration issue."
	if result.HasError {
		fallback = fmt.Sprintf("Team AI service error: %s. Please check your configuration or try again later.", result.LastError)
	} else if result.HasContent {
		fallback = "The team workflow completed but produced no text output. This may indicate a configuration issue with the model."
	}
	return apierror.Internal("CHAT_TEAM_NATIVE", fallback)
}

// finishTeamRunWithError is a helper that sets turnStatus to error, calls finishRunErr,
// and optionally rolls back the runner session. Returns the error for convenient use in error paths.
func (r *Runner) finishTeamRunWithError(ctx context.Context, run *biz.TeamRunRecord, t0 time.Time, errMsg string, turnStatus *string, rollbackFn func()) {
	*turnStatus = biz.TeamMemberStepStatusError
	r.finishRunErr(ctx, run, t0, errMsg)
	if rollbackFn != nil {
		rollbackFn()
	}
}

// logTeamRunError logs an error to the team emitter if present.
func logTeamRunError(emitter *event.TraceEmitter, step, msg, mode string) {
	if emitter == nil {
		return
	}
	emitter.LogError(step, msg, event.P("mode", mode))
}

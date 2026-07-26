package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/event"
	"aranea-agents/internal/telemetry/turntrace"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"

	"github.com/google/uuid"
)

// turnAdmissionResult holds the outputs of the ADMISSION phase.
// Stability:internal
type turnAdmissionResult struct {
	runID      string
	dialogMode string
	provider   string
	model      string
	durableCtx durableResumeTurnCtx
}

// turnExecuteResult holds the outputs of the EXECUTE phase.
// Stability:internal
type turnExecuteResult struct {
	userMsg             biz.ChatMessage
	userMsgPersisted    bool
	result              chatagent.EventStreamResult
	resultPromptTok     int
	resultCompletionTok int
	sessionRunID        string
	turnArtCollector    *artifactbiz.TurnCollector
}

// turnPersistResult holds the outputs of the PERSIST phase.
// Stability:internal
type turnPersistResult struct {
	assistantMsg  biz.ChatMessage
	promptTok     int
	completionTok int
	cachedTok     int
}

// turnPipeline encapsulates the core ADMISSION → EXECUTE → PERSIST turn phases.
// It embeds *ChatOrchestrator so that implementation helpers (run status, event
// publishing, stream consumption, etc.) remain accessible without duplication.
// Stability:internal
type turnPipeline struct {
	*ChatOrchestrator
}

// pipeline returns the turn pipeline for this orchestrator.
func (o *ChatOrchestrator) pipeline() *turnPipeline {
	return &turnPipeline{ChatOrchestrator: o}
}

// ────────────────────────────────────────────────────────────
// ADMISSION phase
// ────────────────────────────────────────────────────────────

// admitTurn validates the turn request and generates run metadata.
// Stability:internal
func (p *turnPipeline) admitTurn(
	ctx context.Context,
	sess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
	dialogMode, prov, mod string,
) (turnAdmissionResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if ak := strings.TrimSpace(input.AgentKey); ak != "" && !strings.EqualFold(ak, ag.AgentKey) {
		te := TurnError(TurnErrAgentForbidden, "")
		p.publishTurnFailure(sessionID, "", "chat-service", te, "")
		return turnAdmissionResult{}, te
	}

	runID := uuid.NewString()
	durableCtx := durableResumeTurnCtxFrom(ctx, runID, dialogMode, prov, mod)
	runID = durableCtx.runID
	dialogMode = durableCtx.dialogMode
	prov = durableCtx.provider
	mod = durableCtx.model
	if durableCtx.active {
		if comp, ok := p.td().Compress.(biz.DurableTurnCompressor); ok {
			if err := comp.BeforeDurableTurn(ctx, sessionID, ag); err != nil {
				p.lg().Warn("BeforeDurableTurn failed", loggateway.StepID("chat.turn.before_durable"), loggateway.Err(err))
			}
		}
	}
	return turnAdmissionResult{
		runID:      runID,
		dialogMode: dialogMode,
		provider:   prov,
		model:      mod,
		durableCtx: durableCtx,
	}, nil
}

// ────────────────────────────────────────────────────────────
// EXECUTE phase (orchestrator + sub-methods)
// ────────────────────────────────────────────────────────────

// executeTurn orchestrates the EXECUTE phase: user options, intent pass,
// user message persistence, LLM invocation, and stream consumption.
// Stability:internal
func (p *turnPipeline) executeTurn(
	ctx context.Context,
	sess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	traceBridge *turntrace.Bridge,
	deps chatagent.TRPCBuilderDeps,
	runner trpcrunner.Runner,
	attachmentRefs []artifactbiz.Ref,
	intentRunOpts []trpcagent.RunOption,
	turnStart time.Time,
) (turnExecuteResult, error) {
	// Step 1: Build user options (with attachments, no intent pass — it ran in parallel with BUILD)
	userOpts, err := p.prepareTurnUserOptions(ctx, input, ag, admit, emitter, attachmentRefs, sess)
	if err != nil {
		return turnExecuteResult{}, err
	}
	attN := len(attachmentRefs)

	// Step 1.5: 巨型用户输入落地 blob（单轮超限治理）；后续持久化与 LLM 调用均使用 preview。
	input.Content = p.gateTurnUserInput(ctx, strings.TrimSpace(input.SessionID), input.Content)

	// Step 2: Persist user message
	userMsg, userMsgPersisted, err := p.persistTurnUserMessage(ctx, input, ag, admit, emitter, userOpts, attN)
	if err != nil {
		return turnExecuteResult{}, err
	}

	// Step 3: Session run lifecycle + run options + LLM call + stream
	return p.invokeTurnLLMAndStream(ctx, sess, input, ag, admit, emitter, traceBridge, deps, runner,
		userMsg, userMsgPersisted, userOpts, intentRunOpts, turnStart)
}

// gateTurnUserInput 对超阈值的用户输入落地 blob 并返回 preview。
// 未超限 / gate 未配置 / 落地失败时返回原文（不阻断对话）。幂等键：
// messageID 取 RootTaskActivityID（与持久化的用户消息 ID 一致），重试复用 replacement。
// Stability:internal
func (p *turnPipeline) gateTurnUserInput(ctx context.Context, sessionID, content string) string {
	gate := p.rt().ToolResultGate
	if gate == nil || utf8.RuneCountInString(content) <= biz.ToolResultSizeThreshold {
		return content
	}
	msgID := string(chatagent.RootTaskActivityIDFromCtx(ctx))
	if msgID == "" {
		msgID = uuid.NewString()
	}
	res, err := gate.CheckUserInput(ctx, sessionID, msgID, biz.ToolResultSourceUserInput, content)
	if err != nil {
		p.lg().Warn("用户输入落地 blob 失败，使用原文继续",
			loggateway.StepID("chat.turn.user_input_gate"),
			loggateway.Err(err))
		return content
	}
	if !res.DidPersist {
		return content
	}
	return res.PreviewText
}

// ────────────────────────────────────────────────────────────
// PERSIST phase
// ────────────────────────────────────────────────────────────

// persistTurn handles timeout degradation, empty reply detection, and message persistence.
// Stability:internal
func (p *turnPipeline) persistTurn(
	ctx *context.Context,
	sess biz.Session,
	ag biz.Agent,
	admit turnAdmissionResult,
	execResult turnExecuteResult,
	emitter *event.TraceEmitter,
	turnStart time.Time,
	turnStatus *string,
	turnErr *error,
	turnErrMsg *string,
) (turnPersistResult, error) {
	sessionID := strings.TrimSpace(execResult.userMsg.SessionID)
	result := execResult.result

	// Timeout degradation: has content but turn deadline actually exceeded.
	// ctx.Err() != nil alone is insufficient — stream completion, HTTP request
	// end, or framework cancel all set ctx.Err without a real timeout. Only
	// DeadlineExceeded indicates the turn timeout actually fired.
	if errors.Is((*ctx).Err(), context.DeadlineExceeded) && result.HasContent {
		*turnStatus = "timeout_degraded"
		emitter.LogWarn("chat.turn.timeout_with_reply", "对话超时但模型已输出，保存回复", "", event.P("timeout", p.turnTimeout().String()), event.P("reply_len", result.Reply.Len()))
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer bgCancel()
		*ctx = bgCtx
	}

	// Empty reply detection
	displayMarkdown, reasoningAsDisplay := chatagent.DisplayMarkdownFromStream(result)
	if displayMarkdown == "" {
		return turnPersistResult{}, p.handleEmptyReply(*ctx, ag, admit, emitter, result, turnStart, turnStatus, turnErr, turnErrMsg, sessionID)
	}

	// Immediate fact extraction: detect <fact> tags in agent response and persist to memory_fact.
	// This bridges the async gap between conversation and Sleep-time consolidation.
	// The tags are removed from the display text sent to the user.
	cleanDisplay, facts := biz.ParseFactMarks(displayMarkdown)
	if len(facts) > 0 {
		p.lg().Info("即时事实提取",
			loggateway.StepID("chat.immediate_fact"),
			loggateway.SessionID(sessionID),
			loggateway.Int("fact_count", len(facts)),
			loggateway.Str("facts_preview", func() string {
				if len(cleanDisplay) > 200 {
					return cleanDisplay[:200] + "..."
				}
				return cleanDisplay
			}()))
		// Use the cleaned display text (without fact tags) for user display
		displayMarkdown = cleanDisplay
		// Fire-and-forget: write facts asynchronously
		if fw := p.factWriter(); fw != nil {
			userID := strings.TrimSpace(sess.UserID)
			fw.WriteFacts(*ctx, sessionID, ag.ID, userID, execResult.userMsg.ID, facts)
		}
	}

	// Pass user content as inputPreview so prompt token estimation works when the
	// model omits usage. Previously passed "" which suppressed estimation and caused
	// the buggy fallback to estimate prompt from output (prompt≈completion tokens).
	promptTok, completionTok := chatagent.EstimateTokensIfMissing(execResult.resultPromptTok, execResult.resultCompletionTok, strings.TrimSpace(execResult.userMsg.ContentMarkdown), displayMarkdown)

	// usage_source observability: log when tokens came from estimation or were missing.
	// Helps diagnose TECH-DEBT(usage-source): framework suppresses usage on stream error
	// (pkg/trpc-agent-go/model/openai/openai.go:emitStreamingFinalResponse), leaving
	// UsageSource="" and tokens=0 until EstimateTokensIfMissing fills them.
	usageSource := execResult.result.UsageSource
	if promptTok != execResult.resultPromptTok || completionTok != execResult.resultCompletionTok {
		usageSource = "estimated"
	}
	if usageSource == "" || usageSource == "estimated" {
		emitter.LogDone("chat.turn.usage_source",
			"token 使用来源追踪",
			event.P("usage_source", usageSource),
			event.P("prompt_tok", promptTok),
			event.P("completion_tok", completionTok),
			event.P("has_error", execResult.result.HasError))
	}

	// Build and persist assistant message
	assistantMsg, err := p.buildAndPersistAssistantMessage(*ctx, ag, admit, execResult, emitter, displayMarkdown, reasoningAsDisplay, promptTok, completionTok, turnStatus, turnErr, turnErrMsg)
	if err != nil {
		return turnPersistResult{}, err
	}

	emitter.LogDone("chat.assistant_msg_persist", "助手消息已持久化", event.P("reply_len", len(displayMarkdown)))
	p.patchSessionContextUsage(*ctx, sessionID, sess, ag, admit.provider, admit.model, promptTok, completionTok)

	return turnPersistResult{
		assistantMsg:  assistantMsg,
		promptTok:     promptTok,
		completionTok: completionTok,
		cachedTok:     execResult.result.CachedTok,
	}, nil
}

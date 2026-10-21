// TECH-DEBT(COG): phase methods extracted to chat_orchestrator_turn_phases.go;
// dispatch methods to chat_orchestrator_turn_dispatch.go;
// API methods to chat_orchestrator_turn_api.go;
// metrics methods to chat_orchestrator_turn_metrics.go
package service

import (
	"context"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/turn"
	"aranea-agents/internal/telemetry/turntrace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"
)

// RunNativeAgentTurnFromInput executes a full agent/team turn from a biz-level TurnInput.
func (o *ChatOrchestrator) RunNativeAgentTurnFromInput(ctx context.Context, input biz.TurnInput) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	result, err := o.RunNativeAgentTurnWithOutcome(ctx, input)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	return result.UserMsg, result.AssistantMsg, nil
}

// RunNativeAgentTurnWithOutcome classifies completed vs queued turns explicitly (P1).
func (o *ChatOrchestrator) RunNativeAgentTurnWithOutcome(ctx context.Context, input biz.TurnInput) (biz.TurnResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)
	if sessionID == "" || content == "" {
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, apierror.BadRequest(apierror.DomainChatNative, "session_id and content are required")
	}

	// Ensure spirit_trace_id is present in context for all Spirit orchestration paths.
	// If not already set (e.g., by TaskPlanner), generate one at the turn entry point.
	if _, ok := biz.SpiritTraceIDFromContext(ctx); !ok {
		ctx = biz.ContextWithSpiritTraceID(ctx, biz.NewSpiritTraceID())
	}

	if ep := strings.TrimSpace(string(input.EntryConfig.EntryPoint)); ep != "" {
		ctx = event.WithEnvelopeSource(ctx, ep)
	}

	// Unify trace id for the whole turn: everything downstream (entry flow
	// events, turn span emitter, team/graph runs) shares one trace id.
	ctx, _ = turntrace.EnsureTraceID(ctx)
	flow := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionID,
		Domain:    event.TraceDomainChat,
		LG:        o.lg(),
		Infra:     event.NewInfraFromBus(o.core.TD.Pipeline.MonitorEventBus),
	})
	flow.LogStart("chat.receive", "收到用户消息", event.P("content_len", len(content)))

	hasActive := o.runs.HasActive(sessionID)
	flow.Log("chat.active_check", event.FlowPhaseDone, "检查活跃运行", event.P("has_active", hasActive))

	// P1 FR-7（89-session-concurrency-hardening）：会话空闲但存在"持久层恢复
	// 且未排空"的 pending 行时，新消息必须先强制入队、再触发整体 FIFO 排空
	// ——直行 turn 会插队到恢复行之前（重启后 FIFO 反转）。返回形态与准入排
	// 队一致（ErrTurnMessageQueued 由调用方按"已受理入队"处理）。
	if !hasActive && input.AllowPendingQueue() && o.chatUC.PendingQueueHasRestored(sessionID) {
		pendingID, rejectReason, ferr := o.chatUC.ForceEnqueueFollowup(sessionID, content)
		if ferr != nil {
			return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, ferr
		}
		if rejectReason != "" {
			return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, enqueueRejectError(rejectReason)
		}
		flow.Log("chat.restored_enqueue", event.FlowPhaseDone, "恢复队列强制入队", event.P("pending_id", pendingID))
		o.triggerPendingDrainFromIdle(ctx, sessionID)
		return biz.TurnResult{Outcome: biz.TurnOutcomeQueued, PendingID: pendingID}, ErrTurnMessageQueued
	}

	contextPressure := o.sessionContextPressure(ctx, input)
	if verdict, handled := o.checkTurnAdmission(input, hasActive, contextPressure); handled {
		return turnResultFromAdmissionVerdict(verdict)
	}

	userMsg, assistantMsg, err := o.runNativeAgentTurnBody(ctx, input, flow)
	if err != nil {
		if isTurnMessageQueued(err) {
			return biz.TurnResult{
				Outcome:   biz.TurnOutcomeQueued,
				PendingID: o.LastPendingMessageID(sessionID),
			}, err
		}
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed, UserMsg: userMsg}, err
	}
	return biz.TurnResult{
		Outcome:      biz.TurnOutcomeCompleted,
		UserMsg:      userMsg,
		AssistantMsg: assistantMsg,
		Reply:        assistantMsg.ContentMarkdown,
	}, nil
}

// TECH-DEBT(BA4): checkTurnAdmission still encodes the reject_busy_queue
// business rule (IngressQueue + intent check) at the service layer. Moving
// this into TurnAdmissionUsecase requires expanding the admission verdict
// model and touching multiple callers (chat_native.go, chat_orchestrator_turn.go).
// Deferred to avoid cross-module churn in this P1 pass.
func (o *ChatOrchestrator) checkTurnAdmission(input biz.TurnInput, hasActive, contextPressure bool) (turn.AdmissionVerdict, bool) {
	if o == nil || o.admitGate() == nil || !hasActive {
		return turn.AdmissionVerdict{}, false
	}
	hasRunner := o.HasActiveRunner(input.SessionID)
	policy := ingressPolicyFromTurnInput(input, true, hasRunner, contextPressure)
	recordIngressIntentMetric(policy.Intent)
	if policy.Decision == IngressQueue && policy.Intent == "reject_busy_queue" {
		return turn.AdmissionVerdict{Action: turn.AdmissionRejectBusy}, true
	}
	verdict := o.admitGate().Check(input)
	switch verdict.Action {
	case turn.AdmissionProceed:
		return verdict, false
	default:
		return verdict, true
	}
}

// runNativeAgentTurnBody executes agent/team turn after admission checks.
func (o *ChatOrchestrator) runNativeAgentTurnBody(ctx context.Context, input biz.TurnInput, flow *event.TraceEmitter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	sessionID := strings.TrimSpace(input.SessionID)

	// P1 FR-6（89-session-concurrency-hardening / M-1）：车道等待移出会话锁
	// 临界区。旧顺序 lock → Sessions.Get → AcquireTurnLane 使被限流 turn
	// 持锁阻塞于车道，同会话后续 turn 被连带拖死在不可取消的互斥上。新顺序：
	// 会话快照（取 OwnerType 定道）→ 车道等待（ctx 可取消，取消即终止）→
	// 锁内只做快速复核与状态拼装。快照前置读的陈旧性可接受：锁内消费同一份
	// 快照（agent_id/归属校验/车道分类），状态修改仍全部在锁内串行。
	sessGetStart := time.Now()
	sess, err := o.td().Sessions.Get(ctx, sessionID)
	if err != nil {
		flow.LogError("chat.session_fetch", "获取会话失败", event.P("error", err.Error()))
		o.lg().With(loggateway.SessionID(sessionID)).Info("runNativeAgentTurnBody: Sessions.Get 失败",
			loggateway.StepID("chat.session_get_fail"), loggateway.Err(err))
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return biz.ChatMessage{}, biz.ChatMessage{}, apierror.NotFound(apierror.DomainSession, "session not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: Sessions.Get",
		loggateway.StepID("chat.session_get"),
		loggateway.Any("elapsed_ms", time.Since(sessGetStart).Milliseconds()))
	flow.LogDone("chat.session_fetch", "会话已获取", event.P("owner_type", sess.OwnerType), event.P("agent_id", sess.AgentID), event.P("team_id", sess.TeamID))

	// 车道等待（锁外）：ctx 取消即终止 turn，不放行无槽位执行（FR-6）。
	releaseLane, laneOK := rt.AcquireTurnLane(ctx, input, sess.OwnerType)
	if !laneOK {
		return biz.ChatMessage{}, biz.ChatMessage{}, ctx.Err()
	}
	defer releaseLane()

	lockStart := time.Now()
	unlock, err := o.lockSessionCtx(ctx, sessionID)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: lockSession",
		loggateway.StepID("chat.lock_session"),
		loggateway.Any("elapsed_ms", time.Since(lockStart).Milliseconds()))
	// TOCTOU 复查：准入阶段的锁外 HasActive 检查与锁获取之间存在窗口，并发
	// turn 可能已在该窗口内注册活跃运行（StoreCancelable）。到达此处的 turn 均
	// 以 hasActive=false 放行，锁内再发现活跃运行只可能是竞态 —— 拒绝为
	// CHAT_TURN_BUSY，防止双跑与取消函数被覆盖。
	if o.runs.HasActive(sessionID) {
		// P0 FR-4：durable resume 赢下 DB claim 后先登记占位（见
		// ResumeDurableSessionRun），claim→turn 注册窗口内对其它 turn 呈现
		// busy。携 DurableResume 标记的本 resume turn 在锁内采用该占位——
		// 后续 StoreCancelable 的 UpdateOrStore 原地升级同一槽位条目（
		// run_registry.go StoreCancelable 保留既有 runner 字段语义）。
		// 非占位占用、或无标记 turn，照常 BUSY 拒绝。
		// 锁内 TOCTOU 安全：从本检查到 StoreCancelable 全程持有会话锁，
		// 占位不会被第三方替换。
		if _, adopt := event.DurableResumeFromContext(ctx); !adopt || !o.runs.IsPlaceholderOnly(sessionID) {
			unlock()
			return biz.ChatMessage{}, biz.ChatMessage{}, turnBusyError()
		}
	}
	if !inPendingLoop(ctx) && o.chatUC != nil {
		if injects := o.chatUC.ConsumeLeadingInjects(sessionID); len(injects) > 0 {
			input.Content = biz.MergeInjectContext(injects, input.Content)
		}
	}

	// P0-01 fix: verify the authenticated user owns this session.
	// Skip for system-initiated turns (default_user) and shared sessions
	// (empty UserID) to avoid breaking cron/channel/durable entry points.
	authUserID := ctxuser.FromContext(ctx)
	if authUserID != ctxuser.DefaultUserID && sess.UserID != "" && authUserID != sess.UserID {
		unlock()
		flow.LogError("chat.session_ownership", "会话归属校验失败",
			event.P("auth_user", authUserID), event.P("session_user", sess.UserID))
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.Forbidden(apierror.DomainChatNative,
			"session does not belong to the authenticated user")
	}

	// 澄清等待态自由回复：cache 命中或（重启/他副本）从信封重建。
	// 放在锁内（快照读取之后），用已加载的会话状态门闩，避免每个 turn 额外 ListSteps。
	input, intentArt := o.resolveClarificationFreeTextHint(ctx, input, &sess)
	if intentArt != nil {
		ctx = intent.WithArtifact(ctx, intentArt)
	}

	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		return o.executeTeamTurnViaHooks(ctx, sess, input, flow, unlock)
	}

	if rtid := strings.TrimSpace(input.TeamID); rtid != "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.Forbidden(apierror.DomainChatTeamNative, "team_id is only valid for team sessions")
	}

	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.BadRequest(apierror.DomainChatNative, "session has no agent_id")
	}
	hydrateStart := time.Now()
	ag, err := o.hydratedAgent(ctx, agentID)
	if err != nil {
		unlock()
		flow.LogError("chat.agent_hydrate", "加载Agent配置失败", event.P("agent_id", agentID), event.P("error", err.Error()))
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return biz.ChatMessage{}, biz.ChatMessage{}, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: hydratedAgent",
		loggateway.StepID("chat.agent_hydrate"),
		loggateway.Any("elapsed_ms", time.Since(hydrateStart).Milliseconds()),
		loggateway.Any("agent_key", ag.AgentKey))
	flow.LogDone("chat.agent_hydrate", "Agent配置已加载", event.P("agent_key", ag.AgentKey), event.P("provider", ag.Provider), event.P("model", ag.Model))
	if ov, ok := biz.EvalRunOverrideFrom(ctx); ok {
		applyEvalOverrideToAgent(&ag, ov)
	}
	if err := o.admission().EnforceChatTurnQuotas(ctx, agentID, chatagent.UserIDFromCtx(ctx)); err != nil {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	dialogMode := strings.TrimSpace(input.Options.DialogMode)
	prov := strings.TrimSpace(input.Options.Provider)
	mod := strings.TrimSpace(input.Options.Model)
	dialogMode = strutil.FirstNonEmpty(dialogMode, sess.DialogMode, "default")
	prov = strutil.FirstNonEmpty(prov, sess.DefaultProvider, ag.Provider)
	mod = strutil.FirstNonEmpty(mod, sess.DefaultModel, ag.Model)
	prov, mod = o.resolveProviderModelFallback(ctx, prov, mod)
	o.syncSessionProviderModel(ctx, sessionID, sess, prov, mod)
	flow.LogDone("chat.provider_resolve", "Provider/Model已解析", event.P("provider", prov), event.P("model", mod), event.P("dialog_mode", dialogMode))

	attN := len(artifactbiz.NormalizeAttachmentIDs(input.Options.AttachmentIDs))

	flow.LogStart("chat.turn.enter", "进入Agent Turn执行", event.P("dialog_mode", dialogMode), event.P("provider", prov), event.P("model", mod), event.P("attachments", attN))

	agentRunID := uuid.NewString()
	turnCtx, turnCancel := context.WithCancel(ctx)
	o.runs.StoreCancelable(sessionID, agentRunID, turnCancel)
	// P0 FR-3：注册点 defer 兜底 panic 泄漏（M-2）。StoreCancelable 与 EXECUTE
	// 收尾 defer（trpc.go）之间存在 admit/状态转换/BUILD 前段等执行窗口，panic
	// 走 defer 链而非 err 分支，注册项残留会让会话永久 busy。CAS 语义：BUILD
	// 阶段 runID 升级为 admit.runID 后此 Finish(agentRunID) 失配跳过，由
	// EXECUTE defer 的 Finish(runID) 收尾；正常/err 路径已清理时幂等 no-op。
	// CAS 销定 runID（UUID 无重放）规避 ABA——本 defer 晚于自身收尾执行，
	// 并发新 turn 即使已占槽，runID 也必不同。
	defer func() { o.runs.Finish(sessionID, agentRunID); turnCancel() }()
	unlock()
	userMsg, assistantMsg, err = o.runSingleAgentViaTRPC(turnCtx, sess, input, ag, dialogMode, prov, mod)
	if err != nil {
		// 早退兜底：admitTurn 失败（如 agent 越权 FORBIDDEN）/附件预检失败等
		// 路径在 EXECUTE 收尾 defer 注册前返回，注册项（agentRunID）残留会让
		// 该会话后续 turn 永久被准入层误判 CHAT_TURN_BUSY（2026-08-20 活体实证）。
		// CAS 语义：正常路径 BUILD 阶段 StoreRunner 已把 runID 换成 admit.runID，
		// 此 Finish(agentRunID) 失配跳过，由 EXECUTE defer 的 Finish(runID) 收尾。
		o.runs.Finish(sessionID, agentRunID)
		turnCancel()
		// P0 FR-2：早退失败同样负责触发 pending 队列排空——否则此刻队列内
		// 消息永久搁浅（H-1）。守卫与 EXECUTE 收尾一致：排空循环内迭代的
		// 失败交由循环自身接管，不递归触发新排水循环。
		if !inPendingLoop(ctx) {
			o.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod,
				string(chatagent.RootTaskActivityIDFromCtx(ctx)))
		}
	}
	return userMsg, assistantMsg, err
}

func (o *ChatOrchestrator) resolveProviderModelFallback(ctx context.Context, prov, mod string) (string, string) {
	// BA4: business rule (RefineLLM → LLM catalog fallback) lives in biz layer.
	resolvedProv, resolvedMod, _ := o.chatUC.ResolveProviderModel(ctx, prov, mod)
	return resolvedProv, resolvedMod
}

func (o *ChatOrchestrator) syncSessionProviderModel(ctx context.Context, sessionID string, sess biz.Session, prov, mod string) {
	// BA4: business rule (sync session defaults when resolved values differ)
	// lives in biz layer. Error is logged inside the biz method.
	if err := o.chatUC.SyncSessionProviderModel(ctx, sessionID, sess, prov, mod); err != nil {
		o.lg().Debug("SyncSessionProviderModel failed",
			loggateway.StepID("chat_orchestrator.sync_provider_model"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
	}
}

// hydratedAgent loads and returns an Agent by ID.
func (o *ChatOrchestrator) hydratedAgent(ctx context.Context, agentID string) (biz.Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return biz.Agent{}, apierror.BadRequest(apierror.DomainChatNative, "agent id is required")
	}
	if o.td().ReadDeps.AgentsUC != nil {
		// P3-1 (2026-10-19): the build/turn path consumes settings + files only,
		// so prefer GetForBuild (skips the display-only extras query) when the
		// concrete usecase is present; narrow test stubs fall back to Get.
		if forBuild, ok := o.td().ReadDeps.AgentsUC.(interface {
			GetForBuild(ctx context.Context, id string) (biz.Agent, error)
		}); ok {
			return forBuild.GetForBuild(ctx, agentID)
		}
		return o.td().ReadDeps.AgentsUC.Get(ctx, agentID)
	}
	if o.td().ReadDeps.Agents == nil {
		return biz.Agent{}, apierror.Internal(apierror.DomainChatNative, "agent repository not configured")
	}
	return o.td().ReadDeps.Agents.GetAgentByID(ctx, agentID)
}

// resolveRootTaskActivityID 决定本 turn 写入 ctx 的根 Task Activity ID。
//
// 不变量：RootTaskActivityID = 本 turn 所属根 Task 的 ID。
//   - 根 turn（ParentTaskID 为空）：预生成新 ID，使 plan_and_execute →
//     PublishV2Board 能在 task.created 落库前把 plan board 关联到未来 Task。
//   - 续跑 turn（synthesis/澄清续答/断点恢复，ParentTaskID 非空）：必须继承父
//     Task ID。否则本 turn 内 plan_and_execute 产出的 plan_board/team_stage/
//     member_session/turn/step 会挂到 tasks_v2 中不存在的"幽灵任务"ID，前端
//     按真实 task_id 水合时整棵子树丢失（刷新后成员历史消失）。
func resolveRootTaskActivityID(input biz.TurnInput) chatagent.RootTaskActivityID {
	if pt := strings.TrimSpace(input.ParentTaskID); pt != "" {
		return chatagent.RootTaskActivityID(pt)
	}
	// Align optimistic FE pending-user-<uuid> with the persisted Task ID so
	// upsertTask merges in place (no session-wide wipe / dual cards).
	if rid := strings.TrimSpace(input.RequestID); rid != "" {
		return chatagent.RootTaskActivityID(rid)
	}
	return chatagent.RootTaskActivityID(uuid.NewString())
}

// preAssignedMessageIDKey 是 submit 同步 ACK 预分配消息 ID 的 ctx 键（SP-1e）。
// submitChatMessageAsync 在异步执行前生成 UUID 并注入 bgCtx，
// runSingleAgentViaTRPC 优先复用为 RootTaskActivityID，保证 HTTP ACK 的
// messageId/turnId 与最终持久化的用户消息 ID（userMsg.ID == userMsg.TurnID）
// 一致。刻意独立于 RootTaskActivityID ctx 键：pending-queue dequeue 路径的
// loopCtx 携带父 turn 的 RootTaskActivityID，若复用同键会让出队消息继承父 ID
// 撞 tasks_v2/messages 主键。
type preAssignedMessageIDKey struct{}

func contextWithPreAssignedMessageID(ctx context.Context, id string) context.Context {
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, preAssignedMessageIDKey{}, id)
}

func preAssignedMessageIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(preAssignedMessageIDKey{}).(string)
	return strings.TrimSpace(v)
}

// runSingleAgentViaTRPC runs a single agent turn via the trpc-agent-go framework.

// ── ADMISSION ──

// ── EARLY ACK ──
// Signal "running" to the frontend immediately after admission so the
// client-side 30s Turn ACK timeout is cleared before the BUILD phase.
// Previously this was sent after BUILD, which could take 2-15s on cache
// miss, causing the frontend to time out.

// ── BUILD (inline — defer interactions prevent extraction) ──

// 上下文预算台账（29-token §9.6 任务 0.1）：per-request 收集器挂载在 turn
// ctx 上，随 runCtx/llmCtx 传入 runner → BeforeModel 注入 hook 计量；turn 末
// 由下方 defer 中的 recordTurnUsage 出口读回发 chat.context_budget 进程日志。

// P1-7: Start the Wire-injected run heartbeat emitter so the frontend
// can detect stale runs within 30s. The stop function is invoked in the
// defer below to avoid goroutine leaks. When the emitter is not wired
// (nil), heartbeats are skipped and stale detection degrades gracefully.
//
// The progress closure reports the current turn phase (building/executing/
// persisting) via an atomic.Value so the heartbeat goroutine can read it
// concurrently without locks.

// Context patching uses the final round's tokens (window occupancy),
// not the billing totals summed across rounds.

// No-Timeout principle (2026-06-18): long tasks run until completion or
// user cancel. The previous 24h hard deadline (longTaskHardDeadline)
// blocked tasks longer than 24h, contradicting the "no time limit"
// requirement. LLM/DB transient failures are now recovered via
// automatic retry (see RetryTransport and ExecInTxWithRetry); process
// crashes are recovered via CheckpointSaver + RecoveryWorker. The
// heartbeat emitter (10s) keeps the frontend informed of progress
// without imposing a timeout. User can cancel at any time via WS
// `cancel` command.

// ── BUILD + INTENT PASS (parallel) ──
// Run BUILD and Intent Pass concurrently since they have no data
// dependency on each other. This saves 0.5-3s when Intent Pass is enabled.

// 输入级安全三档（Q6）：Deny 在 BUILD/LLM 之前零调用拒绝；HITL 继续
// 本 turn（工具仍走确认）；Inform 仅 shadow 日志。h2 类「BGP+模拟故障」
// 必须落 HITL 不得 Deny。无 intent 产物时 HITL flags 仍经降级注入。

// ── PROACTIVE RECALL (P3-11) ──
// Surface relevant memories based on conversation context before building
// the runner. The hits are stored in ctx so the MemoryInject before-model
// hook can merge them with RecallComposite results. Failures are non-fatal:
// only a warning is logged and the turn continues without proactive hits.
//
// Voice Fast-Path（2026-08-09）：recall 从串行段移入 errgroup 与 BUILD/INTENT
// 三方并行——真机实测串行 recall 0.3-3.3s 且语音轮次 hits 恒为 0，纯开销。
// WithProactiveHits 在 eg.Wait() 后注入 ctx：MemoryInject before-model 钩子在
// 请求时才读取，语义不变；goroutine→Wait 的 happens-before 由 errgroup 保证。

// QuickAssess is pure computation; run it before the errgroup so simple /
// DirectReply turns can skip Intent Pass instead of waiting on that LLM.

// 闲聊/simple：per-request 关 thinking（BUILD 缓存不含入口，不能烘进 GenerationConfig）。
// capability_thinking=false 的模型（Ollama qwen2.5vl 等）不得注入 thinking key。

// Goroutine 1: BUILD
// P1-8: CheckpointSaver is force-enabled for all graph-based Runs at the
// graph builder factory (internal/graph/adapter.runtime_adapter.createAgent),
// so every Run persists checkpoints and can be recovered by RecoveryWorker
// after a process restart.

// Goroutine 2: PROACTIVE RECALL
// 语音轮次跳过：真机实测语音轮次 recall hits 恒为 0，而 recall 含 query
// embedding + 向量检索耗时 0.3-3.3s；eg.Wait() 以最慢 goroutine 收口，
// 零产出召回直接击穿 ≤2s 停口到首音预算（2026-08-11）。

// 2026-08-21 全链路审查 B4：eg.Wait() 以最慢腿收口，recall 实测
// 0.3-3.3s 且无界。加 4s 预算兜底异常长尾（embedding/向量库卡顿），
// 超时走既有 non-fatal 降级（warn + nil hits），不阻塞首 token。

// Recall failure is non-fatal; runProactiveRecall warns internally

// Goroutine 3: Intent Pass —— 2026-08-21 全链路审查 C2（方案 D 分级时限）：
// intent 从 errgroup 拆出，eg 只收 BUILD+Recall；eg.Wait 后按轮次类型
// 分级收口（见 awaitIntentArtifact）。注意 intentCtx 必须挂 turn ctx：
// egCtx 在 eg.Wait() 返回时即取消（见下方 P0 修复注释），挂 egCtx 会在
// rendezvous 开始时就把 intent 杀掉。
// 澄清续跑复用：ctx 携带澄清门前的预解析产物时直接复用，跳过重复 LLM 调用
// （重写后的输入 = 澄清上下文 + 原始需求，产物语义不变）。复用前剥离
// 已作答的澄清残留（问题/歧义/needs_clarification 标记），避免 LLM 重问。
// 否则对非 A2A 且启用 intent 的 agent 正常执行 Intent Pass。
// 注：产物的 RunOptionInject 统一在澄清门之后执行（自动默认路径会先剥离
// 澄清残留），此处只赋值 intentArtifact。

// C2 投机复用（语音 L2/L3）：ASR partial 稳定后预跑的产物，fresh 语义——
// 未经澄清门评估，保留澄清残留（不剥离），澄清门照常判定。

// 2026-08-21 全链路审查 B4/C1：Intent Pass 实测 0.5-3s，保险丝 2.5s
// 覆盖 P95 正常完成；超时视同 Intent 失败（non-fatal，artifact=nil，
// pre-planning 门按无意图降级），不允许单轮意图识别拖住首 token。

// 兜底取消：awaitIntentArtifact 正常消费后此为幂等空转；异常/早退
// 路径（EXECUTE/PERSIST 失败 return）保证 cancel 不丢失（vet lostcancel）。

// BUILD 失败：停掉在途 intent，省 provider token

// C2 方案 D 分级收口：欠规格（澄清门目标场景）与 complex（长规划轮，
// refined goal 影响 Plan 质量）全等至保险丝；moderate 非欠规格限
// 500ms rendezvous，超时降级直接 Run，TTFT 净增量 ≤500ms。

// 并行段收口：recall 命中注入 ctx，供下游 MemoryInject before-model 钩子在
// 请求时合并（原串行位置在 errgroup 之前，并行化后语义保持）。

// SP-1e：submit 路径预分配的 message ID（同步 ACK 已返回给客户端）优先，
// 保证 ACK 的 messageId/turnId 与落库的用户消息 ID 一致；其余入口
// （WS/pending-dequeue/channel/cron）无预分配，走原解析逻辑。

// Session orchestration phase is resolved from persisted teams before the
// Spirit LLM runs. Idle still uses ForcePlanning; Ready/Orchestrating/
// Interrupted keep the current DAG unless the user asked for a new task.

// ── PRE-PLANNING GATE (P1-2) ──
// After intent pass, run a quick complexity assessment. If Moderate/Complex
// AND the session is Idle (or an explicit new task), force the planning path.

// SP-2a：硬路由标记随 turn ctx 传入 run——提示注入仍是首选路径；
// LLM 终答未调 plan_and_execute 时由 AfterModel 钩子直调
// （跳过重试，见 internal/agent/force_planning_route.go）。

// 治理岗不是干活人：有交付信号但门控未 ForcePlanning 时仍标记硬路由，
// AfterModel 在无 plan_and_execute 时改调 subagents_spawn（0831-r2 S08 GM DIY）。

// 包B B4（session-eval-20260825）：编排路由决策落 flowlog——S07 三次执行
// 三种路径（直答 / plan_and_execute roster miss / build_orchestration_graph）
// 才可审计回放（兼治 P-ORCH-NONDET 审计面）。字段按「可沉淀为路由训练
// 语料」设计（R5 RouteLLM 数据飞轮）：query 特征 + skip/gate 决策 + 档位
// 与理由；build 结果回标（team_runs 是否成行）离线 join team_runs 表获得；
// roster 命中明细由 orchestration_progress allocate 事件 match_layer 承载，
// 不在此重复落。

// P0 fix: BUILD runs inside the errgroup above, whose derived ctx is
// cancelled as soon as eg.Wait() returns. The AwaitHook created during
// BUILD captured that ctx (chat_orch_agent_build.go), so the first
// tool-confirmation / await-user wait aborted instantly with
// "context canceled" (select hit the already-closed runCtx.Done()).
// Re-bind the hook to the turn ctx, which lives until the turn ends;
// user cancel still propagates through turnCtx → ctx.

// SetRunStatus("running") was already sent in the EARLY ACK section
// above (right after ADMISSION) to clear the frontend 30s timeout early.

// C-10 / P1-03: cancelled wins over failed. cancelActiveRun sets
// status to "cancelled" before Finish; also honour context.Canceled
// / context.Cause so a racing fail path cannot overwrite cancel.

// Ensure frontend runStatus is reset to "failed" on EXECUTE/PERSIST
// errors (e.g. turn timeout). BUILD-phase errors already publish
// run_status=failed above (line ~386). Without this, the frontend
// runStatus stays "running" → isActiveRun() returns true → new
// messages go through enqueueDuringRun instead of direct send →
// input box never clears (root cause of "no response after send").
// Use context.Background() because ctx may be cancelled (timeout).

// Skip processPendingQueue when this turn was started from inside
// the iterative pending-queue loop (see processPendingQueue). The
// loop owns draining the queue; re-entering would spawn a new
// goroutine per message, re-introducing the chain we eliminated.

// ── CLARIFICATION GATE (2026-07-22) ──
// After Intent Pass, check if the LLM detected blocking ambiguities that
// require user clarification before proceeding. If triggered, publish a
// clarify step and pause the turn until the user responds.
//
// 位置约束：必须位于上方 turn 收尾 defer 注册之后。澄清触发即早退，
// 早退必须经统一清理路径（runs.Finish + runner.Close + processPendingQueue）
// 终结本 run——runner 从未启动 Run()，不存在"同 run 恢复"；续跑
// （resumeTurnWithClarification / 自由回复）以 ParentTaskID 非空的全新
// turn 通过准入。若 run 注册项残留，续跑会被准入层误判为"运行中"而入队
// 到无人消费的 pending 队列（曾致澄清续跑永久挂起）。

// Clarification gate failure is non-fatal; log and continue with original flow.

// Clarification triggered: publish run status and return empty reply.
// The turn will resume when the user submits clarification answers.

// Run 状态用 awaiting_user（Run FSM 合法态：running→awaiting_user→running），
// 与工具确认等待同语义；awaiting_confirmation 是 Session 层状态，
// Run FSM 无此态，直接使用会被 FSM 拒绝导致前端停在"运行中"。

// 假设式前进：澄清上下文已注入输入，产物剥离澄清残留后继续执行。

// 意图产物注入统一在澄清门之后：自动默认路径会替换为剥离澄清残留的
// 产物，早注入会把 needs_clarification 残留烘焙进系统消息（RunOptionInject
// 在创建时即序列化），诱发下游 LLM 重问已作答的问题。prepend 保持
// [inject, forcedPlanning?] 的既有顺序。

// 方案② S3-2 降级注入：intent pass 无产物（失败/超时/跳过）但确定性
// 扫描命中 → 注入风险提示，提醒主 LLM 对潜在破坏性操作先确认再执行。

// ── EXECUTE ──

// EXECUTE phase error: only markTurnError; the defer block above
// (runSingleAgentViaTRPC L477-503) handles publishRunStatus +
// transitionSessionStatus + publishTurnFailure.

// F14: propagate stream-consumer cancellation to session run lifecycle.
// The consumer cancels runCtx (child of ctx), but ctx itself may not
// carry the signal. Flag it explicitly so session_runs.phase aligns
// with turns_v2.status (both "cancelled").

// R4-Q10：取消轮的 usage 记账状态须为 cancelled——此前只置 turnErr，
// turnStatus 保持 "ok"，usage 行被记成 success（S10 实测失真）。

// ── PERSIST ──

// PERSIST phase error: only markTurnError; the defer block above
// (runSingleAgentViaTRPC L477-503) handles publishRunStatus +
// transitionSessionStatus + publishTurnFailure.

// userMsg status rollback on failure

// ── POST-TURN CLARIFICATION (P2, 2026-08-21) ──
// 模型以纯文本阻断性提问收尾但澄清门未命中时，升级为结构化澄清卡片挂起。
// 必须先于 postProcessTurn：session FSM 不允许 completed→awaiting_confirmation，
// 挂起翻转（running→awaiting_confirmation）要抢在落 completed 之前。

// ── POST-PROCESS ──

// shouldSkipIntentPass reports whether this turn should skip the extra intent
// LLM: DirectReply patterns (介绍自己 / 不要调用工具 / 请记住 / 现在几点),
// or QuickAssess==simple when the message is not an underspecified task
// (帮我做个应用 still needs the clarification gate).
//
// 第二返回值携带 QuickAssess 等级（未评估时为空串）：C2 方案 D 分级时限用
// 其区分 intent 收口策略——complex 全等至保险丝，moderate 走 rendezvous。
// 第三返回值是 skip 判定的机器可读理由（包B B4 路由语料）：direct_reply /
// underspecified_task / agent_skip_disabled / task_action_signal /
// planner_unavailable / planner_error / not_simple / confident_simple /
// low_confidence_simple。
//
// 包B（session-eval-20260825 B1）两道闸：
//  1. agent 维度闸：ag.Settings.IntentSkipEnabled=false（管理层 agent：3 GM +
//     部门主管 + spirit，SQL 置 0）时禁用 QuickAssess==simple skip——任务型
//     消息被误判 simple 导致 intent pass 跳过、组织路由失效（P-INTENT-SKIP）。
//     DirectReply 是确定性模式匹配（非误判源），不受此闸影响。
//  2. R4 低置信不 skip：QuickAssess score 贴近 0.3 阈值的边界 simple 说明
//     评分器对「简单」不确信——走完整 intent pass（宁重勿轻，RouteLLM
//     非对称代价：任务型误判为简单 ≫ 闲聊误跑 intent pass）。仅高置信
//     simple（score < intentSkipConfidentSimpleMaxScore）允许 skip。
//
// ADR-79-V V2（2026-08-26）第三道闸：任务信号排除。分类器输出只可用于
// 增加义务、不可用于免除义务——凡含任务动作词的轮次，即使评分器误判
// confident simple 也不得 skip（与 QuickAssess 内部 taskRequestWords
// 升级互为纵深：那里是第一道，这里兜底评分器漏判）。

// intentSkipConfidentSimpleMaxScore R4 低置信闸（包B B1）：QuickAssess 判
// simple 的阈值为 score<0.3，但 [0.2,0.3) 属贴线边界——评分器对「简单」
// 不确信，不允许 skip intent pass。0.2 = 阈值 0.3 内收 0.1 置信边距。

// intentKindOf 安全取 artifact 的 IntentKind（nil → ""），包B B4 路由语料用。

// intentRendezvousWindow C2 方案 D：moderate 非欠规格轮次在 BUILD 收口后
// 再等 intent 的窗口。Intent Pass 实测 0.5–2.3s，500ms 会合几乎必然
// raced_out（产物丢弃、2.3s 白烧）。对齐保险丝 2.5s，让中档有机会吃到
// refined goal / 澄清门。超时 cancel intent，按既有无意图路径降级。

// awaitIntentArtifact C2 方案 D 分级收口：fullWait（欠规格/complex）全等至
// intentCtx 自带的 2.5s 保险丝；否则限 rendezvous 窗口。artifact 只经
// channel 传递（容量 1，goroutine 永不阻塞退出），主流程无共享变量竞争。
// outcome 取值：completed / fused（保险丝熔断）/ raced_out（窗口未命中）。

// shouldRunProactiveRecall 判定本 turn 是否执行主动召回（P3-11）。
//
// 语音轮次（input.Voice != nil）跳过：真机实测语音轮次 recall hits 恒为 0
// （语音输入多为短口语句，实体提及稀少），而 recall 含 query embedding +
// 向量检索，耗时 0.3-3.3s；虽然 recall 与 BUILD/Intent Pass 同 errgroup
// 并行，eg.Wait() 以最慢 goroutine 收口，零产出召回会成为关键路径纯开销，
// 直接击穿 ≤2s 停口到首音预算（2026-08-11）。
//
// 文字 DirectReply / 短闲聊同样跳过：中文分词几乎总会抽出「实体」，不能
// 靠 extractMentionedEntities 判断；L3 硬注入仍覆盖工号召回。
// Stability:internal

// runProactiveRecall triggers proactive memory recall at turn start to surface
// relevant memories based on the conversation context (P3-11). The returned
// hits are later merged with RecallComposite results by the MemoryInject
// before-model hook.
//
// Failures are non-fatal: only a warning is logged and nil is returned so the
// turn continues without proactive hits. The method is a no-op when memory is
// disabled, the composite recaller is not wired, or the recaller does not
// implement biz.ProactiveRecaller.

// extractMentionedEntities extracts candidate entity keywords from the user
// message for proactive recall. This is a simple tokenisation (YAGNI: no NLP,
// no stemming) that splits on whitespace and common punctuation (ASCII + CJK),
// filters out very short tokens, and caps the result to avoid excessive queries.

// SP-1b：统一入口一次完成 decision_records + flowlog 双写；collector 为
// nil 时 EmitDecision 内部记 collector_nil 且 flowlog 仍落（D1）。

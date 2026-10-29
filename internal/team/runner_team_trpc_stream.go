package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/telemetry/turntrace"

	v2 "aranea-agents/internal/agent/v2"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// teamStreamAttemptInput 是 runTeamTRPCFromInput 流式执行段（Phase 7）的全部
// 输入。该段从 runTeamTRPCFromInput 平移至此（语义逐字节不变），目的有二：
//  1. attempt 闭包 + invalid-retry + 终态判定是一个自洽单元，独立成方法后
//     可用替身 runner 直接驱动回归测试（2026-10-29 冤杀事故证明：埋在大
//     函数里的闭包 wiring 退化时没有任何测试能观测到）。
//  2. 终态判定契约在此单点可见：只允许「闭包内、defer abortRun() 触发前
//     捕获的运行期间取消态」驱动中止分支；defer 清理性 cancel 不构成证据。
type teamStreamAttemptInput struct {
	// outerCtx 是 runTeamTRPCFromInput 的函数级 ctx（含 tracing），用于
	// tee/otel 包装与终态持久化（finishRunErr 等）。
	outerCtx context.Context
	// runCtx 是 buildTeamRunContext 产物（含 definition 超时、await-hook、
	// knowledge 注入），作为 invalid-retry 的父 ctx。
	runCtx             context.Context
	sess               biz.Session
	teamRow            biz.Team
	def                Definition
	mode               string
	run                *biz.TeamRunRecord
	runner             trpcrunner.Runner
	uid                string
	userTurnMsg        trpcmodel.Message
	runOpts            []trpcagent.RunOption
	projectMeta        agent.ProjectMeta
	v2Meta             v2.ProjectMeta
	graphExecID        string
	onGraphInterrupt   func(nodeID, lineageID string)
	firstByteBudget    time.Duration
	stallTimeout       time.Duration
	stallMaxReconnects int
	teamEmitter        *event.TraceEmitter
	teamBridge         *turntrace.Bridge
	// turnStatus 指向调用方的 turn 状态（终态失败时置 error，供 deferred
	// tracing/rollback 观测）。
	turnStatus *string
	t0         time.Time
	// rollbackRunnerSession 是 invoke 硬失败路径的一次性回滚（可为 nil）。
	rollbackRunnerSession func()
}

// executeTeamStreamAttempts 执行团队流式消费（含 invalid-retry 包装）并完成
// 终态判定。返回 (result, nil) 表示流自然收官、调用方继续成功路径；
// 返回 (result, err) 表示 run 已按失败终态化（finishRunErr 已调用），
// 调用方直接透传 err。
func (r *Runner) executeTeamStreamAttempts(in teamStreamAttemptInput) (agent.EventStreamResult, error) {
	// lastRunErr 是「运行期间真实取消态」的快照：attempt 闭包在 defer
	// abortRun() 触发【前】捕获 attemptRunCtx.Err()。闭包返回后再读 ctx
	// 会被清理性 cancel 无条件污染（2026-10-29 冤杀事故：干净收官的团队
	// run 恒被误判 stream aborted 落 failed）。终态判定只认此快照。
	var lastRunErr error
	invokeFailed := false
	attemptFn := func(attemptCtx context.Context, attemptNo int) (agent.EventStreamResult, error) {
		lastRunErr = nil
		// invalid-retry stall 共账（P0-4 §12.2③）：第 1 次尝试保持全量
		// stall 重连池（equals 接线前行为，OFF 直通时逐字节一致）；后续
		// 每次重试递减重连预算，经 provider.WithStallBudget 同一 ctx key
		// 注入，消费端守卫总时限按剩余重连数同步重算。
		remainingReconnects := agent.InvalidStallReconnects(in.stallMaxReconnects, attemptNo)
		attemptRunCtx := provider.WithFirstByteRetryBudget(attemptCtx, in.firstByteBudget)
		attemptRunCtx = provider.WithStallBudget(attemptRunCtx, in.stallTimeout, remainingReconnects)
		attemptRunCtx, abortRun := context.WithCancel(attemptRunCtx)
		defer abortRun()

		// 每次尝试新建 consume options（内嵌独立 v2 projector，attempt 间
		// 无共享态；factory 语义见 newStreamConsumeOptions）。
		streamOpts := r.newStreamConsumeOptions()
		if streamOpts == nil {
			streamOpts = &agent.StreamConsumeOptions{}
		}
		streamOpts.AbortOnStall = abortRun

		// 输出预算护栏（2026-10-29 44 实测缺口）：推理型成员 reasoning 无上限
		// 空转会把输出烧到 provider 上下文溢出（实测 reasoning 3 分钟 → 流中段
		// "Context size has been exceeded"）；消费端超限自中止并立即失败收敛。
		streamOpts.MaxReasoningChars, streamOpts.MaxOutputChars = agent.StreamOutputGuardLimitsFromEnv(
			"ARANEA_TEAM", agent.DefaultTeamMaxReasoningChars, agent.DefaultTeamMaxOutputChars)

		streamOpts.OnPromptTokensAccumulated = func(delta int) {
			if r.accumulateRunTokenBudgetFromStream(attemptRunCtx, *in.run, in.teamRow.ID, delta) {
				abortRun()
			}
		}

		if r.heartbeatWriter != nil && in.run.TeamRunV2ID != "" {
			pinger := newTeamRunHeartbeatPinger(r.heartbeatWriter, in.run.TeamRunV2ID, r.lg)
			streamOpts.OnStreamActivity = func() { pinger.Ping(attemptRunCtx) }
		}
		if streamOpts.V2Projector != nil {
			streamOpts.V2Projector.Configure(in.v2Meta)
			attemptRunCtx = biz.WithActivityEmitter(attemptRunCtx, streamOpts.V2Projector)
		}

		attemptEvents, runErr := agent.RunTRPCUserTurnMsg(attemptRunCtx, in.runner, in.uid, in.sess.ID, in.userTurnMsg, in.runOpts...)
		if runErr != nil {
			invokeFailed = true
			return agent.EventStreamResult{}, runErr
		}
		invokeFailed = false
		// F-C: 团队 Graph 路径绕过 trpcGraphRuntime.Run（唯一 EventBridge 宿主），
		// node_start/node_end 到不了 coordinator 的 step watch，per-member
		// team_run_steps 不持久化。tee 框架事件流，把图节点生命周期以 graph_stage
		// system notice 重发到 EventBus（watch 按 spiritSessionID + execution_id 过滤）。
		// Y3: HITL interrupt 经 tee 内联同步标记（先于流结束），避免 async bus
		// 握手与 run 收尾竞态（DeferTeamRunSuccessIfHITL 读到陈旧 Running 把暂停误判完成）。
		attemptEvents = teeGraphStageNotices(in.outerCtx, attemptEvents, r.td.Pipeline.EventBus, in.sess.ID, deriveSpiritSessionID(in.sess), StageGraphAssetID(in.teamRow.LinkedGraphID, in.teamRow.DefinitionJSON), in.graphExecID, in.onGraphInterrupt, r.lg)
		attemptEvents = event.WrapFrameworkEventsWithOtel(attemptEvents, in.teamEmitter, in.teamBridge, in.teamBridge)

		firstByteTimeout := provider.FirstByteGuardWithRetry(in.firstByteBudget, remainingReconnects)
		streamResult, consumeErr := agent.ConsumeWithFirstByteGuard(attemptRunCtx, firstByteTimeout, attemptEvents, in.projectMeta, streamOpts, r.lg)
		// 在 defer abortRun() 触发【前】快照取消态——此时捕获到的只有运行
		// 期间的真实取消（用户取消 / 定义超时 / token 预算跳闸 / 首字节守卫
		// abort）；闭包返回后的清理性 cancel 不进入终态判定。
		lastRunErr = attemptRunCtx.Err()
		return streamResult, consumeErr
	}

	// invalid-retry（P0-4 A10）：默认关闭（env 开关），关闭时单次直通——
	// 与接线前逐字节一致。
	retryOpts := agent.InvalidRetryOptionsFromEnv()
	retryOpts.Emitter = in.teamEmitter
	result, streamErr := agent.RunTurnWithInvalidRetry(in.runCtx, attemptFn, retryOpts, r.lg)
	if streamErr != nil && invokeFailed {
		// LLM 调用发起失败（RunTRPCUserTurnMsg err）：保留原错误路径。
		logTeamRunError(in.teamEmitter, "team.run.execute", streamErr.Error(), in.mode)
		r.finishTeamRunWithError(in.outerCtx, in.run, in.t0, streamErr.Error(), in.turnStatus, in.rollbackRunnerSession)
		return result, streamErr
	}
	if streamErr != nil {
		*in.turnStatus = biz.TeamMemberStepStatusError
		logTeamRunError(in.teamEmitter, "team.run.execute", streamErr.Error(), in.mode)
		rt.PublishLLMFailureNotice(r.td.Pipeline.EventBus, r.lg, in.outerCtx, deriveSpiritSessionID(in.sess), streamErr)
		r.finishRunErr(in.outerCtx, in.run, in.t0, streamErr.Error())
		return result, streamErr
	}
	// 终态流错误守卫（2026-10-29 44 实测缺口）：流中段 response_error
	// （如 "Context size has been exceeded"）只置 HasError/LastError 不中止
	// 消费；若不拦截，下方中止快照块会把真实 provider 错误包装成
	// "stream aborted before first byte or mid-stream liveness"
	// （ErrFirstByteTimeout）→ PlanExecutor 判 transient → 复活第二个团队
	// （实测双团队并存）。真实错误必须原样透传，由 ClassifyStepFailure
	// 分类为 overflow/output_budget（终态不重试）。doom loop 除外——
	// doom 走既有的截断持久化语义。
	if agent.IsTerminalStreamFailure(result) {
		errMsg := strings.TrimSpace(result.LastError)
		*in.turnStatus = biz.TeamMemberStepStatusError
		logTeamRunError(in.teamEmitter, "team.run.execute", errMsg+" hint=stream_terminal_error", in.mode)
		rt.PublishLLMFailureNotice(r.td.Pipeline.EventBus, r.lg, in.outerCtx, deriveSpiritSessionID(in.sess), errors.New(errMsg))
		r.finishRunErr(in.outerCtx, in.run, in.t0, errMsg)
		return result, errors.New(errMsg)
	}
	// 终态中止分支由「消费者观测事实」驱动：lastRunErr 是 attempt 闭包
	// 在 defer abortRun() 触发【前】捕获的 attemptRunCtx.Err() 快照——
	// 只含运行期间的真实取消（用户取消 / 定义超时 / token 预算跳闸 /
	// 首字节守卫 abort）。干净收官的 run 快照为 nil，不入此分支
	// （2026-10-29 冤杀事故：旧实现读闭包返回后的 ctx，被清理性 cancel
	// 无条件污染 → 干净团队 run 恒误判 stream aborted 落 failed）。
	if lastRunErr != nil {
		hint := ""
		errMsg := lastRunErr.Error()
		outErr := lastRunErr
		switch {
		case errors.Is(lastRunErr, context.DeadlineExceeded):
			if dur := TurnDeadlineDuration(in.def); dur > 0 {
				hint = fmt.Sprintf(" hint=definition_timeout_hit effective=%s", dur)
			}
		case errors.Is(lastRunErr, context.Canceled):
			if reason := r.cancelReason(in.sess.ID); reason != "" {
				hint = " hint=cancel_reason:" + reason
				errMsg += " (cancel_reason=" + reason + ")"
				// Only user_cancel (and aliases) keep bare Canceled so
				// StartTeamTurn's isUserCancelledErr maps to cancelled.
				// First-byte / stall / AbortOnStall leave reason empty —
				// wrap as ErrFirstByteTimeout so they stay failed, not
				// user_cancelled (live44 orch probe 2026-10-29).
				if !isUserInitiatedCancelReason(reason) {
					errMsg = "stream aborted before first byte or mid-stream liveness"
					hint = " hint=infra_liveness_abort"
					outErr = fmt.Errorf("%w: %s", agent.ErrFirstByteTimeout, errMsg)
				}
			} else {
				errMsg = "stream aborted before first byte or mid-stream liveness"
				hint = " hint=infra_liveness_abort"
				outErr = fmt.Errorf("%w: %s", agent.ErrFirstByteTimeout, errMsg)
			}
		}
		logTeamRunError(in.teamEmitter, "team.run.execute", errMsg+hint, in.mode)
		*in.turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(in.outerCtx, in.run, in.t0, errMsg)
		return result, outErr
	}
	return result, nil
}

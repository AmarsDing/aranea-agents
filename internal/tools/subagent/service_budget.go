package subagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// runBudgetGuard 是单 run 的流式预算哨兵（C4-②）：每个 usage 事件后
// 核对累计 input token——超阈则置取消原因并取消 run ctx（只触发一次，
// 触发后继续排空事件流，避免生产者阻塞），同时把增量记账到父会话合计
// （供 Spawn 时的 aggregate 闸）。与 team runner
// accumulateRunTokenBudgetFromStream 同型。
type runBudgetGuard struct {
	svc       *Service
	runID     string
	parentSID string

	lastPromptTotal int
	tripped         bool
	tripReason      string
}

func (g *runBudgetGuard) observe(a *usageAccum) {
	if g == nil || g.svc == nil || a == nil {
		return
	}
	prompt, _, _, _, _ := a.totals()
	delta := prompt - g.lastPromptTotal
	if delta < 0 {
		delta = 0
	}
	g.lastPromptTotal = prompt

	var cancelFn context.CancelFunc
	var trip *BudgetTripInfo
	g.svc.mu.Lock()
	if delta > 0 && g.parentSID != "" {
		g.svc.parentInputTokens[g.parentSID] += int64(delta)
	}
	if !g.tripped && g.svc.runBudgetInputTokens > 0 && int64(prompt) > g.svc.runBudgetInputTokens {
		g.tripped = true
		g.tripReason = fmt.Sprintf("subagent input-token budget tripped (used %d, budget %d)", prompt, g.svc.runBudgetInputTokens)
		if r := g.svc.running[g.runID]; r != nil {
			r.cancelRequested = true
			r.cancelReason = g.tripReason
			cancelFn = r.cancel
		}
		trip = &BudgetTripInfo{
			RunID:             g.runID,
			ParentSessionID:   g.parentSID,
			Scope:             BudgetScopeRun,
			UsedInputTokens:   int64(prompt),
			BudgetInputTokens: g.svc.runBudgetInputTokens,
		}
	}
	g.svc.mu.Unlock()
	if cancelFn != nil {
		cancelFn()
	}
	if trip != nil {
		g.svc.emitBudgetTrip(context.Background(), *trip)
	}
}

// emitBudgetTrip 是预算跳闸的审计出口（C4-②）：Warn 日志 + onBudgetTrip
// 钩子（service 层接线 decision-records 双写，对齐 team
// tripRunTokenBudget 的 EmitGate 四件套中的决策落账）。best-effort：
// 钩子 nil 时仅日志，闸本身的取消/拒绝不依赖钩子可用性。
func (s *Service) emitBudgetTrip(ctx context.Context, info BudgetTripInfo) {
	if s == nil {
		return
	}
	s.lg.Warn("subagent input-token 预算跳闸",
		loggateway.StepID("subagent.token_budget.tripped"),
		loggateway.Str("run_id", info.RunID),
		loggateway.Str("parent_session_id", info.ParentSessionID),
		loggateway.Str("scope", info.Scope),
		loggateway.Int64("budget_used_input_tokens", info.UsedInputTokens),
		loggateway.Int64("budget_limit_input_tokens", info.BudgetInputTokens),
	)
	s.mu.RLock()
	hook := s.onBudgetTrip
	s.mu.RUnlock()
	if hook != nil {
		hook(ctx, info)
	}
}

// recordRunUsage persists the subagent run's LLM token usage as an
// aux_subagent usage event (P1-2, 2026-08-19). Zero-token runs (spawn
// failures, provider returned no usage) are skipped: nothing observable was
// consumed. Best-effort: failures are Warn-logged, never fatal.
// tripReason 非空时写入 usage 行 ErrMsg（预算跳闸留痕，C4-②）——跳闸
// 路径 runErr=context.Canceled，仅靠 status=cancelled 无法与用户主动
// 取消区分。
func (s *Service) recordRunUsage(record *runRecord, started runningRun, usage *usageAccum, runErr error, tripReason string) {
	if s == nil || record == nil || usage == nil {
		return
	}
	s.mu.RLock()
	rec := s.usageRecorder
	s.mu.RUnlock()
	if rec == nil {
		return
	}
	promptTok, completionTok, cachedTok, cacheWriteTok, reasoningTok := usage.totals()
	if promptTok <= 0 && completionTok <= 0 {
		return
	}
	status := "success"
	var errMsg string
	switch {
	case errors.Is(runErr, context.Canceled):
		status = "cancelled"
	case errors.Is(runErr, context.DeadlineExceeded):
		status = "timeout"
	case runErr != nil:
		status = "failed"
		errMsg = runErr.Error()
	}
	if tripReason != "" {
		errMsg = tripReason
	}
	attr := record.Attribution

	model := usage.model
	if model == "" {
		model = attr.Model
	}
	latency := time.Duration(0)
	if !started.startedAt.IsZero() {
		latency = s.clock().Sub(started.startedAt)
	}
	if err := rec.RecordAuxLLMUsage(context.Background(), biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxSubagent,
		SessionID:     record.ParentSessionID,
		RunID:         record.ID,
		AgentID:       attr.AgentID,
		AgentKey:      attr.AgentKey,
		UserID:        record.OwnerUserID,
		Provider:      attr.Provider,
		Model:         model,
		Status:        status,
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		CachedTok:     cachedTok,
		CacheWriteTok: cacheWriteTok,
		ReasoningTok:  reasoningTok,
		UsageSource:   "streaming",
		Latency:       latency,
		ErrMsg:        errMsg,
	}); err != nil {
		s.lg.Warn("subagent aux usage record failed",
			loggateway.StepID("tool.subagent_usage_record"),
			loggateway.Str("run_id", record.ID),
			loggateway.Err(err))
	}
}

// usageAccum accumulates per-round billing totals from the run's event
// stream. Semantics mirror agent.accumulateStreamUsage (2026-08-19): each
// LLM round is a separately billed API call → prompt/cached/completion are
// SUMMED across rounds; within one round streaming usage is cumulative →
// track the max. A usage payload whose prompt differs from the current
// round's prompt marks a new billable round.
type usageAccum struct {
	prevPrompt, prevCompletion, prevCached, prevWrite, prevReasoning int
	curPrompt, curCompletion, curCached, curWrite, curReasoning      int
	// model is the last non-empty provider-reported model id seen.
	model string
}

func (a *usageAccum) consume(evt *trpcevent.Event) {
	if a == nil || evt == nil || evt.Response == nil {
		return
	}
	if m := strings.TrimSpace(evt.Response.Model); m != "" {
		a.model = m
	}
	u := evt.Response.Usage
	if u == nil {
		return
	}
	promptTok, completionTok, cachedTok, writeTok, reasoningTok := provider.NormalizeUsageTokens(u)
	if promptTok != a.curPrompt {
		a.prevPrompt += a.curPrompt
		a.prevCompletion += a.curCompletion
		a.prevCached += a.curCached
		a.prevWrite += a.curWrite
		a.prevReasoning += a.curReasoning
		a.curPrompt = promptTok
		a.curCompletion = completionTok
		a.curCached = cachedTok
		a.curWrite = writeTok
		a.curReasoning = reasoningTok
	} else {
		if completionTok > a.curCompletion {
			a.curCompletion = completionTok
		}
		if cachedTok > a.curCached {
			a.curCached = cachedTok
		}
		if writeTok > a.curWrite {
			a.curWrite = writeTok
		}
		if reasoningTok > a.curReasoning {
			a.curReasoning = reasoningTok
		}
	}
}

func (a *usageAccum) totals() (prompt, completion, cached, cacheWrite, reasoning int) {
	if a == nil {
		return 0, 0, 0, 0, 0
	}
	return a.prevPrompt + a.curPrompt,
		a.prevCompletion + a.curCompletion,
		a.prevCached + a.curCached,
		a.prevWrite + a.curWrite,
		a.prevReasoning + a.curReasoning
}

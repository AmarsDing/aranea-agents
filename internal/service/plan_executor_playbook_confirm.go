// Code owned by package service; extracted from plan_executor.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func (e *PlanExecutor) hydrateBoardOrgFields(ctx context.Context, board *biz.PlanBoard) {
	if e == nil || e.taskPlans == nil || board == nil || len(board.Steps) == 0 {
		return
	}
	planID := taskPlanIDFromBoard(board.ID)
	if planID == "" && len(board.Steps) > 0 {
		planID = board.Steps[0].PlanID
	}
	if planID == "" {
		return
	}
	plan, err := e.taskPlans.GetByID(ctx, planID)
	if err != nil || plan == nil {
		return
	}
	biz.HydratePlanStepsFromSubTasks(plan.ID, board.Steps, plan.SubTasks)
}

func playbookConfirmKey(sessionID, stepID string) string {
	return strings.TrimSpace(sessionID) + "\x00" + strings.TrimSpace(stepID)
}

func playbookConfirmLookupIDs(sessionID, id string) []string {
	id = strings.TrimSpace(id)
	sessionID = strings.TrimSpace(sessionID)
	ids := []string{id}
	if stepID, ok := biz.ParsePlaybookConfirmActivityID(sessionID, id); ok {
		ids = append(ids, stepID)
	} else if id != "" {
		ids = append(ids, biz.PlaybookConfirmActivityID(sessionID, id))
	}
	return ids
}

// HasPlaybookStageConfirm reports whether a playbook stage is waiting for R18 confirm.
// id may be the plan-step id or PlaybookConfirmActivityID (ConfirmBlock card).
func (e *PlanExecutor) HasPlaybookStageConfirm(sessionID, stepID string) bool {
	if e == nil {
		return false
	}
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		if _, ok := e.playbookConfirm.Load(playbookConfirmKey(sessionID, id)); ok {
			return true
		}
	}
	return false
}

// ResolvePlaybookStageConfirm unblocks a held playbook stage. Returns false when
// no waiter is registered (already resolved or not a confirm_before step).
func (e *PlanExecutor) ResolvePlaybookStageConfirm(sessionID, stepID string, approved bool) bool {
	if e == nil {
		return false
	}
	w := e.loadPlaybookConfirmWait(sessionID, stepID)
	if w == nil || w.ch == nil {
		return false
	}
	select {
	case w.ch <- approved:
	default:
	}
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		e.playbookConfirm.Delete(playbookConfirmKey(sessionID, id))
	}
	e.finalizePlaybookConfirmCard(context.Background(), w.confirmStep(), approved)
	return true
}

func (e *PlanExecutor) loadPlaybookConfirmWait(sessionID, stepID string) *playbookConfirmWait {
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		v, ok := e.playbookConfirm.Load(playbookConfirmKey(sessionID, id))
		if !ok {
			continue
		}
		if w, ok := v.(*playbookConfirmWait); ok && w != nil {
			return w
		}
	}
	return nil
}

func (e *PlanExecutor) registerPlaybookConfirm(sessionID, stepID string) *playbookConfirmWait {
	w := &playbookConfirmWait{ch: make(chan bool, 1)}
	primary := playbookConfirmKey(sessionID, stepID)
	if actual, loaded := e.playbookConfirm.LoadOrStore(primary, w); loaded {
		if existing, ok := actual.(*playbookConfirmWait); ok && existing != nil {
			w = existing
		}
	}
	e.playbookConfirm.Store(playbookConfirmKey(sessionID, biz.PlaybookConfirmActivityID(sessionID, stepID)), w)
	return w
}

func (e *PlanExecutor) finalizePlaybookConfirmCard(ctx context.Context, step biz.Step, approved bool) {
	if e == nil || e.seq == nil || strings.TrimSpace(step.ID) == "" {
		return
	}
	now := time.Now()
	step.CompletedAt = &now
	step.Version++
	step.Status = biz.StepStatusCompleted
	if !approved {
		step.Status = biz.StepStatusCancelled
	}
	e.seq.Publish(ctx, biz.NewStepUpdatedEvent(step))
}

// NotePlaybookConfirmDecision 决议入口（signalPlaybookConfirm 单点调用）：
// 进程内 map 双键同写；D6 waiterDecisions 在籍时决议表双写——CAS 先到者
// 赢，0 行命中＝别副本已决议（严禁 UPDATE 翻案），读现值并收敛本进程缓存
// （双副本并行 approve/deny 冲突仅留 Warn 审计，先到决议不变）。
//
// 返回值=权威决议值（CAS 收敛后）：调用方必须以返回值驱动
// ResolvePlaybookStageConfirm——D6-f 评审确认项：收敛只写图而 chan 仍发
// 原始落选值，会把 HITL 先到批准静默丢弃（审计=approve 与结局=deny-skip
// 互相矛盾）。
func (e *PlanExecutor) NotePlaybookConfirmDecision(sessionID, id string, approved bool) bool {
	if e == nil {
		return approved
	}
	authoritative := approved
	converged := false
	for _, keyID := range playbookConfirmLookupIDs(sessionID, id) {
		value := approved
		if e.waiterDecisions != nil {
			key := biz.ExecutorWaiterDecisionKey{
				Kind:      biz.WaiterDecisionKindPlaybookConfirm,
				SessionID: strings.TrimSpace(sessionID),
				ScopeID:   keyID,
			}
			recorded, err := e.waiterDecisions.TryRecordDecision(context.Background(), biz.ExecutorWaiterDecision{
				ExecutorWaiterDecisionKey: key,
				Approved:                  approved,
				Actor:                     "chat.playbook_confirm",
				DecidedAt:                 time.Now().UTC(),
			})
			switch {
			case err != nil:
				// 决议表写失败不堵同进程 waiter 流（进程内语义兜底）；
				// 跨副本收敛责任留 waiter 侧确认步已持久卡（confirmSteps 读径）。
				e.lg.Warn("record playbook confirm decision failed (跨副本收敛降级为进程内语义)",
					loggateway.Str("session_id", sessionID),
					loggateway.Str("scope_id", keyID), loggateway.Err(err))
			case !recorded:
				if dec, found, lerr := e.waiterDecisions.LoadDecision(context.Background(), key); lerr == nil && found {
					if dec.Approved != approved {
						e.lg.Warn("playbook confirm 双副本并行决议冲突，保留先到决议",
							loggateway.Str("session_id", sessionID),
							loggateway.Str("scope_id", keyID),
							loggateway.Str("recorded_actor", dec.Actor))
					}
					value = dec.Approved
					if !converged {
						authoritative = dec.Approved
						converged = true
					}
				}
			}
		}
		e.playbookConfirmDecided.Store(playbookConfirmKey(sessionID, keyID), value)
	}
	return authoritative
}

func (e *PlanExecutor) lookupPlaybookConfirmDecision(sessionID, stepID string) (approved bool, ok bool) {
	if e == nil {
		return false, false
	}
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		if v, loaded := e.playbookConfirmDecided.Load(playbookConfirmKey(sessionID, id)); loaded {
			approved, ok = v.(bool)
			return approved, ok
		}
	}
	// D6（97）：决议表读径（waiterDecisions 在籍才读；off 档 nil 跳过零读）——
	// 跨副本决议的唯一持久快路径，不依赖别副本确认卡定案的持久化时序。
	// 命中即收敛本进程缓存（后续 tick/chan 同值短路，与 NotePlaybookConfirmDecision
	// 的 CAS 收敛同向）。
	if e.waiterDecisions != nil {
		for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
			dec, found, err := e.waiterDecisions.LoadDecision(context.Background(), biz.ExecutorWaiterDecisionKey{
				Kind:      biz.WaiterDecisionKindPlaybookConfirm,
				SessionID: strings.TrimSpace(sessionID),
				ScopeID:   id,
			})
			if err == nil && found {
				e.playbookConfirmDecided.Store(playbookConfirmKey(sessionID, id), dec.Approved)
				return dec.Approved, true
			}
		}
	}
	if e.confirmSteps == nil {
		return false, false
	}
	actID := biz.PlaybookConfirmActivityID(sessionID, stepID)
	st, err := e.confirmSteps.GetStep(context.Background(), actID)
	if err != nil {
		return false, false
	}
	switch st.Status {
	case biz.StepStatusCompleted:
		return true, true
	case biz.StepStatusCancelled:
		return false, true
	default:
		return false, false
	}
}

func (e *PlanExecutor) abortPlaybookConfirmWaiters(ctx context.Context, sessionID, stepID string) {
	if e == nil {
		return
	}
	w := e.loadPlaybookConfirmWait(sessionID, stepID)
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		e.playbookConfirm.Delete(playbookConfirmKey(sessionID, id))
	}
	if w != nil {
		e.finalizePlaybookConfirmCard(ctx, w.confirmStep(), false)
	} else {
		e.finalizePlaybookConfirmCard(ctx, biz.Step{
			ID:              biz.PlaybookConfirmActivityID(sessionID, stepID),
			SessionID:       sessionID,
			SpiritSessionID: sessionID,
			Kind:            biz.StepKindConfirm,
			ToolName:        biz.ToolPlaybookConfirmBefore,
			Status:          biz.StepStatusToolBlocked,
			Version:         1,
		}, false)
	}
}

// settlePlaybookConfirmWaiters D6（97-multi-replica-executor-waiters）：
// 轮询拍醒/dispatchStep ctx 终查命中决议后的 waiter 清理与确认卡定案——
// 与 ResolvePlaybookStageConfirm 尾部同构但无 chan 发送（轮询方自持决议
// 返回，决议表/进程内图为唯一事实源；chan 侧迟到的 Resolve 因登记已删
// 变 no-op，双定案竞态与 abort/Resolve 既有竞态同类、终值一致）。
// 只负责 waiter 与确认卡；plan_step 终态转移仍归 dispatchStep 的
// abort/skip 消费链。
func (e *PlanExecutor) settlePlaybookConfirmWaiters(ctx context.Context, sessionID, stepID string, approved bool) {
	if e == nil {
		return
	}
	w := e.loadPlaybookConfirmWait(sessionID, stepID)
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		e.playbookConfirm.Delete(playbookConfirmKey(sessionID, id))
	}
	if w != nil {
		e.finalizePlaybookConfirmCard(ctx, w.confirmStep(), approved)
	}
}

func (r *dagRun) abortPlaybookConfirm(ctx context.Context, step *biz.PlanStep) {
	if r == nil || r.pe == nil || step == nil {
		return
	}
	r.pe.abortPlaybookConfirmWaiters(ctx, r.board.SessionID, step.ID)
}

func (r *dagRun) holdPlaybookConfirm(ctx context.Context, step *biz.PlanStep) (approved bool, held bool) {
	if step == nil || !step.ConfirmBefore {
		return true, false
	}
	if biz.NeedsUserConfirm(biz.ConfirmInput{PlaybookConfirmBefore: true}) != biz.ConfirmPlaybookStage {
		return true, false
	}
	if r == nil || r.pe == nil {
		return true, false
	}
	if decided, ok := r.pe.lookupPlaybookConfirmDecision(r.board.SessionID, step.ID); ok {
		return decided, true
	}
	wait := r.pe.registerPlaybookConfirm(r.board.SessionID, step.ID)
	r.publishConfirmRequired(ctx, step, wait)
	// D6（97-multi-replica-executor-waiters）：waiterDecisions 在籍时叠加
	// 跨副本决议轮询——off 档 nil 跳过，纯 wait.ch+ctx.Done 双 case 字节级
	// 旧行为。醒后先复查再消费，消费即 settle（finalize 真卡+清登记）。
	var tick <-chan time.Time
	var ticker *time.Ticker
	if r.pe.waiterDecisions != nil {
		ticker = time.NewTicker(executorWaiterPollInterval)
		tick = ticker.C
		defer ticker.Stop()
	}
	for {
		select {
		case approved = <-wait.ch:
			return approved, true
		case <-tick:
			if decided, ok := r.pe.lookupPlaybookConfirmDecision(r.board.SessionID, step.ID); ok {
				r.pe.settlePlaybookConfirmWaiters(ctx, r.board.SessionID, step.ID, decided)
				return decided, true
			}
		case <-ctx.Done():
			// D6：ctx 死亡前终查决议（confirmSteps/决议表单读兜底）——
			// 跨副本已批准绝不允许被误判为取消；卡片定案统一归 dispatchStep。
			if decided, ok := r.pe.lookupPlaybookConfirmDecision(r.board.SessionID, step.ID); ok {
				return decided, true
			}
			return false, true
		}
	}
}

func (r *dagRun) publishConfirmRequired(ctx context.Context, step *biz.PlanStep, wait *playbookConfirmWait) {
	if r == nil || r.pe == nil || step == nil {
		return
	}
	summary := "请确认后继续阶段：" + step.Label
	biz.PublishUpwardProgress(r.pe.bus, ctx, r.board.SessionID, biz.PipeConfirmRequired, summary, map[string]any{
		"step_id":      step.ID,
		"confirm_kind": string(biz.ConfirmPlaybookStage),
	})
	if r.pe.seq == nil {
		return
	}
	now := time.Now()
	activityID := biz.PlaybookConfirmActivityID(r.board.SessionID, step.ID)
	confirm := biz.Step{
		ID:              activityID,
		TurnID:          r.board.TurnID,
		TaskID:          r.board.TaskID,
		SessionID:       r.board.SessionID,
		SpiritSessionID: r.board.SessionID,
		Kind:            biz.StepKindConfirm,
		Version:         1,
		Content:         summary,
		ToolName:        biz.ToolPlaybookConfirmBefore,
		Status:          biz.StepStatusToolBlocked,
		StartedAt:       now,
	}
	if wait != nil {
		wait.setStep(confirm)
	}
	r.pe.seq.Publish(ctx, biz.NewStepCreatedEvent(confirm))
}

func (r *dagRun) skipPlaybookConfirmDenied(ctx context.Context, step *biz.PlanStep) {
	now := time.Now()
	r.mu.Lock()
	_ = step.Transition(biz.PlanStepStatusSkipped)
	// 标记 skip 来源（R1 审查修复）：人工拒绝确认的 step 不被 resume 复活。
	step.Error = &biz.StepError{Code: biz.StepErrCodeConfirmDenied, Message: "playbook stage confirmation denied"}
	step.CompletedAt = &now
	step.Version++
	skipped := *step
	r.mu.Unlock()
	if _, err := r.pe.repos.UpsertPlanStep(ctx, skipped); err != nil {
		r.pe.lg.Error("upsert plan_step (confirm denied) failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewPlanStepSkippedEvent(skipped, r.board.SessionID, "playbook stage confirmation denied"))
	r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusInterrupted, "")
	r.cascadeSkip(ctx, step.ID)
	// 发现#7：确认拒绝释放预标记时持有的配额槽位（confirm_before 步骤
	// 持槽等待是保守语义），终态转移后全局重扫准入后续就绪 pending；
	// cascade 子树内部已全量 skipped，不会产生子树内准入。
	r.admitReadySteps(ctx)
}

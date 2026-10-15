package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/session"
)

// sessionActivityLister adapts biz.StepV2Reader + biz.TaskV2Reader to
// session.ActivityLister. This bridges the package boundary: biz/session
// cannot import biz (circular), so the adapter is provided from the biz
// package side via Wire.
//
// Adapts the v2 persistence model (Task = user message, Step = agent-side
// activity) to the v1 Activity shape via StepToActivity / taskToActivity for
// backward compat with the session.ActivityEntry conversion.
//
// Both sources MUST be merged: steps_v2 only persists agent-side steps
// (thinking/action/reply/notice); user inputs live in tasks_v2
// (Task.UserMessage). A steps-only timeline drops every user message, which
// breaks downstream consumers that filter on role=user — auto-memory L4 graph
// extraction (WriteFromUserText gate), L3 consolidation quality, and the
// ListSessionMessages RPC backing the frontend chat history.
type sessionActivityLister struct {
	stepReader StepV2Reader
	taskReader TaskV2Reader
	turnReader TurnV2Reader // optional; enables COMPRESS-IO AfterTurn range loads
}

// NewSessionActivityLister creates a session.ActivityLister from the v2
// step/task readers. Returns nil when stepReader is nil so NewSessionUsecase
// falls back to the legacy sessions-repo code path (used by tests/CLI without
// StepV2Reader wired). taskReader may be nil (tasks skipped) for the same
// fallback scenarios.
func NewSessionActivityLister(stepReader StepV2Reader, taskReader TaskV2Reader) session.ActivityLister {
	if stepReader == nil {
		return nil
	}
	return &sessionActivityLister{stepReader: stepReader, taskReader: taskReader}
}

// ProvideSessionActivityLister wires step/task/turn readers. Turn reader enables
// ActivityTurnRangeLister for compression AfterTurn (COMPRESS-IO).
func ProvideSessionActivityLister(stepReader StepV2Reader, taskReader TaskV2Reader, turnReader TurnV2Reader) session.ActivityLister {
	l := NewSessionActivityLister(stepReader, taskReader)
	if l == nil {
		return nil
	}
	if a, ok := l.(*sessionActivityLister); ok {
		a.turnReader = turnReader
	}
	return l
}

// ListBySessionAfterTurnNumber implements session.ActivityTurnRangeLister when
// turnReader is wired. afterTurn is compared to turns_v2.seq.
func (a *sessionActivityLister) ListBySessionAfterTurnNumber(ctx context.Context, sessionID string, afterTurn int) (session.ActivityTurnRangeResult, error) {
	if a == nil || a.turnReader == nil {
		return session.ActivityTurnRangeResult{}, session.ErrTurnRangeUnavailable
	}
	turns, err := a.turnReader.ListTurnsBySessionAfterSeq(ctx, sessionID, int64(afterTurn))
	if err != nil {
		return session.ActivityTurnRangeResult{}, err
	}
	if len(turns) == 0 {
		return session.ActivityTurnRangeResult{TurnSeq: map[string]int{}}, nil
	}
	turnSeq := make(map[string]int, len(turns))
	turnSet := make(map[string]struct{}, len(turns))
	turnIDs := make([]string, 0, len(turns))
	taskIDs := make(map[string]struct{}, len(turns))
	for _, t := range turns {
		turnSeq[t.ID] = int(t.Seq)
		turnSet[t.ID] = struct{}{}
		turnIDs = append(turnIDs, t.ID)
		if tid := strings.TrimSpace(t.TaskID); tid != "" {
			taskIDs[tid] = struct{}{}
		}
	}
	// COMPRESS-IO: load steps only for turns in the AfterTurn window (no full
	// session step scan). Prefer batch IN query when the repo supports it.
	steps, err := listStepsForTurns(ctx, a.stepReader, turnIDs)
	if err != nil {
		return session.ActivityTurnRangeResult{}, err
	}
	var acts []Activity
	for _, s := range steps {
		acts = append(acts, StepToActivity(s))
	}
	if a.taskReader != nil {
		// Attribute user(task) rows via turns_v2 first-turn (min seq), not
		// firstTurnIDByTask(allSteps) — avoids scanning pre-window steps.
		for taskID := range taskIDs {
			ft, ok, err := firstTurnIDForTask(ctx, a.turnReader, taskID)
			if err != nil {
				return session.ActivityTurnRangeResult{}, err
			}
			if !ok {
				continue
			}
			if _, in := turnSet[ft]; !in {
				continue // first turn already outside AfterTurn window
			}
			t, err := a.taskReader.GetTask(ctx, taskID)
			if err != nil {
				return session.ActivityTurnRangeResult{}, err
			}
			act := taskToActivity(t)
			act.TurnID = ft
			acts = append(acts, act)
		}
	}
	return session.ActivityTurnRangeResult{
		Entries: activitiesToSessionEntries(acts),
		TurnSeq: turnSeq,
	}, nil
}

func (a *sessionActivityLister) ListBySessionTurn(ctx context.Context, sessionID, turnID string) ([]session.ActivityEntry, error) {
	steps, err := a.stepReader.ListStepsByTurn(ctx, turnID)
	if err != nil {
		return nil, err
	}
	acts := make([]Activity, 0, len(steps))
	for _, s := range steps {
		acts = append(acts, StepToActivity(s))
	}
	return activitiesToSessionEntries(acts), nil
}

func (a *sessionActivityLister) ListBySession(ctx context.Context, sessionID string) ([]session.ActivityEntry, error) {
	steps, err := a.stepReader.ListStepsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	acts := make([]Activity, 0, len(steps))
	for _, s := range steps {
		acts = append(acts, StepToActivity(s))
	}
	if a.taskReader != nil {
		tasks, err := a.taskReader.ListTasksBySession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		// 79-runtime-governance fix2：user(task) 行回填首 turn id。
		// 空 TurnID → synthesizeTurnNumbers 序号 0 → 压缩 body 窗口
		// （m.TurnNumber > maxSummarized）恒排除用户消息，摘要丢失用户意图。
		firstTurn := firstTurnIDByTask(steps)
		for _, t := range tasks {
			act := taskToActivity(t)
			act.TurnID = firstTurn[t.ID]
			acts = append(acts, act)
		}
	}
	return activitiesToSessionEntries(acts), nil
}

// firstTurnIDByTask maps each task to the turn that owns its user message:
// the turn of the task's minimum-seq step. Step seq is monotonic per spirit
// session (SeqAssigner), so a task's earliest step belongs to its first
// (user-input) turn; system-push continuation turns share the task but have
// larger seqs. Tasks whose turns produced no steps (crash between
// task.created and the first step) stay unmapped — their user row keeps an
// empty TurnID, matching prior behavior for unattributed rows.
func firstTurnIDByTask(steps []Step) map[string]string {
	type ref struct {
		seq    int64
		turnID string
	}
	best := make(map[string]ref)
	for _, s := range steps {
		if s.TaskID == "" || s.TurnID == "" {
			continue
		}
		cur, ok := best[s.TaskID]
		if !ok || s.Seq < cur.seq {
			best[s.TaskID] = ref{seq: s.Seq, turnID: s.TurnID}
		}
	}
	out := make(map[string]string, len(best))
	for taskID, r := range best {
		out[taskID] = r.turnID
	}
	return out
}

// activitiesToSessionEntries converts []biz.Activity to []session.ActivityEntry.
// Only the fields needed for ChatMessage conversion are copied.
func activitiesToSessionEntries(acts []Activity) []session.ActivityEntry {
	out := make([]session.ActivityEntry, 0, len(acts))
	for _, a := range acts {
		entry := session.ActivityEntry{
			ID:         a.ID,
			Kind:       string(a.Kind),
			Status:     string(a.Status),
			SessionID:  a.SessionID,
			TurnID:     a.TurnID,
			Timestamp:  a.Timestamp,
			Content:    a.Content,
			Reasoning:  a.Reasoning,
			ToolName:   a.ToolName,
			ToolResult: a.ToolResult,
			AgentKey:   a.AgentKey,
		}
		// Carry the notice classification through so the message view can
		// filter system-internal notices (F2). Meta key written by
		// StepToActivity from Step.NoticeType.
		if nt, ok := a.Meta["notice_type"].(string); ok {
			entry.NoticeType = nt
		}
		out = append(out, entry)
	}
	return out
}


// StepTurnBatchLister optionally loads steps for many turn IDs in one query.
type StepTurnBatchLister interface {
	ListStepsByTurnIDs(ctx context.Context, turnIDs []string) ([]Step, error)
}

func listStepsForTurns(ctx context.Context, reader StepV2Reader, turnIDs []string) ([]Step, error) {
	if len(turnIDs) == 0 {
		return nil, nil
	}
	if bl, ok := reader.(StepTurnBatchLister); ok {
		return bl.ListStepsByTurnIDs(ctx, turnIDs)
	}
	var out []Step
	for _, id := range turnIDs {
		steps, err := reader.ListStepsByTurn(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, steps...)
	}
	return out, nil
}

func firstTurnIDForTask(ctx context.Context, turns TurnV2Reader, taskID string) (string, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if turns == nil || taskID == "" {
		return "", false, nil
	}
	list, err := turns.ListTurnsByTask(ctx, taskID)
	if err != nil {
		return "", false, err
	}
	var bestID string
	var bestSeq int64
	found := false
	for _, t := range list {
		if t.ID == "" {
			continue
		}
		if !found || t.Seq < bestSeq {
			bestID = t.ID
			bestSeq = t.Seq
			found = true
		}
	}
	return bestID, found, nil
}

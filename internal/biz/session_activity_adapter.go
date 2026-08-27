package biz

import (
	"context"

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

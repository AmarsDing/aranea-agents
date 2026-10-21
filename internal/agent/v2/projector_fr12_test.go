package v2

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// FR-12（89 批次3）：projector 双终态守卫。
// OnError 已将 root task 终态化（task.failed 已发布）后，OnTurnEnd 的
// cancel fallback 不得再发第二个终态 task 事件（duplicate_terminal 违例）。
// 机制：OnError 终态化 root task 时锁内置 turnFailed；OnTurnEnd cancel
// fallback 加 `canceled && !turnFailed` 守卫。

// fr12TerminalKinds 汇总 capture 中全部 Task 终态事件（Task.completed /
// Task.failed / Task.cancelled），返回各 kind 计数与终态事件总数。
func fr12TerminalKinds(c *capturingSequencer) (counts map[biz.EventKind]int, total int, lastStatus map[biz.EventKind]biz.TaskStatus) {
	counts = map[biz.EventKind]int{}
	lastStatus = map[biz.EventKind]biz.TaskStatus{}
	for _, e := range c.events {
		switch ev := e.(type) {
		case *biz.TaskCompletedEvent:
			counts[biz.EventKindTaskCompleted]++
			lastStatus[biz.EventKindTaskCompleted] = ev.Task.Status
		case *biz.TaskFailedEvent:
			counts[biz.EventKindTaskFailed]++
			lastStatus[biz.EventKindTaskFailed] = ev.Task.Status
		case *biz.TaskCancelledEvent:
			counts[biz.EventKindTaskCancelled]++
			lastStatus[biz.EventKindTaskCancelled] = ev.Task.Status
		}
	}
	total = counts[biz.EventKindTaskCompleted] + counts[biz.EventKindTaskFailed] + counts[biz.EventKindTaskCancelled]
	return counts, total, lastStatus
}

// 1. 核心场景：OnTurnStart → OnError → OnTurnEnd(canceled=true)。
// 断言恰好一个终态 task 事件且为 failed——cancel fallback 不得补发
// task.cancelled 覆写 live 结局（FR-12 双终态守卫）。
func TestProjector_FR12_ErrorThenCancel_SingleFailedTerminal(t *testing.T) {
	p, capture := testProjector()
	ctx := context.Background()
	capture.events = nil // 只观察 OnError/OnTurnEnd 之后的事件

	p.OnError(ctx, "boom", "run_error", "")
	p.OnTurnEnd(ctx, p.meta, true)

	counts, total, _ := fr12TerminalKinds(capture)
	if total != 1 {
		t.Fatalf("FR-12: terminal task events = %d (kinds=%v), want exactly 1", total, counts)
	}
	if counts[biz.EventKindTaskFailed] != 1 {
		t.Fatalf("FR-12: failed terminal count = %d, want 1 (kinds=%v)", counts[biz.EventKindTaskFailed], counts)
	}
	if counts[biz.EventKindTaskCancelled] != 0 {
		t.Fatalf("FR-12: cancel fallback must not emit task.cancelled after failed (kinds=%v)", counts)
	}
}

// 2. 纯取消（无 OnError）→ cancel 语义保留：恰好一个 task.cancelled。
// 守卫不得误伤合法取消路径。
func TestProjector_FR12_CancelWithoutError_EmitsCancelled(t *testing.T) {
	p, capture := testProjector()
	ctx := context.Background()
	capture.events = nil

	p.OnTurnEnd(ctx, p.meta, true)

	counts, total, _ := fr12TerminalKinds(capture)
	if total != 1 || counts[biz.EventKindTaskCancelled] != 1 {
		t.Fatalf("FR-12: cancel without error must emit exactly one task.cancelled (counts=%v total=%d)", counts, total)
	}
}

// 3. task 已被前移终态化（非 OnError 路径，如 team dispatch 已移出
// activeTask）且未失败 → cancel fallback 仍补发 task.cancelled
// （turnFailed=false 不拦）。直接清 activeTask 模拟前移路径。
func TestProjector_FR12_CancelFallbackWithoutFailure_StillEmits(t *testing.T) {
	p, capture := testProjector()
	ctx := context.Background()

	p.mu.Lock()
	delete(p.activeTask, "task-1") // 模拟 team dispatch path 已移除，且无失败
	p.mu.Unlock()
	capture.events = nil

	p.OnTurnEnd(ctx, p.meta, true)

	counts, total, _ := fr12TerminalKinds(capture)
	if total != 1 || counts[biz.EventKindTaskCancelled] != 1 {
		t.Fatalf("FR-12: fallback cancel (no failure) must still emit task.cancelled (counts=%v total=%d)", counts, total)
	}
}

// 4. OnError 终态化后 OnTurnEnd(canceled=false)：turn 事件携带 failed
// 态（turnFailed 置位生效），且不重复发 task 终态事件。
func TestProjector_FR12_ErrorThenNormalEnd_TurnFailedNoDupTask(t *testing.T) {
	p, capture := testProjector()
	ctx := context.Background()
	capture.events = nil

	p.OnError(ctx, "boom", "run_error", "")
	capture.events = nil // 只观察 OnTurnEnd 之后的事件
	p.OnTurnEnd(ctx, p.meta, false)

	counts, total, _ := fr12TerminalKinds(capture)
	if total != 0 {
		t.Fatalf("FR-12: OnTurnEnd must not re-emit task terminal after OnError (counts=%v)", counts)
	}
	var turnEv *biz.TurnCompletedEvent
	for _, e := range capture.events {
		if ev, ok := e.(*biz.TurnCompletedEvent); ok {
			turnEv = ev
		}
	}
	if turnEv == nil {
		t.Fatal("FR-12: expected turn.completed event")
	}
	if turnEv.Turn.Status != biz.TurnStatusFailed {
		t.Fatalf("FR-12: turn status = %q, want failed (turnFailed flag set by OnError)", turnEv.Turn.Status)
	}
}

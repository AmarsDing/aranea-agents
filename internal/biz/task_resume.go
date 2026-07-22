package biz

import (
	"fmt"
	"sort"
	"strings"
)

// L3 (2026-07-22)：interrupted task 续跑的上下文组装。
// 进程重启把 in-flight task 终态化为 interrupted 后，用户可从 UI 显式
// 「继续执行」。续跑不是原地复活 runner，而是带完整执行轨迹重跑：
// 把中断前已完成的 action/reply step 渲染为紧凑轨迹注入 prompt，
// agent 据此跳过已完成步骤、继续未完成的工作。

const (
	// resumeTraceMaxEntries caps how many trace entries are injected so a
	// long pre-crash run cannot blow up the prompt budget.
	resumeTraceMaxEntries = 20
	// resumeTraceFieldMaxLen truncates tool args / reply content per entry.
	resumeTraceFieldMaxLen = 200
)

// BuildTaskResumeTrace renders a compact numbered execution trace from
// persisted steps. Only completed action/reply steps are included —
// thinking/notice/error steps are noise for resume context. Entries are
// ordered by Seq (turn-local order is preserved well enough for context).
//
// Returns "" when no completed action/reply steps exist (task crashed before
// producing anything useful; resume then degrades to a plain rerun).
func BuildTaskResumeTrace(steps []Step) string {
	kept := make([]Step, 0, len(steps))
	for _, st := range steps {
		if st.Status != StepStatusCompleted {
			continue
		}
		if st.Kind != StepKindAction && st.Kind != StepKindReply {
			continue
		}
		kept = append(kept, st)
	}
	if len(kept) == 0 {
		return ""
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].StartedAt.Equal(kept[j].StartedAt) {
			return kept[i].Seq < kept[j].Seq
		}
		return kept[i].StartedAt.Before(kept[j].StartedAt)
	})
	truncated := false
	if len(kept) > resumeTraceMaxEntries {
		kept = kept[len(kept)-resumeTraceMaxEntries:]
		truncated = true
	}
	var b strings.Builder
	if truncated {
		fmt.Fprintf(&b, "（轨迹过长，仅保留最近 %d 条）\n", resumeTraceMaxEntries)
	}
	for i, st := range kept {
		switch st.Kind {
		case StepKindAction:
			args := truncateResumeField(strings.TrimSpace(string(st.ToolArgs)))
			if args == "" {
				fmt.Fprintf(&b, "%d. [工具] %s → 成功\n", i+1, st.ToolName)
			} else {
				fmt.Fprintf(&b, "%d. [工具] %s(%s) → 成功\n", i+1, st.ToolName, args)
			}
		case StepKindReply:
			content := truncateResumeField(strings.TrimSpace(st.Content))
			if content == "" {
				continue
			}
			fmt.Fprintf(&b, "%d. [回复] %s\n", i+1, content)
		}
	}
	return strings.TrimSpace(b.String())
}

// InterruptedResumeUserContent builds the user content for resuming an
// interrupted task (L3). The original user message is preserved verbatim so
// the rerun stays faithful to the user's request; the trace (when present)
// tells the agent what is already done.
func InterruptedResumeUserContent(userMessage, trace string) string {
	userMessage = strings.TrimSpace(userMessage)
	trace = strings.TrimSpace(trace)
	if trace == "" {
		return "[系统] 该任务此前因服务重启中断，未完成任何已持久化的步骤。请重新执行用户的原始任务：\n" + userMessage
	}
	return "[系统] 该任务此前因服务重启中断。以下是中断前已完成的执行轨迹：\n" + trace +
		"\n请基于以上进度继续完成任务，不要重复已完成的步骤；已失败的步骤可直接重试。\n\n原始任务：\n" + userMessage
}

func truncateResumeField(s string) string {
	r := []rune(s)
	if len(r) <= resumeTraceFieldMaxLen {
		return s
	}
	return string(r[:resumeTraceFieldMaxLen]) + "…"
}

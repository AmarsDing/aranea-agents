package subagent

import (
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz"

	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
)

func enrichRunView(run trpcsubagent.Run, now time.Time) map[string]any {
	b, err := json.Marshal(run)
	if err != nil {
		return map[string]any{"id": run.ID, "status": run.Status, "kind": run.Kind}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{"id": run.ID, "status": run.Status, "kind": run.Kind}
	}
	start := run.CreatedAt
	if run.StartedAt != nil && !run.StartedAt.IsZero() {
		start = *run.StartedAt
	}
	if !start.IsZero() && (run.Status == trpcsubagent.StatusQueued || run.Status == trpcsubagent.StatusRunning) {
		ms := now.Sub(start).Milliseconds()
		if ms < 0 {
			ms = 0
		}
		m["running_for_ms"] = ms
	}
	return m
}

// upwardTruncatedMarker 告知模型结果被截断及取全量的方式。
const upwardTruncatedMarker = "\n…[truncated: full result %d runes — re-call subagents_get with full_result=true]"

// clipUpwardResultView 把 get 视图的 result 裁剪到 ≤biz.UpwardPipeMaxRunes
// 并附截断标记与元数据（result_truncated/result_full_runes）。
func clipUpwardResultView(view map[string]any) {
	raw, ok := view["result"].(string)
	if !ok || raw == "" {
		return
	}
	total := utf8.RuneCountInString(raw)
	if total <= biz.UpwardPipeMaxRunes {
		return
	}
	view["result"] = biz.ClipUpwardPayload(raw) + fmt.Sprintf(upwardTruncatedMarker, total)
	view["result_truncated"] = true
	view["result_full_runes"] = total
}

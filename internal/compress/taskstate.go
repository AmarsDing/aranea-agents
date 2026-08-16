package compress

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
)

// ExtractTaskState 从压缩 LLM 产出中拆出末尾的 task_state JSON 块，
// 返回剥离块后的叙事 Markdown 与结构化任务状态。
//
// 契约（v4）：只有位于产出末尾（尾部仅剩空白）的 ```json 块才被识别；
// 块解析失败、无任务键（status/done/next/blockers 全空）时视为无状态，
// 且原文保持不动（避免误剥普通 JSON 代码块）。空对象 {} 视为 LLM 显式
// 产出"无任务状态"：剥离块但不产状态。
func ExtractTaskState(markdown string) (string, *biz.TaskState) {
	in := strings.TrimSpace(markdown)
	// 末尾必须以 ``` 收尾，否则块不在尾部。
	if !strings.HasSuffix(in, "```") {
		return markdown, nil
	}
	// 定位最后一个 ```json 开围栏（大小写不敏感：LLM 可能输出 ```JSON/```Json；
	// 小写副本与原串仅差 ASCII 大小写，字节偏移一致）。
	openIdx := strings.LastIndex(strings.ToLower(in), "```json")
	if openIdx < 0 {
		return markdown, nil
	}
	body := strings.TrimSpace(in[openIdx+len("```json") : len(in)-len("```")])
	if body == "" {
		return markdown, nil
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(body), &raw) != nil {
		return markdown, nil
	}
	// 只认领含任务键的块；普通 JSON 块原样保留。
	isTask := false
	for _, k := range []string{"status", "done", "next", "blockers"} {
		if _, ok := raw[k]; ok {
			isTask = true
			break
		}
	}
	if !isTask {
		if len(raw) == 0 {
			return strings.TrimSpace(in[:openIdx]), nil
		}
		return markdown, nil
	}
	stripped := strings.TrimSpace(in[:openIdx])
	var state biz.TaskState
	if json.Unmarshal([]byte(body), &state) != nil {
		return stripped, nil
	}
	state.Normalize()
	if state.Empty() {
		return stripped, nil
	}
	return stripped, &state
}

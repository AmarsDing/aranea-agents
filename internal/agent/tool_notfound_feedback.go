package agent

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// toolNotFoundMarker 是框架 tool-not-found 错误文本的固定子串
// （trpc-agent-go processor.ErrorToolNotFound = "Error: tool not found"，
// 包裹后为 "tool execution error: executeToolCall: Error: tool not found: <name>"）。
// 业务层只做只读匹配，不回写框架。
const toolNotFoundMarker = "Error: tool not found"

// toolNotFoundGuidanceTag 防止同一请求内重复追加纠错指引。
const toolNotFoundGuidanceTag = "[系统纠错指引]"

// newToolNotFoundFeedbackBeforeHook 返回 BeforeModel hook：扫描请求消息中的
// tool-not-found 错误回执，在最后一条上追加「当前可用工具清单 + 纠偏指引」。
//
// 背景（17:03 会话根因）：框架默认不带工具名建议（WithToolNameSuggestions 未开启），
// 模型收到光秃秃的 "Error: tool not found: X" 后倾向于臆造变体名重试
// （hostexec_exec_command → exec_command），空转消耗轮次。框架侧开启建议需改
// vendored llmagent（FW-R1 禁止），故在业务层 BeforeModel 改写错误回执消息——
// req.Tools 由框架在回调前按 invocation 实际工具面填充（含动态激活工具），清单准确。
//
// 只改写 req.Messages 副本（每次 LLM 调用前重新扫描，会话存储原文不变），幂等。
func newToolNotFoundFeedbackBeforeHook(lg loggateway.Logger) callbacks.Callback {
	return callbacks.NewBeforeModelHook(2, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		req := args.Request
		lastIdx := -1
		for i := range req.Messages {
			m := &req.Messages[i]
			if m.Role == trpcmodel.RoleTool &&
				strings.Contains(m.Content, toolNotFoundMarker) &&
				!strings.Contains(m.Content, toolNotFoundGuidanceTag) {
				lastIdx = i
			}
		}
		if lastIdx < 0 || len(req.Tools) == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		names := make([]string, 0, len(req.Tools))
		for name := range req.Tools {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				names = append(names, trimmed)
			}
		}
		if len(names) == 0 {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		sort.Strings(names)
		var b strings.Builder
		b.WriteString("\n\n")
		b.WriteString(toolNotFoundGuidanceTag)
		b.WriteString(" 该工具名不存在于当前可用工具列表，臆造/变体名重试只会再次失败。当前可用工具（")
		b.WriteString(strconv.Itoa(len(names)))
		b.WriteString(" 个）：")
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("。下一步只能三选一：1) 直接使用列表中的工具；2) 用 tool_load 加载确切工具名后再调用；3) 能力缺失时如实告知用户，禁止假设式前进。")
		req.Messages[lastIdx].Content += b.String()
		if lg != nil {
			lg.Debug("tool not found feedback injected",
				loggateway.Str("tool_message_index", strconv.Itoa(lastIdx)),
				loggateway.Str("available_tools", strings.Join(names, ",")))
		}
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

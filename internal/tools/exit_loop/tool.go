// Package exit_loop 封装 ADK 的退出循环语义（设置 Escalate / SkipSummarization），仅可由 Runner 上下文生效。
package exit_loop

import (
	"context"
	"fmt"

	"aranea-agents/internal/tools/toolapi"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/exitlooptool"
)

// impl 将「跳出循环」行为委托给框架 tool：运行时会改写 EventActions.Escalate。
type impl struct{}

func New() toolapi.Tool {
	return &impl{}
}

const exitLoopDescription = "Exits the loop.\n\nCall this function only when you are instructed to do so.\n"

func (*impl) Meta() toolapi.Meta {
	return toolapi.Meta{
		Name:        "exit_loop",
		TitleZh:     "跳出循环",
		SummaryZh:   "在满足策略条件时标记 Escalate 以中断当前 Runner 环路（仅可由 ADK 调度上下文正确生效）。",
		Description: exitLoopDescription,
	}
}

func (*impl) SupportsLocalInvoke() bool { return false }

func (*impl) InvokeLocal(ctx context.Context, args map[string]any) (map[string]any, error) {
	_ = ctx
	_ = args
	return nil, fmt.Errorf("exit_loop 仅能在 ADK Runner 内生效")
}

func (*impl) OpenAIFunction() map[string]any {
	return nil
}

func (*impl) ADKTool() (tool.Tool, error) {
	return exitlooptool.New()
}

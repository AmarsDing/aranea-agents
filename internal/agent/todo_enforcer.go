package agent

import (
	"aranea-agents/pkg/loggateway"

	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/agent/extension/todoenforcer"
	"trpc.group/trpc-go/trpc-agent-go/tool/todo"
)

// NewTodoEnforcerOption returns an llmagent.Option that enables the framework's
// TodoEnforcer extension. The TodoEnforcer ensures that an Agent follows its
// todo list: if open todo items remain, the agent cannot declare itself "done"
// without either completing them or formally declaring an external blocker.
//
// The enforcer contributes both the todo_write and todo_declare_blocker tools
// to the agent, so callers do not need to register a separate todo.Tool.
//
// Usage:
//
//	opt := NewTodoEnforcerOption(nil, lg)
//	ag := llmagent.New("planner", llmagent.WithModel(m), opt)
func NewTodoEnforcerOption(todoTool *todo.Tool, lg loggateway.Logger) trpcllmagent.Option {
	enforcerOpts := []todoenforcer.Option{
		todoenforcer.WithMaxRetries(3),
		todoenforcer.WithOnEnforce(func(evt todoenforcer.EnforceEvent) {
			if lg == nil {
				return
			}
			lg.Info("TodoEnforcer 事件",
				loggateway.StepID("todoenforcer.event"),
				loggateway.Str("reason", string(evt.Reason)),
				loggateway.Str("agent", evt.AgentName),
				loggateway.Int("attempt", evt.AttemptNumber),
				loggateway.Int("max_retries", evt.MaxRetries),
				loggateway.Int("pending", evt.PendingCount),
				loggateway.Int("in_progress", evt.InProgressCount),
			)
		}),
	}
	if todoTool != nil {
		enforcerOpts = append(enforcerOpts, todoenforcer.WithTodoTool(todoTool))
	}
	enforcer := todoenforcer.New(enforcerOpts...)
	return trpcllmagent.WithExtensions(enforcer)
}

// NewTodoEnforcerOptionWithScope returns an llmagent.Option that enables
// TodoEnforcer only for agents whose names are in the scoped list.
// This is useful when sharing a single enforcer configuration across
// multiple agents but only wanting enforcement on specific ones.
func NewTodoEnforcerOptionWithScope(todoTool *todo.Tool, scopedAgents []string, lg loggateway.Logger) trpcllmagent.Option {
	enforcerOpts := []todoenforcer.Option{
		todoenforcer.WithMaxRetries(3),
		todoenforcer.WithScopedAgents(scopedAgents...),
		todoenforcer.WithOnEnforce(func(evt todoenforcer.EnforceEvent) {
			if lg == nil {
				return
			}
			lg.Info("TodoEnforcer 事件(限定)",
				loggateway.StepID("todoenforcer.scoped_event"),
				loggateway.Str("reason", string(evt.Reason)),
				loggateway.Str("agent", evt.AgentName),
				loggateway.Int("attempt", evt.AttemptNumber),
			)
		}),
	}
	if todoTool != nil {
		enforcerOpts = append(enforcerOpts, todoenforcer.WithTodoTool(todoTool))
	}
	enforcer := todoenforcer.New(enforcerOpts...)
	return trpcllmagent.WithExtensions(enforcer)
}

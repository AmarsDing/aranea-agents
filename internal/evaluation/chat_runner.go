package evaluation

import (
	"context"

	"aranea-agents/pkg/safego"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// chatRunnerAdapter implements runner.Runner by delegating to AgentRunner.
type chatRunnerAdapter struct {
	agentID string
	runFn   AgentRunner
}

// NewChatRunnerAdapter bridges ChatService turns to trpc-agent-go runner.Runner.
func NewChatRunnerAdapter(agentID string, runFn AgentRunner) runner.Runner {
	return &chatRunnerAdapter{agentID: agentID, runFn: runFn}
}

func (r *chatRunnerAdapter) Run(
	ctx context.Context,
	_ string,
	_ string,
	message model.Message,
	_ ...agent.RunOption,
) (<-chan *event.Event, error) {
	ch := make(chan *event.Event, 2)
	safego.Go(ctx, "eval-chat-runner", func() {
		defer close(ch)
		output, err := r.runFn(ctx, r.agentID, message.Content)
		if err != nil {
			ch <- &event.Event{
				Response: &model.Response{
					Done:  true,
					Error: &model.ResponseError{Message: err.Error(), Type: model.ErrorTypeAPIError},
				},
			}
			ch <- runnerCompletionEvent("")
			return
		}
		ch <- &event.Event{
			InvocationID: "eval-inv",
			Response: &model.Response{
				Done: true,
				Choices: []model.Choice{
					{Message: model.Message{Role: model.RoleAssistant, Content: output}},
				},
			},
		}
		ch <- runnerCompletionEvent("eval-inv")
	})
	return ch, nil
}

func (r *chatRunnerAdapter) Close() error { return nil }

func runnerCompletionEvent(invocationID string) *event.Event {
	return &event.Event{
		InvocationID: invocationID,
		Response: &model.Response{
			Object: model.ObjectTypeRunnerCompletion,
			Done:   true,
		},
	}
}

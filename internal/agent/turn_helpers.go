package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	sessiontrpc "aranea-agents/internal/session/trpc"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

type EventStreamResult struct {
	Reply         strings.Builder
	Reasoning     strings.Builder
	PromptTok     int
	CompletionTok int
	// HasError is true when at least one error event was observed during the turn.
	HasError bool
	// LastError records the last error message from the event stream.
	LastError string
	// HasContent is true when at least one message with non-empty text was received.
	HasContent bool
}

func NewRunnerDepsFromRuntime(trpcSession trpcsession.Service, memory trpcmemory.Service, artifact trpcartifact.Service, plugins ...trpcplugin.Plugin) TRPCRunnerDeps {
	deps := TRPCRunnerDeps{}
	if trpcSession != nil {
		deps.SessionService = trpcSession
	}
	if memory != nil {
		deps.MemoryService = memory
		deps.Ingestor = NewBizSessionIngestor(deps.MemoryService)
	}
	if artifact != nil {
		deps.ArtifactService = artifact
	}
	if deps.SessionService == nil {
		deps.SessionService = sessiontrpc.NewInMemorySessionService()
	}
	if len(plugins) > 0 {
		deps.Plugins = plugins
	}
	return deps
}

func ConsumeEventStream(
	ctx context.Context,
	events <-chan *trpcevent.Event,
	eventBus event.Bus,
	projectMeta ProjectMeta,
) EventStreamResult {
	var result EventStreamResult

	var projector *EventProjector
	if eventBus != nil {
		projector = NewEventProjector(eventBus)
		if projectMeta.TeamID != "" && len(projectMeta.MemberAgentKeys) > 0 {
			projector.memberStarted = make(map[string]bool)
		}
	}

	for ev := range events {
		if ctx.Err() != nil {
			return result
		}
		if ev == nil {
			continue
		}

		// Track error events so callers can surface them to users.
		if ev.Response != nil && ev.Response.Error != nil {
			result.HasError = true
			result.LastError = ev.Response.Error.Message
		}

		if projector != nil {
			projector.ProjectAndPublish(ctx, ev, projectMeta)
		}

		if ev.IsRunnerCompletion() {
			if ev.Response != nil && ev.Response.Usage != nil {
				result.PromptTok = ev.Response.Usage.PromptTokens
				result.CompletionTok = ev.Response.Usage.CompletionTokens
			}
			continue
		}

		// Runner completion events may still carry an error set.
		if ev.Response != nil && ev.Response.Error != nil {
			result.HasError = true
			result.LastError = ev.Response.Error.Message
			continue
		}

		if ev.Response == nil {
			continue
		}
		if usage := ev.Response.Usage; usage != nil {
			result.PromptTok = usage.PromptTokens
			result.CompletionTok = usage.CompletionTokens
		}
		for _, choice := range ev.Response.Choices {
			msg := choice.Message
			if text := strings.TrimSpace(msg.Content); text != "" {
				_ = provider.VisibleStreamingDelta(&result.Reply, text)
				result.HasContent = true
			}
			if rc := strings.TrimSpace(msg.ReasoningContent); rc != "" {
				_ = provider.VisibleStreamingDelta(&result.Reasoning, rc)
				result.HasContent = true
			}
		}
	}

	return result
}

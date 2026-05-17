package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/internal/provider"
	sessiontrpc "aranea-agents/internal/session/trpc"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

type EventStreamResult struct {
	Reply         strings.Builder
	Reasoning     strings.Builder
	PromptTok     int
	CompletionTok int
}

func NewRunnerDepsFromRuntime(trpcSession trpcsession.Service, sessionMemory *sessionmemory.Store, plugins ...trpcplugin.Plugin) TRPCRunnerDeps {
	deps := TRPCRunnerDeps{}
	if trpcSession != nil {
		deps.SessionService = trpcSession
	}
	if sessionMemory != nil {
		deps.MemoryService = memtrpc.NewSQLiteMemoryService(sessionMemory)
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
	}

	for ev := range events {
		if ctx.Err() != nil {
			return result
		}
		if ev == nil {
			continue
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
			}
			if rc := strings.TrimSpace(msg.ReasoningContent); rc != "" {
				_ = provider.VisibleStreamingDelta(&result.Reasoning, rc)
			}
		}
	}

	return result
}

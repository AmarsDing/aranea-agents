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

// MemberTokenUsage is per team member (agent_key) usage observed in the event stream.
type MemberTokenUsage struct {
	PromptTokens     int
	CompletionTokens int
}

type EventStreamResult struct {
	Reply         strings.Builder
	Reasoning     strings.Builder
	PromptTok     int
	CompletionTok int
	// MemberUsage maps agent_key → latest usage from member completion events (Team parallel/swarm).
	MemberUsage map[string]MemberTokenUsage
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
	return ConsumeEventStreamWithFirstByte(ctx, ctx, events, eventBus, projectMeta, nil)
}

func ConsumeEventStreamWithFirstByte(
	firstByteCtx context.Context,
	turnCtx context.Context,
	events <-chan *trpcevent.Event,
	eventBus event.Bus,
	projectMeta ProjectMeta,
	firstByteReceived *bool,
) EventStreamResult {
	var result EventStreamResult

	var projector *EventProjector
	if eventBus != nil {
		projector = NewEventProjector(eventBus)
		if projectMeta.TeamID != "" && len(projectMeta.MemberAgentKeys) > 0 {
			projector.memberStarted = make(map[string]bool)
		}
	}

	received := false
	for ev := range events {
		if turnCtx.Err() != nil {
			return result
		}
		if !received {
			received = true
			if firstByteReceived != nil {
				*firstByteReceived = true
			}
		}
		if firstByteCtx.Err() != nil && !received {
			return result
		}
		if ev == nil {
			continue
		}

		if ev.Response != nil && ev.Response.Error != nil {
			result.HasError = true
			result.LastError = ev.Response.Error.Message
		}

		if projector != nil {
			projector.ProjectAndPublish(turnCtx, ev, projectMeta)
		}

		if ev.IsRunnerCompletion() {
			if ev.Response != nil && ev.Response.Usage != nil {
				result.PromptTok = ev.Response.Usage.PromptTokens
				result.CompletionTok = ev.Response.Usage.CompletionTokens
			}
			continue
		}

		if ev.Response != nil && ev.Response.Error != nil {
			result.HasError = true
			result.LastError = ev.Response.Error.Message
			continue
		}

		if ev.Response == nil {
			continue
		}
		if usage := ev.Response.Usage; usage != nil {
			accumulateStreamUsage(&result, ev, projectMeta, usage.PromptTokens, usage.CompletionTokens)
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

func accumulateStreamUsage(result *EventStreamResult, ev *trpcevent.Event, meta ProjectMeta, promptTok, completionTok int) {
	if result == nil {
		return
	}
	result.PromptTok = promptTok
	result.CompletionTok = completionTok
	if !isTeamMemberAuthor(ev.Author, meta) {
		return
	}
	key := strings.TrimSpace(ev.Author)
	if key == "" {
		return
	}
	if result.MemberUsage == nil {
		result.MemberUsage = make(map[string]MemberTokenUsage)
	}
	prev := result.MemberUsage[key]
	if promptTok >= prev.PromptTokens || completionTok >= prev.CompletionTokens {
		result.MemberUsage[key] = MemberTokenUsage{
			PromptTokens:     promptTok,
			CompletionTokens: completionTok,
		}
	}
}

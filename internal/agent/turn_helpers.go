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

type StreamConsumeOptions struct {
	MetaResolver      ActivityMetaResolver
	ActivityPersister ActivityPersister
}

func ConsumeEventStream(
	ctx context.Context,
	events <-chan *trpcevent.Event,
	eventBus event.Bus,
	projectMeta ProjectMeta,
	opts *StreamConsumeOptions,
) EventStreamResult {
	return ConsumeEventStreamWithFirstByte(ctx, ctx, events, eventBus, projectMeta, nil, opts)
}

func ConsumeEventStreamWithFirstByte(
	firstByteCtx context.Context,
	turnCtx context.Context,
	events <-chan *trpcevent.Event,
	eventBus event.Bus,
	projectMeta ProjectMeta,
	firstByteReceived *bool,
	opts *StreamConsumeOptions,
) EventStreamResult {
	var result EventStreamResult

	var projector *EventProjector
	if eventBus != nil {
		projector = NewEventProjector(eventBus)
		if projectMeta.TeamID != "" && len(projectMeta.MemberAgentKeys) > 0 {
			projector.memberStarted = make(map[string]bool)
		}
		if opts != nil {
			projector.Configure(projectMeta, opts.MetaResolver)
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
			envelopes := projector.Project(turnCtx, ev, projectMeta)
			for _, env := range envelopes {
				eventBus.Publish(turnCtx, env)
			}
			if opts != nil {
				PublishActivityEnvelopes(turnCtx, projectMeta, opts.ActivityPersister, envelopes)
			}
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

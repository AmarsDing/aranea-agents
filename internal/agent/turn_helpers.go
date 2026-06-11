package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/event"
	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/pkg/loggateway"

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
	// MemberToolCalls maps agent_key → tool_call envelope count observed during the turn.
	MemberToolCalls map[string]int
	// HasError is true when at least one error event was observed during the turn.
	HasError bool
	// LastError records the last error message from the event stream.
	LastError string
	// HasContent is true when at least one message with non-empty text was received.
	HasContent bool
}

func NewRunnerDepsFromRuntime(trpcSession trpcsession.Service, memory trpcmemory.Service, artifact trpcartifact.Service, plugins ...trpcplugin.Plugin) TRPCRunnerDeps {
	return NewRunnerDepsFromRuntimeWithLogger(trpcSession, memory, artifact, nil, plugins...)
}

func NewRunnerDepsFromRuntimeWithLogger(trpcSession trpcsession.Service, memory trpcmemory.Service, artifact trpcartifact.Service, lg loggateway.Logger, plugins ...trpcplugin.Plugin) TRPCRunnerDeps {
	deps := TRPCRunnerDeps{}
	if trpcSession != nil {
		deps.SessionService = trpcSession
	}
	if memory != nil {
		deps.MemoryService = memory
		deps.Ingestor = NewBizSessionIngestor(deps.MemoryService, lg)
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
	lg loggateway.Logger,
) EventStreamResult {
	return ConsumeEventStreamWithFirstByte(ctx, ctx, events, eventBus, projectMeta, nil, opts, lg)
}

func ConsumeEventStreamWithFirstByte(
	firstByteCtx context.Context,
	turnCtx context.Context,
	events <-chan *trpcevent.Event,
	eventBus event.Bus,
	projectMeta ProjectMeta,
	firstByteReceived *bool,
	opts *StreamConsumeOptions,
	lg loggateway.Logger,
) EventStreamResult {
	consumer := newTurnStreamConsumer(firstByteCtx, turnCtx, eventBus, projectMeta, firstByteReceived, opts, lg)
	return consumer.consume(events)
}

func accumulateStreamUsage(result *EventStreamResult, ev *trpcevent.Event, meta ProjectMeta, promptTok, completionTok int) {
	if result == nil {
		return
	}
	if promptTok > result.PromptTok {
		result.CompletionTok = completionTok
		result.PromptTok = promptTok
	} else if promptTok == result.PromptTok && completionTok > result.CompletionTok {
		result.CompletionTok = completionTok
	}
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
	if promptTok > prev.PromptTokens {
		result.MemberUsage[key] = MemberTokenUsage{
			PromptTokens:     promptTok,
			CompletionTokens: completionTok,
		}
	} else if promptTok == prev.PromptTokens && completionTok > prev.CompletionTokens {
		result.MemberUsage[key] = MemberTokenUsage{
			PromptTokens:     promptTok,
			CompletionTokens: completionTok,
		}
	}
}

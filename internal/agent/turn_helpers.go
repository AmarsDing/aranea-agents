package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/v2"
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
	// CachedTokens is the cache-hit portion of PromptTokens (DeepSeek/OpenAI
	// prompt caching). Billed at the cache-read price downstream.
	CachedTokens int
}

type EventStreamResult struct {
	Reply         strings.Builder
	Reasoning     strings.Builder
	PromptTok     int
	CompletionTok int
	// CachedTok is the cache-hit portion of PromptTok (max across rounds,
	// same accumulation semantics as PromptTok).
	CachedTok int
	// UsageSource indicates where PromptTok/CompletionTok came from:
	//   ""                   — no usage data observed (stream errored before usage emission; TECH-DEBT)
	//   "streaming"          — accumulated from streaming chat.completion events
	//   "runner_completion"  — final usage from RunnerCompletion event (most accurate)
	//   "estimated"          — filled by EstimateTokensIfMissing from text (rough estimate)
	// Useful for diagnosing why tokens=0 or why prompt≈completion (estimation fallback).
	UsageSource string
	// prevRoundsCompletionTok tracks the sum of completion tokens from
	// previous LLM rounds (when promptTok increased). CompletionTok is
	// always prevRoundsCompletionTok + current round's max completion.
	// This enables correct multi-round accumulation: prompt tokens are
	// cumulative (take max), but completion tokens are per-round (sum).
	prevRoundsCompletionTok int
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
	// FirstByteTimedOut is true when the first-byte guard fired because the
	// provider produced no meaningful stream event before the deadline.
	// Distinct from a user cancel: AbortOnStall may cancel the run context
	// afterwards, so callers must not treat parentCtx.Err() alone as "user abort".
	FirstByteTimedOut bool
	// DoomLoopDetected is true when the stream consumer aborted the turn after
	// detecting repetitive LLM output (doom loop). Callers should treat the
	// reply as truncated and may retry with different sampling parameters.
	DoomLoopDetected bool
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
	V2Projector *v2.ActivityProjector // v2 phase: projects runtime events into v2 events (Step/Task/Turn)
	// AbortOnStall cancels the LLM HTTP request when the first-byte deadline
	// fires with no meaningful event. Without this, the 60-minute task HTTP
	// timeout keeps the stream channel silent and the guard cannot wake.
	AbortOnStall context.CancelFunc
}

func ConsumeEventStream(
	ctx context.Context,
	events <-chan *trpcevent.Event,
	projectMeta ProjectMeta,
	opts *StreamConsumeOptions,
	lg loggateway.Logger,
) EventStreamResult {
	return ConsumeEventStreamWithFirstByte(ctx, ctx, events, projectMeta, nil, opts, lg)
}

func ConsumeEventStreamWithFirstByte(
	firstByteCtx context.Context,
	turnCtx context.Context,
	events <-chan *trpcevent.Event,
	projectMeta ProjectMeta,
	firstByteReceived *bool,
	opts *StreamConsumeOptions,
	lg loggateway.Logger,
) EventStreamResult {
	consumer := newTurnStreamConsumer(firstByteCtx, turnCtx, projectMeta, firstByteReceived, opts, lg)
	return consumer.consume(events)
}

func accumulateStreamUsage(result *EventStreamResult, ev *trpcevent.Event, meta ProjectMeta, promptTok, completionTok, cachedTok int) {
	if result == nil {
		return
	}
	// Cached tokens share prompt-token semantics: reported cumulatively
	// within a round, so take the max.
	if cachedTok > result.CachedTok {
		result.CachedTok = cachedTok
	}
	// Multi-round accumulation strategy:
	// - Prompt tokens are cumulative across rounds (each round includes prior
	//   context), so we take the max.
	// - Completion tokens are per-round, so we sum them across rounds.
	// - Within a single round (streaming chunks), usage is reported cumulatively,
	//   so we take the max for that round.
	// prevRoundsCompletionTok tracks the locked-in total from prior rounds;
	// CompletionTok = prevRoundsCompletionTok + current round's max completion.
	if promptTok > result.PromptTok {
		// New LLM round detected: lock in previous rounds' total.
		result.prevRoundsCompletionTok = result.CompletionTok
		result.PromptTok = promptTok
		result.CompletionTok = result.prevRoundsCompletionTok + completionTok
	} else if promptTok == result.PromptTok && completionTok > (result.CompletionTok-result.prevRoundsCompletionTok) {
		// Same round, streaming update: take max completion for this round.
		result.CompletionTok = result.prevRoundsCompletionTok + completionTok
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
	if cachedTok > prev.CachedTokens {
		prev.CachedTokens = cachedTok
		result.MemberUsage[key] = prev
	}
	if promptTok > prev.PromptTokens {
		result.MemberUsage[key] = MemberTokenUsage{
			PromptTokens:     promptTok,
			CompletionTokens: prev.CompletionTokens + completionTok,
			CachedTokens:     prev.CachedTokens,
		}
	} else if promptTok == prev.PromptTokens && completionTok > prev.CompletionTokens {
		result.MemberUsage[key] = MemberTokenUsage{
			PromptTokens:     promptTok,
			CompletionTokens: completionTok,
			CachedTokens:     prev.CachedTokens,
		}
	}
}

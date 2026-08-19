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
// PromptTokens/CompletionTokens/CachedTokens are billing totals summed across
// LLM rounds (each tool-call round is a separate billed API call).
type MemberTokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	// CachedTokens is the cache-hit portion of PromptTokens (DeepSeek/OpenAI
	// prompt caching). Billed at the cache-read price downstream.
	CachedTokens int
	// Per-round accumulators: within a round usage is reported cumulatively
	// (max); on a round boundary the previous round's values are locked into
	// the prev* sums.
	prevPrompt, prevCompletion, prevCached int
	curPrompt, curCompletion, curCached    int
}

// Usage source values for EventStreamResult.UsageSource. Persisted into
// usage.metadata_json["usage_source"] so DB rows whose tokens came from text
// estimation stay distinguishable from provider-reported usage (P0-2).
const (
	// UsageSourceStreaming — accumulated from streaming chat.completion events.
	UsageSourceStreaming = "streaming"
	// UsageSourceRunnerCompletion — final usage from a RunnerCompletion event.
	UsageSourceRunnerCompletion = "runner_completion"
	// UsageSourceEstimated — filled by EstimateTokensIfMissing from text (rough).
	UsageSourceEstimated = "estimated"
)

type EventStreamResult struct {
	Reply         strings.Builder
	Reasoning     strings.Builder
	// PromptTok/CompletionTok/CachedTok are BILLING totals summed across all
	// LLM rounds of the turn (each tool-call round re-sends the full prompt
	// and is billed separately). Use these for cost/quota recording.
	PromptTok     int
	CompletionTok int
	// CachedTok is the cache-hit portion of PromptTok (summed across rounds).
	CachedTok int
	// LastRoundPromptTok/LastRoundCompletionTok describe the FINAL LLM round
	// only — i.e. the current context-window occupancy after the turn. Use
	// these (not the billing totals) for context_used_tokens patching and
	// context occupancy display.
	LastRoundPromptTok     int
	LastRoundCompletionTok int
	// UsageSource indicates where PromptTok/CompletionTok came from:
	//   ""                   — no usage data observed (stream errored before usage emission; TECH-DEBT)
	//   "streaming"          — accumulated from streaming chat.completion events
	//   "runner_completion"  — final usage from RunnerCompletion event (currently
	//                          never fires: the framework does not attach Usage to
	//                          the runner.completion event; kept for forward compat)
	//   "estimated"          — filled by EstimateTokensIfMissing from text (rough estimate)
	// Useful for diagnosing why tokens=0 or why prompt≈completion (estimation fallback).
	UsageSource string
	// Per-round billing accumulators. Within one LLM round, streaming usage
	// is reported cumulatively (track the max); when a new round is detected
	// (prompt value changes), the previous round's maxima are locked into
	// the prevRounds* sums. Exported totals are always prevRounds* + curRound*.
	prevRoundsPromptTok     int
	prevRoundsCompletionTok int
	prevRoundsCachedTok     int
	curRoundPromptTok       int
	curRoundCompletionTok   int
	curRoundCachedTok       int
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
	// Billing semantics (2026-08-19): every LLM round of a turn is a separate
	// API call billed at its FULL prompt, so Prompt/Cached/Completion tokens
	// are SUMMED across rounds (the previous max-based prompt accumulation
	// undercounted input cost ~round-count-fold on tool-call-heavy turns).
	// Within one round, streaming usage is reported cumulatively → per-round
	// values track the max.
	// Round boundary: a usage payload whose prompt differs from the current
	// round's prompt marks a new billable call (prompt grows across rounds in
	// tool loops; it shrinks after mid-run compaction — both are detected).
	if promptTok != result.curRoundPromptTok {
		result.prevRoundsPromptTok += result.curRoundPromptTok
		result.prevRoundsCompletionTok += result.curRoundCompletionTok
		result.prevRoundsCachedTok += result.curRoundCachedTok
		result.curRoundPromptTok = promptTok
		result.curRoundCompletionTok = completionTok
		result.curRoundCachedTok = cachedTok
	} else {
		if completionTok > result.curRoundCompletionTok {
			result.curRoundCompletionTok = completionTok
		}
		if cachedTok > result.curRoundCachedTok {
			result.curRoundCachedTok = cachedTok
		}
	}
	result.PromptTok = result.prevRoundsPromptTok + result.curRoundPromptTok
	result.CompletionTok = result.prevRoundsCompletionTok + result.curRoundCompletionTok
	result.CachedTok = result.prevRoundsCachedTok + result.curRoundCachedTok
	result.LastRoundPromptTok = result.curRoundPromptTok
	result.LastRoundCompletionTok = result.curRoundCompletionTok
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
	// Per-member usage follows the same per-round billing semantics as the
	// root totals above.
	mu := result.MemberUsage[key]
	if promptTok != mu.curPrompt {
		mu.prevPrompt += mu.curPrompt
		mu.prevCompletion += mu.curCompletion
		mu.prevCached += mu.curCached
		mu.curPrompt = promptTok
		mu.curCompletion = completionTok
		mu.curCached = cachedTok
	} else {
		if completionTok > mu.curCompletion {
			mu.curCompletion = completionTok
		}
		if cachedTok > mu.curCached {
			mu.curCached = cachedTok
		}
	}
	mu.PromptTokens = mu.prevPrompt + mu.curPrompt
	mu.CompletionTokens = mu.prevCompletion + mu.curCompletion
	mu.CachedTokens = mu.prevCached + mu.curCached
	result.MemberUsage[key] = mu
}

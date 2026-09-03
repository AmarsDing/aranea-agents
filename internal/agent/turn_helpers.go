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
	// FirstTokenMs is the turn TTFT (first meaningful model byte relative to
	// consume start), in milliseconds. 0 means no first byte was observed
	// (stream errored/stalled before any model output).
	FirstTokenMs int
	// ModelCallCount counts distinct LLM rounds observed in the stream
	// (deduped by response ID). Tool-call loops produce one per round.
	ModelCallCount int
	// ToolCallCount counts distinct tool calls requested by the model
	// (deduped by tool-call ID across streaming deltas).
	ToolCallCount int
	// Per-author round state for NON-member authors (team orchestrator /
	// planner / single-agent); member authors live in MemberUsage. Splitting
	// by author is required under parallel member execution (2026-08-27):
	// interleaved cumulative usage reports from different members would
	// otherwise trip a shared round-boundary detection and double count
	// (A(10k)→B(8k)→A(10k same-round re-report) counted A's round twice —
	// inflating both billing totals and the run token-budget gate).
	rootUsage map[string]*MemberTokenUsage
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
	// OnPromptTokensAccumulated, when non-nil, is invoked synchronously from
	// the consume loop after each usage event with the incremental billed
	// prompt tokens observed (delta of the running billing total, always >0).
	// Used by the team run-level token budget gate to accumulate mid-stream
	// (2026-08-26 M80 fix): under graph runtime every persisted member usage
	// row carries an attribution marker, so the recordMemberUsage accumulate
	// branch (attribution=="") never fires and the gate would never trip.
	// Keep the hook cheap — it runs on the single consume goroutine.
	OnPromptTokensAccumulated func(deltaPromptTokens int)
	// OnStreamActivity, when non-nil, is invoked synchronously from the consume
	// loop on every event (text delta, tool call, usage, completion…). P2-1
	// (2026-09-03): the team runner wires this to a throttled heartbeat writer
	// so a long single-step streaming generation still signals liveness to the
	// biz idle probe (LangGraph progress-signal / Temporal heartbeat 语义).
	// Keep the hook cheap — it runs on the single consume goroutine; throttle
	// inside the closure.
	OnStreamActivity func()
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

// accumulateRound applies the per-round billing semantics to one author's
// round state: within one LLM round streaming usage is reported cumulatively
// (track the max); a prompt value differing from the current round's marks a
// new billable call (prompt grows across rounds in tool loops; it shrinks
// after mid-run compaction — both are detected), locking the previous round's
// maxima into the prev* sums.
func accumulateRound(u *MemberTokenUsage, promptTok, completionTok, cachedTok int) {
	if promptTok != u.curPrompt {
		u.prevPrompt += u.curPrompt
		u.prevCompletion += u.curCompletion
		u.prevCached += u.curCached
		u.curPrompt = promptTok
		u.curCompletion = completionTok
		u.curCached = cachedTok
	} else {
		if completionTok > u.curCompletion {
			u.curCompletion = completionTok
		}
		if cachedTok > u.curCached {
			u.curCached = cachedTok
		}
	}
	u.PromptTokens = u.prevPrompt + u.curPrompt
	u.CompletionTokens = u.prevCompletion + u.curCompletion
	u.CachedTokens = u.prevCached + u.curCached
}

func accumulateStreamUsage(result *EventStreamResult, ev *trpcevent.Event, meta ProjectMeta, promptTok, completionTok, cachedTok int) {
	if result == nil {
		return
	}
	// Billing semantics (2026-08-19): every LLM round of a turn is a separate
	// API call billed at its FULL prompt, so Prompt/Cached/Completion tokens
	// are SUMMED across rounds (the previous max-based prompt accumulation
	// undercounted input cost ~round-count-fold on tool-call-heavy turns).
	//
	// Round state is tracked PER AUTHOR (2026-08-27): members execute in
	// parallel and their cumulative usage reports interleave on the single
	// consume loop — a shared round state would false-detect round boundaries
	// and double count (see rootUsage field comment).
	author := strings.TrimSpace(ev.Author)
	var lastPrompt, lastCompletion int
	if isTeamMemberAuthor(ev.Author, meta) && author != "" {
		if result.MemberUsage == nil {
			result.MemberUsage = make(map[string]MemberTokenUsage)
		}
		mu := result.MemberUsage[author]
		accumulateRound(&mu, promptTok, completionTok, cachedTok)
		result.MemberUsage[author] = mu
		lastPrompt, lastCompletion = mu.curPrompt, mu.curCompletion
	} else {
		if result.rootUsage == nil {
			result.rootUsage = make(map[string]*MemberTokenUsage)
		}
		ru := result.rootUsage[author]
		if ru == nil {
			ru = &MemberTokenUsage{}
			result.rootUsage[author] = ru
		}
		accumulateRound(ru, promptTok, completionTok, cachedTok)
		lastPrompt, lastCompletion = ru.curPrompt, ru.curCompletion
	}
	// Billing totals = sum over every author's billed rounds.
	p, cpl, cch := 0, 0, 0
	for _, mu := range result.MemberUsage {
		p += mu.PromptTokens
		cpl += mu.CompletionTokens
		cch += mu.CachedTokens
	}
	for _, ru := range result.rootUsage {
		p += ru.PromptTokens
		cpl += ru.CompletionTokens
		cch += ru.CachedTokens
	}
	result.PromptTok = p
	result.CompletionTok = cpl
	result.CachedTok = cch
	// Context occupancy = current round of the author that produced this
	// usage event (last reporter wins; RunnerCompletion may overwrite).
	result.LastRoundPromptTok = lastPrompt
	result.LastRoundCompletionTok = lastCompletion
}

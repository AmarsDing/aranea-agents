package usage

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"

	"aranea-agents/pkg/loggateway"
)

// TurnUsageInput captures the data needed to record usage for a single chat turn.
type TurnUsageInput struct {
	SessionID     string
	RunID         string
	AgentKey      string
	AgentID       string
	Provider      string
	Model         string
	Status        string
	PromptTok     int
	CompletionTok int
	// CachedTok is the cache-hit portion of PromptTok (billed at cache-read price).
	CachedTok    int
	Latency      time.Duration
	ErrMsg       string
	MetadataJSON string
	TraceID      string
	// TimeToFirstTokenMS is stream-observed TTFT; 0 means unobserved.
	TimeToFirstTokenMS int
	// WaitMS is HITL confirmation wait accumulated on the turn ctx.
	WaitMS int
}

// RecordTurnUsage records token usage for a completed chat turn.
// It persists the usage event, accumulates session metrics, publishes an
// envelope, and links the runner completion row.
func (u *Usecase) RecordTurnUsage(ctx context.Context, in TurnUsageInput) error {
	if u == nil {
		return nil
	}
	now := u.now()
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	meta = MergeWaitMSMetadata(meta, in.WaitMS, int(in.Latency.Milliseconds()))
	usageID := newUsageID()
	latencyMS := int(in.Latency.Milliseconds())
	ev := TokenUsageEvent{
		ID:                 usageID,
		SessionID:          in.SessionID,
		AgentKey:           in.AgentKey,
		AgentID:            in.AgentID,
		ModelAPIID:         in.Model,
		ModelDisplayName:   in.Model,
		ProviderCode:       in.Provider,
		InputTokens:        in.PromptTok,
		OutputTokens:       in.CompletionTok,
		CachedInputTokens:  in.CachedTok,
		TotalTokens:        in.PromptTok + in.CompletionTok,
		LatencyMS:          latencyMS,
		TimeToFirstTokenMS: in.TimeToFirstTokenMS,
		WaitMS:             in.WaitMS,
		ModelLatencyMS:     ModelLatencyMS(latencyMS, in.WaitMS),
		Status:             in.Status,
		UsageKind:          KindChatTurn,
		MetadataJSON:       meta,
		OccurredAt:         now.Format(time.RFC3339),
		DateKey:            now.Format("2006-01-02"),
		HourKey:            now.Format("2006-01-02T15"),
		ErrorMessage:       in.ErrMsg,
	}
	if in.RunID != "" {
		ev.MessageID = in.RunID
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(usageRecordTimeoutSec)*time.Second)
	defer cancel()

	recEv, err := u.RecordTokenUsageEvent(recCtx, ev)
	if err != nil {
		return err
	}
	if u.sessAccum != nil && strings.TrimSpace(in.SessionID) != "" {
		u.sessAccum.AccumulateMetricsDelta(SessionMetricsDelta{
			SessionID:         in.SessionID,
			ModelCallCount:    recEv.CallCount,
			InputTokens:       int64(recEv.InputTokens),
			OutputTokens:      int64(recEv.OutputTokens),
			TotalTokens:       int64(recEv.TotalTokens),
			TotalCostMicroUsd: recEv.TotalCostMicroUSD,
		})
	}

	if u.completion != nil && strings.TrimSpace(in.SessionID) != "" && strings.TrimSpace(in.RunID) != "" {
		linkCtx, linkCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(usageLinkTimeoutSec)*time.Second)
		defer linkCancel()
		if err := u.completion.LinkRunnerCompletionUsage(linkCtx, in.SessionID, in.RunID, usageID, in.TraceID); err != nil {
			u.lg.Warn("link runner completion usage failed", loggateway.Err(err), loggateway.Str("session_id", in.SessionID), loggateway.Str("run_id", in.RunID))
		}
	}
	return nil
}

// newUsageID returns a new UUID for usage events.
func newUsageID() string {
	return uuid.NewString()
}

// UsageSourceResponse marks provider-reported usage read from a non-streaming
// HTTP response body (intent pass / title generation / evolution calls).
// Complements the stream-path sources defined in internal/agent/turn_helpers.go.
const UsageSourceResponse = "response"

// AuxLLMUsageInput captures the data needed to record usage for an auxiliary
// (non-chat-turn) LLM call: subagent runs, session-title generation, intent
// pass classification, evolution drafting, etc. (P1-1, 2026-08-19).
type AuxLLMUsageInput struct {
	// Kind is one of KindAux*; required.
	Kind string
	// SessionID is optional — empty for session-less callers (e.g. evolution).
	// When set, session metrics are accumulated (same as chat turns).
	SessionID string
	// RunID is an optional correlation id (subagent run id / intent run id);
	// stored in MessageID for cross-referencing.
	RunID    string
	AgentID  string
	AgentKey string
	TeamID   string
	UserID   string
	Provider string
	Model    string
	Status   string
	// PromptTok/CompletionTok are provider-reported usage; CachedTok is the
	// cache-hit portion of PromptTok. When the caller only knows the total
	// (e.g. biz.LLMCaller collapses the breakdown), put it in CompletionTok
	// and leave PromptTok=0 — cost is then priced at the output rate, which
	// overestimates slightly; prefer plumbing the real breakdown where possible.
	PromptTok     int
	CompletionTok int
	CachedTok     int
	// EmbeddingTok is the embedding-call token count (KindAuxEmbedding only);
	// persisted into the EmbeddingTokens column and priced at the embedding
	// rate. Mutually exclusive with the Prompt/Completion split: embedding
	// calls have input-only consumption.
	EmbeddingTok int
	// UsageSource records how the token counts were obtained
	// (UsageSourceResponse / "streaming" / "estimated"); merged into
	// metadata_json["usage_source"].
	UsageSource string
	// Effort is the resolved thinking effort actually applied to the call
	// (off/low/medium/high/max; M83 LBG-6); merged into metadata_json["effort"].
	// Empty = call site did not route effort → key omitted.
	Effort       string
	Latency      time.Duration
	ErrMsg       string
	MetadataJSON string
}

// RecordAuxLLMUsage records token usage for an auxiliary LLM call.
// Like RecordTurnUsage it persists the event (with pricing enrichment and
// budget-alert scheduling), accumulates session metrics when SessionID is
// set, and publishes an envelope; unlike it there is no runner-completion
// row to link.
func (u *Usecase) RecordAuxLLMUsage(ctx context.Context, in AuxLLMUsageInput) error {
	if u == nil {
		return nil
	}
	if strings.TrimSpace(in.Kind) == "" {
		return apierror.BadRequest("USAGE", "kind is required")
	}
	now := u.now()
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	meta = MergeUsageSourceMetadata(meta, in.UsageSource)
	meta = MergeEffortMetadata(meta, in.Effort)
	ev := TokenUsageEvent{
		ID:                newUsageID(),
		SessionID:         in.SessionID,
		AgentKey:          in.AgentKey,
		AgentID:           in.AgentID,
		TeamID:            in.TeamID,
		UserID:            in.UserID,
		ModelAPIID:        in.Model,
		ModelDisplayName:  in.Model,
		ProviderCode:      in.Provider,
		InputTokens:       in.PromptTok,
		OutputTokens:      in.CompletionTok,
		CachedInputTokens: in.CachedTok,
		EmbeddingTokens:   in.EmbeddingTok,
		TotalTokens:       in.PromptTok + in.CompletionTok + in.EmbeddingTok,
		LatencyMS:         int(in.Latency.Milliseconds()),
		Status:            in.Status,
		UsageKind:         in.Kind,
		MetadataJSON:      meta,
		OccurredAt:        now.Format(time.RFC3339),
		DateKey:           now.Format("2006-01-02"),
		HourKey:           now.Format("2006-01-02T15"),
		ErrorMessage:      in.ErrMsg,
	}
	if in.RunID != "" {
		ev.MessageID = in.RunID
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(usageRecordTimeoutSec)*time.Second)
	defer cancel()
	recEv, err := u.RecordTokenUsageEvent(recCtx, ev)
	if err != nil {
		return err
	}
	if u.sessAccum != nil && strings.TrimSpace(in.SessionID) != "" {
		u.sessAccum.AccumulateMetricsDelta(SessionMetricsDelta{
			SessionID:         in.SessionID,
			ModelCallCount:    recEv.CallCount,
			InputTokens:       int64(recEv.InputTokens),
			OutputTokens:      int64(recEv.OutputTokens),
			TotalTokens:       int64(recEv.TotalTokens),
			TotalCostMicroUsd: recEv.TotalCostMicroUSD,
		})
	}

	return nil
}

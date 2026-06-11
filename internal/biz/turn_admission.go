package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"

	sessstatus "aranea-agents/internal/biz/session"
)

// ---------------------------------------------------------------------------
// TurnAdmissionUsecase — centralizes the pre-turn admission rules that were
// previously scattered across internal/service:
//
//  1. Chat turn quota checks (agent + user + global) — was enforceChatTurnQuotas.
//  2. Team member quota checks — was CheckTeamMemberQuotas.
//  3. Context-pressure evaluation — was sessionContextPressure.
//
// These are pure business rules. They do not own any goroutines, transports,
// or framework types. The service layer is expected to call Admit() once per
// inbound turn and short-circuit on any non-nil error or non-empty decision.
//
// The usecase depends only on three narrow ports: a QuotaEnforcer (for
// quota checks), an AgentHydrator (for the agent L0 threshold fallback),
// and a ContextThresholdResolver (the service layer's policy for picking
// the right threshold per session — typically channel long-task config
// for IM sessions, the agent's L0 threshold otherwise).
// ---------------------------------------------------------------------------

// QuotaEnforcer abstracts the UsageUsecase quota primitives so the admission
// usecase can be unit-tested without instantiating the full UsageUsecase.
type QuotaEnforcer interface {
	CheckQuota(ctx context.Context, scopeType, scopeID string) (UsageQuotaCheck, error)
	CheckTeamMemberQuotas(ctx context.Context, teamID string) error
}

// AgentHydrator loads a hydrated Agent for context-threshold resolution.
// In tests this can be a stub that returns an Agent with custom Settings.
type AgentHydrator interface {
	GetAgentByID(ctx context.Context, id string) (Agent, error)
}

// ContextThresholdResolver returns the admission threshold for a session.
// The service layer implements this to encode the full lookup policy
// (channel long-task config for IM sessions, agent L0 threshold
// otherwise). The biz layer never inspects session metadata directly.
// A nil-safe no-op resolver always returns DefaultContextAdmissionThreshold.
type ContextThresholdResolver interface {
	ResolveContextAdmissionThreshold(ctx context.Context, sess Session) float64
}

// ThresholdResolverFunc is the function-type adapter for
// ContextThresholdResolver. Allows callers to pass a closure directly
// (e.g. orchestrator-bound methods) without declaring a new struct.
type ThresholdResolverFunc func(ctx context.Context, sess Session) float64

// ResolveContextAdmissionThreshold implements ContextThresholdResolver.
func (f ThresholdResolverFunc) ResolveContextAdmissionThreshold(ctx context.Context, sess Session) float64 {
	if f == nil {
		return DefaultContextAdmissionThreshold
	}
	return f(ctx, sess)
}

// TurnAdmissionUsecaseConfig configures the TurnAdmissionUsecase constructor.
type TurnAdmissionUsecaseConfig struct {
	// Quota is the quota enforcer. Required; nil = quotas are no-ops.
	Quota QuotaEnforcer
	// Agents is the agent hydrator. Used as a fallback when the threshold
	// resolver returns <= 0. Nil = use the default threshold.
	Agents AgentHydrator
	// ThresholdResolver is consulted for every EvaluateContextPressure call.
	// Optional. When nil the usecase falls back to the default
	// DefaultContextAdmissionThreshold.
	ThresholdResolver ContextThresholdResolver
}

// TurnAdmissionUsecase is the biz-layer entry point for turn admission rules.
// Construct via NewTurnAdmissionUsecase; methods are safe for concurrent use
// as long as the underlying Quota/Agents/ThresholdResolver are safe.
type TurnAdmissionUsecase struct {
	quota             QuotaEnforcer
	agents            AgentHydrator
	thresholdResolver ContextThresholdResolver
}

// NewTurnAdmissionUsecase constructs a TurnAdmissionUsecase from dependencies.
// Any nil dependency is treated as "feature disabled" (see method docstrings).
func NewTurnAdmissionUsecase(cfg TurnAdmissionUsecaseConfig) *TurnAdmissionUsecase {
	return &TurnAdmissionUsecase{
		quota:             cfg.Quota,
		agents:            cfg.Agents,
		thresholdResolver: cfg.ThresholdResolver,
	}
}

// EnforceChatTurnQuotas validates agent/user/global scopes before a chat turn.
// A nil Quota or empty scopeID is a no-op. Returns the first failing scope as
// a kratos Forbidden error (USAGE_QUOTA) so the service layer can pass it
// through to the transport unchanged.
//
// This is the canonical replacement for the legacy service.enforceChatTurnQuotas.
func (u *TurnAdmissionUsecase) EnforceChatTurnQuotas(ctx context.Context, agentID, userID string) error {
	if u == nil || u.quota == nil {
		return nil
	}
	if err := u.enforceScope(ctx, "agent", agentID); err != nil {
		return err
	}
	if err := u.enforceScope(ctx, "user", userID); err != nil {
		return err
	}
	return u.enforceScope(ctx, QuotaScopeGlobal, GlobalQuotaScopeID)
}

// EnforceTeamMemberQuotas validates that all enabled team members are within
// their agent-scope quota. Nil-safe: returns nil when teamID is blank or the
// quota enforcer is not configured.
func (u *TurnAdmissionUsecase) EnforceTeamMemberQuotas(ctx context.Context, teamID string) error {
	if u == nil || u.quota == nil {
		return nil
	}
	return u.quota.CheckTeamMemberQuotas(ctx, teamID)
}

// ContextPressureResult reports whether the session is under context pressure
// and the resolved threshold used for the decision. Callers should treat a true
// Pressure result as "tighten admission" — usually by forcing queue/reject.
type ContextPressureResult struct {
	Pressure  bool
	Threshold float64
}

// EvaluateContextPressure determines whether the current session context usage
// should tighten admission. The threshold is resolved by the configured
// ContextThresholdResolver (or by direct agent lookup as a fallback).
// Nil-safe: returns Pressure=false when sessions are missing.
func (u *TurnAdmissionUsecase) EvaluateContextPressure(ctx context.Context, sess Session) ContextPressureResult {
	if u == nil || sess.ID == "" {
		return ContextPressureResult{}
	}
	threshold := u.resolveAdmissionThreshold(ctx, sess)
	return ContextPressureResult{
		Pressure:  ContextPressureActive(sess.ContextUsedRatio, threshold),
		Threshold: threshold,
	}
}

// SetThresholdResolver installs (or replaces) the threshold resolver used
// by EvaluateContextPressure. The service layer calls this once during
// orchestrator construction, binding the resolver to the orchestrator's
// own lookup policy. Nil-safe.
func (u *TurnAdmissionUsecase) SetThresholdResolver(r ContextThresholdResolver) {
	if u == nil {
		return
	}
	u.thresholdResolver = r
}

func (u *TurnAdmissionUsecase) resolveAdmissionThreshold(ctx context.Context, sess Session) float64 {
	if u == nil {
		return DefaultContextAdmissionThreshold
	}
	// Preferred path: ask the configured threshold resolver (service layer
	// implements this to consult channel long-task config + agent
	// L0SummaryThreshold). The resolver owns the lookup policy.
	if u.thresholdResolver != nil {
		if v := u.thresholdResolver.ResolveContextAdmissionThreshold(ctx, sess); v > 0 {
			return v
		}
	}
	// Fallback: consult the agent's L0SummaryThreshold directly. This path
	// is used in unit tests and in the rare case the service layer did
	// not wire a resolver.
	threshold := DefaultContextAdmissionThreshold
	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" || u.agents == nil {
		return threshold
	}
	ag, err := u.agents.GetAgentByID(ctx, agentID)
	if err != nil || ag.Settings == nil || ag.Settings.L0SummaryThreshold <= 0 {
		return threshold
	}
	return ag.Settings.L0SummaryThreshold
}

// enforceScope is the internal helper for EnforceChatTurnQuotas. Empty scopeID
// is treated as "not configured" and short-circuits to nil — matching the
// legacy service.enforceQuota behavior.
func (u *TurnAdmissionUsecase) enforceScope(ctx context.Context, scopeType, scopeID string) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil
	}
	check, err := u.quota.CheckQuota(ctx, scopeType, scopeID)
	if err != nil {
		return err
	}
	if !check.Allowed {
		return apierror.Forbidden("USAGE_QUOTA", check.Reason)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Compile-time assertions that the constants used in error messages and the
// quota enforcer surface are stable across the biz boundary.
// ---------------------------------------------------------------------------

var (
	_ = sessstatus.SessionStatus("")
	_ = QuotaScopeGlobal
	_ = GlobalQuotaScopeID
)

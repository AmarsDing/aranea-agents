package outbound

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

const (
	runtimeStateDeliveryChannel = "aranea.delivery.channel"
	runtimeStateDeliveryTarget  = "aranea.delivery.target"
)

var (
	sessionResolversMu sync.RWMutex
	sessionResolvers   []SessionResolver
)

// SessionResolver maps a session ID to an outbound delivery target.
//
// Product semantics: ok=false means "this resolver cannot resolve" and the
// chain continues. That includes Web Chat sessions with no channel metadata —
// they MUST return false, not an error or panic. Lookup/storage failures are
// also ok=false so the chain is not aborted, but they MUST be logged (see
// NewLoggingSessionResolver) so they stay observable.
type SessionResolver func(sessionID string) (DeliveryTarget, bool)

func RegisterSessionResolver(fn SessionResolver) {
	if fn == nil {
		return
	}
	sessionResolversMu.Lock()
	defer sessionResolversMu.Unlock()
	sessionResolvers = append(sessionResolvers, fn)
}

// NewLoggingSessionResolver wraps a lookup that may fail. Lookup errors are
// logged and converted to ok=false (resolver-chain semantics). A nil error
// with an empty Channel/Target is also ok=false (session has no channel meta).
func NewLoggingSessionResolver(lg loggateway.Logger, lookup func(sessionID string) (DeliveryTarget, error)) SessionResolver {
	if lookup == nil {
		return func(string) (DeliveryTarget, bool) { return DeliveryTarget{}, false }
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return func(sessionID string) (DeliveryTarget, bool) {
		dt, err := lookup(sessionID)
		if err != nil {
			lg.Warn("outbound session resolver lookup failed",
				loggateway.StepID("outbound.resolve.session"),
				loggateway.Str("session_id", sessionID),
				loggateway.Err(err),
			)
			return DeliveryTarget{}, false
		}
		clean := sanitizeTarget(dt)
		if clean.Channel == "" || clean.Target == "" {
			return DeliveryTarget{}, false
		}
		return clean, true
	}
}

func ResolveTarget(ctx context.Context, explicit DeliveryTarget) (DeliveryTarget, error) {
	target := sanitizeTarget(explicit)
	target = fillFromRuntime(ctx, target)
	target = fillFromSession(ctx, target)
	if strings.TrimSpace(target.Channel) == "" || strings.TrimSpace(target.Target) == "" {
		return DeliveryTarget{}, apierror.BadRequest(apierror.DomainOutbound, "unable to resolve delivery target")
	}
	return target, nil
}

func RuntimeStateForTarget(target DeliveryTarget) map[string]any {
	clean := sanitizeTarget(target)
	if clean.Channel == "" || clean.Target == "" {
		return nil
	}
	return map[string]any{
		runtimeStateDeliveryChannel: clean.Channel,
		runtimeStateDeliveryTarget:  clean.Target,
	}
}

func sanitizeTarget(target DeliveryTarget) DeliveryTarget {
	return DeliveryTarget{
		Channel: strings.TrimSpace(target.Channel),
		Target:  strings.TrimSpace(target.Target),
	}
}

func fillFromRuntime(ctx context.Context, target DeliveryTarget) DeliveryTarget {
	if ctx == nil {
		return target
	}
	if target.Channel == "" {
		if v, ok := trpcagent.GetRuntimeStateValueFromContext[string](ctx, runtimeStateDeliveryChannel); ok {
			target.Channel = strings.TrimSpace(v)
		}
	}
	if target.Target == "" {
		if v, ok := trpcagent.GetRuntimeStateValueFromContext[string](ctx, runtimeStateDeliveryTarget); ok {
			target.Target = strings.TrimSpace(v)
		}
	}
	return target
}

func fillFromSession(ctx context.Context, target DeliveryTarget) DeliveryTarget {
	if ctx == nil {
		return target
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return target
	}
	resolved, ok := ResolveTargetFromSessionID(inv.Session.ID)
	if !ok {
		return target
	}
	if target.Channel == "" {
		target.Channel = resolved.Channel
	}
	if target.Target == "" {
		target.Target = resolved.Target
	}
	return target
}

func ResolveTargetFromSessionID(sessionID string) (DeliveryTarget, bool) {
	sessionResolversMu.RLock()
	resolvers := make([]SessionResolver, len(sessionResolvers))
	copy(resolvers, sessionResolvers)
	sessionResolversMu.RUnlock()
	for _, fn := range resolvers {
		if dt, ok := fn(sessionID); ok {
			return dt, true
		}
	}
	return DeliveryTarget{}, false
}

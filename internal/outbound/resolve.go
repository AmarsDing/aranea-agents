package outbound

import (
	"context"
	"strings"
	"sync"

	kerrors "github.com/go-kratos/kratos/v2/errors"

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

type SessionResolver func(sessionID string) (DeliveryTarget, bool)

func RegisterSessionResolver(fn SessionResolver) {
	sessionResolversMu.Lock()
	defer sessionResolversMu.Unlock()
	sessionResolvers = append(sessionResolvers, fn)
}

func ResolveTarget(ctx context.Context, explicit DeliveryTarget) (DeliveryTarget, error) {
	target := sanitizeTarget(explicit)
	target = fillFromRuntime(ctx, target)
	target = fillFromSession(ctx, target)
	if strings.TrimSpace(target.Channel) == "" || strings.TrimSpace(target.Target) == "" {
		return DeliveryTarget{}, kerrors.BadRequest("OUTBOUND", "unable to resolve delivery target")
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

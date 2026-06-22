package event

import (
	"context"

	"aranea-agents/internal/event/contract"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	frameworkbus "trpc.group/trpc-go/trpc-agent-go/event/bus"
)

// busAdapter wraps a framework bus.Bus[Envelope] to implement contract.Bus.
// This allows the project to delegate to the framework implementation while
// keeping the contract.Bus interface unchanged for all downstream consumers.
type busAdapter struct {
	inner frameworkbus.Bus[Envelope]
	lg    loggateway.Logger
}

// newBusAdapter creates a contract.Bus backed by the framework bus implementation.
func newBusAdapter(lg loggateway.Logger) contract.Bus {
	dropLogger := frameworkbus.DropLogger[Envelope](func(env Envelope, policy string, totalDrops uint64) {
		arametrics.EventBusDropped.WithLabelValues(string(env.Type), policy).Inc()
		if lg != nil {
			lg.Warn("[event_bus] drop",
				loggateway.Str("policy", policy),
				loggateway.Str("type", string(env.Type)),
				loggateway.Str("channel", env.Channel),
				loggateway.SessionID(env.SessionID),
				loggateway.Int64("total_drops", int64(totalDrops)),
			)
		}
	})

	inner := frameworkbus.New[Envelope](frameworkbus.WithDropLogger(dropLogger))
	return &busAdapter{inner: inner, lg: lg}
}

func (a *busAdapter) Publish(ctx context.Context, env Envelope) {
	if env.Channel == "" {
		env.Channel = RouteChannel(env)
	}
	arametrics.EventBusPublished.WithLabelValues(string(env.Type)).Inc()
	a.inner.Publish(ctx, env)
}

func (a *busAdapter) Subscribe(opts contract.SubscribeOptions) (<-chan Envelope, func()) {
	// Convert contract.SubscribeOptions to framework SubscribeOptions
	fwOpts := frameworkbus.SubscribeOptions[Envelope]{
		Priority:   convertPriority(opts.Priority),
		BufferSize: opts.BufferSize,
		Reliable:   opts.Reliable,
		DropPolicy: convertDropPolicy(opts.DropPolicy),
		BlockFor:   opts.BlockFor,
	}

	// Build filter from contract options
	var matchers []frameworkbus.EventMatcher[Envelope]
	if opts.SessionID != "" {
		matchers = append(matchers, func(env Envelope) bool {
			return env.SessionID == opts.SessionID
		})
	}
	if opts.TeamID != "" {
		matchers = append(matchers, func(env Envelope) bool {
			return env.TeamID == opts.TeamID
		})
	}
	if opts.Channel != "" {
		matchers = append(matchers, func(env Envelope) bool {
			return env.Channel == opts.Channel
		})
	}
	if opts.FilterKey != "" {
		matchers = append(matchers, func(env Envelope) bool {
			return contract.MatchFilterKey(opts.FilterKey, env.FilterKey)
		})
	}
	if len(opts.EventTypes) > 0 {
		typeSet := make(map[contract.EnvelopeType]bool, len(opts.EventTypes))
		for _, t := range opts.EventTypes {
			typeSet[t] = true
		}
		matchers = append(matchers, func(env Envelope) bool {
			return typeSet[env.Type]
		})
	}
	if opts.LevelFilter != "" {
		matchers = append(matchers, func(env Envelope) bool {
			if env.Type != EnvelopeTypeLog {
				return true
			}
			level, _ := env.Metadata["level"].(string)
			return frameworkbus.MatchLevelFilter(opts.LevelFilter, level)
		})
	}
	if opts.Selector != nil {
		matchers = append(matchers, func(env Envelope) bool {
			return opts.Selector(env.Type)
		})
	}

	if len(matchers) > 0 {
		fwOpts.Filter = func(env Envelope) bool {
			for _, m := range matchers {
				if !m(env) {
					return false
				}
			}
			return true
		}
	}

	return a.inner.Subscribe(fwOpts)
}

func (a *busAdapter) DropCount() uint64 {
	return a.inner.DropCount()
}

func convertPriority(p contract.ChannelPriority) frameworkbus.ChannelPriority {
	switch p {
	case contract.ChannelPriorityCritical:
		return frameworkbus.PriorityCritical
	default:
		return frameworkbus.PriorityNormal
	}
}

func convertDropPolicy(p contract.DropPolicy) frameworkbus.DropPolicy {
	switch p {
	case contract.DropNewest:
		return frameworkbus.DropNewest
	case contract.BlockUpTo:
		return frameworkbus.BlockUpTo
	default:
		return frameworkbus.DropOldest
	}
}

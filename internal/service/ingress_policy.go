package service

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime/turn"
)

// IngressDecision classifies how an inbound request should be handled before Turn execution.
type IngressDecision string

const (
	IngressAdmit           IngressDecision = "admit"
	IngressQueue           IngressDecision = "queue"
	IngressSteer           IngressDecision = "steer"
	IngressRejectBusy      IngressDecision = "reject_busy"
	IngressRouteAsync      IngressDecision = "route_async"
	IngressRouteBackground IngressDecision = "route_background"
	IngressCancel          IngressDecision = "cancel"
	IngressStatus          IngressDecision = "status"
	IngressSkipDuplicate   IngressDecision = "skip_duplicate"
)

// IngressPolicyInput is the pure input for L2 ingress policy evaluation.
type IngressPolicyInput struct {
	Text            string
	EntryPoint      biz.TurnEntryPoint
	AllowQueue      bool
	HasActiveRun    bool
	HasActiveRunner bool
	RouteAsync        bool
	IsCancelCommand    bool
	IsStatusQuery      bool
	IsBackgroundCommand bool
	IsRecentDuplicate  bool
	ContextPressure    bool
}

// IngressPolicyResult is the outcome of ingress policy evaluation.
type IngressPolicyResult struct {
	Decision       IngressDecision
	Intent         string
	SuggestDurable bool
}

// EvaluateIngressPolicy decides how an inbound message should be routed before Turn execution.
// It is a pure function: no I/O, no trpc imports (DECO-08).
func EvaluateIngressPolicy(in IngressPolicyInput) IngressPolicyResult {
	if in.IsRecentDuplicate {
		return IngressPolicyResult{Decision: IngressSkipDuplicate, Intent: "dedupe"}
	}
	if in.IsCancelCommand {
		return IngressPolicyResult{Decision: IngressCancel, Intent: "cancel"}
	}
	if in.IsBackgroundCommand {
		return IngressPolicyResult{Decision: IngressRouteBackground, Intent: "route_background"}
	}
	if in.RouteAsync {
		return IngressPolicyResult{Decision: IngressRouteAsync, Intent: "route_async"}
	}
	if in.IsStatusQuery && in.HasActiveRun {
		return IngressPolicyResult{Decision: IngressStatus, Intent: "status"}
	}
	if in.ContextPressure && in.HasActiveRun {
		if in.AllowQueue && in.EntryPoint != biz.EntryPointChannel {
			return IngressPolicyResult{Decision: IngressQueue, Intent: "context_force_queue"}
		}
		return IngressPolicyResult{Decision: IngressRejectBusy, Intent: "context_pressure"}
	}
	if in.HasActiveRun {
		allowQueue := in.AllowQueue
		if in.EntryPoint == "" {
			allowQueue = true
		}
		switch turn.EvaluateAdmission(in.HasActiveRun, in.HasActiveRunner, allowQueue) {
		case biz.AdmitEnqueue:
			if in.EntryPoint == biz.EntryPointChannel {
				return IngressPolicyResult{Decision: IngressSteer, Intent: "steer"}
			}
			return IngressPolicyResult{Decision: IngressQueue, Intent: "queue"}
		case biz.AdmitReject:
			return IngressPolicyResult{Decision: IngressRejectBusy, Intent: "reject_busy"}
		}
	}
	return IngressPolicyResult{Decision: IngressAdmit, Intent: "admit"}
}

// ResolveChannelAcceptRoute decides sync vs async execution plane for accepted inbound (DECO-11).
func ResolveChannelAcceptRoute(text string, ltCfg biz.ChannelLongTaskConfig, allowQueue bool) IngressPolicyResult {
	result := EvaluateIngressPolicy(channelIngressPolicyInput(text, ltCfg, allowQueue, false, false, false))
	if result.Decision == IngressAdmit && ltCfg.SuggestDurableRun(text) && !ltCfg.ShouldRunAsync(text) {
		result.SuggestDurable = true
	}
	return result
}

// channelAcceptOutcomeFromRoute maps ingress route policy to accept outcome flags.
func channelAcceptOutcomeFromRoute(route IngressPolicyResult) inboundAcceptOutcome {
	switch route.Decision {
	case IngressRouteAsync:
		return inboundAcceptOutcome{DispatchAsync: true}
	default:
		return inboundAcceptOutcome{ExecuteSync: true}
	}
}

// AllowPendingQueueFromEntry resolves queue policy for legacy callers without EntryPoint set.
func AllowPendingQueueFromEntry(entry biz.TurnEntryPoint, allowQueue bool) bool {
	if entry == "" {
		return true
	}
	return allowQueue
}

func ingressPolicyFromTurnInput(input biz.TurnInput, hasActive, hasRunner, contextPressure bool) IngressPolicyResult {
	return EvaluateIngressPolicy(IngressPolicyInput{
		Text:            input.Content,
		EntryPoint:      input.EntryConfig.EntryPoint,
		AllowQueue:      input.EntryConfig.AllowQueue,
		HasActiveRun:    hasActive,
		HasActiveRunner: hasRunner,
		IsCancelCommand: biz.IsChannelCancelCommand(input.Content),
		IsStatusQuery:   biz.IsChannelStatusQuery(input.Content),
		ContextPressure: contextPressure,
	})
}

func channelIngressPolicyInput(text string, ltCfg biz.ChannelLongTaskConfig, allowQueue, hasActive, hasRunner, contextPressure bool) IngressPolicyInput {
	return IngressPolicyInput{
		Text:                text,
		EntryPoint:          biz.EntryPointChannel,
		AllowQueue:          allowQueue,
		HasActiveRun:        hasActive,
		HasActiveRunner:     hasRunner,
		RouteAsync:          ltCfg.ShouldRunAsync(text),
		IsCancelCommand:     biz.IsChannelCancelCommand(text),
		IsStatusQuery:       biz.IsChannelStatusQuery(text),
		IsBackgroundCommand: biz.IsChannelBackgroundCommand(text),
		ContextPressure:     contextPressure,
	}
}

func channelAllowQueueFromConfig(configJSON string) bool {
	return biz.ChannelBusyInputQueue(configJSON)
}

func ingressDecisionNeedsTurn(decision IngressDecision) bool {
	switch decision {
	case IngressAdmit, IngressQueue, IngressSteer:
		return true
	default:
		return false
	}
}

func ingressIntentLabel(decision IngressDecision) string {
	return strings.TrimSpace(string(decision))
}

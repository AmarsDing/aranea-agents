package biz

import "strings"

// IngressDecision classifies how an inbound request should be handled before Turn execution.
type IngressDecision string

const (
	IngressAdmit           IngressDecision = "admit"
	IngressQueue           IngressDecision = "queue"
	IngressInterrupt       IngressDecision = "interrupt"
	IngressRouteAsync      IngressDecision = "route_async"
	IngressRouteBackground IngressDecision = "route_background"
	IngressCancel          IngressDecision = "cancel"
	IngressStatus          IngressDecision = "status"
	IngressSkipDuplicate   IngressDecision = "skip_duplicate"
)

// IngressPolicyInput is the pure input for L2 ingress policy evaluation.
type IngressPolicyInput struct {
	Text               string
	EntryPoint         TurnEntryPoint
	AllowQueue         bool
	HasActiveRun       bool
	HasActiveRunner    bool
	RouteAsync         bool
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
		if in.AllowQueue && in.EntryPoint != EntryPointChannel {
			return IngressPolicyResult{Decision: IngressQueue, Intent: "context_force_queue"}
		}
		return IngressPolicyResult{Decision: IngressQueue, Intent: "context_pressure"}
	}
	if in.HasActiveRun {
		allowQueue := in.AllowQueue
		if in.EntryPoint == "" {
			allowQueue = true
		}
		// Inline admission logic (equivalent to turn.EvaluateAdmission)
		// to avoid circular import: biz → turn → biz.
		var admission TurnAdmissionDecision
		if !allowQueue {
			admission = AdmitReject
		} else if in.HasActiveRunner {
			admission = AdmitEnqueue
		} else {
			admission = AdmitReject
		}
		switch admission {
		case AdmitEnqueue:
			if in.EntryPoint == EntryPointChannel {
				return IngressPolicyResult{Decision: IngressInterrupt, Intent: "interrupt"}
			}
			return IngressPolicyResult{Decision: IngressQueue, Intent: "queue"}
		case AdmitReject:
			return IngressPolicyResult{Decision: IngressQueue, Intent: "reject_busy_queue"}
		}
	}
	return IngressPolicyResult{Decision: IngressAdmit, Intent: "admit"}
}

// ResolveChannelAcceptRoute decides sync vs async execution plane for accepted inbound (DECO-11).
func ResolveChannelAcceptRoute(text string, ltCfg ChannelLongTaskConfig, allowQueue bool) IngressPolicyResult {
	result := EvaluateIngressPolicy(ChannelIngressPolicyInput(text, ltCfg, allowQueue, false, false, false))
	if result.Decision == IngressAdmit && ltCfg.SuggestDurableRun(text) && !ltCfg.ShouldRunAsync(text) {
		result.SuggestDurable = true
	}
	return result
}

// AllowPendingQueueFromEntry resolves queue policy for legacy callers without EntryPoint set.
func AllowPendingQueueFromEntry(entry TurnEntryPoint, allowQueue bool) bool {
	if entry == "" {
		return true
	}
	return allowQueue
}

// IngressPolicyFromTurnInput evaluates ingress policy from a TurnInput.
func IngressPolicyFromTurnInput(input TurnInput, hasActive, hasRunner, contextPressure bool) IngressPolicyResult {
	return EvaluateIngressPolicy(IngressPolicyInput{
		Text:            input.Content,
		EntryPoint:      input.EntryConfig.EntryPoint,
		AllowQueue:      input.EntryConfig.AllowQueue,
		HasActiveRun:    hasActive,
		HasActiveRunner: hasRunner,
		IsCancelCommand: IsChannelCancelCommand(input.Content),
		IsStatusQuery:   IsChannelStatusQuery(input.Content),
		ContextPressure: contextPressure,
	})
}

// ChannelIngressPolicyInput builds an IngressPolicyInput for channel ingress.
func ChannelIngressPolicyInput(text string, ltCfg ChannelLongTaskConfig, allowQueue, hasActive, hasRunner, contextPressure bool) IngressPolicyInput {
	return IngressPolicyInput{
		Text:                text,
		EntryPoint:          EntryPointChannel,
		AllowQueue:          allowQueue,
		HasActiveRun:        hasActive,
		HasActiveRunner:     hasRunner,
		RouteAsync:          ltCfg.ShouldRunAsync(text),
		IsCancelCommand:     IsChannelCancelCommand(text),
		IsStatusQuery:       IsChannelStatusQuery(text),
		IsBackgroundCommand: IsChannelBackgroundCommand(text),
		ContextPressure:     contextPressure,
	}
}

// IngressDecisionNeedsTurn reports whether the decision requires a Turn execution.
func IngressDecisionNeedsTurn(decision IngressDecision) bool {
	switch decision {
	case IngressAdmit, IngressQueue, IngressInterrupt:
		return true
	default:
		return false
	}
}

// IngressIntentLabel returns the intent label for an ingress decision.
func IngressIntentLabel(decision IngressDecision) string {
	return strings.TrimSpace(string(decision))
}

package service

import (
	"aranea-agents/internal/biz"
)

// IngressDecision classifies how an inbound request should be handled before Turn execution.
// Delegated to biz layer; type alias preserves service-level API compatibility.
type IngressDecision = biz.IngressDecision

const (
	IngressAdmit           = biz.IngressAdmit
	IngressQueue           = biz.IngressQueue
	IngressSteer           = biz.IngressSteer
	IngressRejectBusy      = biz.IngressRejectBusy
	IngressRouteAsync      = biz.IngressRouteAsync
	IngressRouteBackground = biz.IngressRouteBackground
	IngressCancel          = biz.IngressCancel
	IngressStatus          = biz.IngressStatus
	IngressSkipDuplicate   = biz.IngressSkipDuplicate
)

// IngressPolicyInput is the pure input for L2 ingress policy evaluation.
type IngressPolicyInput = biz.IngressPolicyInput

// IngressPolicyResult is the outcome of ingress policy evaluation.
type IngressPolicyResult = biz.IngressPolicyResult

// EvaluateIngressPolicy delegates to biz.EvaluateIngressPolicy.
func EvaluateIngressPolicy(in IngressPolicyInput) IngressPolicyResult {
	return biz.EvaluateIngressPolicy(in)
}

// ResolveChannelAcceptRoute delegates to biz.ResolveChannelAcceptRoute.
func ResolveChannelAcceptRoute(text string, ltCfg biz.ChannelLongTaskConfig, allowQueue bool) IngressPolicyResult {
	return biz.ResolveChannelAcceptRoute(text, ltCfg, allowQueue)
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

// AllowPendingQueueFromEntry delegates to biz.AllowPendingQueueFromEntry.
func AllowPendingQueueFromEntry(entry biz.TurnEntryPoint, allowQueue bool) bool {
	return biz.AllowPendingQueueFromEntry(entry, allowQueue)
}

// ingressPolicyFromTurnInput delegates to biz.IngressPolicyFromTurnInput.
func ingressPolicyFromTurnInput(input biz.TurnInput, hasActive, hasRunner, contextPressure bool) IngressPolicyResult {
	return biz.IngressPolicyFromTurnInput(input, hasActive, hasRunner, contextPressure)
}

// channelIngressPolicyInput delegates to biz.ChannelIngressPolicyInput.
func channelIngressPolicyInput(text string, ltCfg biz.ChannelLongTaskConfig, allowQueue, hasActive, hasRunner, contextPressure bool) IngressPolicyInput {
	return biz.ChannelIngressPolicyInput(text, ltCfg, allowQueue, hasActive, hasRunner, contextPressure)
}

func channelAllowQueueFromConfig(configJSON string) bool {
	return biz.ChannelBusyInputQueue(configJSON)
}

// ingressDecisionNeedsTurn delegates to biz.IngressDecisionNeedsTurn.
func ingressDecisionNeedsTurn(decision IngressDecision) bool {
	return biz.IngressDecisionNeedsTurn(decision)
}

// ingressIntentLabel delegates to biz.IngressIntentLabel.
func ingressIntentLabel(decision IngressDecision) string {
	return biz.IngressIntentLabel(decision)
}

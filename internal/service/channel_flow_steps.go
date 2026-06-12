package service

// Channel FlowLog / SysLog step identifiers (see docs/需求/52-flow-logger.design.md §5.1).
const (
	flowStepChannelIngressJob  = "channel.ingress.job"
	flowStepChannelTurnDone    = "channel.turn.done"
	flowStepChannelTurnTimeout = "channel.turn.timeout"
	flowStepChannelPreview     = "channel.preview.patch"
	flowStepChannelToolCard    = "channel.tool.card"
	flowStepChannelOutbound    = "channel.outbound.text"
)

package outbound

import ch "aranea-agents/internal/channel"

// RegisterFromInboundEvent registers an OutboundText sender with the Router.
func RegisterFromInboundEvent(router *Router, platformType string, sender ch.OutboundText) {
	if sender == nil || router == nil {
		return
	}
	router.RegisterOutboundText(sender)
}

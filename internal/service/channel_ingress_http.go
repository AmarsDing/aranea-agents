package service

import (
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

// processInboundHTTP runs ProcessInbound and maps errors to HTTP responses.
func (h *ChannelIngress) processInboundHTTP(w http.ResponseWriter, r *http.Request, chRow biz.Channel, ev port.InboundEvent) {
	if err := h.ProcessInboundWebhook(r.Context(), chRow, ev); err != nil {
		http.Error(w, "agent error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

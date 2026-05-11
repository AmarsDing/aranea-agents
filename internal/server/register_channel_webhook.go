package server

import (
	"aranea-agents/internal/service"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RegisterChannelWebhook mounts POST /webhooks/{channel_key} (Feishu runtime).
func RegisterChannelWebhook(srv *kratoshttp.Server, ingress *service.ChannelIngress) {
	if srv == nil || ingress == nil {
		return
	}
	r := srv.Route("/")
	r.POST("/webhooks/{channel_key}", ingress.FeishuWebhookHTTP())
}

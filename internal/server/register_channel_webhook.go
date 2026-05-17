package server

import (
	"aranea-agents/internal/service"
	"aranea-agents/pkg/auth"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RegisterChannelWebhook mounts POST /webhooks/{channel_key} (Feishu runtime).
// EP-SEC-03: registers the /webhooks/ prefix so the auth middleware allows it through;
// unregistered sub-paths return 403.
func RegisterChannelWebhook(srv *kratoshttp.Server, ingress *service.ChannelIngress) {
	if srv == nil || ingress == nil {
		return
	}
	auth.RegisterWebhookPath("/webhooks/")
	r := srv.Route("/")
	r.POST("/webhooks/{channel_key}", ingress.FeishuWebhookHTTP())
}

package server

import (
	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/service"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RegisterChatIngress registers kratos chat.v1 (unary HTTP) plus streaming POST documented in ChatService.
func RegisterChatIngress(srv *kratoshttp.Server, chat *service.ChatService) {
	chatv1.RegisterChatServiceHTTPServer(srv, chat)
	r := srv.Route("/")
	r.POST("/v1/chat/messages/stream", chat.ProxyStream)
}

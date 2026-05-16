package server

import (
	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/service"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

func RegisterChatIngress(srv *kratoshttp.Server, chat *service.ChatService) {
	chatv1.RegisterChatServiceHTTPServer(srv, chat)
}

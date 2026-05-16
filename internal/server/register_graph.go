package server

import (
	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/service"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

func RegisterGraphHTTP(srv *kratoshttp.Server, graphSvc *service.GraphService) {
	graphv1.RegisterGraphServiceHTTPServer(srv, graphSvc)
}

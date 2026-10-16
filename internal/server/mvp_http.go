package server

import (
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/artifactfs"
	"aranea-agents/internal/mvp"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

func configureMVPMedia(readiness ReadinessProbe, lg loggateway.Logger) {
	if d, ok := readiness.(*data.Data); ok && d != nil {
		mvp.Configure(mvp.Deps{
			Content:     data.NewContentItemRepo(d),
			Review:      data.NewReviewTaskRepo(d),
			Publish:     data.NewPublishJobRepo(d),
			Opportunity: data.NewOpportunityRepo(d),
			Supplier:    data.NewSupplierCandidateRepo(d),
			Artifacts:   artifactfs.NewFSArtifactRepo(lg),
		})
		return
	}
	_ = mvp.Ensure()
}

func registerMVPMediaRoutes(srv *kratoshttp.Server) {
	service.RegisterMVPMediaRoutes(srv)
}

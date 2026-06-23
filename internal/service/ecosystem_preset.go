package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// EcosystemPresetService handles ecosystem preset load/unload/status HTTP endpoints.
type EcosystemPresetService struct {
	uc *biz.EcosystemPresetUsecase
	lg loggateway.Logger
}

// NewEcosystemPresetService creates an EcosystemPresetService.
func NewEcosystemPresetService(uc *biz.EcosystemPresetUsecase, lg loggateway.Logger) *EcosystemPresetService {
	return &EcosystemPresetService{uc: uc, lg: lg}
}

// ecosystemPresetLoadRequest is the request body for the load endpoint.
type ecosystemPresetLoadRequest struct {
	Industries []string `json:"industries"`
	Force      bool     `json:"force"`
}

// ecosystemPresetUnloadRequest is the request body for the unload endpoint.
type ecosystemPresetUnloadRequest struct {
	Industries []string `json:"industries"`
}

// HandleLoad handles POST /api/v1/admin/ecosystem/preset/load
func (s *EcosystemPresetService) HandleLoad() func(ctx kratoshttp.Context) error {
	return func(ctx kratoshttp.Context) error {
		body, err := io.ReadAll(ctx.Request().Body)
		if err != nil {
			return apierror.BadRequest("ECOSYSTEM_PRESET", "failed to read request body")
		}
		defer ctx.Request().Body.Close()

		var req ecosystemPresetLoadRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return apierror.BadRequest("ECOSYSTEM_PRESET", "invalid request body")
		}

		if len(req.Industries) == 0 {
			return apierror.BadRequest("ECOSYSTEM_PRESET", "industries must not be empty")
		}

		resp, err := s.uc.LoadEcosystemPreset(ctx, req.Industries, req.Force)
		if err != nil {
			s.lg.Warn("ecosystem preset load failed", loggateway.Err(err))
			return apierror.Wrap(err, apierror.CodeInternal, "ECOSYSTEM_PRESET")
		}

		s.lg.Info("ecosystem preset loaded", loggateway.Str("industries", strings.Join(req.Industries, ",")))
		return ctx.JSON(http.StatusOK, resp)
	}
}

// HandleUnload handles POST /api/v1/admin/ecosystem/preset/unload
func (s *EcosystemPresetService) HandleUnload() func(ctx kratoshttp.Context) error {
	return func(ctx kratoshttp.Context) error {
		body, err := io.ReadAll(ctx.Request().Body)
		if err != nil {
			return apierror.BadRequest("ECOSYSTEM_PRESET", "failed to read request body")
		}
		defer ctx.Request().Body.Close()

		var req ecosystemPresetUnloadRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return apierror.BadRequest("ECOSYSTEM_PRESET", "invalid request body")
		}

		if len(req.Industries) == 0 {
			return apierror.BadRequest("ECOSYSTEM_PRESET", "industries must not be empty")
		}

		resp, err := s.uc.UnloadEcosystemPreset(ctx, req.Industries)
		if err != nil {
			s.lg.Warn("ecosystem preset unload failed", loggateway.Err(err))
			return apierror.Wrap(err, apierror.CodeInternal, "ECOSYSTEM_PRESET")
		}

		s.lg.Info("ecosystem preset unloaded", loggateway.Str("industries", strings.Join(req.Industries, ",")))
		return ctx.JSON(http.StatusOK, resp)
	}
}

// HandleStatus handles GET /api/v1/admin/ecosystem/preset/status
func (s *EcosystemPresetService) HandleStatus() func(ctx kratoshttp.Context) error {
	return func(ctx kratoshttp.Context) error {
		status, err := s.uc.GetEcosystemStatus(ctx)
		if err != nil {
			return apierror.Wrap(err, apierror.CodeInternal, "ECOSYSTEM_PRESET")
		}
		return ctx.JSON(http.StatusOK, status)
	}
}

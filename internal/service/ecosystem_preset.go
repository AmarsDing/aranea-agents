package service

import (
	"encoding/json"
	"io"
	"net/http"

	"aranea-agents/internal/biz"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// EcosystemPresetService handles ecosystem preset load/unload/status HTTP endpoints.
type EcosystemPresetService struct {
	uc *biz.EcosystemPresetUsecase
	// clientProvider lazily provides the ent.Client for seed operations.
	clientProvider func() any
}

// NewEcosystemPresetService creates an EcosystemPresetService.
func NewEcosystemPresetService(uc *biz.EcosystemPresetUsecase, clientProvider func() any) *EcosystemPresetService {
	return &EcosystemPresetService{uc: uc, clientProvider: clientProvider}
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
			return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		}
		defer ctx.Request().Body.Close()

		var req ecosystemPresetLoadRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}

		client := s.clientProvider()
		if client == nil {
			return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "database client not available"})
		}

		resp, err := s.uc.LoadEcosystemPreset(ctx, req.Industries, req.Force, client)
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return ctx.JSON(http.StatusOK, resp)
	}
}

// HandleUnload handles POST /api/v1/admin/ecosystem/preset/unload
func (s *EcosystemPresetService) HandleUnload() func(ctx kratoshttp.Context) error {
	return func(ctx kratoshttp.Context) error {
		body, err := io.ReadAll(ctx.Request().Body)
		if err != nil {
			return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		}
		defer ctx.Request().Body.Close()

		var req ecosystemPresetUnloadRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}

		resp, err := s.uc.UnloadEcosystemPreset(ctx, req.Industries)
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return ctx.JSON(http.StatusOK, resp)
	}
}

// HandleStatus handles GET /api/v1/admin/ecosystem/preset/status
func (s *EcosystemPresetService) HandleStatus() func(ctx kratoshttp.Context) error {
	return func(ctx kratoshttp.Context) error {
		status, err := s.uc.GetEcosystemStatus(ctx)
		if err != nil {
			return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return ctx.JSON(http.StatusOK, status)
	}
}

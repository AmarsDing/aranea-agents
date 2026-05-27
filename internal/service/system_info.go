package service

import (
	"context"
	"encoding/json"
	nethttp "net/http"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// SystemInfoResponse is the JSON payload returned by GET /v1/system/info.
// Fields are best-effort; missing values are returned as empty strings.
type SystemInfoResponse struct {
	Version             string            `json:"version"`
	GitCommit           string            `json:"git_commit"`
	BuildTime           string            `json:"build_time"`
	DefaultProvider     string            `json:"default_provider"`
	DefaultModel        string            `json:"default_model"`
	SkillStorageRoot    string            `json:"skill_storage_root"`
	Features            map[string]string `json:"features"`
}

// GetSystemInfoHandler returns a Kratos HTTP handler for GET /v1/system/info.
// The version/commit/buildTime params are injected at wire-time from ldflags.
func (s *SystemSettingService) GetSystemInfoHandler(version, gitCommit, buildTime string) kratoshttp.HandlerFunc {
	return func(ctx kratoshttp.Context) error {
		row, err := s.uc.Get(context.Background())
		if err != nil {
			w := ctx.Response()
			w.WriteHeader(nethttp.StatusInternalServerError)
			return nil
		}
		resp := SystemInfoResponse{
			Version:          version,
			GitCommit:        gitCommit,
			BuildTime:        buildTime,
			DefaultProvider:  row.DefaultRefineLLM.Provider,
			DefaultModel:     row.DefaultRefineLLM.Model,
			SkillStorageRoot: row.RootDirectory,
			Features:         map[string]string{},
		}
		w := ctx.Response()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(nethttp.StatusOK)
		return json.NewEncoder(w).Encode(resp)
	}
}

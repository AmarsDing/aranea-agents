package service

import (
	"encoding/json"
	nethttp "net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

type SystemInfoResponse struct {
	Version          string            `json:"version"`
	GitCommit        string            `json:"git_commit"`
	BuildTime        string            `json:"build_time"`
	DefaultProvider  string            `json:"default_provider"`
	DefaultModel     string            `json:"default_model"`
	SkillStorageRoot string            `json:"skill_storage_root"`
	Features         map[string]string `json:"features"`
}

func (s *SystemSettingService) GetSystemInfoHandler(version, gitCommit, buildTime string) kratoshttp.HandlerFunc {
	return func(ctx kratoshttp.Context) error {
		row, err := s.uc.Get(ctx.Request().Context())
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
			Features:         buildSystemFeatures(s.crypto),
		}
		w := ctx.Response()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(nethttp.StatusOK)
		return json.NewEncoder(w).Encode(resp)
	}
}

func buildSystemFeatures(crypto *biz.CredentialCrypto) map[string]string {
	features := map[string]string{}
	if crypto != nil && crypto.IsAvailable() {
		features["credential_encryption"] = "available"
	} else {
		features["credential_encryption"] = "unavailable"
	}
	// M27 Phase 5: 前端据此决定是否展示「打开文件夹」按钮。
	if conf.LocalRevealEnabled() {
		features["local_reveal"] = "enabled"
	} else {
		features["local_reveal"] = "disabled"
	}
	return features
}

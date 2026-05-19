package service

import (
	"encoding/json"
	"net/http"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RegisterSkillImportMultipart mounts POST /v1/skills/import (multipart ZIP).
// JSON import routes use skill.proto HTTP bindings on SkillService.
// Multipart upload stays here (not proto-generated) because file upload cannot be expressed
// cleanly in google.api.http without a custom codec; see docs/需求/20 skill.design.md.
func (s *SkillService) RegisterSkillImportMultipart(srv *kratoshttp.Server) {
	if s.import_ == nil {
		return
	}
	srv.Route("/").POST("/v1/skills/import", func(ctx kratoshttp.Context) error {
		req := ctx.Request()
		if err := req.ParseMultipartForm(25 << 20); err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		file, header, err := req.FormFile("file")
		if err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		defer file.Close()
		job, err := s.import_.Import(req.Context(), file, header)
		if err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		return writeSkillImportJSON(ctx, http.StatusCreated, map[string]string{"job_id": job.JobID})
	})
}

func writeSkillImportJSON(ctx kratoshttp.Context, status int, payload map[string]string) error {
	w := ctx.Response()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

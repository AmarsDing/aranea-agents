package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/skillimport"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RegisterSkillImportHTTPServer mounts **/v1/skills/import*** (multipart ZIP + JSON) on cmd/admin.
func RegisterSkillImportHTTPServer(srv *kratoshttp.Server, eng *skillimport.Engine) {
	if eng == nil {
		return
	}
	r := srv.Route("/")

	r.POST("/v1/skills/import", func(ctx kratoshttp.Context) error {
		req := ctx.Request()
		if err := req.ParseMultipartForm(25 << 20); err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		file, header, err := req.FormFile("file")
		if err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		defer file.Close()
		job, err := eng.Import(req.Context(), file, header)
		if err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		return writeSkillImportJSON(ctx, http.StatusCreated, map[string]string{"job_id": job.JobID})
	})

	r.GET("/v1/skills/import/{job_id}", func(ctx kratoshttp.Context) error {
		jobID, ok := parseSkillImportGetJob(ctx.Request().URL.Path)
		if !ok {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": "invalid import job path"})
		}
		job, err := eng.GetImportJob(jobID)
		if err != nil {
			if errors.Is(err, skillimport.ErrImportJobNotFound) {
				return writeSkillImportJSON(ctx, http.StatusNotFound, map[string]string{"message": err.Error()})
			}
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		return writeSkillImportRawJSON(ctx, http.StatusOK, job)
	})

	r.POST("/v1/skills/import/{job_id}/apply", func(ctx kratoshttp.Context) error {
		jobID, ok := parseSkillImportApply(ctx.Request().URL.Path)
		if !ok {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": "invalid import apply path"})
		}
		body, err := io.ReadAll(io.LimitReader(ctx.Request().Body, 8<<20))
		if err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		var in biz.SkillImportApplyRequest
		if err := json.Unmarshal(body, &in); err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		result, err := eng.ApplyImport(ctx.Request().Context(), jobID, in)
		if err != nil {
			if errors.Is(err, skillimport.ErrImportJobNotFound) {
				return writeSkillImportJSON(ctx, http.StatusNotFound, map[string]string{"message": err.Error()})
			}
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		return writeSkillImportRawJSON(ctx, http.StatusOK, result)
	})

	r.POST("/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine", func(ctx kratoshttp.Context) error {
		jobID, groupID, ok := parseSkillImportRefine(ctx.Request().URL.Path)
		if !ok {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": "invalid conflict refine path"})
		}
		body, err := io.ReadAll(io.LimitReader(ctx.Request().Body, 1<<20))
		if err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		var in biz.SkillRefineRequest
		if err := json.Unmarshal(body, &in); err != nil {
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		result, err := eng.RefineConflictGroup(ctx.Request().Context(), jobID, groupID, in)
		if err != nil {
			if errors.Is(err, skillimport.ErrImportJobNotFound) || errors.Is(err, skillimport.ErrConflictGroupNotFound) {
				return writeSkillImportJSON(ctx, http.StatusNotFound, map[string]string{"message": err.Error()})
			}
			return writeSkillImportJSON(ctx, http.StatusBadRequest, map[string]string{"message": err.Error()})
		}
		return writeSkillImportRawJSON(ctx, http.StatusOK, result)
	})
}

func skillImportPathSegments(path string) ([]string, bool) {
	const prefix = "/v1/skills/import/"
	if !strings.HasPrefix(path, prefix) {
		return nil, false
	}
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if rest == "" {
		return nil, false
	}
	return strings.Split(rest, "/"), true
}

func parseSkillImportGetJob(path string) (jobID string, ok bool) {
	segs, ok := skillImportPathSegments(path)
	if !ok || len(segs) != 1 {
		return "", false
	}
	if segs[0] == "" || strings.Contains(segs[0], "..") {
		return "", false
	}
	return segs[0], true
}

func parseSkillImportApply(path string) (jobID string, ok bool) {
	segs, ok := skillImportPathSegments(path)
	if !ok || len(segs) != 2 || segs[1] != "apply" {
		return "", false
	}
	if segs[0] == "" || strings.Contains(segs[0], "..") {
		return "", false
	}
	return segs[0], true
}

func parseSkillImportRefine(path string) (jobID string, groupID string, ok bool) {
	segs, ok := skillImportPathSegments(path)
	if !ok || len(segs) != 4 || segs[1] != "conflict-groups" || segs[3] != "refine" {
		return "", "", false
	}
	jobID, groupID = segs[0], segs[2]
	if jobID == "" || groupID == "" || strings.Contains(jobID, "..") || strings.Contains(groupID, "..") {
		return "", "", false
	}
	return jobID, groupID, true
}

func writeSkillImportJSON(ctx kratoshttp.Context, status int, payload map[string]string) error {
	w := ctx.Response()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

func writeSkillImportRawJSON(ctx kratoshttp.Context, status int, payload any) error {
	w := ctx.Response()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

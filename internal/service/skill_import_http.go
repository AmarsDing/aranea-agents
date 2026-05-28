package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aranea-agents/internal/metrics"
	"aranea-agents/internal/skill/importer"
	authpkg "aranea-agents/pkg/auth"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

func (s *SkillService) RegisterSkillImportMultipart(srv *kratoshttp.Server) {
	if s.import_ == nil {
		return
	}
	srv.Route("/").POST("/v1/skills/import", skillImportMultipartHandler(s))
}

func skillImportMultipartHandler(s *SkillService) func(ctx kratoshttp.Context) error {
	return func(ctx kratoshttp.Context) error {
		start := time.Now()
		tracer := otel.Tracer("aranea/skill-import")
		spanCtx, span := tracer.Start(ctx.Request().Context(), "SkillImport/Upload",
			trace.WithAttributes(attribute.String("phase", "upload")))
		defer span.End()

		defer func() {
			if r := recover(); r != nil {
				span.SetStatus(codes.Error, "panic recovered")
				span.RecordError(fmt.Errorf("panic: %v", r))
				metrics.SkillImportTotal.WithLabelValues("upload", "panic").Inc()
				metrics.SkillImportDuration.WithLabelValues("upload").Observe(time.Since(start).Seconds())
				_ = writeSkillImportJSON(ctx, http.StatusInternalServerError,
					map[string]string{"message": "internal server error"})
			}
		}()

		a, ok := authpkg.FromContext(spanCtx)
		if !ok || a == nil || !a.HasAdminAccess() {
			metrics.SkillImportTotal.WithLabelValues("upload", "forbidden").Inc()
			metrics.SkillImportDuration.WithLabelValues("upload").Observe(time.Since(start).Seconds())
			return writeSkillImportJSON(ctx, http.StatusForbidden,
				map[string]string{"message": "admin access required"})
		}

		req := ctx.Request().WithContext(spanCtx)
		if err := req.ParseMultipartForm(int64(importer.MaxZipBytes)); err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			metrics.SkillImportTotal.WithLabelValues("upload", "bad_request").Inc()
			metrics.SkillImportDuration.WithLabelValues("upload").Observe(time.Since(start).Seconds())
			return writeSkillImportJSON(ctx, http.StatusBadRequest,
				map[string]string{"message": err.Error()})
		}
		file, header, err := req.FormFile("file")
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			metrics.SkillImportTotal.WithLabelValues("upload", "bad_request").Inc()
			metrics.SkillImportDuration.WithLabelValues("upload").Observe(time.Since(start).Seconds())
			return writeSkillImportJSON(ctx, http.StatusBadRequest,
				map[string]string{"message": err.Error()})
		}
		defer file.Close()

		span.SetAttributes(attribute.String("file.name", header.Filename), attribute.Int64("file.size", header.Size))
		job, err := s.import_.Import(spanCtx, file, header)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			metrics.SkillImportTotal.WithLabelValues("upload", "error").Inc()
			metrics.SkillImportDuration.WithLabelValues("upload").Observe(time.Since(start).Seconds())
			return writeSkillImportJSON(ctx, http.StatusBadRequest,
				map[string]string{"message": err.Error()})
		}

		span.SetAttributes(attribute.String("job_id", job.JobID))
		span.SetStatus(codes.Ok, "")
		metrics.SkillImportTotal.WithLabelValues("upload", "success").Inc()
		metrics.SkillImportDuration.WithLabelValues("upload").Observe(time.Since(start).Seconds())
		return writeSkillImportJSON(ctx, http.StatusCreated, map[string]string{"job_id": job.JobID})
	}
}

func writeSkillImportJSON(ctx kratoshttp.Context, status int, payload map[string]string) error {
	w := ctx.Response()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

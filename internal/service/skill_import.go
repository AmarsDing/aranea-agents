package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/skill/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/pkg/apierror"
	authpkg "aranea-agents/pkg/auth"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

func (s *SkillService) importEngine() (*importer.Engine, error) {
	if s.import_ == nil {
		return nil, apierror.Internal("SKILL_IMPORT", "skill import engine is not configured")
	}
	return s.import_, nil
}

// assertImportAdmin gates ZIP import RPCs behind admin access.
func (s *SkillService) assertImportAdmin(ctx context.Context) error {
	a, ok := authpkg.FromContext(ctx)
	if !ok || a == nil {
		return authpkg.ErrUnauthorized
	}
	if !a.HasAdminAccess() {
		return authpkg.ErrForbidden
	}
	return nil
}

// DecodeSkillImportRequest lets POST /v1/skills/import accept both proto JSON
// (bytes file + filename) and legacy multipart/form-data field "file".
// Other RPCs fall through to the default Kratos decoder.
func DecodeSkillImportRequest(r *http.Request, v any) error {
	if req, ok := v.(*v1.ImportSkillZipRequest); ok {
		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/form-data") {
			return bindImportSkillZipMultipart(r, req)
		}
	}
	return kratoshttp.DefaultRequestDecoder(r, v)
}

func bindImportSkillZipMultipart(r *http.Request, req *v1.ImportSkillZipRequest) error {
	if err := r.ParseMultipartForm(int64(importer.MaxZipBytes)); err != nil {
		return err
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, importer.MaxZipBytes+1))
	if err != nil {
		return err
	}
	req.File = data
	if name := strings.TrimSpace(r.FormValue("filename")); name != "" {
		req.Filename = name
	} else if header != nil {
		req.Filename = header.Filename
	}
	return nil
}

func (s *SkillService) ImportSkillZip(ctx context.Context, req *v1.ImportSkillZipRequest) (*v1.ImportSkillZipResponse, error) {
	start := time.Now()
	defer func() {
		metrics.SkillImportDuration.WithLabelValues("upload").Observe(time.Since(start).Seconds())
	}()
	if err := s.assertImportAdmin(ctx); err != nil {
		metrics.SkillImportTotal.WithLabelValues("upload", "forbidden").Inc()
		return nil, err
	}
	eng, err := s.importEngine()
	if err != nil {
		metrics.SkillImportTotal.WithLabelValues("upload", "error").Inc()
		return nil, err
	}
	filename := strings.TrimSpace(req.GetFilename())
	data := req.GetFile()
	if filename == "" || len(data) == 0 {
		metrics.SkillImportTotal.WithLabelValues("upload", "bad_request").Inc()
		return nil, apierror.BadRequest("SKILL_IMPORT", "skill zip file is required")
	}
	job, err := eng.ImportZip(ctx, filename, data)
	if err != nil {
		metrics.SkillImportTotal.WithLabelValues("upload", "error").Inc()
		return nil, apierror.BadRequest("SKILL_IMPORT", err.Error())
	}
	metrics.SkillImportTotal.WithLabelValues("upload", "success").Inc()
	return &v1.ImportSkillZipResponse{JobId: job.JobID}, nil
}

func (s *SkillService) GetSkillImportJob(ctx context.Context, req *v1.GetSkillImportJobRequest) (*v1.SkillImportJob, error) {
	if err := s.assertImportAdmin(ctx); err != nil {
		return nil, err
	}
	eng, err := s.importEngine()
	if err != nil {
		return nil, err
	}
	job, err := eng.GetImportJob(ctx, req.GetJobId())
	if err != nil {
		if errors.Is(err, importer.ErrImportJobNotFound) {
			return nil, apierror.NotFound("SKILL_IMPORT", err.Error())
		}
		return nil, apierror.BadRequest("SKILL_IMPORT", err.Error())
	}
	return skillImportJobToProto(job), nil
}

func (s *SkillService) ApplySkillImport(ctx context.Context, req *v1.ApplySkillImportRequest) (*v1.SkillImportApplyResult, error) {
	if err := s.assertImportAdmin(ctx); err != nil {
		return nil, err
	}
	eng, err := s.importEngine()
	if err != nil {
		return nil, err
	}
	result, err := eng.ApplyImport(ctx, req.GetJobId(), skillImportApplyFromProto(req))
	if err != nil {
		if errors.Is(err, importer.ErrImportJobNotFound) {
			return nil, apierror.NotFound("SKILL_IMPORT", err.Error())
		}
		return nil, apierror.BadRequest("SKILL_IMPORT", err.Error())
	}
	return skillImportApplyResultToProto(result), nil
}

func (s *SkillService) RefineSkillImportConflict(ctx context.Context, req *v1.RefineSkillImportConflictRequest) (*v1.SkillRefineResult, error) {
	if err := s.assertImportAdmin(ctx); err != nil {
		return nil, err
	}
	eng, err := s.importEngine()
	if err != nil {
		return nil, err
	}
	result, err := eng.RefineConflictGroup(ctx, req.GetJobId(), req.GetGroupId(), biz.SkillRefineRequest{
		Provider:     req.GetProvider(),
		Model:        req.GetModel(),
		Instructions: req.GetInstructions(),
	})
	if err != nil {
		if errors.Is(err, importer.ErrImportJobNotFound) || errors.Is(err, importer.ErrConflictGroupNotFound) {
			return nil, apierror.NotFound("SKILL_IMPORT", err.Error())
		}
		return nil, apierror.BadRequest("SKILL_IMPORT", err.Error())
	}
	return skillRefineResultToProto(result), nil
}

func skillImportJobToProto(j biz.SkillImportJob) *v1.SkillImportJob {
	out := &v1.SkillImportJob{
		JobId:            j.JobID,
		Status:           j.Status,
		ValidationStatus: j.ValidationStatus,
		StorageRoot:      j.StorageRoot,
		Message:          j.Message,
	}
	for _, c := range j.Candidates {
		out.Candidates = append(out.Candidates, skillImportCandidateToProto(c))
	}
	for _, g := range j.ConflictGroups {
		out.ConflictGroups = append(out.ConflictGroups, skillConflictGroupToProto(g))
	}
	return out
}

func skillImportCandidateToProto(c biz.SkillImportCandidate) *v1.SkillImportCandidate {
	out := &v1.SkillImportCandidate{
		CandidateId:      c.CandidateID,
		Name:             c.Name,
		Slug:             c.Slug,
		Description:      c.Description,
		BodyPreview:      c.BodyPreview,
		TargetDir:        c.TargetDir,
		ValidationStatus: c.ValidationStatus,
		StatusIcon:       c.StatusIcon,
	}
	for _, w := range c.Warnings {
		out.Warnings = append(out.Warnings, &v1.SkillImportIssue{Type: w.Type, Message: w.Message})
	}
	for _, b := range c.Blocks {
		out.Blocks = append(out.Blocks, &v1.SkillImportIssue{Type: b.Type, Message: b.Message})
	}
	return out
}

func skillConflictGroupToProto(g biz.SkillConflictGroup) *v1.SkillConflictGroup {
	out := &v1.SkillConflictGroup{
		GroupId:                g.GroupID,
		HighestSimilarityScore: g.HighestSimilarityScore,
		Metrics: &v1.SkillSimilarityMetrics{
			SimilarityScore:       g.Metrics.SimilarityScore,
			NameSimilarity:        g.Metrics.NameSimilarity,
			DescriptionSimilarity: g.Metrics.DescriptionSimilarity,
			BodySimilarity:        g.Metrics.BodySimilarity,
			TriggerSimilarity:     g.Metrics.TriggerSimilarity,
			ToolSimilarity:        g.Metrics.ToolSimilarity,
			ConflictRisk:          g.Metrics.ConflictRisk,
			Recommendation:        g.Metrics.Recommendation,
			Confidence:            g.Metrics.Confidence,
		},
		Reason:       g.Reason,
		Evidence:     append([]string(nil), g.Evidence...),
		CandidateIds: append([]string(nil), g.CandidateIDs...),
		CanRefine:    g.CanRefine,
	}
	for _, e := range g.ExistingSkills {
		out.ExistingSkills = append(out.ExistingSkills, &v1.SkillSimilaritySource{
			Id:          e.ID,
			Name:        e.Name,
			Slug:        e.Slug,
			Description: e.Description,
			Version:     e.Version,
			BodyPreview: e.BodyPreview,
		})
	}
	return out
}

func skillImportApplyFromProto(req *v1.ApplySkillImportRequest) biz.SkillImportApplyRequest {
	out := biz.SkillImportApplyRequest{}
	for _, d := range req.GetDecisions() {
		dec := biz.SkillImportDecision{
			CandidateID:       d.GetCandidateId(),
			GroupID:           d.GetGroupId(),
			Action:            d.GetAction(),
			MergedName:        d.GetMergedName(),
			MergedDescription: d.GetMergedDescription(),
			MergedBody:        d.GetMergedBody(),
			RetireSources:     d.GetRetireSources(),
		}
		for _, t := range d.GetMergedTags() {
			dec.MergedTags = append(dec.MergedTags, biz.SkillTag{Name: t.GetName(), Source: t.GetSource()})
		}
		out.Decisions = append(out.Decisions, dec)
	}
	return out
}

func skillImportApplyResultToProto(r biz.SkillImportApplyResult) *v1.SkillImportApplyResult {
	return &v1.SkillImportApplyResult{
		CreatedSkillIds:     append([]string(nil), r.CreatedSkillIDs...),
		SkippedCandidateIds: append([]string(nil), r.SkippedCandidateIDs...),
		Message:             r.Message,
	}
}

func skillRefineResultToProto(r biz.SkillRefineResult) *v1.SkillRefineResult {
	out := &v1.SkillRefineResult{
		MergedName:             r.MergedName,
		MergedDescription:      r.MergedDescription,
		MergedBody:             r.MergedBody,
		SourceCandidateIds:     append([]string(nil), r.SourceCandidateIDs...),
		SourceExistingSkillIds: append([]string(nil), r.SourceExistingSkillIDs...),
	}
	for _, t := range r.MergedTags {
		out.MergedTags = append(out.MergedTags, &v1.SkillTag{Name: t.Name, Source: t.Source})
	}
	return out
}

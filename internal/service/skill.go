package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	v1 "aranea-agents/api/kratos/skill/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/skill/manifest"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
)

// SkillService implements kratos skill.v1.
type SkillService struct {
	v1.UnimplementedSkillServiceServer

	uc       *biz.SkillUsecase
	agentUC  *biz.AgentUsecase
	healthUC *biz.SkillHealthUsecase
	// skillHealth feeds historical-performance ranking in PreviewSkillRuntime
	// (R1, 2026-08-13) so the preview matches the production routing path.
	// Nil disables the dynamic ranking branch in the preview.
	skillHealth *SkillHealthMetricsAdapter
	fs          biz.SkillFilesystem
	import_     *importer.Engine
	gate        *evaluation.PublishGate
	lg          loggateway.Logger
}

func NewSkillService(uc *biz.SkillUsecase, agentUC *biz.AgentUsecase, healthUC *biz.SkillHealthUsecase, skillHealth *SkillHealthMetricsAdapter, fs biz.SkillFilesystem, importEng *importer.Engine, gate *evaluation.PublishGate, lg loggateway.Logger) *SkillService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SkillService{uc: uc, agentUC: agentUC, healthUC: healthUC, skillHealth: skillHealth, fs: fs, import_: importEng, gate: gate, lg: lg}
}

// assertSkillAccess 校验 caller 是否可访问指定 skill（P2-B IDOR 防护）。
// 跨租户访问返回 NotFound（避免泄露 skill 存在性）。
// 系统 caller（cron/admin）绕过校验；空 workspace_id 的 skill 视为全局共享。
func (s *SkillService) assertSkillAccess(ctx context.Context, skillID string) error {
	if skillID == "" {
		return nil
	}
	sk, err := s.uc.Get(ctx, skillID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound("SKILL", "skill not found")
		}
		return err
	}
	if err := workspace.AssertWorkspaceOrShared(workspace.IDFromContext(ctx), sk.WorkspaceID); err != nil {
		s.lg.Warn("skill access denied: workspace mismatch",
			loggateway.StepID("skill.idor"),
			loggateway.Str("skill_id", skillID),
			loggateway.Str("caller_ws", workspace.IDFromContext(ctx)))
		return apierror.NotFound("SKILL", "skill not found")
	}
	return nil
}

func (s *SkillService) GetSkillFilesystemHealth(ctx context.Context, _ *emptypb.Empty) (*v1.SkillFilesystemHealth, error) {
	root := s.fs.ResolveRoot(ctx)
	stats, err := s.uc.FilesystemHealthStats(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.SkillFilesystemHealth{
		RootAccessible:         s.fs.RootAccessible(ctx),
		ResolvedRoot:           root,
		MissingCount:           int32(stats.MissingCount),
		PendingFilesystemCount: int32(stats.PendingFilesystemCount),
	}, nil
}

func toProtoSkill(s biz.Skill) *v1.Skill {
	out := &v1.Skill{
		Id:                   s.ID,
		Name:                 s.Name,
		Slug:                 s.Slug,
		Description:          s.Description,
		ExtendsSkillId:       s.ExtendsSkillID,
		Status:               s.Status,
		Enabled:              s.Enabled,
		FilesystemMissing:    s.FilesystemMissing,
		SyncOrigin:           s.SyncOrigin,
		Visibility:           s.Visibility,
		DefaultConfigJson:    s.DefaultConfigJSON,
		InvokeCount:          int32(s.InvokeCount),
		SuccessCount:         int32(s.SuccessCount),
		FailureCount:         int32(s.FailureCount),
		UsageCount_7D:        int32(s.UsageCount7d),
		LastAgentId:          s.LastAgentID,
		LastAgentDisplayName: s.LastAgentDisplayName,
		LastInvokedAt:        s.LastInvokedAt,
		CreatedAt:            s.CreatedAt,
		UpdatedAt:            s.UpdatedAt,
		Permissions: &v1.SkillPermissions{
			CanEdit:          s.Permissions.CanEdit,
			CanDelete:        s.Permissions.CanDelete,
			CanToggleEnabled: s.Permissions.CanToggleEnabled,
			CanDuplicate:     s.Permissions.CanDuplicate,
		},
	}
	if s.AvgDurationMS != nil {
		out.AvgDurationMs = *s.AvgDurationMS
	}
	if s.LastDurationMS != nil {
		out.LastDurationMs = int32(*s.LastDurationMS)
	}
	for _, t := range s.Tags {
		out.Tags = append(out.Tags, &v1.SkillTag{Name: t.Name, Source: t.Source})
	}
	if s.CurrentVersion != nil {
		out.CurrentVersion = &v1.SkillVersionSummary{
			Id:               s.CurrentVersion.ID,
			Version:          s.CurrentVersion.Version,
			ValidationStatus: s.CurrentVersion.ValidationStatus,
			PublishedAt:      s.CurrentVersion.PublishedAt,
		}
	}
	return out
}

func toProtoInvocation(x biz.SkillInvocation) *v1.SkillInvocation {
	return &v1.SkillInvocation{
		Id:               x.ID,
		SkillId:          x.SkillID,
		SkillName:        x.SkillName,
		SkillVersion:     x.SkillVersion,
		AgentId:          x.AgentID,
		AgentDisplayName: x.AgentDisplayName,
		UserId:           x.UserID,
		SessionId:        x.SessionID,
		Status:           x.Status,
		DurationMs:       int32(x.DurationMS),
		StartedAt:        x.StartedAt,
		EndedAt:          x.EndedAt,
		InputPreview:     x.InputPreview,
		InputHash:        x.InputHash,
		OutputPreview:    x.OutputPreview,
		ErrorCode:        x.ErrorCode,
		ErrorMessage:     x.ErrorMessage,
		Source:           x.Source,
		ActivationId:     x.ActivationID,
		MessageId:        x.MessageID,
		Permissions: &v1.SkillInvocationPermissions{
			CanViewDetail: x.Permissions.CanViewDetail,
		},
	}
}

// normalizeSkillSortBy 白名单化排序字段；非法值回落为空（默认排序）。
func normalizeSkillSortBy(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "tag":
		return "tag"
	case "name":
		return "name"
	default:
		return ""
	}
}

// normalizeSkillSortOrder 白名单化排序方向；非法值回落为 asc。
func normalizeSkillSortOrder(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "desc") {
		return "desc"
	}
	return "asc"
}

func (s *SkillService) ListSkills(ctx context.Context, req *v1.ListSkillsRequest) (*v1.ListSkillsResponse, error) {
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	q := biz.SkillListQuery{
		Search:            req.GetSearch(),
		Tags:              req.GetTags(),
		Enabled:           req.GetEnabled(),
		Status:            req.GetStatus(),
		FilesystemMissing: req.GetFilesystemMissing(),
		SyncOrigin:        req.GetSyncOrigin(),
		SortBy:            normalizeSkillSortBy(req.GetSortBy()),
		SortOrder:         normalizeSkillSortOrder(req.GetSortOrder()),
		Limit:             limit,
		Offset:            offset,
	}
	// P2-B: workspace visibility filter.
	// System caller (cron/admin) sees all; tenant caller sees shared + own.
	if !workspace.IsSystem(ctx) {
		q.WorkspaceID = workspace.IDFromContext(ctx)
	}
	result, err := s.uc.List(ctx, q)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListSkillsResponse{
		Total:    int32(result.Total),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range result.Items {
		resp.Items = append(resp.Items, toProtoSkill(result.Items[i]))
	}
	return resp, nil
}

func (s *SkillService) ToggleSkillEnabled(ctx context.Context, req *v1.ToggleSkillEnabledRequest) (*v1.Skill, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	s.lg.Info("toggle skill enabled",
		loggateway.StepID("service.skill"),
		loggateway.Str("skill_id", req.GetId()),
		loggateway.Bool("enabled", req.GetEnabled()))
	out, err := s.uc.ToggleEnabled(ctx, req.GetId(), req.GetEnabled())
	if err != nil {
		s.lg.Error("toggle skill enabled failed",
			loggateway.StepID("service.skill"),
			loggateway.Str("skill_id", req.GetId()),
			loggateway.Err(err))
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("SKILL", "skill not found")
		}
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	return toProtoSkill(out), nil
}

func (s *SkillService) DuplicateSkill(ctx context.Context, req *v1.DuplicateSkillRequest) (*v1.Skill, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	out, err := s.uc.Duplicate(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("SKILL", "skill not found")
		}
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	return toProtoSkill(out), nil
}

func (s *SkillService) DeleteSkill(ctx context.Context, req *v1.DeleteSkillRequest) (*emptypb.Empty, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	s.lg.Info("delete skill",
		loggateway.StepID("service.skill"),
		loggateway.Str("skill_id", req.GetId()))
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		s.lg.Error("delete skill failed",
			loggateway.StepID("service.skill"),
			loggateway.Str("skill_id", req.GetId()),
			loggateway.Err(err))
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	return &emptypb.Empty{}, nil
}

func (s *SkillService) ListSkillFiles(ctx context.Context, req *v1.ListSkillFilesRequest) (*v1.ListSkillFilesResponse, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	dir, err := s.skillDir(ctx, req.GetId())
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	entries, err := s.fs.ListFiles(dir)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	items := make([]*v1.SkillFile, 0, len(entries))
	for _, e := range entries {
		items = append(items, &v1.SkillFile{
			Path: e.Path, Name: e.Name, Language: e.Language, Size: e.Size, UpdatedAt: e.UpdatedAt,
		})
	}
	return &v1.ListSkillFilesResponse{Items: items}, nil
}

func (s *SkillService) GetSkillFile(ctx context.Context, req *v1.GetSkillFileRequest) (*v1.SkillFileContent, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	dir, err := s.skillDir(ctx, req.GetId())
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	content, err := s.fs.ReadFile(dir, req.GetPath())
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	return &v1.SkillFileContent{Path: content.Path, Content: content.Content, Language: content.Language}, nil
}

func (s *SkillService) UpdateSkillFile(ctx context.Context, req *v1.UpdateSkillFileRequest) (*v1.SkillFileContent, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	s.lg.Info("update skill file",
		loggateway.StepID("service.skill"),
		loggateway.Str("skill_id", req.GetId()),
		loggateway.Str("path", req.GetPath()))
	dir, err := s.skillDir(ctx, req.GetId())
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	if err := s.fs.WriteFile(dir, req.GetPath(), req.GetContent()); err != nil {
		s.lg.Error("update skill file failed",
			loggateway.StepID("service.skill"),
			loggateway.Str("skill_id", req.GetId()),
			loggateway.Str("path", req.GetPath()),
			loggateway.Err(err))
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	invalidateAllAgentBuildCaches()
	return s.GetSkillFile(ctx, &v1.GetSkillFileRequest{Id: req.GetId(), Path: req.GetPath()})
}

func (s *SkillService) GetSkill(ctx context.Context, req *v1.GetSkillRequest) (*v1.GetSkillResponse, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	sk, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("SKILL", "skill not found")
		}
		return nil, err
	}
	body, err := s.uc.GetLatestMarkdown(ctx, req.GetId())
	if err != nil && !apierror.IsCode(err, apierror.CodeNotFound) {
		return nil, err
	}
	return &v1.GetSkillResponse{Skill: toProtoSkill(sk), BodyMarkdown: body}, nil
}

func (s *SkillService) CreateSkill(ctx context.Context, req *v1.CreateSkillRequest) (*v1.Skill, error) {
	name := strings.TrimSpace(req.GetName())
	slug := strings.TrimSpace(req.GetSlug())
	if slug != "" && (strings.ContainsAny(slug, `/\`) || strings.Contains(slug, "..")) {
		return nil, apierror.BadRequest("SKILL", "invalid slug")
	}
	body := strings.TrimSpace(req.GetBodyMarkdown())
	dir, err := s.fs.CreateSkillDir(slug, body)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	tags := make([]biz.SkillTag, 0, len(req.GetTags()))
	for _, t := range req.GetTags() {
		tags = append(tags, biz.SkillTag{Name: t.GetName(), Source: t.GetSource()})
	}
	// P2-B: tenant caller owns the new skill; system caller creates shared (workspace_id="").
	wsID := ""
	if !workspace.IsSystem(ctx) {
		wsID = workspace.IDFromContext(ctx)
	}
	out, err := s.uc.Create(ctx, biz.SkillCreateInput{
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(req.GetDescription()),
		Body:        body,
		Tags:        tags,
		Triggers:    manifest.Parse(body).Triggers,
		StorageDir:  dir,
		WorkspaceID: wsID,
	})
	if err != nil {
		// Clean up filesystem directory on DB failure
		if cleanErr := os.RemoveAll(dir); cleanErr != nil {
			s.lg.Warn("CreateSkill: failed to clean up filesystem dir after DB error",
				loggateway.StepID("service.skill"),
				loggateway.Str("dir", dir),
				loggateway.Err(cleanErr))
		}
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	invalidateAllAgentBuildCaches()
	return toProtoSkill(out), nil
}

func (s *SkillService) UpdateSkill(ctx context.Context, req *v1.UpdateSkillRequest) (*v1.Skill, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	patch := biz.SkillUpdateDraft{}
	if req.Name != nil {
		patch.HasName = true
		patch.Name = req.GetName()
	}
	if req.Description != nil {
		patch.HasDescription = true
		patch.Description = req.GetDescription()
	}
	if req.GetReplaceTags() {
		patch.HasTags = true
		for _, t := range req.GetTags() {
			patch.Tags = append(patch.Tags, biz.SkillTag{Name: t.GetName(), Source: t.GetSource()})
		}
	}
	if req.BodyMarkdown != nil {
		patch.HasBody = true
		patch.Body = req.GetBodyMarkdown()
		// P1-3：正文变更时从新 frontmatter 同步刷新确定性触发词。
		patch.HasTriggers = true
		patch.Triggers = manifest.Parse(patch.Body).Triggers
	}
	out, err := s.uc.Patch(ctx, req.GetId(), patch)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("SKILL", "skill not found")
		}
		if _, ok := apierror.From(err); ok {
			return nil, err // Already a properly typed domain error
		}
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	invalidateAllAgentBuildCaches()
	return toProtoSkill(out), nil
}

func (s *SkillService) PublishSkill(ctx context.Context, req *v1.PublishSkillRequest) (*v1.Skill, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	s.lg.Info("publish skill",
		loggateway.StepID("service.skill"),
		loggateway.Str("skill_id", req.GetId()))
	// P2-1: publish regression gate — blocked publishes return Conflict and
	// the skill stays unpublished. Nil gate / disabled config = no-op.
	if err := s.gate.Check(ctx, biz.EvalGateTriggerSkillPublish); err != nil {
		return nil, err
	}
	out, err := s.uc.Publish(ctx, req.GetId())
	if err != nil {
		s.lg.Error("publish skill failed",
			loggateway.StepID("service.skill"),
			loggateway.Str("skill_id", req.GetId()),
			loggateway.Err(err))
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("SKILL", "skill not found")
		}
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	return toProtoSkill(out), nil
}

func (s *SkillService) DeleteSkillFile(ctx context.Context, req *v1.DeleteSkillFileRequest) (*emptypb.Empty, error) {
	if err := s.assertSkillAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	rel := strings.TrimSpace(req.GetPath())
	if rel == "" {
		return nil, apierror.BadRequest("SKILL", "path is required")
	}
	if strings.EqualFold(filepath.ToSlash(rel), "SKILL.md") || strings.EqualFold(filepath.ToSlash(rel), "skill.md") {
		return nil, apierror.BadRequest("SKILL", "cannot delete primary SKILL.md via DeleteSkillFile")
	}
	dir, err := s.skillDir(ctx, req.GetId())
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	if err := s.fs.DeleteFile(dir, rel); err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	invalidateAllAgentBuildCaches()
	return &emptypb.Empty{}, nil
}

func (s *SkillService) PreviewSkillRuntime(ctx context.Context, req *v1.PreviewSkillRuntimeRequest) (*v1.PreviewSkillRuntimeResponse, error) {
	root := s.fs.ResolveRoot(ctx)
	agentID := strings.TrimSpace(req.GetAgentId())
	userQuery := strings.TrimSpace(req.GetUserQuery())

	if agentID != "" {
		runtime, err := s.agentUC.GetAgentRuntimeSettings(ctx, agentID)
		if err != nil {
			return nil, err
		}
		opts := &skillruntime.SkillToolsetOptions{Runtime: &runtime, UserQuery: userQuery}
		// R1: preview must reflect the production routing path, including
		// historical-performance fusion. Typed-nil guard: a nil adapter would
		// produce a non-nil interface and panic inside lookup.
		if s.skillHealth != nil {
			opts.HealthProvider = s.skillHealth
		}
		result, err := skillruntime.ResolveSkillSlugsDetailed(ctx, s.uc, opts, s.lg)
		if err != nil {
			return nil, err
		}
		return &v1.PreviewSkillRuntimeResponse{
			ResolvedStorageRoot:   root,
			EnabledPublishedCount: int32(len(result.Slugs)),
			EnabledSkillSlugs:     result.Slugs,
			Reasons:               result.Reasons,
		}, nil
	}

	slugs, err := s.uc.ListEnabledPublishedSkillKeys(ctx)
	if err != nil {
		return nil, err
	}
	reasons := make(map[string]string, len(slugs))
	for _, slug := range slugs {
		reasons[slug] = "enabled and published"
	}
	return &v1.PreviewSkillRuntimeResponse{
		ResolvedStorageRoot:   root,
		EnabledPublishedCount: int32(len(slugs)),
		EnabledSkillSlugs:     slugs,
		Reasons:               reasons,
	}, nil
}

func (s *SkillService) ListSkillRuns(ctx context.Context, req *v1.ListSkillRunsRequest) (*v1.ListSkillRunsResponse, error) {
	if err := s.assertSkillAccess(ctx, req.GetSkillId()); err != nil {
		return nil, err
	}
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	q := biz.SkillRunQuery{
		SkillID:   req.GetSkillId(),
		AgentID:   req.GetAgentId(),
		SessionID: req.GetSessionId(),
		Status:    req.GetStatus(),
		From:      req.GetFrom(),
		To:        req.GetTo(),
		Limit:     limit,
		Offset:    offset,
	}
	result, err := s.uc.SearchRuns(ctx, q)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListSkillRunsResponse{Total: int32(result.Total), Page: page, PageSize: pageSize}
	for i := range result.Items {
		resp.Items = append(resp.Items, toProtoInvocation(result.Items[i]))
	}
	return resp, nil
}

func (s *SkillService) GetSkillVersions(ctx context.Context, req *v1.GetSkillVersionsRequest) (*v1.GetSkillVersionsResponse, error) {
	if err := s.assertSkillAccess(ctx, req.GetSkillId()); err != nil {
		return nil, err
	}
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	result, err := s.uc.ListVersions(ctx, biz.SkillVersionListQuery{
		SkillID: req.GetSkillId(),
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return nil, err
	}
	resp := &v1.GetSkillVersionsResponse{Total: int32(result.Total), Page: page, PageSize: pageSize}
	for i := range result.Items {
		resp.Items = append(resp.Items, toProtoVersionDetail(result.Items[i]))
	}
	return resp, nil
}

func (s *SkillService) RollbackSkillVersion(ctx context.Context, req *v1.RollbackSkillVersionRequest) (*v1.Skill, error) {
	if err := s.assertSkillAccess(ctx, req.GetSkillId()); err != nil {
		return nil, err
	}
	out, err := s.uc.RollbackVersion(ctx, req.GetSkillId(), req.GetVersionId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("SKILL", "skill or version not found")
		}
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	return toProtoSkill(out), nil
}

func toProtoVersionDetail(v biz.SkillVersionDetail) *v1.SkillVersionDetail {
	return &v1.SkillVersionDetail{
		Id:               v.ID,
		SkillId:          v.SkillID,
		Version:          v.Version,
		Status:           v.Status,
		ContentMarkdown:  v.ContentMarkdown,
		ValidationStatus: v.ValidationStatus,
		PublishedAt:      v.PublishedAt,
		CreatedAt:        v.CreatedAt,
		FileManifestJson: v.FileManifestJSON,
	}
}

func (s *SkillService) GetSkillHealth(ctx context.Context, req *v1.GetSkillHealthRequest) (*v1.SkillHealthMetric, error) {
	skillID := strings.TrimSpace(req.GetSkillId())
	if skillID == "" {
		return nil, apierror.BadRequest("SKILL_INTELLIGENCE", "skill_id is required")
	}
	if err := s.assertSkillAccess(ctx, skillID); err != nil {
		return nil, err
	}
	if s.healthUC == nil {
		return nil, apierror.Unavailable("SKILL_INTELLIGENCE", "skill health usecase not available")
	}
	detail, err := s.healthUC.GetSkillHealth(ctx, skillID)
	if err != nil {
		return nil, err
	}
	dailyMetrics := make([]*v1.SkillHealthDailyMetric, 0, len(detail.DailyMetrics))
	for _, dm := range detail.DailyMetrics {
		dailyMetrics = append(dailyMetrics, &v1.SkillHealthDailyMetric{
			Date:          dm.Date,
			Invocations:   int32(dm.Invocations),
			Successes:     int32(dm.Successes),
			AvgDurationMs: dm.AvgDurationMs,
			RoutedCount:   int32(dm.RoutedCount),
			LoadedCount:   int32(dm.LoadedCount),
		})
	}
	return &v1.SkillHealthMetric{
		SkillId:              detail.SkillID,
		TotalInvocations_7D:  int32(detail.TotalInvocations7d),
		SuccessCount_7D:      int32(detail.SuccessCount7d),
		SuccessRate_7D:       detail.SuccessRate7d,
		P95DurationMs_7D:     int32(detail.P95DurationMs7d),
		TotalInvocations_30D: int32(detail.TotalInvocations30d),
		SuccessCount_30D:     int32(detail.SuccessCount30d),
		SuccessRate_30D:      detail.SuccessRate30d,
		P95DurationMs_30D:    int32(detail.P95DurationMs30d),
		DailyMetrics:         dailyMetrics,
		RouteHitRate_7D:      detail.RouteHitRate7d,
		RouteHitRate_30D:     detail.RouteHitRate30d,
		RoutedCount_7D:       int32(detail.RoutedCount7d),
		LoadedCount_7D:       int32(detail.LoadedCount7d),
		RoutedCount_30D:      int32(detail.RoutedCount30d),
		LoadedCount_30D:      int32(detail.LoadedCount30d),
	}, nil
}

func skillTagInfoToProto(t biz.SkillTagInfo) *v1.SkillTagInfo {
	return &v1.SkillTagInfo{
		Name:      t.Name,
		Dimension: t.Dimension,
		Source:    t.Source,
		UsedCount: int32(t.UsedCount),
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

func (s *SkillService) ListSkillTags(ctx context.Context, _ *emptypb.Empty) (*v1.ListSkillTagsResponse, error) {
	items, err := s.uc.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.SkillTagInfo, 0, len(items))
	for _, t := range items {
		out = append(out, skillTagInfoToProto(t))
	}
	return &v1.ListSkillTagsResponse{Items: out}, nil
}

func (s *SkillService) CreateSkillTag(ctx context.Context, req *v1.CreateSkillTagRequest) (*v1.SkillTagInfo, error) {
	t, err := s.uc.CreateTag(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	return skillTagInfoToProto(t), nil
}

func (s *SkillService) RenameSkillTag(ctx context.Context, req *v1.RenameSkillTagRequest) (*v1.RenameSkillTagResponse, error) {
	rewritten, err := s.uc.RenameTag(ctx, req.GetOldName(), req.GetNewName())
	if err != nil {
		return nil, err
	}
	s.lg.Info("skill tag renamed",
		loggateway.StepID("skill.tag"),
		loggateway.Str("old", req.GetOldName()),
		loggateway.Str("new", req.GetNewName()),
		loggateway.Int("rewritten", rewritten))
	return &v1.RenameSkillTagResponse{Rewritten: int32(rewritten)}, nil
}

func (s *SkillService) DeleteSkillTag(ctx context.Context, req *v1.DeleteSkillTagRequest) (*v1.DeleteSkillTagResponse, error) {
	rewritten, err := s.uc.DeleteTag(ctx, req.GetName())
	if err != nil {
		s.lg.Error("delete skill tag failed",
			loggateway.StepID("skill.tag"),
			loggateway.Str("name", req.GetName()),
			loggateway.Err(err))
		return nil, err
	}
	s.lg.Info("skill tag deleted",
		loggateway.StepID("skill.tag"),
		loggateway.Str("name", req.GetName()),
		loggateway.Int("rewritten", rewritten))
	return &v1.DeleteSkillTagResponse{Rewritten: int32(rewritten)}, nil
}

func (s *SkillService) skillDir(ctx context.Context, id string) (string, error) {
	dir, err := s.uc.GetStorageDir(ctx, id)
	if err != nil || strings.TrimSpace(dir) == "" {
		current, getErr := s.uc.Get(ctx, id)
		if getErr != nil {
			if err != nil {
				return "", err
			}
			return "", getErr
		}
		dir = filepath.Join(s.fs.ResolveRoot(ctx), current.Slug)
	}
	if !s.fs.DirExists(dir) {
		return "", os.ErrNotExist
	}
	return dir, nil
}

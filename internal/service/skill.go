package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"

	v1 "aranea-agents/api/kratos/skill/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/skill/storage"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SkillService implements kratos skill.v1.
type SkillService struct {
	v1.UnimplementedSkillServiceServer

	uc      *biz.SkillUsecase
	sys     biz.SystemSettingRepo
	import_ *importer.Engine
}

func NewSkillService(uc *biz.SkillUsecase, sys biz.SystemSettingRepo, importEng *importer.Engine) *SkillService {
	return &SkillService{uc: uc, sys: sys, import_: importEng}
}

func (s *SkillService) resolvedStorageRoot(ctx context.Context) string {
	if s.sys != nil {
		if st, err := s.sys.Get(ctx); err == nil {
			return storage.ResolveRootWithPlatform(st.RootDirectory)
		}
	}
	return storage.ResolveRoot()
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
		Permissions: &v1.SkillInvocationPermissions{
			CanViewDetail: x.Permissions.CanViewDetail,
		},
	}
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
		Limit:             limit,
		Offset:            offset,
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

func (s *SkillService) GetSkillFilesystemHealth(ctx context.Context, _ *emptypb.Empty) (*v1.SkillFilesystemHealth, error) {
	root := s.resolvedStorageRoot(ctx)
	rootAccessible := true
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		rootAccessible = false
	}
	stats, err := s.uc.FilesystemHealthStats(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.SkillFilesystemHealth{
		RootAccessible:         rootAccessible,
		ResolvedRoot:           root,
		MissingCount:           int32(stats.MissingCount),
		PendingFilesystemCount: int32(stats.PendingFilesystemCount),
	}, nil
}

func (s *SkillService) ToggleSkillEnabled(ctx context.Context, req *v1.ToggleSkillEnabledRequest) (*v1.Skill, error) {
	out, err := s.uc.ToggleEnabled(ctx, req.GetId(), req.GetEnabled())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("SKILL", "skill not found")
		}
		return nil, err
	}
	return toProtoSkill(out), nil
}

func (s *SkillService) DuplicateSkill(ctx context.Context, req *v1.DuplicateSkillRequest) (*v1.Skill, error) {
	out, err := s.uc.Duplicate(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("SKILL", "skill not found")
		}
		return nil, err
	}
	return toProtoSkill(out), nil
}

func (s *SkillService) DeleteSkill(ctx context.Context, req *v1.DeleteSkillRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *SkillService) ListSkillFiles(ctx context.Context, req *v1.ListSkillFilesRequest) (*v1.ListSkillFilesResponse, error) {
	root, err := s.skillDir(ctx, req.GetId())
	if err != nil {
		return nil, kerrors.BadRequest("SKILL", err.Error())
	}
	var items []*v1.SkillFile
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		items = append(items, &v1.SkillFile{
			Path: rel, Name: pathBase(rel), Language: languageForPath(rel), Size: info.Size(), UpdatedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
		return nil
	})
	if walkErr != nil {
		return nil, kerrors.BadRequest("SKILL", walkErr.Error())
	}
	return &v1.ListSkillFilesResponse{Items: items}, nil
}

func (s *SkillService) GetSkillFile(ctx context.Context, req *v1.GetSkillFileRequest) (*v1.SkillFileContent, error) {
	root, path, err := s.safeSkillFilePath(ctx, req.GetId(), req.GetPath())
	if err != nil {
		return nil, kerrors.BadRequest("SKILL", err.Error())
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, kerrors.BadRequest("SKILL", "skill file path points to a directory")
	}
	if info.Size() > 2*1024*1024 {
		return nil, kerrors.BadRequest("SKILL", "skill file is too large to edit")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rel, _ := filepath.Rel(root, path)
	rel = filepath.ToSlash(rel)
	return &v1.SkillFileContent{Path: rel, Content: string(raw), Language: languageForPath(rel)}, nil
}

func (s *SkillService) UpdateSkillFile(ctx context.Context, req *v1.UpdateSkillFileRequest) (*v1.SkillFileContent, error) {
	_, path, err := s.safeSkillFilePath(ctx, req.GetId(), req.GetPath())
	if err != nil {
		return nil, kerrors.BadRequest("SKILL", err.Error())
	}
	if err := os.WriteFile(path, []byte(req.GetContent()), 0o644); err != nil {
		return nil, err
	}
	return s.GetSkillFile(ctx, &v1.GetSkillFileRequest{Id: req.GetId(), Path: req.GetPath()})
}

func (s *SkillService) GetSkill(ctx context.Context, req *v1.GetSkillRequest) (*v1.GetSkillResponse, error) {
	sk, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("SKILL", "skill not found")
		}
		return nil, err
	}
	body, err := s.uc.GetLatestMarkdown(ctx, req.GetId())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return &v1.GetSkillResponse{Skill: toProtoSkill(sk), BodyMarkdown: body}, nil
}

func (s *SkillService) CreateSkill(ctx context.Context, req *v1.CreateSkillRequest) (*v1.Skill, error) {
	name := strings.TrimSpace(req.GetName())
	slug := strings.TrimSpace(req.GetSlug())
	if slug != "" && (strings.ContainsAny(slug, `/\`) || strings.Contains(slug, "..")) {
		return nil, kerrors.BadRequest("SKILL", "invalid slug")
	}
	root := s.resolvedStorageRoot(ctx)
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	body := strings.TrimSpace(req.GetBodyMarkdown())
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		return nil, err
	}
	tags := make([]biz.SkillTag, 0, len(req.GetTags()))
	for _, t := range req.GetTags() {
		tags = append(tags, biz.SkillTag{Name: t.GetName(), Source: t.GetSource()})
	}
	out, err := s.uc.Create(ctx, biz.SkillCreateInput{
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(req.GetDescription()),
		Body:        body,
		Tags:        tags,
		StorageDir:  dir,
	})
	if err != nil {
		return nil, kerrors.BadRequest("SKILL", err.Error())
	}
	return toProtoSkill(out), nil
}

func (s *SkillService) UpdateSkill(ctx context.Context, req *v1.UpdateSkillRequest) (*v1.Skill, error) {
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
	}
	out, err := s.uc.Patch(ctx, req.GetId(), patch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("SKILL", "skill not found")
		}
		return nil, kerrors.BadRequest("SKILL", err.Error())
	}
	return toProtoSkill(out), nil
}

func (s *SkillService) PublishSkill(ctx context.Context, req *v1.PublishSkillRequest) (*v1.Skill, error) {
	out, err := s.uc.Publish(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("SKILL", "skill not found")
		}
		return nil, err
	}
	return toProtoSkill(out), nil
}

func (s *SkillService) DeleteSkillFile(ctx context.Context, req *v1.DeleteSkillFileRequest) (*emptypb.Empty, error) {
	rel := strings.TrimSpace(req.GetPath())
	if rel == "" {
		return nil, kerrors.BadRequest("SKILL", "path is required")
	}
	if strings.EqualFold(filepath.ToSlash(rel), "SKILL.md") || strings.EqualFold(filepath.ToSlash(rel), "skill.md") {
		return nil, kerrors.BadRequest("SKILL", "cannot delete primary SKILL.md via DeleteSkillFile")
	}
	_, path, err := s.safeSkillFilePath(ctx, req.GetId(), rel)
	if err != nil {
		return nil, kerrors.BadRequest("SKILL", err.Error())
	}
	if err := os.Remove(path); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *SkillService) PreviewSkillRuntime(ctx context.Context, _ *v1.PreviewSkillRuntimeRequest) (*v1.PreviewSkillRuntimeResponse, error) {
	root := s.resolvedStorageRoot(ctx)
	slugs, err := s.uc.ListEnabledPublishedSkillKeys(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.PreviewSkillRuntimeResponse{
		ResolvedStorageRoot:   root,
		EnabledPublishedCount: int32(len(slugs)),
		EnabledSkillSlugs:     slugs,
	}, nil
}

func (s *SkillService) ListSkillRuns(ctx context.Context, req *v1.ListSkillRunsRequest) (*v1.ListSkillRunsResponse, error) {
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
		dir = filepath.Join(s.resolvedStorageRoot(ctx), current.Slug)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		return "", statErr
	}
	return dir, nil
}

func (s *SkillService) safeSkillFilePath(ctx context.Context, id string, relPath string) (string, string, error) {
	root, err := s.skillDir(ctx, id)
	if err != nil {
		return "", "", err
	}
	relPath = strings.TrimSpace(filepath.ToSlash(relPath))
	if relPath == "" || strings.Contains(relPath, "..") || strings.HasPrefix(relPath, "/") {
		return "", "", errors.New("unsafe skill file path")
	}
	path := filepath.Join(root, filepath.FromSlash(relPath))
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	if absPath != absRoot && !strings.HasPrefix(absPath, absRoot+string(os.PathSeparator)) {
		return "", "", errors.New("skill file path escapes skill directory")
	}
	return absRoot, absPath, nil
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return "markdown"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".py":
		return "python"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".sh":
		return "shell"
	default:
		return "text"
	}
}

func pathBase(name string) string {
	name = strings.Trim(filepath.ToSlash(name), "/")
	if name == "" || name == "." {
		return ""
	}
	parts := strings.Split(name, "/")
	return parts[len(parts)-1]
}

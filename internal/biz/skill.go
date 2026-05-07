package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// SkillTag mirrors admin JSON.
type SkillTag struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type SkillVersionSummary struct {
	ID               string
	Version          string
	ValidationStatus string
	PublishedAt      string
}

type SkillPermissions struct {
	CanEdit          bool
	CanDelete        bool
	CanToggleEnabled bool
	CanDuplicate     bool
}

// Skill is one skill row + aggregates for list/detail.
type Skill struct {
	ID                   string
	Name                 string
	Slug                 string
	Description          string
	Tags                 []SkillTag
	ExtendsSkillID       string
	Status               string
	Enabled              bool
	CurrentVersion       *SkillVersionSummary
	InvokeCount          int
	SuccessCount         int
	FailureCount         int
	UsageCount7d         int
	AvgDurationMS        *float64
	LastAgentID          string
	LastAgentDisplayName string
	LastInvokedAt        string
	LastDurationMS       *int
	CreatedAt            string
	UpdatedAt            string
	Permissions          SkillPermissions
}

type SkillListQuery struct {
	Search  string
	Tags    string
	Enabled string
	Status  string
	Limit   int
	Offset  int
}

type SkillListResult struct {
	Items  []Skill
	Total  int
	Limit  int
	Offset int
}

type SkillInvocationPermissions struct {
	CanViewDetail bool
}

type SkillInvocation struct {
	ID               string
	SkillID          string
	SkillName        string
	SkillVersion     string
	AgentID          string
	AgentDisplayName string
	UserID           string
	SessionID        string
	Status           string
	DurationMS       int
	StartedAt        string
	EndedAt          string
	InputPreview     string
	InputHash        string
	OutputPreview    string
	ErrorCode        string
	ErrorMessage     string
	Source           string
	ActivationID     string
	Permissions      SkillInvocationPermissions
}

type SkillRunQuery struct {
	SkillID   string
	AgentID   string
	SessionID string
	Status    string
	From      string
	To        string
	Limit     int
	Offset    int
}

type SkillRunResult struct {
	Items  []SkillInvocation
	Total  int
	Limit  int
	Offset int
}

type SkillRepo interface {
	SearchSkills(ctx context.Context, q SkillListQuery) (SkillListResult, error)
	GetSkillByID(ctx context.Context, id string) (Skill, error)
	UpdateSkillEnabled(ctx context.Context, id string, enabled bool) (Skill, error)
	DuplicateSkill(ctx context.Context, id string) (Skill, error)
	DeleteSkill(ctx context.Context, id string) error
	SearchSkillInvocations(ctx context.Context, q SkillRunQuery) (SkillRunResult, error)
	GetSkillStorageDir(ctx context.Context, id string) (string, error)
	ListSkillSimilaritySources(ctx context.Context) ([]SkillSimilaritySource, error)
	CreateSkillWithVersion(ctx context.Context, in SkillCreateInput) (Skill, error)
	GetSkillBySkillKey(ctx context.Context, skillKey string) (Skill, error)
	UpsertSkillFromDisk(ctx context.Context, in SkillDiskSyncInput) (Skill, error)
	ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error)
	ListEnabledPublishedSkillCandidates(ctx context.Context) ([]SkillRuntimeCandidate, error)
	RecordSkillInvocation(ctx context.Context, in SkillInvocationWrite) error
	GetLatestSkillMarkdown(ctx context.Context, skillID string) (string, error)
	PatchSkill(ctx context.Context, id string, patch SkillUpdateDraft) (Skill, error)
	PublishSkill(ctx context.Context, id string) (Skill, error)
	MarkSkillFilesystemMissing(ctx context.Context, slug string, missing bool) error
}

// SkillUpdateDraft is a partial update for admin edits (optional fields via booleans).
type SkillUpdateDraft struct {
	HasName        bool
	Name           string
	HasDescription bool
	Description    string
	HasTags        bool
	Tags           []SkillTag
	HasBody        bool
	Body           string
}

type SkillUsecase struct {
	repo SkillRepo
}

func NewSkillUsecase(repo SkillRepo) *SkillUsecase {
	return &SkillUsecase{repo: repo}
}

func (u *SkillUsecase) List(ctx context.Context, q SkillListQuery) (SkillListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	q.Enabled = strings.TrimSpace(q.Enabled)
	if q.Enabled != "" && q.Enabled != "true" && q.Enabled != "false" {
		return SkillListResult{}, errors.BadRequest("SKILL", "enabled must be true or false")
	}
	q.Status = strings.TrimSpace(q.Status)
	if q.Status != "" && q.Status != "draft" && q.Status != "published" && q.Status != "archived" {
		return SkillListResult{}, errors.BadRequest("SKILL", "unsupported skill status")
	}
	return u.repo.SearchSkills(ctx, q)
}

func (u *SkillUsecase) Get(ctx context.Context, id string) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	s, err := u.repo.GetSkillByID(ctx, id)
	if err != nil {
		return Skill{}, err
	}
	return s, nil
}

func (u *SkillUsecase) Create(ctx context.Context, in SkillCreateInput) (Skill, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	in.Description = strings.TrimSpace(in.Description)
	in.Body = strings.TrimSpace(in.Body)
	in.StorageDir = strings.TrimSpace(in.StorageDir)
	if in.Name == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill name is required")
	}
	if in.Slug == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill slug is required")
	}
	return u.repo.CreateSkillWithVersion(ctx, in)
}

func (u *SkillUsecase) ToggleEnabled(ctx context.Context, id string, enabled bool) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.UpdateSkillEnabled(ctx, id, enabled)
}

func (u *SkillUsecase) Duplicate(ctx context.Context, id string) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.DuplicateSkill(ctx, id)
}

func (u *SkillUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.DeleteSkill(ctx, id)
}

func (u *SkillUsecase) SearchRuns(ctx context.Context, q SkillRunQuery) (SkillRunResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	q.Status = strings.TrimSpace(q.Status)
	if q.Status != "" && q.Status != "success" && q.Status != "failure" && q.Status != "pending" {
		return SkillRunResult{}, errors.BadRequest("SKILL", "unsupported run status")
	}
	return u.repo.SearchSkillInvocations(ctx, q)
}

func (u *SkillUsecase) GetStorageDir(ctx context.Context, id string) (string, error) {
	return u.repo.GetSkillStorageDir(ctx, id)
}

// SkillInvocationWrite inserts a skill_invocation row (filesystem sync, runtime, etc.).
type SkillInvocationWrite struct {
	SkillID       string
	SkillName     string
	SkillVersion  string
	AgentID       string
	UserID        string
	SessionID     string
	Status        string
	DurationMS    int
	StartedAt     string
	EndedAt       string
	InputPreview  string
	OutputPreview string
	ErrorCode     string
	ErrorMessage  string
	Source        string
	ActivationID  string
}

func (u *SkillUsecase) GetBySkillKey(ctx context.Context, skillKey string) (Skill, error) {
	skillKey = strings.TrimSpace(skillKey)
	if skillKey == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill key is required")
	}
	return u.repo.GetSkillBySkillKey(ctx, skillKey)
}

func (u *SkillUsecase) UpsertSkillFromDisk(ctx context.Context, in SkillDiskSyncInput) (Skill, error) {
	return u.repo.UpsertSkillFromDisk(ctx, in)
}

func (u *SkillUsecase) ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error) {
	return u.repo.ListEnabledPublishedSkillKeys(ctx)
}

func (u *SkillUsecase) ListEnabledPublishedSkillCandidates(ctx context.Context) ([]SkillRuntimeCandidate, error) {
	return u.repo.ListEnabledPublishedSkillCandidates(ctx)
}

func (u *SkillUsecase) RecordInvocation(ctx context.Context, in SkillInvocationWrite) error {
	return u.repo.RecordSkillInvocation(ctx, in)
}

func (u *SkillUsecase) GetLatestMarkdown(ctx context.Context, id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.GetLatestSkillMarkdown(ctx, id)
}

func (u *SkillUsecase) Patch(ctx context.Context, id string, patch SkillUpdateDraft) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.PatchSkill(ctx, id, patch)
}

func (u *SkillUsecase) Publish(ctx context.Context, id string) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.PublishSkill(ctx, id)
}

func (u *SkillUsecase) MarkFilesystemMissing(ctx context.Context, slug string, missing bool) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return errors.BadRequest("SKILL", "skill slug is required")
	}
	return u.repo.MarkSkillFilesystemMissing(ctx, slug, missing)
}

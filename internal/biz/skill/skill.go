// Package skill implements skill CRUD, import, and runtime routing workflows.
package skill

import (
	"context"
	"encoding/json"
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

type ListQuery struct {
	Search  string
	Tags    string
	Enabled string
	Status  string
	Limit   int
	Offset  int
}

type ListResult struct {
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

type RunQuery struct {
	SkillID   string
	AgentID   string
	SessionID string
	Status    string
	From      string
	To        string
	Limit     int
	Offset    int
}

type RunResult struct {
	Items  []SkillInvocation
	Total  int
	Limit  int
	Offset int
}

type Repo interface {
	SearchSkills(ctx context.Context, q ListQuery) (ListResult, error)
	GetSkillByID(ctx context.Context, id string) (Skill, error)
	UpdateSkillEnabled(ctx context.Context, id string, enabled bool) (Skill, error)
	DuplicateSkill(ctx context.Context, id string) (Skill, error)
	DeleteSkill(ctx context.Context, id string) error
	SearchSkillInvocations(ctx context.Context, q RunQuery) (RunResult, error)
	GetSkillStorageDir(ctx context.Context, id string) (string, error)
	ListSkillSimilaritySources(ctx context.Context) ([]SimilaritySource, error)
	CreateSkillWithVersion(ctx context.Context, in CreateInput) (Skill, error)
	GetSkillBySkillKey(ctx context.Context, skillKey string) (Skill, error)
	UpsertSkillFromDisk(ctx context.Context, in DiskSyncInput) (Skill, error)
	ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error)
	ListEnabledPublishedSkillCandidates(ctx context.Context) ([]RuntimeCandidate, error)
	RecordSkillInvocation(ctx context.Context, in InvocationWrite) error
	GetLatestSkillMarkdown(ctx context.Context, skillID string) (string, error)
	PatchSkill(ctx context.Context, id string, patch UpdateDraft) (Skill, error)
	PublishSkill(ctx context.Context, id string) (Skill, error)
	MarkSkillFilesystemMissing(ctx context.Context, slug string, missing bool) error
}

// UpdateDraft is a partial update for admin edits (optional fields via booleans).
type UpdateDraft struct {
	HasName        bool
	Name           string
	HasDescription bool
	Description    string
	HasTags        bool
	Tags           []SkillTag
	HasBody        bool
	Body           string
}

// InvocationWrite inserts a skill_invocation row (filesystem sync, runtime, etc.).
type InvocationWrite struct {
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

// CreateInput creates platform skill + initial skill_version (import / directory sync).
type CreateInput struct {
	Name        string
	Slug        string
	Description string
	Body        string
	Tags        []SkillTag
	StorageDir  string
}

// DiskSyncInput upserts skill rows from on-disk packages (directory watcher).
type DiskSyncInput struct {
	Name        string
	Slug        string
	Description string
	Body        string
	Tags        []SkillTag
	StorageDir  string
}

// Usecase implements skill CRUD workflows.
type Usecase struct {
	repo Repo
}

// NewUsecase constructs a SkillUsecase.
func NewUsecase(repo Repo) *Usecase {
	return &Usecase{repo: repo}
}

func (u *Usecase) List(ctx context.Context, q ListQuery) (ListResult, error) {
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
		return ListResult{}, errors.BadRequest("SKILL", "enabled must be true or false")
	}
	q.Status = strings.TrimSpace(q.Status)
	if q.Status != "" && q.Status != "draft" && q.Status != "published" && q.Status != "archived" {
		return ListResult{}, errors.BadRequest("SKILL", "unsupported skill status")
	}
	return u.repo.SearchSkills(ctx, q)
}

func (u *Usecase) Get(ctx context.Context, id string) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	s, err := u.repo.GetSkillByID(ctx, id)
	if err != nil {
		return Skill{}, err
	}
	return s, nil
}

func (u *Usecase) Create(ctx context.Context, in CreateInput) (Skill, error) {
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

func (u *Usecase) ToggleEnabled(ctx context.Context, id string, enabled bool) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.UpdateSkillEnabled(ctx, id, enabled)
}

func (u *Usecase) Duplicate(ctx context.Context, id string) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.DuplicateSkill(ctx, id)
}

func (u *Usecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.DeleteSkill(ctx, id)
}

func (u *Usecase) SearchRuns(ctx context.Context, q RunQuery) (RunResult, error) {
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
		return RunResult{}, errors.BadRequest("SKILL", "unsupported run status")
	}
	return u.repo.SearchSkillInvocations(ctx, q)
}

func (u *Usecase) GetStorageDir(ctx context.Context, id string) (string, error) {
	return u.repo.GetSkillStorageDir(ctx, id)
}

func (u *Usecase) GetBySkillKey(ctx context.Context, skillKey string) (Skill, error) {
	skillKey = strings.TrimSpace(skillKey)
	if skillKey == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill key is required")
	}
	return u.repo.GetSkillBySkillKey(ctx, skillKey)
}

func (u *Usecase) UpsertSkillFromDisk(ctx context.Context, in DiskSyncInput) (Skill, error) {
	return u.repo.UpsertSkillFromDisk(ctx, in)
}

func (u *Usecase) ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error) {
	return u.repo.ListEnabledPublishedSkillKeys(ctx)
}

func (u *Usecase) ListEnabledPublishedSkillCandidates(ctx context.Context) ([]RuntimeCandidate, error) {
	return u.repo.ListEnabledPublishedSkillCandidates(ctx)
}

func (u *Usecase) RecordInvocation(ctx context.Context, in InvocationWrite) error {
	return u.repo.RecordSkillInvocation(ctx, in)
}

func (u *Usecase) GetLatestMarkdown(ctx context.Context, id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.GetLatestSkillMarkdown(ctx, id)
}

func (u *Usecase) Patch(ctx context.Context, id string, patch UpdateDraft) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.PatchSkill(ctx, id, patch)
}

func (u *Usecase) Publish(ctx context.Context, id string) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.PublishSkill(ctx, id)
}

func (u *Usecase) MarkFilesystemMissing(ctx context.Context, slug string, missing bool) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return errors.BadRequest("SKILL", "skill slug is required")
	}
	return u.repo.MarkSkillFilesystemMissing(ctx, slug, missing)
}

// GetBySlug returns a skill by its slug (alias for GetSkillBySkillKey).
func (u *Usecase) GetBySlug(ctx context.Context, slug string) (Skill, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Skill{}, errors.BadRequest("SKILL", "skill slug is required")
	}
	return u.repo.GetSkillBySkillKey(ctx, slug)
}

// ListEnabledPublishedCandidates is a shorter alias for ListEnabledPublishedSkillCandidates.
func (u *Usecase) ListEnabledPublishedCandidates(ctx context.Context) ([]RuntimeCandidate, error) {
	return u.repo.ListEnabledPublishedSkillCandidates(ctx)
}

// ── Skill invocation source constants ─────────────────────────────────────────

const (
	InvocationSourceRuntime         = "runtime"
	InvocationSourceFilesystemScan  = "filesystem_scan"
	InvocationSourceFilesystemWatch = "filesystem_watch"
)

// ── Import / similarity DTOs ──────────────────────────────────────────────────

// SimilaritySource is a lightweight skill row for import similarity matching.
type SimilaritySource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Version     string `json:"version"`
	BodyPreview string `json:"body_preview"`
	Body        string `json:"-"`
}

type ImportJob struct {
	JobID            string                 `json:"job_id"`
	Status           string                 `json:"status"`
	ValidationStatus string                 `json:"validation_status"`
	StorageRoot      string                 `json:"storage_root"`
	Candidates       []ImportCandidate      `json:"candidates"`
	ConflictGroups   []ConflictGroup        `json:"conflict_groups"`
	Message          string                 `json:"message,omitempty"`
}

type ImportCandidate struct {
	CandidateID      string        `json:"candidate_id"`
	Name             string        `json:"name"`
	Slug             string        `json:"slug"`
	Description      string        `json:"description"`
	BodyPreview      string        `json:"body_preview"`
	TargetDir        string        `json:"target_dir"`
	ValidationStatus string        `json:"validation_status"`
	StatusIcon       string        `json:"status_icon"`
	Warnings         []ImportIssue `json:"warnings"`
	Blocks           []ImportIssue `json:"blocks"`
}

type ImportIssue struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type SimilarityMetrics struct {
	SimilarityScore       float64 `json:"similarity_score"`
	NameSimilarity        float64 `json:"name_similarity"`
	DescriptionSimilarity float64 `json:"description_similarity"`
	BodySimilarity        float64 `json:"body_similarity"`
	TriggerSimilarity     float64 `json:"trigger_similarity"`
	ToolSimilarity        float64 `json:"tool_similarity"`
	ConflictRisk          string  `json:"conflict_risk"`
	Recommendation        string  `json:"recommendation"`
	Confidence            float64 `json:"confidence"`
}

type ConflictGroup struct {
	GroupID                string                 `json:"group_id"`
	HighestSimilarityScore float64                `json:"highest_similarity_score"`
	Metrics                SimilarityMetrics      `json:"metrics"`
	Reason                 string                 `json:"reason"`
	Evidence               []string               `json:"evidence"`
	CandidateIDs           []string               `json:"candidate_ids"`
	ExistingSkills         []SimilaritySource     `json:"existing_skills"`
	CanRefine              bool                   `json:"can_refine"`
}

type RefineRequest struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
}

type RefineResult struct {
	MergedName             string     `json:"merged_name"`
	MergedDescription      string     `json:"merged_description"`
	MergedBody             string     `json:"merged_body"`
	MergedTags             []SkillTag `json:"merged_tags"`
	SourceCandidateIDs     []string   `json:"source_candidate_ids"`
	SourceExistingSkillIDs []string   `json:"source_existing_skill_ids"`
}

type ImportDecision struct {
	CandidateID       string     `json:"candidate_id,omitempty"`
	GroupID           string     `json:"group_id,omitempty"`
	Action            string     `json:"action"`
	MergedName        string     `json:"merged_name,omitempty"`
	MergedDescription string     `json:"merged_description,omitempty"`
	MergedBody        string     `json:"merged_body,omitempty"`
	MergedTags        []SkillTag `json:"merged_tags,omitempty"`
}

type ImportApplyRequest struct {
	Decisions []ImportDecision `json:"decisions"`
}

type ImportApplyResult struct {
	CreatedSkillIDs     []string `json:"created_skill_ids"`
	SkippedCandidateIDs []string `json:"skipped_candidate_ids"`
	Message             string   `json:"message"`
}

// ── Runtime policy ────────────────────────────────────────────────────────────

// RuntimePolicy configures how many published Skills are exposed as ADK tools for an agent turn.
type RuntimePolicy struct {
	AllowedSlugs []string `json:"allowed_slugs"`
	DeniedSlugs  []string `json:"denied_slugs"`
	AllowedTags  []string `json:"allowed_tags"`

	IntentRoutingEnabled bool `json:"intent_routing_enabled"`
	IntentMaxPaths       int  `json:"intent_max_paths"`
	MaxSkillsInToolset   int  `json:"max_skills_in_toolset"`
}

// RuntimeCandidate is a lightweight Skill row for routing.
type RuntimeCandidate struct {
	Slug          string
	Name          string
	Description   string
	Tags          []SkillTag
	TaxonomyPaths []string
}

// ParseRuntimePolicy unmarshals skill_runtime_json with safe defaults.
func ParseRuntimePolicy(raw string) RuntimePolicy {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	var wire struct {
		AllowedSlugs         []string `json:"allowed_slugs"`
		DeniedSlugs          []string `json:"denied_slugs"`
		AllowedTags          []string `json:"allowed_tags"`
		IntentRoutingEnabled *bool    `json:"intent_routing_enabled"`
		IntentMaxPaths       int      `json:"intent_max_paths"`
		MaxSkillsInToolset   int      `json:"max_skills_in_toolset"`
	}
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		wire = struct {
			AllowedSlugs         []string `json:"allowed_slugs"`
			DeniedSlugs          []string `json:"denied_slugs"`
			AllowedTags          []string `json:"allowed_tags"`
			IntentRoutingEnabled *bool    `json:"intent_routing_enabled"`
			IntentMaxPaths       int      `json:"intent_max_paths"`
			MaxSkillsInToolset   int      `json:"max_skills_in_toolset"`
		}{}
	}
	p := RuntimePolicy{
		AllowedSlugs: wire.AllowedSlugs,
		DeniedSlugs:  wire.DeniedSlugs,
		AllowedTags:  wire.AllowedTags,
	}
	if wire.IntentRoutingEnabled != nil {
		p.IntentRoutingEnabled = *wire.IntentRoutingEnabled
	} else {
		p.IntentRoutingEnabled = true
	}
	p.IntentMaxPaths = wire.IntentMaxPaths
	p.MaxSkillsInToolset = wire.MaxSkillsInToolset

	if p.IntentMaxPaths <= 0 {
		p.IntentMaxPaths = 3
	}
	if p.MaxSkillsInToolset <= 0 {
		p.MaxSkillsInToolset = 32
	}
	if p.MaxSkillsInToolset > 256 {
		p.MaxSkillsInToolset = 256
	}
	normalizeLowerSlice(&p.AllowedSlugs)
	normalizeLowerSlice(&p.DeniedSlugs)
	normalizeTagTokens(&p.AllowedTags)
	return p
}

func normalizeLowerSlice(s *[]string) {
	out := make([]string, 0, len(*s))
	seen := map[string]bool{}
	for _, x := range *s {
		x = strings.TrimSpace(strings.ToLower(x))
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	*s = out
}

func normalizeTagTokens(s *[]string) {
	out := make([]string, 0, len(*s))
	seen := map[string]bool{}
	for _, x := range *s {
		x = strings.TrimSpace(strings.ToLower(x))
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	*s = out
}

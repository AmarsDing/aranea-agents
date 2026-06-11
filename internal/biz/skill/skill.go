// Package skill implements skill CRUD, import, and runtime routing workflows.
package skill

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	authpkg "aranea-agents/pkg/auth"
	"aranea-agents/pkg/apierror"
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

type SkillVersionDetail struct {
	ID               string
	SkillID          string
	Version          string
	Status           string
	ContentMarkdown  string
	ValidationStatus string
	PublishedAt      string
	CreatedAt        string
	FileManifestJSON string
}

type VersionListQuery struct {
	SkillID string
	Limit   int
	Offset  int
}

type VersionListResult struct {
	Items  []SkillVersionDetail
	Total  int
	Limit  int
	Offset int
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
	FilesystemMissing    bool
	SyncOrigin           string
	Visibility           string
	DefaultConfigJSON    string
	ParentVersionID      string
	EvolutionReason      string
	LifecycleStatus      string
}

type ListQuery struct {
	Search            string
	Tags              string
	Enabled           string
	Status            string
	FilesystemMissing string
	SyncOrigin        string
	Limit             int
	Offset            int
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
	MessageID        string
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

type SkillQueryReader interface {
	SearchSkills(ctx context.Context, q ListQuery) (ListResult, error)
	SearchSkillInvocations(ctx context.Context, q RunQuery) (RunResult, error)
	ListSkillVersions(ctx context.Context, q VersionListQuery) (VersionListResult, error)
	ListSkillSimilaritySources(ctx context.Context) ([]SimilaritySource, error)
	ListRegisteredSlugs(ctx context.Context) ([]string, error)
}

type SkillLookupReader interface {
	GetSkillByID(ctx context.Context, id string) (Skill, error)
	GetSkillBySkillKey(ctx context.Context, skillKey string) (Skill, error)
	GetSkillStorageDir(ctx context.Context, id string) (string, error)
	GetLatestSkillMarkdown(ctx context.Context, skillID string) (string, error)
}

type SkillRuntimeReader interface {
	BatchGetSkillMarkdownBySlugs(ctx context.Context, slugs []string) (map[string]string, error)
	ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error)
	ListEnabledPublishedSkillCandidates(ctx context.Context) ([]RuntimeCandidate, error)
	FilesystemHealthStats(ctx context.Context) (FilesystemHealthStats, error)
}

type SkillReader interface {
	SkillQueryReader
	SkillLookupReader
	SkillRuntimeReader
}

type SkillMutationWriter interface {
	CreateSkillWithVersion(ctx context.Context, in CreateInput) (Skill, error)
	UpdateSkillEnabled(ctx context.Context, id string, enabled bool) (Skill, error)
	DuplicateSkill(ctx context.Context, id string) (Skill, error)
	DeleteSkill(ctx context.Context, id string) error
	PatchSkill(ctx context.Context, id string, patch UpdateDraft) (Skill, error)
}

type SkillSyncWriter interface {
	PublishSkill(ctx context.Context, id string) (Skill, error)
	UpsertSkillFromDisk(ctx context.Context, in DiskSyncInput) (Skill, DiskSyncOutcome, error)
	MarkSkillFilesystemMissing(ctx context.Context, slug string, missing bool) error
	RecordSkillInvocation(ctx context.Context, in InvocationWrite) error
	RollbackSkillVersion(ctx context.Context, skillID string, versionID string) (Skill, error)
}

type SkillWriter interface {
	SkillMutationWriter
	SkillSyncWriter
}

type Repo interface {
	SkillReader
	SkillWriter
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
	SkillID         string
	SkillName       string
	SkillVersion    string
	AgentID         string
	UserID          string
	SessionID       string
	Status          string
	DurationMS      int
	StartedAt       string
	EndedAt         string
	InputPreview    string
	InputHash       string
	OutputPreview   string
	ErrorCode       string
	ErrorMessage    string
	Source          string
	ActivationID    string
	MessageID       string
	SelectionReason map[string]any // routing path, candidate slugs, scoring factors
	Outcome         string         // success / failure / partial / cancelled
	TokenUsage      map[string]any // {prompt, completion, total}
	RoutedSlugs     []string       // slugs routed by Layer A+B for this turn
	LoadedSlug      string         // slug actually loaded via skill_load/skill_run
}

// DiskSyncOutcome describes side effects of a filesystem upsert.
type DiskSyncOutcome struct {
	ContentChanged  bool
	RevertedToDraft bool
}

// CreateInput creates platform skill + initial skill_version (import / directory sync).
type CreateInput struct {
	Name              string
	Slug              string
	Description       string
	Body              string
	Tags              []SkillTag
	StorageDir        string
	SyncOrigin        string
	Visibility        string
	DefaultConfigJSON string
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

type SkillFileEntry struct {
	Path      string
	Name      string
	Language  string
	Size      int64
	UpdatedAt string
}

type SkillFileContent struct {
	Path     string
	Content  string
	Language string
}

type SkillFilePathResolver interface {
	ResolveRoot(ctx context.Context) string
	SafeFilePath(dir string, relPath string) (root string, absPath string, err error)
}

type SkillFileReader interface {
	SkillFilePathResolver
	ListFiles(dir string) ([]SkillFileEntry, error)
	ReadFile(dir string, relPath string) (SkillFileContent, error)
	RootAccessible(ctx context.Context) bool
	DirExists(dir string) bool
}

type SkillFileWriter interface {
	SkillFilePathResolver
	CreateSkillDir(slug string, body string) (dir string, err error)
	WriteFile(dir string, relPath string, content string) error
	DeleteFile(dir string, relPath string) error
}

type SkillFilesystem interface {
	SkillFileReader
	SkillFileWriter
}

// SkillEmbedder generates text embeddings for semantic skill scoring.
// Defined here to avoid circular import with parent biz package.
type SkillEmbedder interface {
	EmbedSingle(ctx context.Context, text string) ([]float32, error)
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// embedEntry pairs a cached embedding vector with its cache timestamp.
type embedEntry struct {
	vector   []float32
	cachedAt time.Time
}

// Usecase implements skill CRUD workflows.
type Usecase struct {
	repo       Repo
	embedder   SkillEmbedder
	embedMu    sync.RWMutex
	embedCache map[string]embedEntry
	embedTTL   time.Duration
}

// NewUsecase constructs a SkillUsecase.
func NewUsecase(repo Repo, embedder SkillEmbedder) *Usecase {
	return &Usecase{repo: repo, embedder: embedder, embedCache: make(map[string]embedEntry), embedTTL: 30 * time.Minute}
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
		return ListResult{}, apierror.BadRequest("SKILL", "enabled must be true or false")
	}
	q.Status = strings.TrimSpace(q.Status)
	if q.Status != "" && q.Status != "draft" && q.Status != "published" && q.Status != "archived" {
		return ListResult{}, apierror.BadRequest("SKILL", "unsupported skill status")
	}
	result, err := u.repo.SearchSkills(ctx, q)
	if err != nil {
		return result, err
	}
	applySkillPermissions(ctx, result.Items)
	return result, nil
}

func (u *Usecase) Get(ctx context.Context, id string) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill id is required")
	}
	s, err := u.repo.GetSkillByID(ctx, id)
	if err != nil {
		return Skill{}, err
	}
	applySkillPermission(ctx, &s)
	return s, nil
}

func (u *Usecase) Create(ctx context.Context, in CreateInput) (Skill, error) {
	if err := requireAdminAccess(ctx); err != nil {
		return Skill{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	in.Description = strings.TrimSpace(in.Description)
	in.Body = strings.TrimSpace(in.Body)
	in.StorageDir = strings.TrimSpace(in.StorageDir)
	if in.Name == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill name is required")
	}
	if in.Slug == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill slug is required")
	}
	s, err := u.repo.CreateSkillWithVersion(ctx, in)
	if err != nil {
		return Skill{}, err
	}
	applySkillPermission(ctx, &s)
	return s, nil
}

func (u *Usecase) ToggleEnabled(ctx context.Context, id string, enabled bool) (Skill, error) {
	if err := requireAdminAccess(ctx); err != nil {
		return Skill{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill id is required")
	}
	s, err := u.repo.UpdateSkillEnabled(ctx, id, enabled)
	if err != nil {
		return Skill{}, err
	}
	u.InvalidateEmbedCacheForSlug(s.Slug)
	applySkillPermission(ctx, &s)
	return s, nil
}

func (u *Usecase) Duplicate(ctx context.Context, id string) (Skill, error) {
	if err := requireAdminAccess(ctx); err != nil {
		return Skill{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill id is required")
	}
	s, err := u.repo.DuplicateSkill(ctx, id)
	if err != nil {
		return Skill{}, err
	}
	// Duplicate creates a new slug, so invalidate the new entry.
	u.InvalidateEmbedCacheForSlug(s.Slug)
	applySkillPermission(ctx, &s)
	return s, nil
}

func (u *Usecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("SKILL", "skill id is required")
	}
	if err := requireAdminAccess(ctx); err != nil {
		return err
	}
	// Fetch the skill first to get its slug for targeted cache invalidation.
	s, err := u.repo.GetSkillByID(ctx, id)
	if err != nil {
		return err
	}
	err = u.repo.DeleteSkill(ctx, id)
	if err != nil {
		return err
	}
	u.InvalidateEmbedCacheForSlug(s.Slug)
	return nil
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
		return RunResult{}, apierror.BadRequest("SKILL", "unsupported run status")
	}
	result, err := u.repo.SearchSkillInvocations(ctx, q)
	if err != nil {
		return result, err
	}
	applyInvocationPermissions(ctx, result.Items)
	return result, nil
}

func (u *Usecase) GetStorageDir(ctx context.Context, id string) (string, error) {
	return u.repo.GetSkillStorageDir(ctx, id)
}

func (u *Usecase) GetBySkillKey(ctx context.Context, skillKey string) (Skill, error) {
	skillKey = strings.TrimSpace(skillKey)
	if skillKey == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill key is required")
	}
	s, err := u.repo.GetSkillBySkillKey(ctx, skillKey)
	if err != nil {
		return Skill{}, err
	}
	applySkillPermission(ctx, &s)
	return s, nil
}

func (u *Usecase) UpsertSkillFromDisk(ctx context.Context, in DiskSyncInput) (Skill, DiskSyncOutcome, error) {
	s, outcome, err := u.repo.UpsertSkillFromDisk(ctx, in)
	if err != nil {
		return Skill{}, outcome, err
	}
	applySkillPermission(ctx, &s)
	return s, outcome, nil
}

func (u *Usecase) ListRegisteredSlugs(ctx context.Context) ([]string, error) {
	return u.repo.ListRegisteredSlugs(ctx)
}

func (u *Usecase) ListSimilaritySources(ctx context.Context) ([]SimilaritySource, error) {
	return u.repo.ListSkillSimilaritySources(ctx)
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
		return "", apierror.BadRequest("SKILL", "skill id is required")
	}
	return u.repo.GetLatestSkillMarkdown(ctx, id)
}

type SkillGuidanceEntry struct {
	Slug     string
	Guidance string
}

func (u *Usecase) BatchGetSkillGuidance(ctx context.Context, slugs []string) ([]SkillGuidanceEntry, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	markdownMap, err := u.repo.BatchGetSkillMarkdownBySlugs(ctx, slugs)
	if err != nil {
		return nil, err
	}
	out := make([]SkillGuidanceEntry, 0, len(markdownMap))
	for _, slug := range slugs {
		md, ok := markdownMap[slug]
		if !ok {
			continue
		}
		out = append(out, SkillGuidanceEntry{Slug: slug, Guidance: md})
	}
	return out, nil
}

func (u *Usecase) Patch(ctx context.Context, id string, patch UpdateDraft) (Skill, error) {
	if err := requireAdminAccess(ctx); err != nil {
		return Skill{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill id is required")
	}
	s, err := u.repo.PatchSkill(ctx, id, patch)
	if err != nil {
		return Skill{}, err
	}
	applySkillPermission(ctx, &s)
	return s, nil
}

func (u *Usecase) Publish(ctx context.Context, id string) (Skill, error) {
	if strings.TrimSpace(id) == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill id is required")
	}
	if err := requireAdminAccess(ctx); err != nil {
		return Skill{}, err
	}
	s, err := u.repo.PublishSkill(ctx, id)
	if err != nil {
		return Skill{}, err
	}
	u.InvalidateEmbedCacheForSlug(s.Slug)
	applySkillPermission(ctx, &s)
	return s, nil
}

func (u *Usecase) MarkFilesystemMissing(ctx context.Context, slug string, missing bool) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return apierror.BadRequest("SKILL", "skill slug is required")
	}
	return u.repo.MarkSkillFilesystemMissing(ctx, slug, missing)
}

func (u *Usecase) FilesystemHealthStats(ctx context.Context) (FilesystemHealthStats, error) {
	return u.repo.FilesystemHealthStats(ctx)
}

func (u *Usecase) ListVersions(ctx context.Context, q VersionListQuery) (VersionListResult, error) {
	q.SkillID = strings.TrimSpace(q.SkillID)
	if q.SkillID == "" {
		return VersionListResult{}, apierror.BadRequest("SKILL", "skill id is required")
	}
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return u.repo.ListSkillVersions(ctx, q)
}

func (u *Usecase) RollbackVersion(ctx context.Context, skillID string, versionID string) (Skill, error) {
	skillID = strings.TrimSpace(skillID)
	versionID = strings.TrimSpace(versionID)
	if skillID == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill id is required")
	}
	if versionID == "" {
		return Skill{}, apierror.BadRequest("SKILL", "version id is required")
	}
	if err := requireAdminAccess(ctx); err != nil {
		return Skill{}, err
	}
	s, err := u.repo.RollbackSkillVersion(ctx, skillID, versionID)
	if err != nil {
		return Skill{}, err
	}
	applySkillPermission(ctx, &s)
	return s, nil
}

func (u *Usecase) GetBySlug(ctx context.Context, slug string) (Skill, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Skill{}, apierror.BadRequest("SKILL", "skill slug is required")
	}
	s, err := u.repo.GetSkillBySkillKey(ctx, slug)
	if err != nil {
		return Skill{}, err
	}
	applySkillPermission(ctx, &s)
	return s, nil
}

// ListEnabledPublishedCandidates is a shorter alias for ListEnabledPublishedSkillCandidates.
func (u *Usecase) ListEnabledPublishedCandidates(ctx context.Context) ([]RuntimeCandidate, error) {
	return u.repo.ListEnabledPublishedSkillCandidates(ctx)
}

// ── Skill invocation source constants ─────────────────────────────────────────

const (
	SyncOriginFilesystem = "filesystem"
	SyncOriginImport     = "import"
	SyncOriginManual     = "manual"

	InvocationSourceRuntime             = "runtime"
	InvocationSourceFilesystemScan      = "filesystem_scan"
	InvocationSourceFilesystemWatch     = "filesystem_watch"
	InvocationSourceFilesystemReconcile = "filesystem_reconcile"
)

// FilesystemHealthStats summarizes on-disk skill registry health.
type FilesystemHealthStats struct {
	MissingCount           int
	PendingFilesystemCount int
}

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
	JobID            string            `json:"job_id"`
	Status           string            `json:"status"`
	ValidationStatus string            `json:"validation_status"`
	StorageRoot      string            `json:"storage_root"`
	Candidates       []ImportCandidate `json:"candidates"`
	ConflictGroups   []ConflictGroup   `json:"conflict_groups"`
	Message          string            `json:"message,omitempty"`
	TempDir          string            `json:"temp_dir,omitempty"` // path to temp directory for file content (DB-persisted jobs)
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
	GroupID                string             `json:"group_id"`
	HighestSimilarityScore float64            `json:"highest_similarity_score"`
	Metrics                SimilarityMetrics  `json:"metrics"`
	Reason                 string             `json:"reason"`
	Evidence               []string           `json:"evidence"`
	CandidateIDs           []string           `json:"candidate_ids"`
	ExistingSkills         []SimilaritySource `json:"existing_skills"`
	CanRefine              bool               `json:"can_refine"`
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

	IntentRoutingEnabled    bool    `json:"intent_routing_enabled"`
	IntentMaxPaths          int     `json:"intent_max_paths"`
	MaxSkillsInToolset      int     `json:"max_skills_in_toolset"`
	EmbeddingScoringEnabled bool    `json:"embedding_scoring_enabled"`
	EmbeddingScoreWeight    float64 `json:"embedding_score_weight"`
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
		AllowedSlugs            []string `json:"allowed_slugs"`
		DeniedSlugs             []string `json:"denied_slugs"`
		AllowedTags             []string `json:"allowed_tags"`
		IntentRoutingEnabled    *bool    `json:"intent_routing_enabled"`
		IntentMaxPaths          int      `json:"intent_max_paths"`
		MaxSkillsInToolset      int      `json:"max_skills_in_toolset"`
		EmbeddingScoringEnabled *bool    `json:"embedding_scoring_enabled"`
		EmbeddingScoreWeight    float64  `json:"embedding_score_weight"`
	}
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		wire = struct {
			AllowedSlugs            []string `json:"allowed_slugs"`
			DeniedSlugs             []string `json:"denied_slugs"`
			AllowedTags             []string `json:"allowed_tags"`
			IntentRoutingEnabled    *bool    `json:"intent_routing_enabled"`
			IntentMaxPaths          int      `json:"intent_max_paths"`
			MaxSkillsInToolset      int      `json:"max_skills_in_toolset"`
			EmbeddingScoringEnabled *bool    `json:"embedding_scoring_enabled"`
			EmbeddingScoreWeight    float64  `json:"embedding_score_weight"`
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
	if wire.EmbeddingScoringEnabled != nil {
		p.EmbeddingScoringEnabled = *wire.EmbeddingScoringEnabled
	}
	p.IntentMaxPaths = wire.IntentMaxPaths
	p.MaxSkillsInToolset = wire.MaxSkillsInToolset
	p.EmbeddingScoreWeight = wire.EmbeddingScoreWeight

	if p.IntentMaxPaths <= 0 {
		p.IntentMaxPaths = 3
	}
	if p.MaxSkillsInToolset <= 0 {
		p.MaxSkillsInToolset = 32
	}
	if p.MaxSkillsInToolset > 256 {
		p.MaxSkillsInToolset = 256
	}
	if p.EmbeddingScoreWeight <= 0 {
		p.EmbeddingScoreWeight = 0.3
	}
	if p.EmbeddingScoreWeight > 1 {
		p.EmbeddingScoreWeight = 1
	}
	normalizeSlugSlice(&p.AllowedSlugs)
	normalizeSlugSlice(&p.DeniedSlugs)
	normalizeLowerSlice(&p.AllowedTags)
	return p
}

// NormalizeSlug normalizes a skill slug for consistent matching across all layers.
// It applies the same rules as the importer's slugify (lowercase, replace
// non-alphanumeric with hyphens, trim separators) so that lookups and filter
// comparisons are consistent regardless of input format.
// Unlike slugify, it does NOT generate a random fallback for empty results.
func NormalizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugNormalizeRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_")
	return s
}

var slugNormalizeRe = regexp.MustCompile(`[^a-z0-9\-_]+`)

// normalizeSlugSlice normalizes a slice of slug strings using NormalizeSlug.
// Used for AllowedSlugs/DeniedSlugs which must match the slugified format
// stored in the database (e.g., "my-skill" not "My Skill").
func normalizeSlugSlice(s *[]string) {
	out := make([]string, 0, len(*s))
	seen := map[string]bool{}
	for _, x := range *s {
		x = NormalizeSlug(x)
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	*s = out
}

// normalizeLowerSlice lowercases and trims a slice of strings.
// Used for tags which have their own naming convention (e.g., "file_type:xlsx")
// and should NOT be slug-normalized.
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

func applySkillPermissions(ctx context.Context, items []Skill) {
	for i := range items {
		applySkillPermission(ctx, &items[i])
	}
}

func applySkillPermission(ctx context.Context, s *Skill) {
	a, ok := authpkg.FromContext(ctx)
	if !ok || a == nil {
		s.Permissions = SkillPermissions{}
		return
	}
	if a.HasAdminAccess() {
		s.Permissions = SkillPermissions{CanEdit: true, CanDelete: true, CanToggleEnabled: true, CanDuplicate: true}
		return
	}
	s.Permissions = SkillPermissions{CanEdit: true, CanDelete: false, CanToggleEnabled: true, CanDuplicate: true}
}

func requireAdminAccess(ctx context.Context) error {
	a, ok := authpkg.FromContext(ctx)
	if !ok || a == nil || !a.HasAdminAccess() {
		return apierror.Forbidden("SKILL", "admin access required")
	}
	return nil
}

func applyInvocationPermissions(ctx context.Context, items []SkillInvocation) {
	a, ok := authpkg.FromContext(ctx)
	canView := ok && a != nil && a.HasAdminAccess()
	for i := range items {
		items[i].Permissions = SkillInvocationPermissions{CanViewDetail: canView}
	}
}

// ScoreByEmbedding computes cosine similarity between the query and each candidate's
// cached embedding. Returns nil if embedding is unavailable or query is empty.
func (u *Usecase) ScoreByEmbedding(ctx context.Context, query string, candidates []RuntimeCandidate) (map[string]float64, error) {
	if u.embedder == nil || strings.TrimSpace(query) == "" || len(candidates) == 0 {
		return nil, nil
	}
	if err := u.refreshEmbedCache(ctx, candidates); err != nil {
		return nil, err
	}
	queryEmb, err := u.embedder.EmbedSingle(ctx, query)
	if err != nil {
		return nil, err
	}
	u.embedMu.RLock()
	defer u.embedMu.RUnlock()
	scores := make(map[string]float64, len(candidates))
	for _, c := range candidates {
		if entry, ok := u.embedCache[c.Slug]; ok && time.Since(entry.cachedAt) <= u.embedTTL {
			scores[c.Slug] = cosineSimilarity32(queryEmb, entry.vector)
		}
	}
	return scores, nil
}

// InvalidateEmbedCache clears all cached skill embeddings.
func (u *Usecase) InvalidateEmbedCache() {
	u.embedMu.Lock()
	defer u.embedMu.Unlock()
	u.embedCache = make(map[string]embedEntry)
}

// InvalidateEmbedCacheForSlug removes a single skill's embedding from cache.
// Prefer this over InvalidateEmbedCache when only one skill changes.
func (u *Usecase) InvalidateEmbedCacheForSlug(slug string) {
	slug = NormalizeSlug(slug)
	if slug == "" {
		return
	}
	u.embedMu.Lock()
	defer u.embedMu.Unlock()
	delete(u.embedCache, slug)
}

func (u *Usecase) refreshEmbedCache(ctx context.Context, candidates []RuntimeCandidate) error {
	u.embedMu.RLock()
	missing := make([]int, 0, len(candidates))
	now := time.Now()
	for i, c := range candidates {
		if entry, ok := u.embedCache[c.Slug]; !ok || now.Sub(entry.cachedAt) > u.embedTTL {
			missing = append(missing, i)
		}
	}
	u.embedMu.RUnlock()
	if len(missing) == 0 {
		return nil
	}
	texts := make([]string, len(missing))
	for i, idx := range missing {
		texts[i] = skillCorpusText(candidates[idx])
	}
	embeddings, err := u.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}
	u.embedMu.Lock()
	defer u.embedMu.Unlock()
	for i, idx := range missing {
		if i < len(embeddings) {
			u.embedCache[candidates[idx].Slug] = embedEntry{vector: embeddings[i], cachedAt: time.Now()}
		}
	}
	return nil
}

func skillCorpusText(c RuntimeCandidate) string {
	var b strings.Builder
	b.WriteString(c.Slug)
	b.WriteString(" ")
	b.WriteString(c.Name)
	b.WriteString(" ")
	b.WriteString(c.Description)
	for _, t := range c.Tags {
		b.WriteString(" ")
		b.WriteString(t.Name)
	}
	for _, p := range c.TaxonomyPaths {
		b.WriteString(" ")
		b.WriteString(p)
	}
	return b.String()
}

func cosineSimilarity32(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

package biz

// Skill invocation source values (skill_invocation.source).
const (
	SkillInvocationSourceRuntime         = "runtime"
	SkillInvocationSourceFilesystemScan  = "filesystem_scan"
	SkillInvocationSourceFilesystemWatch = "filesystem_watch"
)

// Skill import / similarity DTOs — JSON matches pkg/backend domain + web expectations.

type SkillSimilaritySource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Version     string `json:"version"`
	BodyPreview string `json:"body_preview"`
	Body        string `json:"-"`
}

type SkillImportJob struct {
	JobID            string                 `json:"job_id"`
	Status           string                 `json:"status"`
	ValidationStatus string                 `json:"validation_status"`
	StorageRoot      string                 `json:"storage_root"`
	Candidates       []SkillImportCandidate `json:"candidates"`
	ConflictGroups   []SkillConflictGroup   `json:"conflict_groups"`
	Message          string                 `json:"message,omitempty"`
}

type SkillImportCandidate struct {
	CandidateID      string             `json:"candidate_id"`
	Name             string             `json:"name"`
	Slug             string             `json:"slug"`
	Description      string             `json:"description"`
	BodyPreview      string             `json:"body_preview"`
	TargetDir        string             `json:"target_dir"`
	ValidationStatus string             `json:"validation_status"`
	StatusIcon       string             `json:"status_icon"`
	Warnings         []SkillImportIssue `json:"warnings"`
	Blocks           []SkillImportIssue `json:"blocks"`
}

type SkillImportIssue struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type SkillSimilarityMetrics struct {
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

type SkillConflictGroup struct {
	GroupID                string                  `json:"group_id"`
	HighestSimilarityScore float64                 `json:"highest_similarity_score"`
	Metrics                SkillSimilarityMetrics  `json:"metrics"`
	Reason                 string                  `json:"reason"`
	Evidence               []string                `json:"evidence"`
	CandidateIDs           []string                `json:"candidate_ids"`
	ExistingSkills         []SkillSimilaritySource `json:"existing_skills"`
	CanRefine              bool                    `json:"can_refine"`
}

type SkillRefineRequest struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
}

type SkillRefineResult struct {
	MergedName             string     `json:"merged_name"`
	MergedDescription      string     `json:"merged_description"`
	MergedBody             string     `json:"merged_body"`
	MergedTags             []SkillTag `json:"merged_tags"`
	SourceCandidateIDs     []string   `json:"source_candidate_ids"`
	SourceExistingSkillIDs []string   `json:"source_existing_skill_ids"`
}

type SkillImportDecision struct {
	CandidateID       string     `json:"candidate_id,omitempty"`
	GroupID           string     `json:"group_id,omitempty"`
	Action            string     `json:"action"`
	MergedName        string     `json:"merged_name,omitempty"`
	MergedDescription string     `json:"merged_description,omitempty"`
	MergedBody        string     `json:"merged_body,omitempty"`
	MergedTags        []SkillTag `json:"merged_tags,omitempty"`
}

type SkillImportApplyRequest struct {
	Decisions []SkillImportDecision `json:"decisions"`
}

type SkillImportApplyResult struct {
	CreatedSkillIDs     []string `json:"created_skill_ids"`
	SkippedCandidateIDs []string `json:"skipped_candidate_ids"`
	Message             string   `json:"message"`
}

// SkillCreateInput creates platform skill + initial skill_version (import / directory sync).
type SkillCreateInput struct {
	Name        string
	Slug        string
	Description string
	Body        string
	Tags        []SkillTag
	StorageDir  string
}

// SkillDiskSyncInput upserts skill rows from on-disk packages (directory watcher).
type SkillDiskSyncInput struct {
	Name        string
	Slug        string
	Description string
	Body        string
	Tags        []SkillTag
	StorageDir  string
}

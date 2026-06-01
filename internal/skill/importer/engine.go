package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/skill/storage"
	"aranea-agents/pkg/loggateway"
)

const MaxZipBytes = 20 * 1024 * 1024

type SkillImportRepo interface {
	CreateSkillWithVersion(ctx context.Context, in biz.SkillCreateInput) (biz.Skill, error)
	DeleteSkill(ctx context.Context, id string) error
	ListSkillSimilaritySources(ctx context.Context) ([]biz.SkillSimilaritySource, error)
}

type llmLister interface {
	List(ctx context.Context) ([]biz.ProviderModel, error)
	GetByProviderAndModel(ctx context.Context, provider, model string) (biz.ProviderModel, error)
}

type Engine struct {
	repo SkillImportRepo
	llm  llmLister
	sys  biz.SystemSettingRepo
	lg   loggateway.Logger

	jobsMu sync.RWMutex
	jobs   map[string]*jobState
	jobTTL time.Duration
}

const defaultJobTTL = 2 * time.Hour

type jobState struct {
	public     biz.SkillImportJob
	candidates map[string]candidateState
	createdAt  time.Time
}

type candidateState struct {
	public biz.SkillImportCandidate
	body   string
	files  map[string][]byte
	tags   []biz.SkillTag
}

// NewEngine constructs the skill ZIP importer. Skill storage root resolves via skillstorage + system settings.
func NewEngine(repo SkillImportRepo, llm llmLister, sys biz.SystemSettingRepo, lg loggateway.Logger) *Engine {
	return &Engine{
		repo:   repo,
		llm:    llm,
		sys:    sys,
		lg:     lg,
		jobs:   make(map[string]*jobState),
		jobTTL: defaultJobTTL,
	}
}

func ProvideLLMLister(uc *biz.LlmProviderModelUsecase) llmLister {
	return uc
}

func (e *Engine) resolveRoot(ctx context.Context) string {
	if e.sys == nil {
		return storage.ResolveRootFromEnv()
	}
	st, err := e.sys.Get(ctx)
	if err != nil {
		return storage.ResolveRootFromEnv()
	}
	return storage.ResolveRootWithPlatform(st.RootDirectory)
}

func candidateRequiresRiskApproval(candidate biz.SkillImportCandidate) bool {
	if candidate.ValidationStatus != "block" {
		return false
	}
	if len(candidate.Blocks) == 0 {
		return false
	}
	for _, block := range candidate.Blocks {
		if block.Type != "high_risk_file" {
			return false
		}
	}
	return true
}

func (e *Engine) Import(ctx context.Context, file multipart.File, header *multipart.FileHeader) (biz.SkillImportJob, error) {
	if file == nil || header == nil {
		return biz.SkillImportJob{}, validationError("skill zip file is required")
	}
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		return biz.SkillImportJob{}, validationError("skill upload must be a .zip file")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxZipBytes+1))
	if err != nil {
		return biz.SkillImportJob{}, err
	}
	if len(data) > MaxZipBytes {
		return biz.SkillImportJob{}, validationError("skill zip must be <= 20MB")
	}
	job := &jobState{
		public: biz.SkillImportJob{
			JobID:            newID(),
			Status:           "processing",
			ValidationStatus: "pass",
			StorageRoot:      e.resolveRoot(ctx),
			Candidates:       []biz.SkillImportCandidate{},
			ConflictGroups:   []biz.SkillConflictGroup{},
		},
		candidates: map[string]candidateState{},
		createdAt:  time.Now(),
	}
	if err = e.inspectSkillZip(ctx, data, job); err != nil {
		job.public.Status = "failed"
		job.public.ValidationStatus = "block"
		job.public.Message = err.Error()
	} else {
		job.public.Status = "completed"
		job.public.ValidationStatus = summarizeImportStatus(job.public.Candidates, job.public.ConflictGroups)
		if job.public.ValidationStatus == "block" {
			job.public.Message = strings.Join(importBlockMessages(job.public.Candidates), "?")
		}
	}
	e.jobsMu.Lock()
	e.jobs[job.public.JobID] = job
	e.jobsMu.Unlock()
	return job.public, nil
}

func (e *Engine) GetImportJob(jobID string) (biz.SkillImportJob, error) {
	trimmed := strings.TrimSpace(jobID)
	e.jobsMu.RLock()
	job := e.jobs[trimmed]
	expired := job != nil && time.Since(job.createdAt) > e.jobTTL
	e.jobsMu.RUnlock()

	if expired {
		e.jobsMu.Lock()
		delete(e.jobs, trimmed)
		e.jobsMu.Unlock()
		job = nil
	}

	if job == nil {
		return biz.SkillImportJob{}, ErrImportJobNotFound
	}
	out := job.public
	out.StorageRoot = e.resolveRoot(context.Background())
	return out, nil
}

func (e *Engine) evictExpiredLocked() {
	now := time.Now()
	for id, job := range e.jobs {
		if now.Sub(job.createdAt) > e.jobTTL {
			delete(e.jobs, id)
		}
	}
}

func (e *Engine) RefineConflictGroup(ctx context.Context, jobID string, groupID string, in biz.SkillRefineRequest) (biz.SkillRefineResult, error) {
	_, group, candidates, err := e.conflictGroupContext(jobID, groupID)
	if err != nil {
		return biz.SkillRefineResult{}, err
	}
	cfg, err := e.resolveChatModel(ctx, in.Provider, in.Model)
	if err != nil {
		return biz.SkillRefineResult{}, err
	}
	prompt := buildRefinePrompt(group, candidates, strings.TrimSpace(in.Instructions))
	raw, err := completeChat(ctx, cfg, prompt)
	if err != nil {
		return biz.SkillRefineResult{}, err
	}
	refined, err := parseRefineResult(raw)
	if err != nil {
		return biz.SkillRefineResult{}, err
	}
	refined.SourceCandidateIDs = group.CandidateIDs
	for _, existing := range group.ExistingSkills {
		refined.SourceExistingSkillIDs = append(refined.SourceExistingSkillIDs, existing.ID)
	}
	return refined, nil
}

// createdSkillRecord holds the identifiers needed to compensate (undo) a createImportedSkill call.
type createdSkillRecord struct {
	id         string
	storageDir string
}

// ApplyImport executes the user-approved import decisions.
//
// TPM-P1-08: each createImportedSkill writes to disk AND inserts a DB row. If any step fails
// after earlier writes succeeded, we compensate by deleting all already-created skills (DB row +
// disk directory). This makes the operation effectively atomic from the caller's perspective —
// either all succeed or all are cleaned up. (Full two-phase Saga is deferred to Wave 2 / TPM-D-S1.)
func (e *Engine) ApplyImport(ctx context.Context, jobID string, in biz.SkillImportApplyRequest) (biz.SkillImportApplyResult, error) {
	e.jobsMu.RLock()
	job := e.jobs[strings.TrimSpace(jobID)]
	e.jobsMu.RUnlock()
	if job == nil {
		return biz.SkillImportApplyResult{}, ErrImportJobNotFound
	}
	if job.public.Status != "completed" {
		return biz.SkillImportApplyResult{}, validationError("import job is not completed")
	}

	var committed []createdSkillRecord
	// compensate rolls back all skills written so far on error.
	// It uses context.Background() so caller cancellation cannot abort cleanup.
	// Failures are logged at Warn level so ops can detect orphan rows.
	compensate := func() {
		cCtx := context.Background()
		for _, r := range committed {
			if err := e.repo.DeleteSkill(cCtx, r.id); err != nil {
				e.lg.Warn("skill.import.compensate_db_delete_fail",
					loggateway.StepID("skill.seed_fail"),
					loggateway.Str("skill_id", r.id),
					loggateway.Err(err))
			}
			if err := os.RemoveAll(r.storageDir); err != nil {
				e.lg.Warn("skill.import.compensate_dir_remove_fail",
					loggateway.StepID("skill.seed_fail"),
					loggateway.Str("storage_dir", r.storageDir),
					loggateway.Err(err))
			}
		}
	}

	// partialErr returns a result that preserves any skip decisions already processed
	// before the failure, so the caller knows what was skipped vs what failed.
	result := biz.SkillImportApplyResult{CreatedSkillIDs: []string{}, SkippedCandidateIDs: []string{}}
	partialErr := func(err error) (biz.SkillImportApplyResult, error) {
		compensate()
		// Return with zero created IDs (compensated away) but preserve skips for diagnostics.
		return biz.SkillImportApplyResult{
			CreatedSkillIDs:     []string{},
			SkippedCandidateIDs: result.SkippedCandidateIDs,
			Message:             fmt.Sprintf("import failed and rolled back: %s", err.Error()),
		}, err
	}

	for _, decision := range in.Decisions {
		switch decision.Action {
		case "import_passed":
			candidate, ok := job.candidates[decision.CandidateID]
			if !ok {
				return partialErr(detailErr(ErrCandidateNotFound, "candidate "+decision.CandidateID+" not found"))
			}
			if candidate.public.ValidationStatus != "pass" {
				return partialErr(detailErr(ErrCandidateNotPass, "candidate "+decision.CandidateID+" is not pass"))
			}
			created, dir, err := e.createImportedSkill(ctx, candidate.public.Name, candidate.public.Slug, candidate.public.Description, candidate.body, candidate.tags, candidate.files)
			if err != nil {
				return partialErr(err)
			}
			committed = append(committed, createdSkillRecord{id: created.ID, storageDir: dir})
			result.CreatedSkillIDs = append(result.CreatedSkillIDs, created.ID)
		case "approve_risky_import":
			candidate, ok := job.candidates[decision.CandidateID]
			if !ok {
				return partialErr(detailErr(ErrCandidateNotFound, "candidate "+decision.CandidateID+" not found"))
			}
			if !candidateRequiresRiskApproval(candidate.public) {
				return partialErr(detailErr(ErrRiskApprovalRequired, "candidate "+decision.CandidateID+" does not require high risk approval"))
			}
			created, dir, err := e.createImportedSkill(ctx, candidate.public.Name, candidate.public.Slug, candidate.public.Description, candidate.body, candidate.tags, candidate.files)
			if err != nil {
				return partialErr(err)
			}
			committed = append(committed, createdSkillRecord{id: created.ID, storageDir: dir})
			result.CreatedSkillIDs = append(result.CreatedSkillIDs, created.ID)
		case "reject_risky_upload":
			if strings.TrimSpace(decision.CandidateID) == "" {
				return partialErr(validationError("candidate_id is required"))
			}
			result.SkippedCandidateIDs = append(result.SkippedCandidateIDs, decision.CandidateID)
		case "merge_group_with_ai":
			if strings.TrimSpace(decision.MergedBody) == "" {
				return partialErr(validationError("merged_body is required"))
			}
			slug := slugify(decision.MergedName)
			files := map[string][]byte{"SKILL.md": []byte(decision.MergedBody)}
			created, dir, err := e.createImportedSkill(ctx, decision.MergedName, slug, decision.MergedDescription, decision.MergedBody, decision.MergedTags, files)
			if err != nil {
				return partialErr(err)
			}
			committed = append(committed, createdSkillRecord{id: created.ID, storageDir: dir})
			result.CreatedSkillIDs = append(result.CreatedSkillIDs, created.ID)
		case "skip_group":
			for _, id := range candidateIDsForGroup(job.public.ConflictGroups, decision.GroupID) {
				result.SkippedCandidateIDs = append(result.SkippedCandidateIDs, id)
			}
		default:
			return partialErr(detailErr(ErrUnsupportedAction, "unsupported import action: "+decision.Action))
		}
	}
	result.Message = "import completed"
	return result, nil
}

func (e *Engine) conflictGroupContext(jobID string, groupID string) (*jobState, biz.SkillConflictGroup, []candidateState, error) {
	e.jobsMu.RLock()
	job := e.jobs[strings.TrimSpace(jobID)]
	e.jobsMu.RUnlock()
	if job == nil {
		return nil, biz.SkillConflictGroup{}, nil, ErrImportJobNotFound
	}
	for _, group := range job.public.ConflictGroups {
		if group.GroupID != groupID {
			continue
		}
		candidates := []candidateState{}
		for _, id := range group.CandidateIDs {
			if candidate, ok := job.candidates[id]; ok {
				candidates = append(candidates, candidate)
			}
		}
		return job, group, candidates, nil
	}
	return nil, biz.SkillConflictGroup{}, nil, ErrConflictGroupNotFound
}

// createImportedSkill writes skill files to disk and inserts the DB row.
// It returns the created Skill and the absolute storage directory so the caller
// can compensate (delete) on failure without an extra DB round-trip.
func (e *Engine) createImportedSkill(ctx context.Context, name string, slug string, description string, body string, tags []biz.SkillTag, files map[string][]byte) (biz.Skill, string, error) {
	slug = slugify(slug)
	if slug == "" {
		slug = slugify(name)
	}
	targetDir := filepath.Join(e.resolveRoot(ctx), slug)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return biz.Skill{}, "", err
	}
	// TPM-P1-07: full zipslip protection — verify joined path stays inside targetDir.
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return biz.Skill{}, "", detailErr(ErrResolveTargetDir, err.Error())
	}
	for fname, data := range files {
		if err := ensurePathWithin(absTarget, fname); err != nil {
			return biz.Skill{}, "", err
		}
		clean := filepath.Clean(fname)
		path := filepath.Join(absTarget, clean)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return biz.Skill{}, "", err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return biz.Skill{}, "", err
		}
	}
	skill, err := e.repo.CreateSkillWithVersion(ctx, biz.SkillCreateInput{Name: name, Slug: slug, Description: description, Body: body, Tags: tags, StorageDir: targetDir, SyncOrigin: biz.SkillSyncOriginImport})
	if err != nil {
		return biz.Skill{}, "", err
	}
	return skill, absTarget, nil
}

func (e *Engine) updateCandidateWarning(job *jobState, candidateID string, metrics biz.SkillSimilarityMetrics) {
	for i := range job.public.Candidates {
		if job.public.Candidates[i].CandidateID != candidateID {
			continue
		}
		job.public.Candidates[i].ValidationStatus = "warn"
		job.public.Candidates[i].StatusIcon = "merge_suggested"
		job.public.Candidates[i].Warnings = append(job.public.Candidates[i].Warnings, biz.SkillImportIssue{
			Type:    "similarity",
			Message: fmt.Sprintf("similarity score capped at %d%%", int(metrics.SimilarityScore*100+0.5)),
		})
		state := job.candidates[candidateID]
		state.public = job.public.Candidates[i]
		job.candidates[candidateID] = state
	}
}

func (e *Engine) inspectSkillZip(ctx context.Context, data []byte, job *jobState) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return validationError("invalid zip file")
	}
	filesByDir := map[string]map[string][]byte{}
	for _, file := range reader.File {
		// TPM-P1-07: normalize separators and reject any path that would escape the
		// skill root after Clean/Join. Reject absolute, traversal, and Windows-drive-prefixed paths.
		name := filepath.ToSlash(file.Name)
		if name == "" {
			continue
		}
		if filepath.IsAbs(file.Name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
			return unsafePathError(ErrUnsafePathAbsolute, file.Name)
		}
		cleaned := filepath.ToSlash(filepath.Clean(name))
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/..") {
			return unsafePathError(ErrUnsafePathTraversal, file.Name)
		}
		if strings.Contains(name, "..") {
			return unsafePathError(ErrUnsafePathDotDot, file.Name)
		}
		if file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(io.LimitReader(rc, 2*1024*1024+1))
		_ = rc.Close()
		if err != nil {
			return err
		}
		if len(content) > 2*1024*1024 {
			return detailErr(ErrSkillFileTooLarge, "skill file too large: "+name)
		}
		dir, relativeName := skillZipGroupPath(name)
		if _, ok := filesByDir[dir]; !ok {
			filesByDir[dir] = map[string][]byte{}
		}
		filesByDir[dir][relativeName] = content
	}
	existing, err := e.repo.ListSkillSimilaritySources(ctx)
	if err != nil {
		return err
	}
	for dir, files := range filesByDir {
		bodyBytes, ok := files["SKILL.md"]
		if !ok {
			bodyBytes, ok = files["skill.md"]
		}
		if !ok {
			continue
		}
		body := string(bodyBytes)
		candidate, tags := ValidateSkillPackage(files, dir, existing, false)
		candidate.TargetDir = filepath.Join(job.public.StorageRoot, candidate.Slug)
		job.candidates[candidate.CandidateID] = candidateState{public: candidate, body: body, files: files, tags: tags}
		job.public.Candidates = append(job.public.Candidates, candidate)
	}
	if len(job.public.Candidates) == 0 {
		return validationError("zip must contain at least one SKILL.md")
	}
	return e.inspectSimilarity(ctx, job, existing)
}

const maxSimilarityLLMCalls = 50

func (e *Engine) inspectSimilarity(ctx context.Context, job *jobState, existing []biz.SkillSimilaritySource) error {
	if len(existing) == 0 {
		return nil
	}
	cfg, err := e.resolveChatModel(ctx, "", "")
	if err != nil {
		for i := range job.public.Candidates {
			if job.public.Candidates[i].ValidationStatus == "pass" {
				job.public.Candidates[i].ValidationStatus = "block"
				job.public.Candidates[i].StatusIcon = "block"
				job.public.Candidates[i].Blocks = append(job.public.Candidates[i].Blocks, biz.SkillImportIssue{Type: "model_unavailable", Message: err.Error()})
				state := job.candidates[job.public.Candidates[i].CandidateID]
				state.public = job.public.Candidates[i]
				job.candidates[job.public.Candidates[i].CandidateID] = state
			}
		}
		return nil
	}
	llmCalls := 0
	for _, candidate := range job.public.Candidates {
		if candidate.ValidationStatus != "pass" {
			continue
		}
		state := job.candidates[candidate.CandidateID]
		for _, source := range existing {
			if llmCalls >= maxSimilarityLLMCalls {
				e.lg.Warn("inspectSimilarity LLM call limit reached, skipping remaining comparisons",
					loggateway.StepID("skill.similarity_cap"),
					loggateway.Int("cap", maxSimilarityLLMCalls))
				return nil
			}
			llmCalls++
			metrics, reason, evidence, err := e.modelSimilarity(ctx, cfg, state, source)
			if err != nil {
				continue
			}
			if metrics.SimilarityScore >= 0.2 {
				group := biz.SkillConflictGroup{
					GroupID:                newID(),
					HighestSimilarityScore: metrics.SimilarityScore,
					Metrics:                metrics,
					Reason:                 reason,
					Evidence:               evidence,
					CandidateIDs:           []string{candidate.CandidateID},
					ExistingSkills:         []biz.SkillSimilaritySource{source},
					CanRefine:              true,
				}
				job.public.ConflictGroups = append(job.public.ConflictGroups, group)
				e.updateCandidateWarning(job, candidate.CandidateID, metrics)
			}
		}
	}
	return nil
}

func (e *Engine) modelSimilarity(ctx context.Context, cfg chatModelCfg, candidate candidateState, source biz.SkillSimilaritySource) (biz.SkillSimilarityMetrics, string, []string, error) {
	prompt := buildSimilarityPrompt(candidate, source)
	raw, err := completeChat(ctx, cfg, prompt)
	if err != nil {
		return biz.SkillSimilarityMetrics{}, "", nil, err
	}
	var out struct {
		biz.SkillSimilarityMetrics
		Reason   string   `json:"reason"`
		Evidence []string `json:"evidence"`
	}
	if err = decodeModelJSON(raw, &out); err != nil {
		return biz.SkillSimilarityMetrics{}, "", nil, err
	}
	out.SkillSimilarityMetrics.SimilarityScore = clamp01(out.SkillSimilarityMetrics.SimilarityScore)
	out.SkillSimilarityMetrics.NameSimilarity = clamp01(out.SkillSimilarityMetrics.NameSimilarity)
	out.SkillSimilarityMetrics.DescriptionSimilarity = clamp01(out.SkillSimilarityMetrics.DescriptionSimilarity)
	out.SkillSimilarityMetrics.BodySimilarity = clamp01(out.SkillSimilarityMetrics.BodySimilarity)
	out.SkillSimilarityMetrics.TriggerSimilarity = clamp01(out.SkillSimilarityMetrics.TriggerSimilarity)
	out.SkillSimilarityMetrics.ToolSimilarity = clamp01(out.SkillSimilarityMetrics.ToolSimilarity)
	out.SkillSimilarityMetrics.Confidence = clamp01(out.SkillSimilarityMetrics.Confidence)
	if out.Recommendation == "" {
		out.Recommendation = "suggest_refine"
	}
	if out.ConflictRisk == "" {
		out.ConflictRisk = "medium"
	}
	return out.SkillSimilarityMetrics, out.Reason, out.Evidence, nil
}

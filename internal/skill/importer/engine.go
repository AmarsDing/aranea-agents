package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
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

// SkillImportJobStore persists import job state to the database.
type SkillImportJobStore interface {
	Create(ctx context.Context, job biz.SkillImportJob) error
	Get(ctx context.Context, jobID string) (*biz.SkillImportJob, error)
	UpdateStatus(ctx context.Context, jobID string, status string, message string) error
	UpdateCandidates(ctx context.Context, jobID string, candidates []biz.SkillImportCandidate, conflictGroups []biz.SkillConflictGroup) error
}

type Engine struct {
	repo   SkillImportRepo
	llm    llmLister
	sys    biz.SystemSettingRepo
	lg     loggateway.Logger
	store  SkillImportJobStore // DB persistence layer (nil = memory-only fallback)

	jobsMu sync.RWMutex
	jobs   map[string]*jobState
	jobTTL time.Duration
}

const defaultJobTTL = 2 * time.Hour

// maxInMemoryJobs caps the in-memory job store to prevent unbounded growth.
// When exceeded, the oldest expired jobs are evicted; if no expired jobs exist,
// the oldest job is evicted regardless (LRU-style).
const maxInMemoryJobs = 200

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

// SetStore sets the DB persistence layer. When set, import jobs are
// persisted to the database so they survive server restarts.
// NOTE: Must only be called during initialization, before any concurrent access.
func (e *Engine) SetStore(store SkillImportJobStore) {
	e.store = store
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
	e.evictExpiredLocked()
	// Enforce max cap: if still over limit after eviction, remove oldest job.
	if len(e.jobs) >= maxInMemoryJobs {
		var oldestID string
		var oldestTime time.Time
		for id, j := range e.jobs {
			if oldestID == "" || j.createdAt.Before(oldestTime) {
				oldestID = id
				oldestTime = j.createdAt
			}
		}
		delete(e.jobs, oldestID)
	}
	e.jobs[job.public.JobID] = job
	e.jobsMu.Unlock()

	// Persist candidate files to a temp directory so ApplyImport can work
	// after a server restart. This implements the TempDir mechanism that was
	// designed but never wired up — without it, the DB fallback in ApplyImport
	// creates candidateState objects missing body/files/tags, silently producing
	// broken/empty skills.
	// Only persist files for completed jobs; failed jobs have no candidates to apply.
	if e.store != nil {
		if job.public.Status == "completed" {
			e.persistCandidateFiles(job)
		}
		if dbErr := e.store.Create(ctx, job.public); dbErr != nil {
			e.lg.Warn("Import: DB persist failed, job only in memory",
				loggateway.StepID("skill.import.db_persist"),
				loggateway.Str("job_id", job.public.JobID),
				loggateway.Err(dbErr))
		}
	}

	return job.public, nil
}

func (e *Engine) GetImportJob(jobID string) (biz.SkillImportJob, error) {
	trimmed := strings.TrimSpace(jobID)

	// Try in-memory cache first (fast path).
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

	if job != nil {
		out := job.public
		out.StorageRoot = e.resolveRoot(context.Background())
		return out, nil
	}

	// Fallback to DB persistence (survives restarts).
	if e.store != nil {
		dbJob, dbErr := e.store.Get(context.Background(), trimmed)
		if dbErr != nil {
			e.lg.Warn("GetImportJob: DB lookup failed",
				loggateway.StepID("skill.import.db_lookup"),
				loggateway.Str("job_id", trimmed),
				loggateway.Err(dbErr))
		} else if dbJob != nil {
			dbJob.StorageRoot = e.resolveRoot(context.Background())
			return *dbJob, nil
		}
	}

	return biz.SkillImportJob{}, ErrImportJobNotFound
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

// skillCreateParams groups the parameters for creating an imported skill.
type skillCreateParams struct {
	name        string
	slug        string
	description string
	body        string
	tags        []biz.SkillTag
	files       map[string][]byte
}

// ApplyImport executes the user-approved import decisions.
//
// TPM-P1-08: each createImportedSkill writes to disk AND inserts a DB row. If any step fails
// after earlier writes succeeded, we compensate by deleting all already-created skills (DB row +
// disk directory). This makes the operation effectively atomic from the caller's perspective —
// either all succeed or all are cleaned up. (Full two-phase Saga is deferred to Wave 2 / TPM-D-S1.)
func (e *Engine) ApplyImport(ctx context.Context, jobID string, in biz.SkillImportApplyRequest) (biz.SkillImportApplyResult, error) {
	trimmed := strings.TrimSpace(jobID)

	// CAS-style status transition: atomically check "completed" and set "applying"
	// under a write lock to prevent concurrent ApplyImport calls from both proceeding.
	var job *jobState
	e.jobsMu.Lock()
	job = e.jobs[trimmed]
	if job != nil {
		if job.public.Status == "applied" {
			e.jobsMu.Unlock()
			return biz.SkillImportApplyResult{}, validationError("import job already applied")
		}
		if job.public.Status == "applying" {
			e.jobsMu.Unlock()
			return biz.SkillImportApplyResult{}, validationError("import job is currently being applied")
		}
		if job.public.Status != "completed" {
			e.jobsMu.Unlock()
			return biz.SkillImportApplyResult{}, validationError("import job is not completed")
		}
		job.public.Status = "applying"
	}
	e.jobsMu.Unlock()

	if job == nil {
		// Fallback to DB persistence (job may have survived a restart).
		if e.store != nil {
			dbJob, dbErr := e.store.Get(ctx, trimmed)
			if dbErr != nil {
				e.lg.Warn("ApplyImport: DB lookup failed",
					loggateway.StepID("skill.import.db_lookup"),
					loggateway.Str("job_id", trimmed),
					loggateway.Err(dbErr))
			} else if dbJob != nil {
				if dbJob.Status == "applied" {
					return biz.SkillImportApplyResult{}, validationError("import job already applied")
				}
				if dbJob.Status == "applying" {
					return biz.SkillImportApplyResult{}, validationError("import job is currently being applied")
				}
				if dbJob.Status != "completed" {
					return biz.SkillImportApplyResult{}, validationError("import job is not completed")
				}
				// CAS in DB: mark as "applying" before proceeding.
				if casErr := e.store.UpdateStatus(ctx, trimmed, "applying", ""); casErr != nil {
					return biz.SkillImportApplyResult{}, casErr
				}
				// Reconstruct jobState from DB data.
				job = &jobState{
					public:     *dbJob,
					candidates: make(map[string]candidateState),
					createdAt:  time.Now(),
				}
				for _, c := range dbJob.Candidates {
					cs := candidateState{public: c}
					// Restore body/files/tags from TempDir if available.
					// Without this, ApplyImport would create broken/empty skills.
					if dbJob.TempDir != "" {
						e.restoreCandidateFiles(dbJob.TempDir, c.CandidateID, &cs)
					}
					job.candidates[c.CandidateID] = cs
				}
				// If TempDir is missing and candidates lack body/files, the apply
				// will fail when trying to create skills — which is correct behavior
				// (fail loudly rather than silently creating broken skills).
			}
		}
	}
	if job == nil {
		return biz.SkillImportApplyResult{}, ErrImportJobNotFound
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
	// It also rolls back the job status from "applying" to "completed" so the job
	// can be retried.
	result := biz.SkillImportApplyResult{CreatedSkillIDs: []string{}, SkippedCandidateIDs: []string{}}
	partialErr := func(err error) (biz.SkillImportApplyResult, error) {
		compensate()
		// Roll back status to "completed" so the job can be retried.
		e.jobsMu.Lock()
		if j, ok := e.jobs[trimmed]; ok {
			j.public.Status = "completed"
		}
		e.jobsMu.Unlock()
		if e.store != nil {
			_ = e.store.UpdateStatus(context.Background(), trimmed, "completed", "apply failed: "+err.Error())
		}
		// Return with zero created IDs (compensated away) but preserve skips for diagnostics.
		return biz.SkillImportApplyResult{
			CreatedSkillIDs:     []string{},
			SkippedCandidateIDs: result.SkippedCandidateIDs,
			Message:             fmt.Sprintf("import failed and rolled back: %s", err.Error()),
		}, err
	}

	for _, decision := range in.Decisions {
		params, skipIDs, err := e.resolveDecision(job, decision)
		if err != nil {
			return partialErr(err)
		}
		if len(skipIDs) > 0 {
			result.SkippedCandidateIDs = append(result.SkippedCandidateIDs, skipIDs...)
			continue
		}
		if params == nil {
			return partialErr(detailErr(ErrUnsupportedAction, "unsupported import action: "+decision.Action))
		}
		created, dir, err := e.createImportedSkill(ctx, *params)
		if err != nil {
			return partialErr(err)
		}
		committed = append(committed, createdSkillRecord{id: created.ID, storageDir: dir})
		result.CreatedSkillIDs = append(result.CreatedSkillIDs, created.ID)
	}
	result.Message = "import completed"

	// Transition in-memory job status from "applying" to "applied".
	e.jobsMu.Lock()
	if j, ok := e.jobs[trimmed]; ok {
		j.public.Status = "applied"
	}
	e.jobsMu.Unlock()

	// Update DB status to "applied".
	if e.store != nil {
		if dbErr := e.store.UpdateStatus(ctx, trimmed, "applied", result.Message); dbErr != nil {
			e.lg.Warn("ApplyImport: DB status update failed",
				loggateway.StepID("skill.import.db_update"),
				loggateway.Str("job_id", trimmed),
				loggateway.Err(dbErr))
		}
	}

	// Clean up temp directory — files have been written to their final locations.
	if job.public.TempDir != "" {
		if err := os.RemoveAll(job.public.TempDir); err != nil {
			e.lg.Warn("ApplyImport: temp dir cleanup failed",
				loggateway.StepID("skill.import.temp_cleanup"),
				loggateway.Str("temp_dir", job.public.TempDir),
				loggateway.Err(err))
		}
	}

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

// resolveDecision maps an import decision to either a skillCreateParams (to create
// a skill), a list of skipped candidate IDs, or an error. Returns (nil, nil, nil)
// for unsupported actions so the caller can report the error.
func (e *Engine) resolveDecision(job *jobState, decision biz.SkillImportDecision) (*skillCreateParams, []string, error) {
	switch decision.Action {
	case "import_passed":
		candidate, ok := job.candidates[decision.CandidateID]
		if !ok {
			return nil, nil, detailErr(ErrCandidateNotFound, "candidate "+decision.CandidateID+" not found")
		}
		if candidate.public.ValidationStatus != "pass" {
			return nil, nil, detailErr(ErrCandidateNotPass, "candidate "+decision.CandidateID+" is not pass")
		}
		return &skillCreateParams{
			name: candidate.public.Name, slug: candidate.public.Slug,
			description: candidate.public.Description, body: candidate.body,
			tags: candidate.tags, files: candidate.files,
		}, nil, nil
	case "approve_risky_import":
		candidate, ok := job.candidates[decision.CandidateID]
		if !ok {
			return nil, nil, detailErr(ErrCandidateNotFound, "candidate "+decision.CandidateID+" not found")
		}
		if !candidateRequiresRiskApproval(candidate.public) {
			return nil, nil, detailErr(ErrRiskApprovalRequired, "candidate "+decision.CandidateID+" does not require high risk approval")
		}
		return &skillCreateParams{
			name: candidate.public.Name, slug: candidate.public.Slug,
			description: candidate.public.Description, body: candidate.body,
			tags: candidate.tags, files: candidate.files,
		}, nil, nil
	case "reject_risky_upload":
		if strings.TrimSpace(decision.CandidateID) == "" {
			return nil, nil, validationError("candidate_id is required")
		}
		return nil, []string{decision.CandidateID}, nil
	case "merge_group_with_ai":
		if strings.TrimSpace(decision.MergedName) == "" {
			return nil, nil, validationError("merged_name is required")
		}
		if strings.TrimSpace(decision.MergedBody) == "" {
			return nil, nil, validationError("merged_body is required")
		}
		slug := slugify(decision.MergedName)
		files := map[string][]byte{"SKILL.md": []byte(decision.MergedBody)}
		return &skillCreateParams{
			name: decision.MergedName, slug: slug,
			description: decision.MergedDescription, body: decision.MergedBody,
			tags: decision.MergedTags, files: files,
		}, nil, nil
	case "skip_group":
		return nil, candidateIDsForGroup(job.public.ConflictGroups, decision.GroupID), nil
	default:
		return nil, nil, nil
	}
}

// createImportedSkill writes skill files to disk and inserts the DB row.
// It returns the created Skill and the absolute storage directory so the caller
// can compensate (delete) on failure without an extra DB round-trip.
func (e *Engine) createImportedSkill(ctx context.Context, p skillCreateParams) (biz.Skill, string, error) {
	slug := slugify(p.slug)
	if slug == "" {
		slug = slugify(p.name)
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
	for fname, data := range p.files {
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
	skill, err := e.repo.CreateSkillWithVersion(ctx, biz.SkillCreateInput{Name: p.name, Slug: slug, Description: p.description, Body: p.body, Tags: p.tags, StorageDir: targetDir, SyncOrigin: biz.SkillSyncOriginImport})
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

// persistCandidateFiles writes each candidate's body, files, and tags to a temp
// directory under the storage root. The temp directory path is stored in
// job.public.TempDir so the DB-persisted job can restore the data after a
// server restart.
func (e *Engine) persistCandidateFiles(job *jobState) {
	tempDir := filepath.Join(job.public.StorageRoot, ".import-tmp", job.public.JobID)
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		e.lg.Warn("Import: failed to create temp dir for candidate files",
			loggateway.StepID("skill.import.temp_dir"),
			loggateway.Err(err))
		return
	}
	for cid, cs := range job.candidates {
		candidateDir := filepath.Join(tempDir, cid)
		if err := os.MkdirAll(candidateDir, 0o755); err != nil {
			e.lg.Warn("Import: failed to create candidate temp dir",
				loggateway.StepID("skill.import.temp_dir"),
				loggateway.Str("candidate_id", cid),
				loggateway.Err(err))
			continue
		}
		// Write all files from the zip.
		for fname, data := range cs.files {
			fpath := filepath.Join(candidateDir, filepath.Clean(fname))
			if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
				e.lg.Warn("Import: failed to create file parent dir in temp",
					loggateway.StepID("skill.import.temp_dir"),
					loggateway.Str("candidate_id", cid),
					loggateway.Str("file", fname),
					loggateway.Err(err))
				continue
			}
			if err := os.WriteFile(fpath, data, 0o644); err != nil {
				e.lg.Warn("Import: failed to write candidate file to temp",
					loggateway.StepID("skill.import.temp_dir"),
					loggateway.Str("candidate_id", cid),
					loggateway.Str("file", fname),
					loggateway.Err(err))
			}
		}
		// Write body as SKILL.md if not already in files.
		if cs.body != "" {
			if _, ok := cs.files["SKILL.md"]; !ok {
				if err := os.WriteFile(filepath.Join(candidateDir, "SKILL.md"), []byte(cs.body), 0o644); err != nil {
					e.lg.Warn("Import: failed to write SKILL.md to temp",
						loggateway.StepID("skill.import.temp_dir"),
						loggateway.Str("candidate_id", cid),
						loggateway.Err(err))
				}
			}
		}
		// Write tags as _tags.json for restoration.
		if len(cs.tags) > 0 {
			if tagsJSON, err := json.Marshal(cs.tags); err == nil {
				if err := os.WriteFile(filepath.Join(candidateDir, "_tags.json"), tagsJSON, 0o644); err != nil {
					e.lg.Warn("Import: failed to write _tags.json to temp",
						loggateway.StepID("skill.import.temp_dir"),
						loggateway.Str("candidate_id", cid),
						loggateway.Err(err))
				}
			}
		}
	}
	job.public.TempDir = tempDir
}

// restoreCandidateFiles reads body, files, and tags from the temp directory
// back into a candidateState. If the temp directory is missing or incomplete,
// the candidateState retains its zero-valued fields — ApplyImport will then
// fail explicitly when trying to create a skill without body/files, which is
// correct behavior (fail loudly rather than silently creating broken skills).
func (e *Engine) restoreCandidateFiles(tempDir, candidateID string, cs *candidateState) {
	candidateDir := filepath.Join(tempDir, candidateID)
	info, err := os.Stat(candidateDir)
	if err != nil || !info.IsDir() {
		return
	}
	// Read all files from the candidate directory.
	files := make(map[string][]byte)
	_ = filepath.Walk(candidateDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(candidateDir, path)
		if err != nil {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		// Skip metadata files — they're restored separately below.
		if rel == "_tags.json" {
			return nil
		}
		// Normalize to forward slashes to match the original files map keys
		// (which come from filepath.ToSlash in inspectSkillZip).
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	cs.files = files

	// Restore body from SKILL.md.
	if body, ok := files["SKILL.md"]; ok {
		cs.body = string(body)
	}

	// Restore tags from _tags.json.
	tagsPath := filepath.Join(candidateDir, "_tags.json")
	if tagsData, err := os.ReadFile(tagsPath); err == nil {
		if unmarshalErr := json.Unmarshal(tagsData, &cs.tags); unmarshalErr != nil {
			e.lg.Warn("restoreCandidateFiles: corrupted _tags.json, tags will be empty",
				loggateway.StepID("skill.import.restore"),
				loggateway.Str("candidate_id", candidateID),
				loggateway.Err(unmarshalErr))
		}
	}
}

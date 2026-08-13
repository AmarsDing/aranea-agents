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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/skill/manifest"
	"aranea-agents/internal/skill/storage"
	"aranea-agents/pkg/loggateway"
	"golang.org/x/sync/errgroup"
)

const MaxZipBytes = 20 * 1024 * 1024

// maxSkillFileBytes caps individual file extraction within a ZIP.
// Files exceeding this limit are rejected to prevent unbounded memory use.
const maxSkillFileBytes = 2 * 1024 * 1024

type SkillImportRepo interface {
	CreateSkillWithVersion(ctx context.Context, in biz.SkillCreateInput) (biz.Skill, error)
	DeleteSkill(ctx context.Context, id string) error
	ListSkillSimilaritySources(ctx context.Context) ([]biz.SkillSimilaritySource, error)
	// GetSkillBySkillKey resolves an existing skill by slug (skill_key) —
	// used by the overwrite_duplicate decision to find the update target.
	GetSkillBySkillKey(ctx context.Context, skillKey string) (biz.Skill, error)
	// GetSkillStorageDir returns the on-disk storage directory of a skill —
	// used to merge aux files from existing skills and to write overwrite files.
	GetSkillStorageDir(ctx context.Context, id string) (string, error)
	// AppendImportedVersion appends a new version to an existing skill and
	// refreshes name/description/tags (overwrite_duplicate apply path).
	AppendImportedVersion(ctx context.Context, in biz.SkillImportVersionInput) (biz.Skill, error)
	// ArchiveSkill retires a skill (status=archived, enabled=false) —
	// merge_group_with_ai with retire_sources=true.
	ArchiveSkill(ctx context.Context, id string) error
	// SetSkillDerivedFrom records merge provenance in the skill metadata.
	SetSkillDerivedFrom(ctx context.Context, id string, sourceIDs []string) error
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
	// CompareAndSwapStatus atomically transitions a job from expectedStatus to newStatus.
	// Returns true if the swap succeeded, false if the current status didn't match expectedStatus.
	CompareAndSwapStatus(ctx context.Context, jobID string, expectedStatus string, newStatus string, message string) (bool, error)
	UpdateCandidates(ctx context.Context, jobID string, candidates []biz.SkillImportCandidate, conflictGroups []biz.SkillConflictGroup) error
}

type Engine struct {
	repo  SkillImportRepo
	llm   llmLister
	sys   biz.SystemSettingRepo
	lg    loggateway.Logger
	bus   contract.MonitorBus // monitor event bus for flow-log emission (nil = skip)
	store SkillImportJobStore // DB persistence layer (nil = memory-only fallback)

	jobsMu    sync.RWMutex
	jobs      map[string]*jobState
	jobTTL    time.Duration
	storeOnce sync.Once // protects SetStore from concurrent/late calls
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

// countCandidateFiles 统计所有候选的附属文件总数（流程日志字段）。
func countCandidateFiles(job *jobState) int {
	total := 0
	for _, c := range job.candidates {
		total += len(c.files)
	}
	return total
}

// NewEngine constructs the skill ZIP importer. Skill storage root resolves via skillstorage + system settings.
// bus may be nil; flow-log events are only emitted when a monitor bus is present.
func NewEngine(repo SkillImportRepo, llm llmLister, sys biz.SystemSettingRepo, bus contract.MonitorBus, lg loggateway.Logger) *Engine {
	return &Engine{
		repo:   repo,
		llm:    llm,
		sys:    sys,
		lg:     lg,
		bus:    bus,
		jobs:   make(map[string]*jobState),
		jobTTL: defaultJobTTL,
	}
}

// SetStore sets the DB persistence layer. When set, import jobs are
// persisted to the database so they survive server restarts.
// Protected by sync.Once — second calls are silently ignored.
func (e *Engine) SetStore(store SkillImportJobStore) {
	e.storeOnce.Do(func() {
		e.store = store
	})
}

func ProvideLLMLister(uc *biz.LlmProviderModelUsecase) llmLister {
	return uc
}

// ProvideEngine is the wire provider for the import Engine. When a
// SkillImportJobStore is available it is attached so import jobs survive
// restarts; otherwise jobs remain memory-only (warned at startup).
func ProvideEngine(repo SkillImportRepo, llm llmLister, sys biz.SystemSettingRepo, store SkillImportJobStore, bus contract.MonitorBus, lg loggateway.Logger) *Engine {
	e := NewEngine(repo, llm, sys, bus, lg)
	if store == nil {
		lg.Warn("import jobs are memory-only: no SkillImportJobStore configured",
			loggateway.StepID("skill.import.store"))
		return e
	}
	e.SetStore(store)
	return e
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

// flowLog returns a run-scoped trace emitter for the skill domain, or nil
// when no monitor bus is configured (flow-log emission disabled).
func (e *Engine) flowLog(ctx context.Context) *event.TraceEmitter {
	if e == nil || e.bus == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainSkill,
		LG:     e.lg,
		Infra:  event.NewInfraFromBus(e.bus),
	})
}

func candidateRequiresRiskApproval(candidate biz.SkillImportCandidate) bool {
	if candidate.ValidationStatus != "block" {
		return false
	}
	for _, block := range candidate.Blocks {
		if block.Type == "high_risk_file" {
			return true
		}
	}
	return false
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
	flow := e.flowLog(ctx)
	if flow != nil {
		// File count is only known after the zip is inspected; it is reported
		// with the skill.import.validate done event instead.
		flow.LogStart("skill.import.start", "Skill 包导入开始",
			event.P("package", header.Filename),
			event.P("source", "multipart_upload"),
			event.P("size_bytes", len(data)))
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
		if flow != nil {
			flow.LogError("skill.import.validate", "Skill 包校验失败",
				event.P("error", err.Error()))
		}
	} else {
		job.public.Status = "completed"
		job.public.ValidationStatus = summarizeImportStatus(job.public.Candidates, job.public.ConflictGroups)
		if job.public.ValidationStatus == "block" {
			job.public.Message = strings.Join(importBlockMessages(job.public.Candidates), "?")
		}
		if flow != nil {
			flow.LogDone("skill.import.validate", "Skill 包校验完成",
				event.P("candidates", len(job.public.Candidates)),
				event.P("files", countCandidateFiles(job)),
				event.P("conflict_groups", len(job.public.ConflictGroups)),
				event.P("validation_status", job.public.ValidationStatus))
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

func (e *Engine) GetImportJob(ctx context.Context, jobID string) (biz.SkillImportJob, error) {
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
		out.StorageRoot = e.resolveRoot(ctx)
		return out, nil
	}

	// Fallback to DB persistence (survives restarts).
	if e.store != nil {
		dbJob, dbErr := e.store.Get(ctx, trimmed)
		if dbErr != nil {
			e.lg.Warn("GetImportJob: DB lookup failed",
				loggateway.StepID("skill.import.db_lookup"),
				loggateway.Str("job_id", trimmed),
				loggateway.Err(dbErr))
		} else if dbJob != nil {
			dbJob.StorageRoot = e.resolveRoot(ctx)
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
	// updateSkillID is set by the overwrite_duplicate decision: instead of
	// creating a new skill, ApplyImport appends a new version to this
	// existing skill and writes files into its storage directory.
	updateSkillID string
	// targetDir overrides the default <root>/<slug> write directory
	// (overwrite writes into the existing skill's storage dir).
	targetDir string
	// retireSkillIDs lists existing skills to archive after a successful
	// merge create (retire_sources=true).
	retireSkillIDs []string
	// derivedFromSkillIDs records merge provenance into the new skill's
	// metadata JSON (derived_from).
	derivedFromSkillIDs []string
}

// ApplyImport executes the user-approved import decisions.
//
// TPM-P1-08: each createImportedSkill writes to disk AND inserts a DB row. If any step fails
// after earlier writes succeeded, we compensate by deleting all already-created skills (DB row +
// disk directory). This makes the operation effectively atomic from the caller's perspective —
// either all succeed or all are cleaned up. (Full two-phase Saga is deferred to Wave 2 / TPM-D-S1.)
func (e *Engine) ApplyImport(ctx context.Context, jobID string, in biz.SkillImportApplyRequest) (biz.SkillImportApplyResult, error) {
	trimmed := strings.TrimSpace(jobID)
	flow := e.flowLog(ctx)

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

	// When the job is in memory AND persisted to DB, keep both in sync.
	// Without this, a crash after the in-memory transition but before DB
	// update would leave the DB at "completed", allowing a duplicate apply.
	if job != nil && e.store != nil {
		swapped, casErr := e.store.CompareAndSwapStatus(ctx, trimmed, "completed", "applying", "")
		if casErr != nil {
			e.lg.Warn("ApplyImport: DB CAS sync failed, rolling back in-memory state",
				loggateway.StepID("skill.import.db_cas"),
				loggateway.Str("job_id", trimmed),
				loggateway.Err(casErr))
			// Roll back in-memory state since DB didn't transition.
			e.jobsMu.Lock()
			if j, ok := e.jobs[trimmed]; ok {
				j.public.Status = "completed"
			}
			e.jobsMu.Unlock()
			return biz.SkillImportApplyResult{}, casErr
		}
		if !swapped {
			// DB status is not "completed" — another process already applied it.
			e.jobsMu.Lock()
			if j, ok := e.jobs[trimmed]; ok {
				j.public.Status = "completed" // roll back in-memory
			}
			e.jobsMu.Unlock()
			return biz.SkillImportApplyResult{}, validationError("import job status changed in DB, cannot apply")
		}
	}

	if job == nil {
		// Fallback to DB persistence (job may have survived a restart).
		// Use CompareAndSwapStatus for atomic CAS — prevents concurrent ApplyImport
		// calls from both proceeding when the job is only in the DB.
		if e.store != nil {
			swapped, casErr := e.store.CompareAndSwapStatus(ctx, trimmed, "completed", "applying", "")
			if casErr != nil {
				e.lg.Warn("ApplyImport: DB CAS failed",
					loggateway.StepID("skill.import.db_cas"),
					loggateway.Str("job_id", trimmed),
					loggateway.Err(casErr))
				return biz.SkillImportApplyResult{}, casErr
			}
			if !swapped {
				// CAS failed — status is not "completed". Read actual status for error message.
				dbJob, dbErr := e.store.Get(ctx, trimmed)
				if dbErr != nil || dbJob == nil {
					return biz.SkillImportApplyResult{}, ErrImportJobNotFound
				}
				switch dbJob.Status {
				case "applied":
					return biz.SkillImportApplyResult{}, validationError("import job already applied")
				case "applying":
					return biz.SkillImportApplyResult{}, validationError("import job is currently being applied")
				default:
					return biz.SkillImportApplyResult{}, validationError("import job is not completed (status: " + dbJob.Status + ")")
				}
			}
			// CAS succeeded — load the full job for processing.
			dbJob, dbErr := e.store.Get(ctx, trimmed)
			if dbErr != nil || dbJob == nil {
				// Roll back CAS: "applying" → "completed" to prevent permanent stuck state
				if rbErr := e.store.UpdateStatus(context.Background(), trimmed, "completed", "CAS succeeded but Get failed, rolled back"); rbErr != nil {
					e.lg.Warn("ApplyImport: DB rollback after Get failed",
						loggateway.StepID("skill.import"),
						loggateway.Str("job_id", trimmed),
						loggateway.Err(rbErr))
				}
				return biz.SkillImportApplyResult{}, ErrImportJobNotFound
			}
			job = &jobState{
				public:     *dbJob,
				candidates: make(map[string]candidateState),
				createdAt:  time.Now(),
			}
			for _, c := range dbJob.Candidates {
				cs := candidateState{public: c}
				if dbJob.TempDir != "" {
					e.restoreCandidateFiles(dbJob.TempDir, c.CandidateID, &cs)
				}
				job.candidates[c.CandidateID] = cs
			}
			// Validate that candidate data survived the restart.
			// If TempDir was empty or files were lost, candidates will have
			// no body and no files, which would silently create broken skills.
			for cid, cs := range job.candidates {
				if strings.TrimSpace(cs.body) == "" && len(cs.files) == 0 {
					return biz.SkillImportApplyResult{}, validationError(
						fmt.Sprintf("candidate %s has no content (temporary files may have been lost after server restart)", cid))
				}
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
		if flow != nil {
			flow.LogError("skill.import.done", "Skill 导入落库失败",
				event.P("job_id", trimmed),
				event.P("error", err.Error()))
		}
		compensate()
		// Roll back status to "completed" so the job can be retried.
		e.jobsMu.Lock()
		if j, ok := e.jobs[trimmed]; ok {
			j.public.Status = "completed"
		}
		e.jobsMu.Unlock()
		if e.store != nil {
			// Use context.Background() so caller cancellation cannot abort the
			// status rollback — same rationale as compensate() above.
			if dbErr := e.store.UpdateStatus(context.Background(), trimmed, "completed", "apply failed: "+err.Error()); dbErr != nil {
				e.lg.Warn("skill.import.partial_err_db_rollback_fail",
					loggateway.StepID("skill.import.apply"),
					loggateway.Str("job_id", trimmed),
					loggateway.Err(dbErr))
			}
		}
		// Return with zero created IDs (compensated away) but preserve skips for diagnostics.
		return biz.SkillImportApplyResult{
			CreatedSkillIDs:     []string{},
			SkippedCandidateIDs: result.SkippedCandidateIDs,
			Message:             fmt.Sprintf("import failed and rolled back: %s", err.Error()),
		}, err
	}

	var newSkills, appendedVersions int
	for _, decision := range in.Decisions {
		params, skipIDs, err := e.resolveDecision(ctx, job, decision)
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
		var created biz.Skill
		if params.updateSkillID != "" {
			// overwrite_duplicate: append a new version to the existing skill.
			// No compensation record — deleting the skill would destroy the
			// pre-existing skill, and the appended version is additive
			// (prior versions remain intact for rollback).
			created, err = e.applyOverwrite(ctx, *params)
			if err != nil {
				return partialErr(err)
			}
			appendedVersions++
		} else {
			var dir string
			created, dir, err = e.createImportedSkill(ctx, *params)
			if err != nil {
				return partialErr(err)
			}
			committed = append(committed, createdSkillRecord{id: created.ID, storageDir: dir})
			newSkills++
		}
		// merge_group_with_ai post-create hooks: provenance is always recorded
		// for merges (C4 血缘链); source retirement is optional (retire_sources).
		if len(params.derivedFromSkillIDs) > 0 {
			if err := e.repo.SetSkillDerivedFrom(ctx, created.ID, params.derivedFromSkillIDs); err != nil {
				return partialErr(err)
			}
		}
		for _, sourceID := range params.retireSkillIDs {
			if err := e.repo.ArchiveSkill(ctx, sourceID); err != nil {
				return partialErr(err)
			}
		}
		result.CreatedSkillIDs = append(result.CreatedSkillIDs, created.ID)
	}
	result.Message = "import completed"

	if flow != nil {
		if len(job.public.ConflictGroups) > 0 {
			flow.LogDone("skill.import.conflict", "Skill 冲突决策完成",
				event.P("groups", len(job.public.ConflictGroups)),
				event.P("keep", len(result.CreatedSkillIDs)),
				event.P("skip", len(result.SkippedCandidateIDs)),
				event.P("decisions", len(in.Decisions)))
		}
		flow.LogDone("skill.import.done", "Skill 导入落库完成",
			event.P("job_id", trimmed),
			event.P("skills", newSkills),
			event.P("versions", newSkills+appendedVersions),
			event.P("skipped", len(result.SkippedCandidateIDs)))
	}

	// Transition in-memory job status from "applying" to "applied".
	e.jobsMu.Lock()
	if j, ok := e.jobs[trimmed]; ok {
		j.public.Status = "applied"
	}
	e.jobsMu.Unlock()

	// Update DB status to "applied".
	// Use context.Background() so caller cancellation cannot leave the DB
	// stuck at "applying" after all skills have been successfully created.
	if e.store != nil {
		if dbErr := e.store.UpdateStatus(context.Background(), trimmed, "applied", result.Message); dbErr != nil {
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

// candidateHasBlock reports whether the candidate carries a block of the given type.
func candidateHasBlock(candidate biz.SkillImportCandidate, blockType string) bool {
	for _, block := range candidate.Blocks {
		if block.Type == blockType {
			return true
		}
	}
	return false
}

// resolveDecision maps an import decision to either a skillCreateParams (to create
// a skill), a list of skipped candidate IDs, or an error. Returns (nil, nil, nil)
// for unsupported actions so the caller can report the error.
func (e *Engine) resolveDecision(ctx context.Context, job *jobState, decision biz.SkillImportDecision) (*skillCreateParams, []string, error) {
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
	case "keep_separate":
		// P-r4-keep-separate: import a warn candidate whose conflict groups were
		// judged non-conflicting (LLM recommendation=keep_separate, low risk).
		// Without this action a genuinely different skill landing in any group
		// can never be installed. Existing skills stay untouched (no update,
		// no archive, no provenance).
		candidate, ok := job.candidates[decision.CandidateID]
		if !ok {
			return nil, nil, detailErr(ErrCandidateNotFound, "candidate "+decision.CandidateID+" not found")
		}
		if candidate.public.ValidationStatus != "warn" {
			return nil, nil, detailErr(ErrCandidateNotPass, "candidate "+decision.CandidateID+" is not warn (keep_separate requires warn)")
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
	case "skip_duplicate":
		// Explicitly drop a duplicate-blocked candidate without installing it.
		candidate, ok := job.candidates[decision.CandidateID]
		if !ok {
			return nil, nil, detailErr(ErrCandidateNotFound, "candidate "+decision.CandidateID+" not found")
		}
		if !candidateHasBlock(candidate.public, "duplicate_name") {
			return nil, nil, detailErr(ErrCandidateNotDuplicate, "candidate "+decision.CandidateID+" is not blocked as duplicate")
		}
		return nil, []string{decision.CandidateID}, nil
	case "overwrite_duplicate":
		// Append the uploaded package as a new version of the existing
		// same-slug skill instead of creating a new skill row.
		candidate, ok := job.candidates[decision.CandidateID]
		if !ok {
			return nil, nil, detailErr(ErrCandidateNotFound, "candidate "+decision.CandidateID+" not found")
		}
		if !candidateHasBlock(candidate.public, "duplicate_name") {
			return nil, nil, detailErr(ErrCandidateNotDuplicate, "candidate "+decision.CandidateID+" is not blocked as duplicate")
		}
		existing, err := e.repo.GetSkillBySkillKey(ctx, candidate.public.Slug)
		if err != nil {
			return nil, nil, detailErr(ErrDuplicateTargetNotFound, "existing skill with slug "+candidate.public.Slug+": "+err.Error())
		}
		storageDir, err := e.repo.GetSkillStorageDir(ctx, existing.ID)
		if err != nil {
			return nil, nil, detailErr(ErrDuplicateTargetNotFound, "storage dir for skill "+existing.ID+": "+err.Error())
		}
		return &skillCreateParams{
			name: candidate.public.Name, slug: candidate.public.Slug,
			description: candidate.public.Description, body: candidate.body,
			tags: candidate.tags, files: candidate.files,
			updateSkillID: existing.ID, targetDir: storageDir,
		}, nil, nil
	case "merge_group_with_ai":
		if strings.TrimSpace(decision.GroupID) == "" {
			return nil, nil, validationError("group_id is required for merge_group_with_ai")
		}
		// 验证冲突组存在，防止引用不存在的 GroupID 创建任意 skill
		var group *biz.SkillConflictGroup
		for i := range job.public.ConflictGroups {
			if job.public.ConflictGroups[i].GroupID == decision.GroupID {
				group = &job.public.ConflictGroups[i]
				break
			}
		}
		if group == nil {
			return nil, nil, detailErr(ErrConflictGroupNotFound, "conflict group "+decision.GroupID+" not found")
		}
		if strings.TrimSpace(decision.MergedName) == "" {
			return nil, nil, validationError("merged_name is required")
		}
		if strings.TrimSpace(decision.MergedBody) == "" {
			return nil, nil, validationError("merged_body is required")
		}
		slug := slugify(decision.MergedName)
		// Files are the union of aux files (everything except SKILL.md):
		// existing skills' on-disk aux files first, then the candidate's aux
		// files win on path conflicts. The merged body always becomes SKILL.md.
		files := map[string][]byte{}
		for _, existing := range group.ExistingSkills {
			dir, err := e.repo.GetSkillStorageDir(ctx, existing.ID)
			if err != nil {
				return nil, nil, detailErr(ErrMergeSourceUnreadable, "storage dir for skill "+existing.ID+": "+err.Error())
			}
			existingFiles, err := ReadSkillDirFiles(dir)
			if err != nil {
				return nil, nil, detailErr(ErrMergeSourceUnreadable, "read files for skill "+existing.ID+": "+err.Error())
			}
			for fname, data := range existingFiles {
				if strings.EqualFold(fname, "SKILL.md") {
					continue
				}
				if _, taken := files[fname]; !taken {
					files[fname] = data
				}
			}
		}
		for _, candidateID := range group.CandidateIDs {
			candidate, ok := job.candidates[candidateID]
			if !ok {
				continue
			}
			for fname, data := range candidate.files {
				if strings.EqualFold(fname, "SKILL.md") {
					continue
				}
				files[fname] = data
			}
		}
		files["SKILL.md"] = []byte(decision.MergedBody)
		params := &skillCreateParams{
			name: decision.MergedName, slug: slug,
			description: decision.MergedDescription, body: decision.MergedBody,
			tags: decision.MergedTags, files: files,
		}
		for _, existing := range group.ExistingSkills {
			params.derivedFromSkillIDs = append(params.derivedFromSkillIDs, existing.ID)
		}
		if decision.RetireSources {
			params.retireSkillIDs = append([]string(nil), params.derivedFromSkillIDs...)
		}
		return params, nil, nil
	case "skip_group":
		return nil, candidateIDsForGroup(job.public.ConflictGroups, decision.GroupID), nil
	default:
		return nil, nil, nil
	}
}

// writeSkillFiles writes all package files under targetDir with full zipslip
// protection (TPM-P1-07) and returns the absolute target directory.
func writeSkillFiles(targetDir string, files map[string][]byte) (string, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	// TPM-P1-07: full zipslip protection — verify joined path stays inside targetDir.
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return "", detailErr(ErrResolveTargetDir, err.Error())
	}
	for fname, data := range files {
		if err := ensurePathWithin(absTarget, fname); err != nil {
			return "", err
		}
		clean := filepath.Clean(fname)
		path := filepath.Join(absTarget, clean)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return "", err
		}
	}
	return absTarget, nil
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
	absTarget, err := writeSkillFiles(targetDir, p.files)
	if err != nil {
		return biz.Skill{}, "", err
	}
	skill, err := e.repo.CreateSkillWithVersion(ctx, biz.SkillCreateInput{Name: p.name, Slug: slug, Description: p.description, Body: p.body, Tags: p.tags, Triggers: manifest.Parse(p.body).Triggers, StorageDir: targetDir, SyncOrigin: biz.SkillSyncOriginImport})
	if err != nil {
		// DB 创建失败时清理已写入的磁盘文件，防止孤儿资源残留。
		// 调用方 ApplyImport 的 compensate() 只清理已提交到 committed 列表的 skill，
		// 失败的 skill 未加入列表，因此必须在此处自行清理。
		if rmErr := os.RemoveAll(absTarget); rmErr != nil {
			e.lg.Warn("createImportedSkill: failed to cleanup disk after DB failure",
				loggateway.StepID("skill.import"),
				loggateway.Str("dir", absTarget),
				loggateway.Err(rmErr))
		}
		return biz.Skill{}, "", err
	}
	return skill, absTarget, nil
}

// applyOverwrite executes the overwrite_duplicate path: writes the candidate
// files into the existing skill's storage directory and appends a new version
// (parent = current latest) while refreshing name/description/tags.
//
// S1 fix (disk consistency): the existing directory is backed up before any
// write. If the DB append fails, the original on-disk content is restored —
// otherwise the filesystem watcher would sync the half-applied new files as
// an unintended version even though the import reported failure.
func (e *Engine) applyOverwrite(ctx context.Context, p skillCreateParams) (biz.Skill, error) {
	backupDir := p.targetDir + ".overwrite-backup"
	_ = os.RemoveAll(backupDir) // clear stale backup left by a crashed run
	backedUp := false
	if _, statErr := os.Stat(p.targetDir); statErr == nil {
		if err := os.Rename(p.targetDir, backupDir); err != nil {
			return biz.Skill{}, fmt.Errorf("backup existing skill dir: %w", err)
		}
		backedUp = true
	}
	restore := func() {
		_ = os.RemoveAll(p.targetDir)
		if backedUp {
			if err := os.Rename(backupDir, p.targetDir); err != nil {
				e.lg.Warn("applyOverwrite: failed to restore disk after DB failure",
					loggateway.StepID("skill.import"),
					loggateway.Str("dir", p.targetDir),
					loggateway.Err(err))
			}
		}
	}

	if _, err := writeSkillFiles(p.targetDir, p.files); err != nil {
		restore()
		return biz.Skill{}, err
	}
	skill, err := e.repo.AppendImportedVersion(ctx, biz.SkillImportVersionInput{
		SkillID:     p.updateSkillID,
		Name:        p.name,
		Description: p.description,
		Body:        p.body,
		Tags:        p.tags,
		Triggers:    manifest.Parse(p.body).Triggers,
	})
	if err != nil {
		restore()
		return biz.Skill{}, err
	}
	_ = os.RemoveAll(backupDir)
	return skill, nil
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
		content, err := io.ReadAll(io.LimitReader(rc, maxSkillFileBytes+1))
		closeErr := rc.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if len(content) > maxSkillFileBytes {
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

	// F2: groups without SKILL.md that live under a SKILL.md group's subtree
	// are merged into that candidate (e.g. root SKILL.md + core/x.py), instead
	// of being silently dropped. Merging only happens when exactly one group
	// contains SKILL.md — multi-skill packages never cross-merge.
	dirs := make([]string, 0, len(filesByDir))
	for dir := range filesByDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	hasSkillMD := func(dir string) bool {
		files := filesByDir[dir]
		if _, ok := files["SKILL.md"]; ok {
			return true
		}
		_, ok := files["skill.md"]
		return ok
	}
	skillDirs := []string{}
	for _, dir := range dirs {
		if hasSkillMD(dir) {
			skillDirs = append(skillDirs, dir)
		}
	}
	mergedInto := map[string]bool{} // group dir absorbed into a candidate
	for _, dir := range skillDirs {
		files := filesByDir[dir]
		bodyBytes, ok := files["SKILL.md"]
		if !ok {
			bodyBytes = files["skill.md"]
		}
		body := string(bodyBytes)
		merged := make(map[string][]byte, len(files))
		for fname, data := range files {
			merged[fname] = data
		}
		if len(skillDirs) == 1 {
			for _, other := range dirs {
				if other == dir || hasSkillMD(other) {
					continue
				}
				// Only groups inside this group's subtree are merged. Since
				// groups key on the top-level path component, this can only
				// ever trigger for the root group (".").
				if dir != "." && !strings.HasPrefix(other, dir+"/") {
					continue
				}
				for rel, data := range filesByDir[other] {
					merged[other+"/"+rel] = data
				}
				mergedInto[other] = true
			}
		}
		candidate, tags, _ := ValidateSkillPackage(merged, dir, existing, false)
		candidate.TargetDir = filepath.Join(job.public.StorageRoot, candidate.Slug)
		job.candidates[candidate.CandidateID] = candidateState{public: candidate, body: body, files: merged, tags: tags}
		job.public.Candidates = append(job.public.Candidates, candidate)
	}
	if len(job.public.Candidates) == 0 {
		return validationError("zip must contain at least one SKILL.md")
	}
	// Groups that were neither imported nor merged must surface a warning —
	// silently dropping files produced incomplete skills (F2).
	skipped := []string{}
	for _, dir := range dirs {
		if hasSkillMD(dir) || mergedInto[dir] {
			continue
		}
		skipped = append(skipped, dir)
	}
	if len(skipped) > 0 {
		warning := biz.SkillImportIssue{
			Type:    "files_skipped",
			Message: "zip entries not imported (no SKILL.md in group): " + strings.Join(skipped, ", "),
		}
		job.public.Candidates[0].Warnings = append(job.public.Candidates[0].Warnings, warning)
		state := job.candidates[job.public.Candidates[0].CandidateID]
		state.public = job.public.Candidates[0]
		job.candidates[job.public.Candidates[0].CandidateID] = state
	}
	return e.inspectSimilarity(ctx, job, existing)
}

// maxSimilarityLLMCalls caps the number of LLM similarity comparisons per import.
const maxSimilarityLLMCalls = 50

// similarityConcurrency caps parallel LLM similarity calls per import job.
// Serial execution made large imports exceed the frontend HTTP timeout (FN-1).
const similarityConcurrency = 5

// similarityCallTimeout bounds a single similarity LLM call so one slow
// provider response cannot stall the whole import. Var for test override.
var similarityCallTimeout = 15 * time.Second

// similarityThreshold is the minimum similarity score to flag a conflict group.
// Scores below this threshold are considered not similar enough to warrant
// user intervention. The value 0.2 was calibrated against embedding model baselines
// where unrelated skills typically score 0.0–0.15.
const similarityThreshold = 0.2

// similarityPair snapshots one (candidate, existing source) comparison. The
// candidate body is captured up front so worker goroutines never touch
// job.candidates (all job mutations happen under mu after g.Wait joins).
type similarityPair struct {
	candidateID string
	state       candidateState
	source      biz.SkillSimilaritySource
}

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
	pairs := make([]similarityPair, 0, len(existing))
	truncated := false
collect:
	for _, candidate := range job.public.Candidates {
		if candidate.ValidationStatus != "pass" {
			continue
		}
		for _, source := range existing {
			if len(pairs) >= maxSimilarityLLMCalls {
				truncated = true
				break collect
			}
			pairs = append(pairs, similarityPair{
				candidateID: candidate.CandidateID,
				state:       job.candidates[candidate.CandidateID],
				source:      source,
			})
		}
	}
	if truncated {
		e.lg.Warn("inspectSimilarity LLM call limit reached, skipping remaining comparisons",
			loggateway.StepID("skill.similarity_cap"),
			loggateway.Int("cap", maxSimilarityLLMCalls))
	}
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(similarityConcurrency)
	for _, pr := range pairs {
		g.Go(func() error {
			callCtx, cancel := context.WithTimeout(gctx, similarityCallTimeout)
			defer cancel()
			metrics, reason, evidence, err := e.modelSimilarity(callCtx, cfg, pr.state, pr.source)
			if err != nil {
				e.lg.Warn("inspectSimilarity: model similarity check failed",
					loggateway.StepID("skill.similarity_llm_fail"),
					loggateway.Str("candidate", pr.candidateID),
					loggateway.Err(err))
				return nil
			}
			if metrics.SimilarityScore < similarityThreshold {
				return nil
			}
			group := biz.SkillConflictGroup{
				GroupID:                newID(),
				HighestSimilarityScore: metrics.SimilarityScore,
				Metrics:                metrics,
				Reason:                 reason,
				Evidence:               evidence,
				CandidateIDs:           []string{pr.candidateID},
				ExistingSkills:         []biz.SkillSimilaritySource{pr.source},
				CanRefine:              true,
			}
			mu.Lock()
			job.public.ConflictGroups = append(job.public.ConflictGroups, group)
			e.updateCandidateWarning(job, pr.candidateID, metrics)
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return nil
}

func (e *Engine) modelSimilarity(ctx context.Context, cfg chatModelCfg, candidate candidateState, source biz.SkillSimilaritySource) (biz.SkillSimilarityMetrics, string, []string, error) {
	prompt := buildSimilarityPrompt(candidate, source)
	raw, err := completeChat(ctx, cfg, prompt)
	if err != nil {
		return biz.SkillSimilarityMetrics{}, "", nil, err
	}
	return parseSimilarityResult(raw)
}

// parseSimilarityResult parses the LLM similarity response tolerantly. LLMs
// routinely violate the declared schema (numeric conflict_risk, evidence as a
// bare string, quoted numbers); strict typed unmarshal fails the whole pair
// comparison and silently produces no conflict groups, so every field is
// coerced from map[string]any instead.
func parseSimilarityResult(raw string) (biz.SkillSimilarityMetrics, string, []string, error) {
	var m map[string]any
	if err := decodeModelJSON(raw, &m); err != nil {
		return biz.SkillSimilarityMetrics{}, "", nil, err
	}
	metrics := biz.SkillSimilarityMetrics{
		SimilarityScore:       clamp01(jsonFloat(m["similarity_score"])),
		NameSimilarity:        clamp01(jsonFloat(m["name_similarity"])),
		DescriptionSimilarity: clamp01(jsonFloat(m["description_similarity"])),
		BodySimilarity:        clamp01(jsonFloat(m["body_similarity"])),
		TriggerSimilarity:     clamp01(jsonFloat(m["trigger_similarity"])),
		ToolSimilarity:        clamp01(jsonFloat(m["tool_similarity"])),
		Confidence:            clamp01(jsonFloat(m["confidence"])),
		Recommendation:        jsonString(m["recommendation"]),
		ConflictRisk:          normalizeConflictRisk(m["conflict_risk"]),
	}
	if metrics.Recommendation == "" {
		metrics.Recommendation = "suggest_refine"
	}
	if metrics.ConflictRisk == "" {
		metrics.ConflictRisk = "medium"
	}
	return metrics, jsonString(m["reason"]), jsonStringSlice(m["evidence"]), nil
}

// jsonFloat coerces a decoded JSON value to float64, accepting both numbers
// and numeric strings (LLMs occasionally quote them). Returns 0 otherwise.
func jsonFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err == nil {
			return f
		}
	}
	return 0
}

// jsonString coerces a decoded JSON value to a trimmed string.
func jsonString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// jsonStringSlice coerces a decoded JSON value to []string, accepting both a
// JSON array and a single bare string (LLMs occasionally return one evidence
// string instead of an array).
func jsonStringSlice(v any) []string {
	switch t := v.(type) {
	case string:
		if s := strings.TrimSpace(t); s != "" {
			return []string{s}
		}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// normalizeConflictRisk maps LLM-returned conflict_risk to the canonical
// "low"/"medium"/"high" strings. Accepts both the documented string form and
// the commonly-hallucinated 0-1 numeric form (bucketed: >=0.66 high, >=0.33
// medium, else low). Returns "" for unrecognized values so callers apply their
// default.
func normalizeConflictRisk(v any) string {
	switch t := v.(type) {
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "low", "medium", "high":
			return s
		}
	case float64:
		switch {
		case t >= 0.66:
			return "high"
		case t >= 0.33:
			return "medium"
		default:
			return "low"
		}
	}
	return ""
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
		absCandidateDir, err := filepath.Abs(candidateDir)
		if err != nil {
			e.lg.Warn("Import: failed to resolve candidate temp dir path",
				loggateway.StepID("skill.import.temp_dir"),
				loggateway.Str("candidate_id", cid),
				loggateway.Err(err))
			continue
		}
		if err := os.MkdirAll(candidateDir, 0o755); err != nil {
			e.lg.Warn("Import: failed to create candidate temp dir",
				loggateway.StepID("skill.import.temp_dir"),
				loggateway.Str("candidate_id", cid),
				loggateway.Err(err))
			continue
		}
		// Write all files from the zip.
		for fname, data := range cs.files {
			// Zipslip defense: verify the file path stays within candidateDir.
			if err := ensurePathWithin(absCandidateDir, fname); err != nil {
				e.lg.Warn("Import: unsafe file path in candidate, skipping",
					loggateway.StepID("skill.import.temp_dir"),
					loggateway.Str("candidate_id", cid),
					loggateway.Str("file", fname),
					loggateway.Err(err))
				continue
			}
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

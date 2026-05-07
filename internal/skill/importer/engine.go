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

	"aranea-agents/internal/biz"
	"aranea-agents/internal/pkg/skillstorage"
)

// Engine holds in-memory import jobs and executes zip inspection / LLM similarity (OpenAI-compatible + Anthropic messages).
type Engine struct {
	repo biz.SkillRepo
	llm  *biz.LlmProviderModelUsecase
	sys  biz.SystemSettingRepo

	jobsMu sync.RWMutex
	jobs   map[string]*jobState
}

type jobState struct {
	public     biz.SkillImportJob
	candidates map[string]candidateState
}

type candidateState struct {
	public biz.SkillImportCandidate
	body   string
	files  map[string][]byte
	tags   []biz.SkillTag
}

// NewEngine constructs the skill ZIP importer. Skill storage root resolves via skillstorage + system settings.
func NewEngine(repo biz.SkillRepo, llm *biz.LlmProviderModelUsecase, sys biz.SystemSettingRepo) *Engine {
	return &Engine{
		repo: repo,
		llm:  llm,
		sys:  sys,
		jobs: make(map[string]*jobState),
	}
}

func (e *Engine) resolveRoot(ctx context.Context) string {
	if e.sys == nil {
		return skillstorage.ResolveRootFromEnv()
	}
	st, err := e.sys.Get(ctx)
	if err != nil {
		return skillstorage.ResolveRootFromEnv()
	}
	return skillstorage.ResolveRootWithPlatform(st.RootDirectory)
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
	const maxZipBytes = 20 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(file, maxZipBytes+1))
	if err != nil {
		return biz.SkillImportJob{}, err
	}
	if len(data) > maxZipBytes {
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
	e.jobsMu.RLock()
	defer e.jobsMu.RUnlock()
	job := e.jobs[strings.TrimSpace(jobID)]
	if job == nil {
		return biz.SkillImportJob{}, ErrImportJobNotFound
	}
	out := job.public
	out.StorageRoot = e.resolveRoot(context.Background())
	return out, nil
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
	result := biz.SkillImportApplyResult{CreatedSkillIDs: []string{}, SkippedCandidateIDs: []string{}}
	for _, decision := range in.Decisions {
		switch decision.Action {
		case "import_passed":
			candidate, ok := job.candidates[decision.CandidateID]
			if !ok {
				return result, fmt.Errorf("candidate %s not found", decision.CandidateID)
			}
			if candidate.public.ValidationStatus != "pass" {
				return result, fmt.Errorf("candidate %s is not pass", decision.CandidateID)
			}
			created, err := e.createImportedSkill(ctx, candidate.public.Name, candidate.public.Slug, candidate.public.Description, candidate.body, candidate.tags, candidate.files)
			if err != nil {
				return result, err
			}
			result.CreatedSkillIDs = append(result.CreatedSkillIDs, created.ID)
		case "approve_risky_import":
			candidate, ok := job.candidates[decision.CandidateID]
			if !ok {
				return result, fmt.Errorf("candidate %s not found", decision.CandidateID)
			}
			if !candidateRequiresRiskApproval(candidate.public) {
				return result, fmt.Errorf("candidate %s does not require high risk approval", decision.CandidateID)
			}
			created, err := e.createImportedSkill(ctx, candidate.public.Name, candidate.public.Slug, candidate.public.Description, candidate.body, candidate.tags, candidate.files)
			if err != nil {
				return result, err
			}
			result.CreatedSkillIDs = append(result.CreatedSkillIDs, created.ID)
		case "reject_risky_upload":
			if strings.TrimSpace(decision.CandidateID) == "" {
				return result, validationError("candidate_id is required")
			}
			result.SkippedCandidateIDs = append(result.SkippedCandidateIDs, decision.CandidateID)
		case "merge_group_with_ai":
			if strings.TrimSpace(decision.MergedBody) == "" {
				return result, validationError("merged_body is required")
			}
			slug := slugify(decision.MergedName)
			files := map[string][]byte{"SKILL.md": []byte(decision.MergedBody)}
			created, err := e.createImportedSkill(ctx, decision.MergedName, slug, decision.MergedDescription, decision.MergedBody, decision.MergedTags, files)
			if err != nil {
				return result, err
			}
			result.CreatedSkillIDs = append(result.CreatedSkillIDs, created.ID)
		case "skip_group":
			for _, id := range candidateIDsForGroup(job.public.ConflictGroups, decision.GroupID) {
				result.SkippedCandidateIDs = append(result.SkippedCandidateIDs, id)
			}
		default:
			return result, fmt.Errorf("unsupported import action: %s", decision.Action)
		}
	}
	result.Message = "????"
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

func (e *Engine) createImportedSkill(ctx context.Context, name string, slug string, description string, body string, tags []biz.SkillTag, files map[string][]byte) (biz.Skill, error) {
	slug = slugify(slug)
	if slug == "" {
		slug = slugify(name)
	}
	targetDir := filepath.Join(e.resolveRoot(ctx), slug)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return biz.Skill{}, err
	}
	for fname, data := range files {
		clean := filepath.Clean(fname)
		if strings.Contains(clean, "..") {
			return biz.Skill{}, fmt.Errorf("unsafe skill file path: %s", fname)
		}
		path := filepath.Join(targetDir, clean)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return biz.Skill{}, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return biz.Skill{}, err
		}
	}
	return e.repo.CreateSkillWithVersion(ctx, biz.SkillCreateInput{Name: name, Slug: slug, Description: description, Body: body, Tags: tags, StorageDir: targetDir})
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
			Message: fmt.Sprintf("??????? %d%%?????", int(metrics.SimilarityScore*100+0.5)),
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
		name := filepath.ToSlash(file.Name)
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
			return fmt.Errorf("unsafe zip path: %s", file.Name)
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
			return fmt.Errorf("skill file too large: %s", name)
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
	for _, candidate := range job.public.Candidates {
		if candidate.ValidationStatus != "pass" {
			continue
		}
		state := job.candidates[candidate.CandidateID]
		for _, source := range existing {
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

package service

import (
	"arenea/backend/internal/kernel/errs"
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	adkr "arenea/backend/internal/conversation/adapters/adkruntime"
	"arenea/backend/internal/domain"
	"arenea/backend/internal/util"
	"github.com/fsnotify/fsnotify"
)

type skillStore interface {
	SearchSkills(query domain.SkillListQuery) (domain.SkillListResult, error)
	GetSkillByID(id string) (domain.Skill, error)
	UpdateSkillEnabled(id string, enabled bool) (domain.Skill, error)
	DuplicateSkill(id string) (domain.Skill, error)
	DeleteSkill(id string) error
	SearchSkillInvocations(query domain.SkillRunQuery) (domain.SkillRunResult, error)
	ListSkillSimilaritySources() ([]domain.SkillSimilaritySource, error)
	CreateSkillWithVersion(input domain.SkillCreateInput) (domain.Skill, error)
	GetSkillStorageDir(id string) (string, error)
	ListPlatformResources(resource string) ([]domain.PlatformResource, error)
}

type SkillService struct {
	store       skillStore
	runtime     *adkr.ADKRuntimeAdapter
	storageRoot string
	importJobs  map[string]*skillImportJobState
	importJobsM sync.RWMutex
}

type skillImportJobState struct {
	public     domain.SkillImportJob
	candidates map[string]skillCandidateState
}

type skillCandidateState struct {
	public domain.SkillImportCandidate
	body   string
	files  map[string][]byte
	tags   []domain.SkillTag
}

func NewSkillService(store skillStore, runtimeAdapter *adkr.ADKRuntimeAdapter, storageRoot string) *SkillService {
	if strings.TrimSpace(storageRoot) == "" {
		storageRoot = util.ResolveSkillStorageRoot()
	}
	return &SkillService{
		store:       store,
		runtime:     runtimeAdapter,
		storageRoot: util.AbsoluteStoragePath(storageRoot),
		importJobs:  map[string]*skillImportJobState{},
	}
}

func (s *SkillService) Search(query domain.SkillListQuery) (domain.SkillListResult, error) {
	query.Limit, query.Offset = normalizeLimitOffset(query.Limit, query.Offset, 20)
	query.Enabled = strings.TrimSpace(query.Enabled)
	if query.Enabled != "" && query.Enabled != "true" && query.Enabled != "false" {
		return domain.SkillListResult{}, validationError("enabled must be true or false")
	}
	query.Status = strings.TrimSpace(query.Status)
	if query.Status != "" && query.Status != "draft" && query.Status != "published" && query.Status != "archived" {
		return domain.SkillListResult{}, validationError("unsupported skill status")
	}
	return s.store.SearchSkills(query)
}

func (s *SkillService) ToggleEnabled(id string, enabled bool) (domain.Skill, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Skill{}, validationError("skill id is required")
	}
	return s.store.UpdateSkillEnabled(id, enabled)
}

func (s *SkillService) Duplicate(id string) (domain.Skill, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Skill{}, validationError("skill id is required")
	}
	return s.store.DuplicateSkill(id)
}

func (s *SkillService) Delete(id string) error {
	if strings.TrimSpace(id) == "" {
		return validationError("skill id is required")
	}
	return s.store.DeleteSkill(id)
}

func (s *SkillService) ListFiles(id string) ([]domain.SkillFile, error) {
	root, err := s.skillDir(id)
	if err != nil {
		return nil, err
	}
	files := []domain.SkillFile{}
	if err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
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
		files = append(files, domain.SkillFile{Path: rel, Name: pathBase(rel), Language: languageForPath(rel), Size: info.Size(), UpdatedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00")})
		return nil
	}); err != nil {
		return nil, err
	}
	return files, nil
}

func (s *SkillService) ReadFile(id string, relPath string) (domain.SkillFileContent, error) {
	root, path, err := s.safeSkillFilePath(id, relPath)
	if err != nil {
		return domain.SkillFileContent{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return domain.SkillFileContent{}, err
	}
	if info.IsDir() {
		return domain.SkillFileContent{}, validationError("skill file path points to a directory")
	}
	if info.Size() > 2*1024*1024 {
		return domain.SkillFileContent{}, validationError("skill file is too large to edit")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.SkillFileContent{}, err
	}
	rel, _ := filepath.Rel(root, path)
	rel = filepath.ToSlash(rel)
	return domain.SkillFileContent{Path: rel, Content: string(raw), Language: languageForPath(rel)}, nil
}

func (s *SkillService) UpdateFile(id string, input domain.SkillFileUpdateInput) (domain.SkillFileContent, error) {
	_, path, err := s.safeSkillFilePath(id, input.Path)
	if err != nil {
		return domain.SkillFileContent{}, err
	}
	if err = os.WriteFile(path, []byte(input.Content), 0o644); err != nil {
		return domain.SkillFileContent{}, err
	}
	return s.ReadFile(id, input.Path)
}

func (s *SkillService) SearchRuns(query domain.SkillRunQuery) (domain.SkillRunResult, error) {
	query.Limit, query.Offset = normalizeLimitOffset(query.Limit, query.Offset, 20)
	query.Status = strings.TrimSpace(query.Status)
	if query.Status != "" && query.Status != "success" && query.Status != "failure" && query.Status != "pending" {
		return domain.SkillRunResult{}, validationError("unsupported run status")
	}
	return s.store.SearchSkillInvocations(query)
}

func (s *SkillService) Import(ctx context.Context, file multipart.File, header *multipart.FileHeader) (domain.SkillImportJob, error) {
	if file == nil || header == nil {
		return domain.SkillImportJob{}, validationError("skill zip file is required")
	}
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		return domain.SkillImportJob{}, validationError("skill upload must be a .zip file")
	}
	const maxZipBytes = 20 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(file, maxZipBytes+1))
	if err != nil {
		return domain.SkillImportJob{}, err
	}
	if len(data) > maxZipBytes {
		return domain.SkillImportJob{}, validationError("skill zip must be <= 20MB")
	}
	job := &skillImportJobState{
		public: domain.SkillImportJob{
			JobID:            newID(),
			Status:           "processing",
			ValidationStatus: "pass",
			StorageRoot:      s.storageRoot,
			Candidates:       []domain.SkillImportCandidate{},
			ConflictGroups:   []domain.SkillConflictGroup{},
		},
		candidates: map[string]skillCandidateState{},
	}
	if err = s.inspectSkillZip(ctx, data, job); err != nil {
		job.public.Status = "failed"
		job.public.ValidationStatus = "block"
		job.public.Message = err.Error()
	} else {
		job.public.Status = "completed"
		job.public.ValidationStatus = summarizeImportStatus(job.public.Candidates, job.public.ConflictGroups)
		if job.public.ValidationStatus == "block" {
			job.public.Message = strings.Join(importBlockMessages(job.public.Candidates), "；")
		}
	}
	s.importJobsM.Lock()
	s.importJobs[job.public.JobID] = job
	s.importJobsM.Unlock()
	return job.public, nil
}

func (s *SkillService) GetImportJob(jobID string) (domain.SkillImportJob, error) {
	s.importJobsM.RLock()
	defer s.importJobsM.RUnlock()
	job := s.importJobs[strings.TrimSpace(jobID)]
	if job == nil {
		return domain.SkillImportJob{}, fmt.Errorf("%w: import job not found", errs.ErrNotFound)
	}
	return job.public, nil
}

func (s *SkillService) RefineConflictGroup(ctx context.Context, jobID string, groupID string, in domain.SkillRefineRequest) (domain.SkillRefineResult, error) {
	job, group, candidates, err := s.conflictGroupContext(jobID, groupID)
	if err != nil {
		return domain.SkillRefineResult{}, err
	}
	providerModel, err := s.resolveSimilarityModel(in.Provider, in.Model)
	if err != nil {
		return domain.SkillRefineResult{}, err
	}
	prompt := buildRefinePrompt(group, candidates, strings.TrimSpace(in.Instructions))
	result, err := s.runtime.Generate(ctx, adkr.GenerateRequest{
		Agent:         skillSystemAgent(),
		ProviderModel: providerModel,
		Messages:      []adkr.ChatMessage{{Role: "user", Content: prompt}},
		Input:         prompt,
	})
	if err != nil {
		return domain.SkillRefineResult{}, err
	}
	refined, err := parseRefineResult(result.Content)
	if err != nil {
		return domain.SkillRefineResult{}, err
	}
	refined.SourceCandidateIDs = group.CandidateIDs
	for _, existing := range group.ExistingSkills {
		refined.SourceExistingSkillIDs = append(refined.SourceExistingSkillIDs, existing.ID)
	}
	_ = job
	return refined, nil
}

func (s *SkillService) ApplyImport(jobID string, in domain.SkillImportApplyRequest) (domain.SkillImportApplyResult, error) {
	s.importJobsM.RLock()
	job := s.importJobs[strings.TrimSpace(jobID)]
	s.importJobsM.RUnlock()
	if job == nil {
		return domain.SkillImportApplyResult{}, fmt.Errorf("%w: import job not found", errs.ErrNotFound)
	}
	if job.public.Status != "completed" {
		return domain.SkillImportApplyResult{}, validationError("import job is not completed")
	}
	result := domain.SkillImportApplyResult{CreatedSkillIDs: []string{}, SkippedCandidateIDs: []string{}}
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
			created, err := s.createImportedSkill(candidate.public.Name, candidate.public.Slug, candidate.public.Description, candidate.body, candidate.tags, candidate.files)
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
			created, err := s.createImportedSkill(candidate.public.Name, candidate.public.Slug, candidate.public.Description, candidate.body, candidate.tags, candidate.files)
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
			created, err := s.createImportedSkill(decision.MergedName, slug, decision.MergedDescription, decision.MergedBody, decision.MergedTags, files)
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
	result.Message = "导入完成"
	return result, nil
}

func normalizeLimitOffset(limit int, offset int, defaultLimit int) (int, int) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (s *SkillService) StartDirectorySync(ctx context.Context, interval int) {
	_ = s.SyncDirectoryOnce()
	if interval <= 0 {
		interval = 1
	}
	debounce := time.Duration(interval) * time.Second
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		s.startDirectorySyncPolling(ctx, debounce)
		return
	}
	defer watcher.Close()
	root := s.storageRoot
	if err = os.MkdirAll(root, 0o755); err != nil {
		return
	}
	watched := map[string]bool{}
	addWatch := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil || watched[abs] {
			return
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return
		}
		if err = watcher.Add(abs); err == nil {
			watched[abs] = true
		}
	}
	addWatch(root)
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if entry.IsDir() {
			addWatch(filepath.Join(root, entry.Name()))
		}
	}
	syncNow := make(chan struct{}, 1)
	queueSync := func() {
		select {
		case syncNow <- struct{}{}:
		default:
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) != 0 {
				addWatch(event.Name)
				queueSync()
			}
		case <-watcher.Errors:
			queueSync()
		case <-syncNow:
			timer := time.NewTimer(debounce)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			_ = s.SyncDirectoryOnce()
			entries, _ := os.ReadDir(root)
			for _, entry := range entries {
				if entry.IsDir() {
					addWatch(filepath.Join(root, entry.Name()))
				}
			}
		}
	}
}

func (s *SkillService) startDirectorySyncPolling(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.SyncDirectoryOnce()
		}
	}
}

func (s *SkillService) SyncDirectoryOnce() error {
	root := s.storageRoot
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(root, 0o755)
		}
		return err
	}
	existing, err := s.store.ListSkillSimilaritySources()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, item := range existing {
		known[strings.ToLower(item.Slug)] = true
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		bodyPath := filepath.Join(dir, "SKILL.md")
		raw, err := os.ReadFile(bodyPath)
		if err != nil {
			continue
		}
		body := string(raw)
		name, desc, tags := parseSkillMarkdown(body)
		slug := slugify(firstNonEmptyString(name, entry.Name()))
		if known[strings.ToLower(slug)] {
			continue
		}
		if strings.TrimSpace(name) == "" || strings.TrimSpace(desc) == "" {
			continue
		}
		if _, err = s.store.CreateSkillWithVersion(domain.SkillCreateInput{Name: name, Slug: slug, Description: desc, Body: body, Tags: tags, StorageDir: dir}); err == nil {
			known[strings.ToLower(slug)] = true
		}
	}
	return nil
}

func (s *SkillService) skillDir(id string) (string, error) {
	dir, err := s.store.GetSkillStorageDir(id)
	if strings.TrimSpace(dir) == "" {
		current, getErr := s.store.GetSkillByID(id)
		if getErr != nil {
			if err != nil {
				return "", err
			}
			return "", getErr
		}
		dir = filepath.Join(s.storageRoot, current.Slug)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		return "", statErr
	}
	return dir, nil
}

func (s *SkillService) safeSkillFilePath(id string, relPath string) (string, string, error) {
	root, err := s.skillDir(id)
	if err != nil {
		return "", "", err
	}
	relPath = strings.TrimSpace(filepath.ToSlash(relPath))
	if relPath == "" || strings.Contains(relPath, "..") || strings.HasPrefix(relPath, "/") {
		return "", "", validationError("unsafe skill file path")
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
		return "", "", validationError("skill file path escapes skill directory")
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

func isBlockedSkillFile(name string) bool {
	lower := strings.ToLower(name)
	blocked := []string{".exe", ".bat", ".cmd", ".ps1", ".dll", ".so", ".dylib"}
	for _, ext := range blocked {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func skillZipGroupPath(name string) (string, string) {
	name = filepath.ToSlash(name)
	name = strings.Trim(name, "/")
	if name == "" {
		return ".", ""
	}
	parts := strings.Split(name, "/")
	if len(parts) == 1 {
		return ".", parts[0]
	}
	return parts[0], strings.Join(parts[1:], "/")
}

func pathBase(name string) string {
	name = strings.Trim(filepath.ToSlash(name), "/")
	if name == "" || name == "." {
		return ""
	}
	parts := strings.Split(name, "/")
	return parts[len(parts)-1]
}

func hasShellScriptAsset(files map[string][]byte) bool {
	for name := range files {
		if strings.HasSuffix(strings.ToLower(name), ".sh") {
			return true
		}
	}
	return false
}

func highRiskFileNames(files map[string][]byte) []string {
	names := []string{}
	for name := range files {
		if isBlockedSkillFile(name) {
			names = append(names, name)
		}
	}
	return names
}

func candidateRequiresRiskApproval(candidate domain.SkillImportCandidate) bool {
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

func parseSkillMarkdown(body string) (string, string, []domain.SkillTag) {
	name := ""
	description := ""
	tags := []domain.SkillTag{}
	lines := strings.Split(body, "\n")
	inFrontmatter := false
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		inFrontmatter = true
		for _, line := range lines[1:] {
			trimmed := strings.TrimSpace(line)
			if trimmed == "---" {
				break
			}
			if key, value, ok := splitMetaLine(trimmed); ok {
				switch strings.ToLower(key) {
				case "name", "title":
					name = strings.Trim(value, `"'`)
				case "description", "summary":
					description = strings.Trim(value, `"'`)
				case "tags":
					for _, tag := range strings.Split(strings.Trim(value, "[]"), ",") {
						tag = strings.Trim(strings.TrimSpace(tag), `"'`)
						if tag != "" {
							tags = append(tags, domain.SkillTag{Name: tag, Source: "user"})
						}
					}
				}
			}
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if name == "" && strings.HasPrefix(trimmed, "# ") {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if description == "" && !inFrontmatter && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			description = trimmed
		}
	}
	return name, description, tags
}

func splitMetaLine(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9\-_]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	if value == "" {
		value = fmt.Sprintf("skill-%d", len(value))
	}
	return value
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func summarizeImportStatus(candidates []domain.SkillImportCandidate, groups []domain.SkillConflictGroup) string {
	for _, candidate := range candidates {
		if candidate.ValidationStatus == "block" {
			return "block"
		}
	}
	if len(groups) > 0 {
		return "warn"
	}
	return "pass"
}

func importBlockMessages(candidates []domain.SkillImportCandidate) []string {
	messages := []string{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		for _, block := range candidate.Blocks {
			message := strings.TrimSpace(block.Message)
			if message == "" || seen[message] {
				continue
			}
			seen[message] = true
			messages = append(messages, message)
		}
	}
	return messages
}

func updateCandidateWarning(job *skillImportJobState, candidateID string, metrics domain.SkillSimilarityMetrics) {
	for i := range job.public.Candidates {
		if job.public.Candidates[i].CandidateID != candidateID {
			continue
		}
		job.public.Candidates[i].ValidationStatus = "warn"
		job.public.Candidates[i].StatusIcon = "merge_suggested"
		job.public.Candidates[i].Warnings = append(job.public.Candidates[i].Warnings, domain.SkillImportIssue{
			Type:    "similarity",
			Message: fmt.Sprintf("模型判定相似度 %d%%，建议炼化", int(metrics.SimilarityScore*100+0.5)),
		})
		state := job.candidates[candidateID]
		state.public = job.public.Candidates[i]
		job.candidates[candidateID] = state
	}
}

func providerModelHasCredentials(row domain.PlatformResource) bool {
	var cfg struct {
		APIBaseURL string `json:"api_base_url"`
		APIKey     string `json:"api_key"`
	}
	if err := json.Unmarshal([]byte(row.ConfigJSON), &cfg); err != nil {
		return false
	}
	return strings.TrimSpace(cfg.APIBaseURL) != "" && strings.TrimSpace(cfg.APIKey) != ""
}

func skillSystemAgent() domain.Agent {
	return domain.Agent{ID: "skill-system", AgentKey: "skill-system", DisplayName: "Skill System", Provider: "system", Model: "system", Status: "active"}
}

func buildSimilarityPrompt(candidate skillCandidateState, source domain.SkillSimilaritySource) string {
	return fmt.Sprintf(`你是 Skill 管理系统的相似度评估器。请比较候选 Skill 与已有 Skill，并只返回 JSON，不要返回 Markdown。
JSON 字段必须包含：
similarity_score,name_similarity,description_similarity,body_similarity,trigger_similarity,tool_similarity,conflict_risk,recommendation,confidence,reason,evidence。
所有相似度和 confidence 为 0 到 1。recommendation 只能是 keep_separate、suggest_refine、block_duplicate。

候选 Skill:
名称: %s
简介: %s
正文:
%s

已有 Skill:
名称: %s
简介: %s
正文:
%s`, candidate.public.Name, candidate.public.Description, truncateRunes(candidate.body, 5000), source.Name, source.Description, truncateRunes(source.Body, 5000))
}

func buildRefinePrompt(group domain.SkillConflictGroup, candidates []skillCandidateState, instructions string) string {
	var b strings.Builder
	b.WriteString("你是 Skill 炼化器。请将下列相似功能 Skill 合并成一个更清晰、不重复的新 Skill，并只返回 JSON。JSON 字段: merged_name, merged_description, merged_body, merged_tags。merged_tags 为 [{\"name\":\"...\",\"source\":\"user\"}]。\n")
	if instructions != "" {
		b.WriteString("额外要求: " + instructions + "\n")
	}
	for _, candidate := range candidates {
		b.WriteString("\n候选 Skill:\n名称: " + candidate.public.Name + "\n简介: " + candidate.public.Description + "\n正文:\n" + truncateRunes(candidate.body, 5000) + "\n")
	}
	for _, existing := range group.ExistingSkills {
		b.WriteString("\n已有 Skill:\n名称: " + existing.Name + "\n简介: " + existing.Description + "\n正文:\n" + truncateRunes(existing.Body, 5000) + "\n")
	}
	return b.String()
}

func parseRefineResult(raw string) (domain.SkillRefineResult, error) {
	var out domain.SkillRefineResult
	if err := decodeModelJSON(raw, &out); err != nil {
		return out, err
	}
	if strings.TrimSpace(out.MergedName) == "" || strings.TrimSpace(out.MergedBody) == "" {
		return out, errors.New("model refine result missing merged_name or merged_body")
	}
	return out, nil
}

func decodeModelJSON(raw string, out any) error {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}
	return json.Unmarshal([]byte(raw), out)
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func candidateIDsForGroup(groups []domain.SkillConflictGroup, groupID string) []string {
	for _, group := range groups {
		if group.GroupID == groupID {
			return group.CandidateIDs
		}
	}
	return nil
}

func (s *SkillService) inspectSkillZip(ctx context.Context, data []byte, job *skillImportJobState) error {
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
	existing, err := s.store.ListSkillSimilaritySources()
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
		name, desc, tags := parseSkillMarkdown(body)
		slug := slugify(firstNonEmptyString(name, pathBase(dir)))
		candidateID := newID()
		candidate := domain.SkillImportCandidate{
			CandidateID:      candidateID,
			Name:             name,
			Slug:             slug,
			Description:      desc,
			BodyPreview:      truncateRunes(body, 240),
			TargetDir:        filepath.Join(job.public.StorageRoot, slug),
			ValidationStatus: "pass",
			StatusIcon:       "check",
			Warnings:         []domain.SkillImportIssue{},
			Blocks:           []domain.SkillImportIssue{},
		}
		if strings.TrimSpace(name) == "" || strings.TrimSpace(desc) == "" {
			candidate.ValidationStatus = "block"
			candidate.StatusIcon = "block"
			candidate.Blocks = append(candidate.Blocks, domain.SkillImportIssue{Type: "invalid_format", Message: "SKILL.md must include a title/name and description"})
		}
		if hasShellScriptAsset(files) {
			candidate.Warnings = append(candidate.Warnings, domain.SkillImportIssue{Type: "script_asset", Message: "包含 .sh 脚本素材，将仅作为 Skill 文件存储，不会自动执行"})
		}
		if highRiskFiles := highRiskFileNames(files); len(highRiskFiles) > 0 {
			candidate.ValidationStatus = "block"
			candidate.StatusIcon = "security"
			candidate.Blocks = append(candidate.Blocks, domain.SkillImportIssue{
				Type:    "high_risk_file",
				Message: "包含高风险文件，需要用户确认后才允许上传整个 Skill：" + strings.Join(highRiskFiles, ", "),
			})
		}
		for _, item := range existing {
			if strings.EqualFold(item.Name, name) || strings.EqualFold(item.Slug, slug) {
				candidate.ValidationStatus = "block"
				candidate.StatusIcon = "block"
				candidate.Blocks = append(candidate.Blocks, domain.SkillImportIssue{Type: "duplicate_name", Message: "Skill name or slug already exists"})
				break
			}
		}
		job.candidates[candidateID] = skillCandidateState{public: candidate, body: body, files: files, tags: tags}
		job.public.Candidates = append(job.public.Candidates, candidate)
	}
	if len(job.public.Candidates) == 0 {
		return validationError("zip must contain at least one SKILL.md")
	}
	return s.inspectSimilarity(ctx, job, existing)
}

func (s *SkillService) inspectSimilarity(ctx context.Context, job *skillImportJobState, existing []domain.SkillSimilaritySource) error {
	if len(existing) == 0 {
		return nil
	}
	providerModel, err := s.resolveSimilarityModel("", "")
	if err != nil {
		for i := range job.public.Candidates {
			if job.public.Candidates[i].ValidationStatus == "pass" {
				job.public.Candidates[i].ValidationStatus = "block"
				job.public.Candidates[i].StatusIcon = "block"
				job.public.Candidates[i].Blocks = append(job.public.Candidates[i].Blocks, domain.SkillImportIssue{Type: "model_unavailable", Message: err.Error()})
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
			metrics, reason, evidence, err := s.modelSimilarity(ctx, providerModel, state, source)
			if err != nil {
				continue
			}
			if metrics.SimilarityScore >= 0.2 {
				group := domain.SkillConflictGroup{
					GroupID:                newID(),
					HighestSimilarityScore: metrics.SimilarityScore,
					Metrics:                metrics,
					Reason:                 reason,
					Evidence:               evidence,
					CandidateIDs:           []string{candidate.CandidateID},
					ExistingSkills:         []domain.SkillSimilaritySource{source},
					CanRefine:              true,
				}
				job.public.ConflictGroups = append(job.public.ConflictGroups, group)
				updateCandidateWarning(job, candidate.CandidateID, metrics)
			}
		}
	}
	return nil
}

func (s *SkillService) modelSimilarity(ctx context.Context, providerModel domain.PlatformResource, candidate skillCandidateState, source domain.SkillSimilaritySource) (domain.SkillSimilarityMetrics, string, []string, error) {
	prompt := buildSimilarityPrompt(candidate, source)
	result, err := s.runtime.Generate(ctx, adkr.GenerateRequest{
		Agent:         skillSystemAgent(),
		ProviderModel: providerModel,
		Messages:      []adkr.ChatMessage{{Role: "user", Content: prompt}},
		Input:         prompt,
	})
	if err != nil {
		return domain.SkillSimilarityMetrics{}, "", nil, err
	}
	var out struct {
		domain.SkillSimilarityMetrics
		Reason   string   `json:"reason"`
		Evidence []string `json:"evidence"`
	}
	if err = decodeModelJSON(result.Content, &out); err != nil {
		return domain.SkillSimilarityMetrics{}, "", nil, err
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

func (s *SkillService) resolveSimilarityModel(provider string, model string) (domain.PlatformResource, error) {
	rows, err := s.store.ListPlatformResources("llm-provider-models")
	if err != nil {
		return domain.PlatformResource{}, err
	}
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		if provider != "" && row.Provider != provider {
			continue
		}
		if model != "" && row.Model != model {
			continue
		}
		if providerModelHasCredentials(row) {
			return row, nil
		}
	}
	return domain.PlatformResource{}, errors.New("no configured model with API base URL and API key is available")
}

func (s *SkillService) conflictGroupContext(jobID string, groupID string) (*skillImportJobState, domain.SkillConflictGroup, []skillCandidateState, error) {
	s.importJobsM.RLock()
	job := s.importJobs[strings.TrimSpace(jobID)]
	s.importJobsM.RUnlock()
	if job == nil {
		return nil, domain.SkillConflictGroup{}, nil, fmt.Errorf("%w: import job not found", errs.ErrNotFound)
	}
	for _, group := range job.public.ConflictGroups {
		if group.GroupID != groupID {
			continue
		}
		candidates := []skillCandidateState{}
		for _, id := range group.CandidateIDs {
			if candidate, ok := job.candidates[id]; ok {
				candidates = append(candidates, candidate)
			}
		}
		return job, group, candidates, nil
	}
	return nil, domain.SkillConflictGroup{}, nil, fmt.Errorf("%w: conflict group not found", errs.ErrNotFound)
}

func (s *SkillService) createImportedSkill(name string, slug string, description string, body string, tags []domain.SkillTag, files map[string][]byte) (domain.Skill, error) {
	slug = slugify(slug)
	if slug == "" {
		slug = slugify(name)
	}
	targetDir := filepath.Join(s.storageRoot, slug)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return domain.Skill{}, err
	}
	for name, data := range files {
		clean := filepath.Clean(name)
		if strings.Contains(clean, "..") {
			return domain.Skill{}, fmt.Errorf("unsafe skill file path: %s", name)
		}
		path := filepath.Join(targetDir, clean)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return domain.Skill{}, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return domain.Skill{}, err
		}
	}
	return s.store.CreateSkillWithVersion(domain.SkillCreateInput{Name: name, Slug: slug, Description: description, Body: body, Tags: tags, StorageDir: targetDir})
}

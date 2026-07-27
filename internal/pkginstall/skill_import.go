package pkginstall

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Two-phase skill import contract (POST /v1/skills/import only creates a job):
//   1. GET  /v1/skills/import/{job_id}                          → job status/candidates/conflict groups
//   2. POST /v1/skills/import/{job_id}/conflict-groups/{g}/refine → AI merge suggestion (optional)
//   3. POST /v1/skills/import/{job_id}/apply                    → {createdSkillIds, skippedCandidateIds}
//
// Engine decision actions (server-side vocabulary):
// import_passed | skip_duplicate | overwrite_duplicate | skip_group | merge_group_with_ai

// ConflictInfo describes one unresolved import conflict group for upper-layer display.
type ConflictInfo struct {
	GroupID         string   `json:"group_id"`
	SimilarityScore float64  `json:"similarity_score"`
	ExistingSkills  []string `json:"existing_skills"` // slugs of existing skills
	CandidateIDs    []string `json:"candidate_ids"`
	CanRefine       bool     `json:"can_refine"`
}

// Skill import step Status values.
const (
	SkillStatusInstalled       = "installed"
	SkillStatusPendingConflict = "pending_conflict"
	SkillStatusFailed          = "failed"
)

// Polling for the (currently synchronous) import validation; retry budget
// exists only to tolerate a future async implementation.
var (
	jobStatusPollAttempts = 3
	jobStatusPollInterval = 500 * time.Millisecond
)

type importJob struct {
	JobID            string                `json:"jobId"`
	Status           string                `json:"status"`
	ValidationStatus string                `json:"validationStatus"`
	Message          string                `json:"message"`
	Candidates       []importCandidate     `json:"candidates"`
	ConflictGroups   []importConflictGroup `json:"conflictGroups"`
}

type importCandidate struct {
	CandidateID      string        `json:"candidateId"`
	Name             string        `json:"name"`
	Slug             string        `json:"slug"`
	ValidationStatus string        `json:"validationStatus"`
	Blocks           []importIssue `json:"blocks"`
}

type importIssue struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type importConflictGroup struct {
	GroupID                string                `json:"groupId"`
	HighestSimilarityScore float64               `json:"highestSimilarityScore"`
	CandidateIDs           []string              `json:"candidateIds"`
	ExistingSkills         []importExistingSkill `json:"existingSkills"`
	CanRefine              bool                  `json:"canRefine"`
}

type importExistingSkill struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type importTag struct {
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`
}

type applyDecision struct {
	CandidateID       string      `json:"candidateId,omitempty"`
	GroupID           string      `json:"groupId,omitempty"`
	Action            string      `json:"action"`
	MergedName        string      `json:"mergedName,omitempty"`
	MergedDescription string      `json:"mergedDescription,omitempty"`
	MergedBody        string      `json:"mergedBody,omitempty"`
	MergedTags        []importTag `json:"mergedTags,omitempty"`
}

type refineResult struct {
	MergedName        string      `json:"mergedName"`
	MergedDescription string      `json:"mergedDescription"`
	MergedBody        string      `json:"mergedBody"`
	MergedTags        []importTag `json:"mergedTags"`
}

// validSkillDecision reports whether a manifest/tool decision value is supported.
func validSkillDecision(decision string) bool {
	switch decision {
	case "", "skip", "keep", "refine":
		return true
	default:
		return false
	}
}

// completeSkillImport drives the second phase of a skill import after the zip
// upload created jobID: poll job → map decision to engine actions → apply.
func (ins *Installer) completeSkillImport(client *http.Client, resource, decision, jobID string) StepResult {
	step := StepResult{Resource: resource, JobID: jobID}

	job, err := ins.fetchImportJob(client, jobID)
	if err != nil {
		step.Action = "error"
		step.Status = SkillStatusFailed
		step.Message = err.Error()
		return step
	}
	if job.Status == "failed" {
		step.Action = "error"
		step.Status = SkillStatusFailed
		step.Message = fmt.Sprintf("import job failed: %s", job.Message)
		return step
	}

	decisions, pending, err := ins.buildImportDecisions(client, jobID, decision, job)
	if err != nil {
		step.Action = "error"
		step.Status = SkillStatusFailed
		step.Message = err.Error()
		return step
	}

	var created, skipped int
	if len(decisions) > 0 {
		created, skipped, err = ins.applyImportDecisions(client, jobID, decisions)
		if err != nil {
			step.Action = "error"
			step.Status = SkillStatusFailed
			step.Message = err.Error()
			return step
		}
	}
	step.CreatedCount = created
	step.SkippedCount = skipped

	if len(pending.groups) > 0 || len(pending.candidates) > 0 {
		step.Action = "pending_conflict"
		step.Status = SkillStatusPendingConflict
		step.Conflicts = pending.groups
		step.Message = pending.summary(jobID)
		return step
	}

	step.Status = SkillStatusInstalled
	if created > 0 {
		step.Action = "created"
		step.Message = fmt.Sprintf("applied: %d created, %d skipped", created, skipped)
	} else {
		step.Action = "skipped"
		step.Message = fmt.Sprintf("applied: nothing created, %d skipped", skipped)
	}
	return step
}

// fetchImportJob polls GET /v1/skills/import/{job_id} until the job reaches a
// terminal status (completed|failed). Import validation is synchronous today;
// the small retry budget only guards against a future async implementation.
func (ins *Installer) fetchImportJob(client *http.Client, jobID string) (*importJob, error) {
	path := "/v1/skills/import/" + url.PathEscape(jobID)
	var lastStatus string
	for attempt := 0; attempt < jobStatusPollAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(jobStatusPollInterval)
		}
		body, status, err := ins.doJSON(client, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("get import job %s: %w", jobID, err)
		}
		if status >= 300 {
			return nil, fmt.Errorf("get import job %s: HTTP %d: %s", jobID, status, errorBodyMessage(body))
		}
		job, err := decodeImportJob(body)
		if err != nil {
			return nil, fmt.Errorf("get import job %s: %w", jobID, err)
		}
		if job.Status == "completed" || job.Status == "failed" {
			return job, nil
		}
		lastStatus = job.Status
	}
	return nil, fmt.Errorf("import job %s not terminal after %d attempts (last status %q)", jobID, jobStatusPollAttempts, lastStatus)
}

// pendingImport collects everything that could not be decided automatically.
type pendingImport struct {
	groups     []ConflictInfo
	candidates []pendingCandidate
}

type pendingCandidate struct {
	CandidateID string `json:"candidateId"`
	Slug        string `json:"slug"`
	Reason      string `json:"reason"`
}

// summary renders the structured conflict summary carried by StepResult.Message.
func (p pendingImport) summary(jobID string) string {
	payload := map[string]any{
		"jobId":             jobID,
		"conflicts":         p.groups,
		"pendingCandidates": p.candidates,
		"hint":              "retry with decision=skip (keep existing, skip new) | keep (overwrite existing with new) | refine (AI merge)",
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("import job %s has unresolved conflicts", jobID)
	}
	return string(b)
}

// buildImportDecisions maps the user-facing decision vocabulary
// (""=auto | skip | keep | refine) onto engine apply actions. Candidates that
// are members of a conflict group are never imported directly.
func (ins *Installer) buildImportDecisions(client *http.Client, jobID, decision string, job *importJob) ([]applyDecision, pendingImport, error) {
	grouped := map[string]bool{}
	for _, g := range job.ConflictGroups {
		for _, id := range g.CandidateIDs {
			grouped[id] = true
		}
	}
	var pass, duplicates, otherBlocked []importCandidate
	for _, c := range job.Candidates {
		switch {
		case c.ValidationStatus == "pass" && !grouped[c.CandidateID]:
			pass = append(pass, c)
		case c.ValidationStatus == "block" && hasDuplicateBlock(c) && !grouped[c.CandidateID]:
			duplicates = append(duplicates, c)
		case grouped[c.CandidateID]:
			// Covered by the conflict-group handling below.
		default:
			otherBlocked = append(otherBlocked, c)
		}
	}

	var decisions []applyDecision
	var pending pendingImport
	importPassed := func() {
		for _, c := range pass {
			decisions = append(decisions, applyDecision{CandidateID: c.CandidateID, Action: "import_passed"})
		}
	}
	pendBlocked := func() {
		for _, c := range otherBlocked {
			pending.candidates = append(pending.candidates, pendingCandidate{
				CandidateID: c.CandidateID, Slug: c.Slug, Reason: "validation blocked"})
		}
	}

	switch decision {
	case "":
		importPassed()
		for _, c := range duplicates {
			pending.candidates = append(pending.candidates, pendingCandidate{
				CandidateID: c.CandidateID, Slug: c.Slug, Reason: "duplicate of existing skill"})
		}
		pendBlocked()
		pending.groups = conflictInfos(job.ConflictGroups)
	case "skip":
		importPassed()
		for _, c := range duplicates {
			decisions = append(decisions, applyDecision{CandidateID: c.CandidateID, Action: "skip_duplicate"})
		}
		for _, g := range job.ConflictGroups {
			decisions = append(decisions, applyDecision{GroupID: g.GroupID, Action: "skip_group"})
		}
		pendBlocked()
	case "keep":
		importPassed()
		for _, c := range duplicates {
			decisions = append(decisions, applyDecision{CandidateID: c.CandidateID, Action: "overwrite_duplicate"})
		}
		for _, g := range job.ConflictGroups {
			decisions = append(decisions, applyDecision{GroupID: g.GroupID, Action: "skip_group"})
		}
		pendBlocked()
	case "refine":
		importPassed()
		for _, c := range duplicates {
			pending.candidates = append(pending.candidates, pendingCandidate{
				CandidateID: c.CandidateID, Slug: c.Slug, Reason: "duplicate of existing skill"})
		}
		pendBlocked()
		for _, g := range job.ConflictGroups {
			if !g.CanRefine {
				pending.groups = append(pending.groups, conflictInfo(g))
				continue
			}
			merged, err := ins.refineConflictGroup(client, jobID, g.GroupID)
			if err != nil {
				pending.groups = append(pending.groups, conflictInfo(g))
				continue
			}
			decisions = append(decisions, applyDecision{
				GroupID:           g.GroupID,
				Action:            "merge_group_with_ai",
				MergedName:        merged.MergedName,
				MergedDescription: merged.MergedDescription,
				MergedBody:        merged.MergedBody,
				MergedTags:        merged.MergedTags,
			})
		}
	default:
		return nil, pendingImport{}, fmt.Errorf("unknown decision: %q (want skip|keep|refine)", decision)
	}
	return decisions, pending, nil
}

func (ins *Installer) refineConflictGroup(client *http.Client, jobID, groupID string) (*refineResult, error) {
	path := fmt.Sprintf("/v1/skills/import/%s/conflict-groups/%s/refine", url.PathEscape(jobID), url.PathEscape(groupID))
	body, status, err := ins.doJSON(client, http.MethodPost, path, map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("refine conflict group %s: %w", groupID, err)
	}
	if status >= 300 {
		return nil, fmt.Errorf("refine conflict group %s: HTTP %d: %s", groupID, status, errorBodyMessage(body))
	}
	normalized, err := normalizeJSONKeys(body)
	if err != nil {
		return nil, fmt.Errorf("refine conflict group %s: decode: %w", groupID, err)
	}
	var out refineResult
	if err := json.Unmarshal(normalized, &out); err != nil {
		return nil, fmt.Errorf("refine conflict group %s: decode: %w", groupID, err)
	}
	return &out, nil
}

// applyImportDecisions POSTs the decisions and returns (created, skipped) counts
// derived from the apply result — never from the upload HTTP status.
func (ins *Installer) applyImportDecisions(client *http.Client, jobID string, decisions []applyDecision) (int, int, error) {
	path := "/v1/skills/import/" + url.PathEscape(jobID) + "/apply"
	body, status, err := ins.doJSON(client, http.MethodPost, path, map[string]any{"decisions": decisions})
	if err != nil {
		return 0, 0, fmt.Errorf("apply import job %s: %w", jobID, err)
	}
	if status >= 300 {
		return 0, 0, fmt.Errorf("apply import job %s: HTTP %d: %s", jobID, status, errorBodyMessage(body))
	}
	normalized, err := normalizeJSONKeys(body)
	if err != nil {
		return 0, 0, fmt.Errorf("apply import job %s: decode: %w", jobID, err)
	}
	var result struct {
		CreatedSkillIDs     []string `json:"createdSkillIds"`
		SkippedCandidateIDs []string `json:"skippedCandidateIds"`
		Message             string   `json:"message"`
	}
	if err := json.Unmarshal(normalized, &result); err != nil {
		return 0, 0, fmt.Errorf("apply import job %s: decode: %w", jobID, err)
	}
	return len(result.CreatedSkillIDs), len(result.SkippedCandidateIDs), nil
}

func hasDuplicateBlock(c importCandidate) bool {
	for _, b := range c.Blocks {
		if b.Type == "duplicate_name" {
			return true
		}
	}
	return false
}

func conflictInfos(groups []importConflictGroup) []ConflictInfo {
	out := make([]ConflictInfo, 0, len(groups))
	for _, g := range groups {
		out = append(out, conflictInfo(g))
	}
	return out
}

func conflictInfo(g importConflictGroup) ConflictInfo {
	slugs := make([]string, 0, len(g.ExistingSkills))
	for _, e := range g.ExistingSkills {
		slugs = append(slugs, e.Slug)
	}
	return ConflictInfo{
		GroupID:         g.GroupID,
		SimilarityScore: g.HighestSimilarityScore,
		ExistingSkills:  slugs,
		CandidateIDs:    append([]string(nil), g.CandidateIDs...),
		CanRefine:       g.CanRefine,
	}
}

// decodeImportJob decodes a job response regardless of whether the server
// emits protojson camelCase or snake_case keys.
func decodeImportJob(data []byte) (*importJob, error) {
	normalized, err := normalizeJSONKeys(data)
	if err != nil {
		return nil, err
	}
	var job importJob
	if err := json.Unmarshal(normalized, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// errorBodyMessage extracts the "message" field from a kratos error body.
func errorBodyMessage(body []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &m) == nil && m.Message != "" {
		return m.Message
	}
	s := strings.TrimSpace(string(body))
	const max = 200
	if len(s) > max {
		s = s[len(s)-max:]
	}
	return s
}

// normalizeJSONKeys rewrites snake_case object keys to camelCase recursively so
// responses decode the same whether the server uses proto names or JSON names.
func normalizeJSONKeys(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return json.Marshal(normalizeJSONValue(v))
}

func normalizeJSONValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[snakeToCamel(k)] = normalizeJSONValue(val)
		}
		return m
	case []any:
		for i := range t {
			t[i] = normalizeJSONValue(t[i])
		}
		return t
	default:
		return v
	}
}

func snakeToCamel(s string) string {
	if !strings.Contains(s, "_") {
		return s
	}
	parts := strings.Split(s, "_")
	var b strings.Builder
	b.WriteString(parts[0])
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

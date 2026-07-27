package pkginstall

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// importBackend fakes the two-phase skill import HTTP API.
type importBackend struct {
	mu          sync.Mutex
	uploadCalls int
	getCalls    int
	// jobBodies are returned in order for successive GET /v1/skills/import/{id} calls;
	// the last one is repeated when exhausted.
	jobBodies      []string
	applyCalls     int
	applyDecisions []map[string]any
	applyBody      string
	refineCalls    []string
	refineBody     string
}

func (b *importBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/skills/import":
		b.uploadCalls++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"job_id":"j1"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/v1/skills/import/j1":
		body := b.jobBodies[len(b.jobBodies)-1]
		if b.getCalls < len(b.jobBodies) {
			body = b.jobBodies[b.getCalls]
		}
		b.getCalls++
		_, _ = w.Write([]byte(body))
	case r.Method == http.MethodPost && r.URL.Path == "/v1/skills/import/j1/apply":
		b.applyCalls++
		var req struct {
			Decisions []map[string]any `json:"decisions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		b.applyDecisions = req.Decisions
		_, _ = w.Write([]byte(b.applyBody))
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/skills/import/j1/conflict-groups/"):
		group := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/skills/import/j1/conflict-groups/"), "/refine")
		b.refineCalls = append(b.refineCalls, group)
		_, _ = w.Write([]byte(b.refineBody))
	default:
		http.NotFound(w, r)
	}
}

func (b *importBackend) decisionActions() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, d := range b.applyDecisions {
		out = append(out, d["action"].(string))
	}
	return out
}

func newImportBackend() *importBackend {
	return &importBackend{
		applyBody:  `{"createdSkillIds":["s1"],"skippedCandidateIds":[],"message":"ok"}`,
		refineBody: `{"mergedName":"merged-skill","mergedDescription":"merged desc","mergedBody":"# Merged","mergedTags":[{"name":"t1","source":"ai"}]}`,
	}
}

// installSkillAgainst spins up the fake backend and runs a single-skill install.
func installSkillAgainst(t *testing.T, backend *importBackend, decision string) *Result {
	t.Helper()
	srv := httptest.NewServer(backend)
	t.Cleanup(srv.Close)

	pkgDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pkgDir, "skill.zip"), []byte("zip-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	ins := &Installer{APIURL: srv.URL, Quiet: true}
	result, err := ins.Install(pkgDir, &Manifest{
		Version:  1,
		Metadata: ManifestMetadata{Name: "pkg"},
		Spec: ManifestSpec{Skills: []SkillSpec{{
			Path:     "skill.zip",
			Decision: decision,
		}}},
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	return result
}

func skillStep(t *testing.T, r *Result) StepResult {
	t.Helper()
	for _, sr := range r.Steps {
		if strings.HasPrefix(sr.Resource, "skill:") {
			return sr
		}
	}
	t.Fatal("no skill step in result")
	return StepResult{}
}

const jobCompletedOnePass = `{
  "jobId":"j1","status":"completed","validationStatus":"pass",
  "candidates":[{"candidateId":"c1","name":"Skill One","slug":"skill-one","validationStatus":"pass"}],
  "conflictGroups":[]
}`

func TestInstallSkillAutoAppliesPassCandidate(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{jobCompletedOnePass}

	result := installSkillAgainst(t, backend, "")

	if backend.uploadCalls != 1 {
		t.Fatalf("upload calls = %d, want 1", backend.uploadCalls)
	}
	if backend.applyCalls != 1 {
		t.Fatalf("apply calls = %d, want 1", backend.applyCalls)
	}
	actions := backend.decisionActions()
	if len(actions) != 1 || actions[0] != "import_passed" {
		t.Fatalf("apply decision actions = %v, want [import_passed]", actions)
	}
	if id, _ := backend.applyDecisions[0]["candidateId"].(string); id != "c1" {
		t.Fatalf("apply decision candidateId = %v, want c1", backend.applyDecisions[0])
	}
	if result.Created != 1 {
		t.Fatalf("Result.Created = %d, want 1 (from createdSkillIds)", result.Created)
	}
	step := skillStep(t, result)
	if step.Action != "created" || step.Status != "installed" {
		t.Fatalf("step = %+v, want action=created status=installed", step)
	}
	if step.JobID != "j1" {
		t.Fatalf("step.JobID = %q, want j1", step.JobID)
	}
}

const jobCompletedPassAndDuplicate = `{
  "jobId":"j1","status":"completed","validationStatus":"block",
  "candidates":[
    {"candidateId":"c1","name":"Skill One","slug":"skill-one","validationStatus":"pass"},
    {"candidateId":"c2","name":"Skill Two","slug":"skill-two","validationStatus":"block",
     "blocks":[{"type":"duplicate_name","message":"Skill name or slug already exists"}]}
  ],
  "conflictGroups":[]
}`

func TestInstallSkillDuplicateDecisionSkip(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{jobCompletedPassAndDuplicate}
	backend.applyBody = `{"createdSkillIds":["s1"],"skippedCandidateIds":["c2"],"message":"ok"}`

	result := installSkillAgainst(t, backend, "skip")

	actions := backend.decisionActions()
	if len(actions) != 2 || actions[0] != "import_passed" || actions[1] != "skip_duplicate" {
		t.Fatalf("apply decision actions = %v, want [import_passed skip_duplicate]", actions)
	}
	if result.Created != 1 || result.Skipped != 1 {
		t.Fatalf("Result created/skipped = %d/%d, want 1/1", result.Created, result.Skipped)
	}
	step := skillStep(t, result)
	if step.Action != "created" || step.Status != "installed" {
		t.Fatalf("step = %+v, want action=created status=installed", step)
	}
}

func TestInstallSkillDuplicateDecisionKeep(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{jobCompletedPassAndDuplicate}

	result := installSkillAgainst(t, backend, "keep")

	actions := backend.decisionActions()
	if len(actions) != 2 || actions[0] != "import_passed" || actions[1] != "overwrite_duplicate" {
		t.Fatalf("apply decision actions = %v, want [import_passed overwrite_duplicate]", actions)
	}
	step := skillStep(t, result)
	if step.Status != "installed" {
		t.Fatalf("step.Status = %q, want installed", step.Status)
	}
}

const jobCompletedWithConflictGroup = `{
  "jobId":"j1","status":"completed","validationStatus":"pass",
  "candidates":[
    {"candidateId":"c1","name":"Skill One","slug":"skill-one","validationStatus":"pass"},
    {"candidateId":"c2","name":"Skill Two","slug":"skill-two","validationStatus":"pass"}
  ],
  "conflictGroups":[{
    "groupId":"g1","highestSimilarityScore":0.92,
    "candidateIds":["c2"],
    "existingSkills":[{"id":"e1","name":"Existing","slug":"existing-skill"}],
    "canRefine":true
  }]
}`

func TestInstallSkillConflictPendingWithoutDecision(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{jobCompletedWithConflictGroup}

	result := installSkillAgainst(t, backend, "")

	// Pass candidate outside the conflict group is still applied.
	if backend.applyCalls != 1 {
		t.Fatalf("apply calls = %d, want 1", backend.applyCalls)
	}
	actions := backend.decisionActions()
	if len(actions) != 1 || actions[0] != "import_passed" {
		t.Fatalf("apply decision actions = %v, want [import_passed]", actions)
	}
	step := skillStep(t, result)
	if step.Action != "pending_conflict" || step.Status != "pending_conflict" {
		t.Fatalf("step = %+v, want action/status=pending_conflict", step)
	}
	if step.JobID != "j1" {
		t.Fatalf("step.JobID = %q, want j1", step.JobID)
	}
	if len(step.Conflicts) != 1 {
		t.Fatalf("step.Conflicts = %+v, want 1 entry", step.Conflicts)
	}
	c := step.Conflicts[0]
	if c.GroupID != "g1" || c.SimilarityScore != 0.92 || !c.CanRefine {
		t.Fatalf("conflict = %+v, want g1/0.92/canRefine", c)
	}
	if len(c.ExistingSkills) != 1 || c.ExistingSkills[0] != "existing-skill" {
		t.Fatalf("conflict.ExistingSkills = %v, want [existing-skill]", c.ExistingSkills)
	}
	if len(c.CandidateIDs) != 1 || c.CandidateIDs[0] != "c2" {
		t.Fatalf("conflict.CandidateIDs = %v, want [c2]", c.CandidateIDs)
	}
	// Structured summary in Message for upper-layer display.
	if !strings.Contains(step.Message, "g1") || !strings.Contains(step.Message, "existing-skill") {
		t.Fatalf("step.Message = %q, want conflict summary with group id and slug", step.Message)
	}
}

func TestInstallSkillAllPendingSkipsApply(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{`{
	  "jobId":"j1","status":"completed","validationStatus":"pass",
	  "candidates":[{"candidateId":"c2","name":"Skill Two","slug":"skill-two","validationStatus":"pass"}],
	  "conflictGroups":[{
	    "groupId":"g1","highestSimilarityScore":0.88,
	    "candidateIds":["c2"],
	    "existingSkills":[{"id":"e1","name":"Existing","slug":"existing-skill"}],
	    "canRefine":false
	  }]
	}`}

	result := installSkillAgainst(t, backend, "")

	if backend.applyCalls != 0 {
		t.Fatalf("apply calls = %d, want 0 (all candidates pending)", backend.applyCalls)
	}
	step := skillStep(t, result)
	if step.Action != "pending_conflict" || step.Status != "pending_conflict" {
		t.Fatalf("step = %+v, want pending_conflict", step)
	}
	if result.Created != 0 {
		t.Fatalf("Result.Created = %d, want 0", result.Created)
	}
}

func TestInstallSkillRefineMergesGroup(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{jobCompletedWithConflictGroup}

	result := installSkillAgainst(t, backend, "refine")

	if len(backend.refineCalls) != 1 || backend.refineCalls[0] != "g1" {
		t.Fatalf("refine calls = %v, want [g1]", backend.refineCalls)
	}
	actions := backend.decisionActions()
	if len(actions) != 2 || actions[0] != "import_passed" || actions[1] != "merge_group_with_ai" {
		t.Fatalf("apply decision actions = %v, want [import_passed merge_group_with_ai]", actions)
	}
	merge := backend.applyDecisions[1]
	if merge["groupId"] != "g1" || merge["mergedName"] != "merged-skill" || merge["mergedBody"] != "# Merged" {
		t.Fatalf("merge decision = %v, want groupId/merged fields", merge)
	}
	step := skillStep(t, result)
	if step.Status != "installed" {
		t.Fatalf("step.Status = %q, want installed", step.Status)
	}
}

func TestInstallSkillRefineNonRefinableGroupPending(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{`{
	  "jobId":"j1","status":"completed","validationStatus":"pass",
	  "candidates":[{"candidateId":"c2","name":"Skill Two","slug":"skill-two","validationStatus":"pass"}],
	  "conflictGroups":[{
	    "groupId":"g1","highestSimilarityScore":0.88,
	    "candidateIds":["c2"],
	    "existingSkills":[{"id":"e1","name":"Existing","slug":"existing-skill"}],
	    "canRefine":false
	  }]
	}`}

	result := installSkillAgainst(t, backend, "refine")

	if len(backend.refineCalls) != 0 {
		t.Fatalf("refine calls = %v, want none (group not refinable)", backend.refineCalls)
	}
	if backend.applyCalls != 0 {
		t.Fatalf("apply calls = %d, want 0 (nothing to decide)", backend.applyCalls)
	}
	step := skillStep(t, result)
	if step.Action != "pending_conflict" || len(step.Conflicts) != 1 {
		t.Fatalf("step = %+v, want pending_conflict with 1 conflict", step)
	}
}

func TestInstallSkillUnknownDecisionFailsFast(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{jobCompletedOnePass}

	result := installSkillAgainst(t, backend, "bogus")

	if backend.uploadCalls != 0 {
		t.Fatalf("upload calls = %d, want 0 (fail before upload)", backend.uploadCalls)
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "unknown decision") {
		t.Fatalf("Result.Errors = %v, want unknown decision error", result.Errors)
	}
}

func TestInstallSkillJobFailed(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{`{"jobId":"j1","status":"failed","message":"invalid skill zip"}`}

	result := installSkillAgainst(t, backend, "")

	if backend.applyCalls != 0 {
		t.Fatalf("apply calls = %d, want 0", backend.applyCalls)
	}
	step := skillStep(t, result)
	if step.Action != "error" || step.Status != "failed" {
		t.Fatalf("step = %+v, want action=error status=failed", step)
	}
	if !strings.Contains(step.Message, "invalid skill zip") {
		t.Fatalf("step.Message = %q, want job failure message", step.Message)
	}
}

func TestInstallSkillJobStatusPollsUntilTerminal(t *testing.T) {
	backend := newImportBackend()
	backend.jobBodies = []string{
		`{"jobId":"j1","status":"validating"}`,
		jobCompletedOnePass,
	}
	origInterval := jobStatusPollInterval
	jobStatusPollInterval = time.Millisecond
	t.Cleanup(func() { jobStatusPollInterval = origInterval })

	result := installSkillAgainst(t, backend, "")

	if backend.getCalls != 2 {
		t.Fatalf("get job calls = %d, want 2 (poll retry)", backend.getCalls)
	}
	if backend.applyCalls != 1 {
		t.Fatalf("apply calls = %d, want 1", backend.applyCalls)
	}
	if result.Created != 1 {
		t.Fatalf("Result.Created = %d, want 1", result.Created)
	}
}

func TestDecodeImportJobAcceptsSnakeCase(t *testing.T) {
	job, err := decodeImportJob([]byte(`{
	  "job_id":"j1","status":"completed","validation_status":"pass",
	  "candidates":[{"candidate_id":"c1","slug":"s","validation_status":"pass"}],
	  "conflict_groups":[{"group_id":"g1","highest_similarity_score":0.5,"candidate_ids":["c1"],
	    "existing_skills":[{"slug":"x"}],"can_refine":true}]
	}`))
	if err != nil {
		t.Fatalf("decodeImportJob() error = %v", err)
	}
	if job.JobID != "j1" || job.Status != "completed" {
		t.Fatalf("job = %+v", job)
	}
	if len(job.Candidates) != 1 || job.Candidates[0].CandidateID != "c1" {
		t.Fatalf("candidates = %+v", job.Candidates)
	}
	if len(job.ConflictGroups) != 1 || job.ConflictGroups[0].GroupID != "g1" || !job.ConflictGroups[0].CanRefine {
		t.Fatalf("groups = %+v", job.ConflictGroups)
	}
	if job.ConflictGroups[0].HighestSimilarityScore != 0.5 {
		t.Fatalf("similarity = %v", job.ConflictGroups[0].HighestSimilarityScore)
	}
}
